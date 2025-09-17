package middlewares

import (
	"fmt"
	"io"
	"net/http"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"strings"
	"time"
)

// FollowRedirectMiddleware allows to proxy requests to any HTTP/HTTPS server
type FollowRedirectMiddleware struct {
	server.DefaultMiddleware
	maxRedirects int
	client       *http.Client
}

func NewFollowRedirectMiddleware() *FollowRedirectMiddleware {
	maxRedirects := 3

	// Create a custom HTTP client with redirect policy
	client := &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 2 * time.Hour, // Fetch with max timeout of 2 hours for large files (default is unlimited)
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return &FollowRedirectMiddleware{
		maxRedirects: maxRedirects,
		client:       client,
	}
}

func (m *FollowRedirectMiddleware) OnResponse(ctx *server.RequestContext, next func()) {
	redirectURL := ctx.Response.Headers.Get(server.HeaderLocation)
	followRedirectHeader := ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect)
	followRedirect := followRedirectHeader == "true" || followRedirectHeader == "1"

	mergeStatusHeader := ctx.Response.Headers.Get(server.HeaderXOwnMergeStatus)
	mergeStatus := mergeStatusHeader == "true" || mergeStatusHeader == "1"

	mergeHeadersHeader := ctx.Response.Headers.Get(server.HeaderXOwnMergeHeaders)
	mergeHeaders := mergeHeadersHeader == "true" || mergeHeadersHeader == "1"

	// If response is not a redirect or X-Follow-Redirect header is false,
	// continue to next middleware
	if redirectURL == "" || !followRedirect {
		next()
		return
	}

	logger.Debug("Following redirect to '%s'", redirectURL)
	// e.g: 302 Location: https://site-bucket.s3.amazonaws.com/site-125/index.html

	// Clear body, it's not needed anymore
	ctx.Response.ClearBody()

	// Remove headers that can cause issues after merging
	ctx.Response.Headers.Del(server.HeaderLocation)
	ctx.Response.Headers.Del(server.HeaderContentLength)
	ctx.Response.Headers.Del(server.HeaderContentType)
	ctx.Response.Headers.Del(server.HeaderContentEncoding)
	ctx.Response.Headers.Del(server.HeaderTransferEncoding)
	ctx.Response.Headers.Del(server.HeaderContentDisposition)

	// Remove internal headers that are not needed anymore
	ctx.Response.Headers.Del(server.HeaderXOwnMergeHeaders)
	ctx.Response.Headers.Del(server.HeaderXOwnFollowRedirect)
	ctx.Response.Headers.Del(server.HeaderXOwnMergeStatus)

	// Store debug information about the redirect
	ctx.Debug("follow-redirect-status=" + fmt.Sprintf("%d", ctx.Response.Status))
	ctx.Debug("follow-redirect-url=" + redirectURL)

	// Normalize the redirect URL (convert relative to absolute if needed)
	redirectURL = m.NormalizeRedirectURL(redirectURL, ctx)

	// Enable streaming for the response
	ctx.Response.EnableStreaming()

	// Start new request to the redirect URL
	req, err := http.NewRequest(ctx.Request.Method, redirectURL, nil)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to create request for redirect to '%s': %v", redirectURL, err)
		ctx.Error(errorMessage, server.StatusInternalError)
		return
	}

	// Copy request headers from the original request
	for k, v := range ctx.Request.Headers {
		if k != server.HeaderHost && !strings.HasPrefix(strings.ToLower(k), strings.ToLower(server.HeaderXOwnPrefix)) {
			req.Header.Set(k, v[0])
		}
	}

	// Execute the request
	resp, err := m.client.Do(req)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to follow redirect to '%s': %v", redirectURL, err)
		ctx.Error(errorMessage, server.StatusInternalError)
		return
	}
	defer resp.Body.Close()

	// Preserve status code from lambda response only if X-Own-Merge-Status is true and the S3 status code is 200 (e.g. succesfully returned file).
	// Otherwise, override status code with the status code from the redirect response.
	// We still need to correctly return 206 Partial Content, 304 Not Modified response statuses etc...
	// to not break the cache or streaming behavior.
	if mergeStatus && resp.StatusCode == 200 {
		ctx.Debug("merge-status=true")
	} else {
		ctx.Debug("merge-status=false")
		ctx.Response.Status = resp.StatusCode
	}

	// Preserve headers from lambda response only if X-Own-Merge-Headers is true.
	// Otherwise, clear all headers and keep only our internal headers and headers from S3.
	if mergeHeaders {
		ctx.Debug("merge-headers=true")
	} else {
		ctx.Debug("merge-headers=false")
		ctx.Response.ClearHeaders(true)
	}
	for k, v := range resp.Header {
		// Don't override x-own-* headers when following redirect to another ownstak site
		if !strings.HasPrefix(strings.ToLower(k), strings.ToLower(server.HeaderXOwnPrefix)) {
			ctx.Response.Headers.Set(k, v[0])
		}
	}

	// The files on S3 can be very large, that's why we need to stream the response
	// back to client right away and not buffer it in memory for other middlewares.
	ctx.Response.EnableStreaming()
	buffer := make([]byte, 32*1024) // 32KB chunks
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Write the chunk to the response
			_, writeErr := ctx.Response.Write(buffer[:n])
			if writeErr != nil {
				// Client peer is gone, stop streaming
				break
			}
		}

		if err != nil {
			if err != io.EOF {
				logger.Error("Error reading from redirect response: %v", err)
			}
			break
		}
	}

	// No other middlewares can be executed, response was already streamed
}

// NormalizeRedirectURL converts a potentially relative URL to an absolute URL
func (m *FollowRedirectMiddleware) NormalizeRedirectURL(redirectURL string, ctx *server.RequestContext) string {
	// Check if the redirect URL is relative
	if !strings.HasPrefix(redirectURL, "http://") && !strings.HasPrefix(redirectURL, "https://") {
		// Determine the current protocol and host to build the absolute URL
		protocol := ctx.Request.Scheme
		host := ctx.Request.Host

		// If the redirect URL doesn't start with a slash, add one
		if !strings.HasPrefix(redirectURL, "/") {
			redirectURL = "/" + redirectURL
		}

		// Create absolute URL from relative URL
		absoluteRedirectURL := fmt.Sprintf("%s://%s%s", protocol, host, redirectURL)
		logger.Debug("Converted relative redirectURL '%s' to absolute: '%s'", redirectURL, absoluteRedirectURL)
		return absoluteRedirectURL
	}

	return redirectURL
}
