package middlewares

import (
	"fmt"
	"io"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"net/http"
	"strings"
)

// FollowRedirectMiddleware allows to proxy requests to any HTTP/HTTPS server
type FollowRedirectMiddleware struct {
	maxRedirects int
}

func NewFollowRedirectMiddleware() *FollowRedirectMiddleware {
	return &FollowRedirectMiddleware{
		maxRedirects: 3,
	}
}

// OnRequest checks for an existing X-Request-ID header and generates a new one if missing
func (m *FollowRedirectMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	// Nothing to do in request phase
	next()
}

func (m *FollowRedirectMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	redirectURL := ctx.Response.Headers.Get(server.HeaderLocation)
	followRedirect := ctx.Response.Headers.Get(server.HeaderFollowRedirect)

	// If the response is a redirect and X-Follow-Redirect header is true, we need to pass it to the next FollowRedirectMiddleware and follow it.
	// e.g: 302 Location: https://site-bucket.s3.amazonaws.com/site-125/index.html
	if redirectURL != "" && (followRedirect == "true" || followRedirect == "1") {
		logger.Debug("Following redirect to '%s'", redirectURL)

		// Normalize the redirect URL (convert relative to absolute if needed)
		redirectURL = m.NormalizeRedirectURL(redirectURL, ctx)

		ctx.Response.Clear()

		// Create a custom HTTP client with redirect policy
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= m.maxRedirects {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}

		// Fetch the redirect URL
		resp, err := client.Get(redirectURL)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to follow redirect to '%s': %v", redirectURL, err)
			ctx.Error(errorMessage, http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Set the response status and headers
		ctx.Response.Status = resp.StatusCode
		ctx.Response.Headers = resp.Header

		// Set the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to read response body while following redirect to '%s': %v", redirectURL, err)
			ctx.Error(errorMessage, http.StatusInternalServerError)
			return
		}
		ctx.Response.Body = body
	}

	// Nothing to do in response phase
	next()
}

// NormalizeRedirectURL converts a potentially relative URL to an absolute URL
func (m *FollowRedirectMiddleware) NormalizeRedirectURL(redirectURL string, ctx *server.ServerContext) string {
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
