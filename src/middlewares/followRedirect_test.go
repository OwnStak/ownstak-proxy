package middlewares

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ownstak-proxy/src/server"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestFollowRedirectMiddleware(t *testing.T) {
	cleanupMockClient := setupFollowRedirectMockClient(t)
	defer cleanupMockClient()

	// Create middleware instance and inject mock client
	middleware := NewFollowRedirectMiddleware()
	middleware.client = http.DefaultClient // Inject the mocked client

	t.Run("should always call next middleware in OnRequest", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}
		middleware.OnRequest(ctx, next)
		assert.True(t, nextCalled, "next() should always be called in OnRequest")
	})

	t.Run("should call next middleware when there is no Location header in the response", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}
		middleware.OnResponse(ctx, next)
		assert.True(t, nextCalled, "next() should be called when there is no redirect")
	})

	t.Run("should call next middleware if there is only x-own-follow-redirect header", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set X-Own-Follow-Redirect header but no Location
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}
		middleware.OnResponse(ctx, next)
		assert.True(t, nextCalled, "next() should be called when there is no Location header")
	})

	t.Run("should call next middleware when X-Own-Follow-Redirect is false", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set Location header but X-Own-Follow-Redirect is false
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "false")

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}
		middleware.OnResponse(ctx, next)
		assert.True(t, nextCalled, "next() should be called when X-Own-Follow-Redirect is false")
	})

	t.Run("should call next middleware when X-Own-Follow-Redirect is not set", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set Location header but no X-Own-Follow-Redirect
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}
		middleware.OnResponse(ctx, next)
		assert.True(t, nextCalled, "next() should be called when X-Own-Follow-Redirect is not set")
	})

	t.Run("should clean up headers when following redirect", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set(server.HeaderContentType, "text/html")
		ctx.Response.Headers.Set(server.HeaderContentLength, "123")

		middleware.OnResponse(ctx, func() {})

		// Verify headers are cleaned up
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
	})

	t.Run("should handle X-Own-Follow-Redirect with value 'true'", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with X-Own-Follow-Redirect = "1"
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "1")

		middleware.OnResponse(ctx, func() {})

		// Verify redirect was followed (headers cleaned up)
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
	})

	t.Run("should handle X-Own-Follow-Redirect with value '1'", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with X-Own-Follow-Redirect = "1"
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "1")

		middleware.OnResponse(ctx, func() {})

		// Verify redirect was followed (headers cleaned up)
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
	})

	t.Run("should not call next when following redirect", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}

		middleware.OnResponse(ctx, next)

		// When following a redirect, next should not be called (due to streaming)
		assert.False(t, nextCalled, "next() should not be called when following redirect")
	})

	t.Run("should preserve request method when following redirect", func(t *testing.T) {
		// Create test request with POST method
		req := httptest.NewRequest("POST", "/test", strings.NewReader("test body"))
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Mock the redirect response
		// Mock the redirect response
		httpmock.RegisterResponder("POST", "https://127.0.0.1/redirect",
			func(req *http.Request) (*http.Response, error) {
				// Verify that original headers are preserved
				assert.Equal(t, "POST", req.Method)

				resp := httpmock.NewStringResponse(200, "redirected response")
				resp.Header.Set("Content-Type", "text/plain")
				resp.Header.Set("custom-header", "custom-value")
				return resp, nil
			})

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed with POST method
		// The mock should have been called with POST method
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Headers.Get("custom-header"), "custom-value")
	})

	t.Run("should preserve request headers when following redirect", func(t *testing.T) {
		// Create test request with custom headers
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer token123")
		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("User-Agent", "test-agent")
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Mock the redirect response
		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
			func(req *http.Request) (*http.Response, error) {
				// Verify that original headers are preserved
				assert.Equal(t, "Bearer token123", req.Header.Get("Authorization"))
				assert.Equal(t, "custom-value", req.Header.Get("X-Custom-Header"))
				assert.Equal(t, "test-agent", req.Header.Get("User-Agent"))
				
				resp := httpmock.NewStringResponse(200, "redirected response")
				resp.Header.Set("Content-Type", "text/plain")
				resp.Header.Set("custom-header", "custom-value")
				return resp, nil
			})

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Headers.Get("custom-header"), "custom-value")
	})

	t.Run("should handle relative redirect URLs correctly", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "https://example.com/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with relative URL
		ctx.Response.Headers.Set(server.HeaderLocation, "/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Mock the redirect response
		httpmock.RegisterResponder("GET", "https://example.com/redirect",
			httpmock.NewStringResponder(200, "redirected response"))

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
	})

	t.Run("should preserve response status code when x-own-merge-status is true", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Status = 404
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set(server.HeaderXOwnMergeStatus, "true")

		// Mock the redirect response with different status code
		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
			httpmock.NewStringResponder(200, "redirected response"))

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Status, 404)
	})

	t.Run("should override response status code when x-own-merge-status is not set", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Status = 404
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")

		// Mock the redirect response with different status code
		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
			httpmock.NewStringResponder(200, "redirected response"))

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Status, 200)
	})


	t.Run("should not override response status code when it's not 200", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Status = 404
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set(server.HeaderXOwnMergeStatus, "true")

		// Mock the redirect response with different status code
		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
			httpmock.NewStringResponder(206, "redirected partial content response"))

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Status, 206)
	})

	t.Run("should always preserve all X-Own- internal response headers", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with internal headers
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/test")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set("X-Own-Test-Header", "test-value")

		middleware.OnResponse(ctx, func() {})

		// Verify internal headers are preserved (in debug headers)
		assert.Equal(t, "test-value", ctx.Response.Headers.Get("X-Own-Test-Header"))
	})

	t.Run("should preserve response headers when x-own-merge-headers is true", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set(server.HeaderXOwnMergeHeaders, "true")
		ctx.Response.Headers.Set("cache-control", "no-cache")

		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
		func(req *http.Request) (*http.Response, error) {
			resp := httpmock.NewStringResponse(200, "redirected response")
			resp.Header.Set("Content-Type", "text/plain")
			return resp, nil
		})

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Headers.Get("cache-control"), "no-cache")
	})

	t.Run("should not preserve response headers when x-own-merge-headers is not set", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set("cache-control", "no-cache")

		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
		func(req *http.Request) (*http.Response, error) {
			resp := httpmock.NewStringResponse(200, "redirected response")
			resp.Header.Set("Content-Type", "text/plain")
			return resp, nil
		})

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Headers.Get("cache-control"), "")
	})

	t.Run("should override conflicting response headers with those from the redirect response", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Set up redirect scenario with merge-status
		ctx.Response.Headers.Set(server.HeaderLocation, "https://127.0.0.1/redirect")
		ctx.Response.Headers.Set(server.HeaderXOwnFollowRedirect, "true")
		ctx.Response.Headers.Set("custom-header", "from-lambda")

		httpmock.RegisterResponder("GET", "https://127.0.0.1/redirect",
		func(req *http.Request) (*http.Response, error) {
			resp := httpmock.NewStringResponse(200, "redirected response")
			resp.Header.Set("Content-Type", "text/plain")
			resp.Header.Set("custom-header", "from-s3")
			return resp, nil
		})

		middleware.OnResponse(ctx, func() {})

		// Verify the redirect was followed and merge-status is handled
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderLocation))
		assert.Empty(t, ctx.Response.Headers.Get(server.HeaderXOwnFollowRedirect))
		assert.Equal(t, ctx.Response.Headers.Get("custom-header"), "from-s3")
	})
}

