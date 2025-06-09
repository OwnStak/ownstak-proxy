package middlewares

import (
	"net/http/httptest"
	"testing"

	"ownstak-proxy/src/server"

	"github.com/stretchr/testify/assert"
)

func TestHealthcheckMiddleware(t *testing.T) {
	t.Run("should call next middleware for non-health paths", func(t *testing.T) {
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
		middleware := NewHealthcheckMiddleware()
		middleware.OnRequest(ctx, next)
		assert.True(t, nextCalled, "next() should be called for non-health paths")
	})

	t.Run("should return OK for health endpoint", func(t *testing.T) {
		// Create test request to health endpoint
		req := httptest.NewRequest("GET", "/__ownstak__/health", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		middleware := NewHealthcheckMiddleware()
		middleware.OnRequest(ctx, func() {})

		// Verify response
		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, "OK", string(ctx.Response.Body))
		assert.Equal(t, "text/plain", ctx.Response.Headers.Get(server.HeaderContentType))
	})

	t.Run("should not call next for health endpoint", func(t *testing.T) {
		// Create test request to health endpoint
		req := httptest.NewRequest("GET", "/__ownstak__/health", nil)
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

		middleware := NewHealthcheckMiddleware()
		middleware.OnRequest(ctx, next)

		assert.False(t, nextCalled, "next() should not be called for health endpoint")
	})

	t.Run("should handle different HTTP methods for health endpoint", func(t *testing.T) {
		methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				// Create test request with specific method
				req := httptest.NewRequest(method, "/__ownstak__/health", nil)
				res := httptest.NewRecorder()

				// Create request context
				serverReq, _ := server.NewRequest(req)
				serverRes := server.NewResponse(res)

				ctx := &server.RequestContext{
					Request:  serverReq,
					Response: serverRes,
				}

				middleware := NewHealthcheckMiddleware()
				middleware.OnRequest(ctx, func() {})

				// Verify response
				assert.Equal(t, 200, ctx.Response.Status)
				assert.Equal(t, "OK", string(ctx.Response.Body))
				assert.Equal(t, "text/plain", ctx.Response.Headers.Get(server.HeaderContentType))
			})
		}
	})
}
