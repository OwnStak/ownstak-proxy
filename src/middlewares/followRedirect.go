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

// OnRequest checks for an existing X-Request-ID header and generates a new one if missing
func (m *FollowRedirectMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
	// Nothing to do in request phase
	next()
}

func (m *FollowRedirectMiddleware) OnResponse(ctx *server.RequestContext, next func()) {
	redirectURL := ctx.Response.Headers.Get(server.HeaderLocation)
	followRedirect := ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect)

	// If response is not a redirect or X-Follow-Redirect header is false, continue to next middleware
	if redirectURL == "" || (followRedirect != "true" && followRedirect != "1") {
		next()
		return
	}

	logger.Debug("Following redirect to '%s'", redirectURL)
	// e.g: 302 Location: https://site-bucket.s3.amazonaws.com/site-125/index.html

	// Preserve our internal headers from existing response
	// in the final response for debugging purposes
	internalHeaders := make(http.Header)
	for k, v := range ctx.Response.Headers {
		if strings.HasPrefix(strings.ToLower(k), strings.ToLower(server.HeaderXOwnPrefix)) {
			internalHeaders[k] = v
		}
	}

	ctx.Response.ClearBody()

	// Remove headers that can cause issues after merging
	ctx.Response.Headers.Del(server.HeaderLocation)
	ctx.Response.Headers.Del(server.HeaderContentLength)
	ctx.Response.Headers.Del(server.HeaderContentType)
	ctx.Response.Headers.Del(server.HeaderContentEncoding)
	ctx.Response.Headers.Del(server.HeaderTransferEncoding)
	ctx.Response.Headers.Del(server.HeaderContentDisposition)

	// If X-Own-Merge-Headers is not true, clear all headers from the lambda
	// response and keep only our internal headers
	if ctx.Response.Headers.Get(server.HeaderXOwnMergeHeaders) != "true" {
		ctx.Response.ClearHeaders()
		ctx.Response.Headers = internalHeaders
		ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "merge-headers=false")
	} else {
		ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "merge-headers=true")
	}

	// Remove X-Own-Merge-Headers header, it's not needed anymore
	ctx.Response.Headers.Del(server.HeaderXOwnMergeHeaders)
	// Clear X-Own-Follow-Redirect header, it's not needed anymore
	ctx.Response.Headers.Del(server.HeaderXOwnFollowRedirect)

	// Add debug information about the redirect
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "follow-redirect-status="+fmt.Sprintf("%d", ctx.Response.Status))
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "follow-redirect-url="+redirectURL)

	// Normalize the redirect URL (convert relative to absolute if needed)
	redirectURL = m.NormalizeRedirectURL(redirectURL, ctx)

	// Enable streaming for the response
	ctx.Response.EnableStreaming()

	// Start a GET request with streaming
	req, err := http.NewRequest("GET", redirectURL, nil)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to create request for redirect to '%s': %v", redirectURL, err)
		ctx.Error(errorMessage, http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request
	for k, v := range ctx.Request.Headers {
		if k != server.HeaderHost && !strings.HasPrefix(strings.ToLower(k), strings.ToLower(server.HeaderXOwnPrefix)) {
			req.Header.Set(k, v[0])
		}
	}

	// Execute the request
	resp, err := m.client.Do(req)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to follow redirect to '%s': %v", redirectURL, err)
		ctx.Error(errorMessage, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Set the response status and merge headers
	ctx.Response.Status = resp.StatusCode
	for k, v := range resp.Header {
		if !strings.HasPrefix(strings.ToLower(k), strings.ToLower(server.HeaderXOwnPrefix)) {
			ctx.Response.Headers.Set(k, v[0])
		}
	}

	// Copy content information headers
	if contentLength := resp.Header.Get(server.HeaderContentLength); contentLength != "" {
		ctx.Response.Headers.Set(server.HeaderContentLength, contentLength)
	}
	if contentType := resp.Header.Get(server.HeaderContentType); contentType != "" {
		ctx.Response.Headers.Set(server.HeaderContentType, contentType)
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

	// No other middlewares can be executed, response is already streamed
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