func TestNormalizeRedirectURL(t *testing.T) {
	middleware := NewFollowRedirectMiddleware()

	t.Run("should preserve absolute HTTP URLs", func(t *testing.T) {
		// Create mock context
		req := httptest.NewRequest("GET", "/test", nil)
		serverReq, _ := server.NewRequest(req)
		ctx := &server.RequestContext{Request: serverReq}

		absoluteURL := "http://127.0.0.1/path"
		result := middleware.NormalizeRedirectURL(absoluteURL, ctx)
		assert.Equal(t, absoluteURL, result)
	})

	t.Run("should preserve absolute HTTPS URLs", func(t *testing.T) {
		// Create mock context
		req := httptest.NewRequest("GET", "/test", nil)
		serverReq, _ := server.NewRequest(req)
		ctx := &server.RequestContext{Request: serverReq}

		absoluteURL := "https://127.0.0.1/path"
		result := middleware.NormalizeRedirectURL(absoluteURL, ctx)
		assert.Equal(t, absoluteURL, result)
	})

	t.Run("should convert relative URL with leading slash", func(t *testing.T) {
		// Create mock context
		req := httptest.NewRequest("GET", "https://127.0.0.1/test", nil)
		serverReq, _ := server.NewRequest(req)
		ctx := &server.RequestContext{Request: serverReq}

		relativeURL := "/path/to/resource"
		expected := "https://127.0.0.1/path/to/resource"
		result := middleware.NormalizeRedirectURL(relativeURL, ctx)
		assert.Equal(t, expected, result)
	})

	t.Run("should convert relative URL without leading slash", func(t *testing.T) {
		// Create mock context
		req := httptest.NewRequest("GET", "https://127.0.0.1/test", nil)
		serverReq, _ := server.NewRequest(req)
		ctx := &server.RequestContext{Request: serverReq}

		relativeURL := "path/to/resource"
		expected := "https://127.0.0.1/path/to/resource"
		result := middleware.NormalizeRedirectURL(relativeURL, ctx)
		assert.Equal(t, expected, result)
	})
}

func setupFollowRedirectMockClient(t *testing.T) func() {
	httpmock.Activate(t)

	httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		resp := httpmock.NewStringResponse(200, "<html><body><h1>Hello, World!</h1></body></html>")
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})

	return func() {
		httpmock.DeactivateAndReset()
	}
}
