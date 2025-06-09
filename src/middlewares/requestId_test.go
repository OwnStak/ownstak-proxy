package middlewares

import (
	"net/http/httptest"
	"testing"

	"ownstak-proxy/src/server"

	"github.com/stretchr/testify/assert"
)

func TestRequestIdMiddleware(t *testing.T) {
	t.Run("should call next middleware", func(t *testing.T) {
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
		middleware := NewRequestIdMiddleware()
		middleware.OnRequest(ctx, next)
		assert.True(t, nextCalled, "next() should be called")
	})

	t.Run("should set new x-request-id header in request and response", func(t *testing.T) {
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

		middleware := NewRequestIdMiddleware()
		middleware.OnRequest(ctx, func() {})
		middleware.OnResponse(ctx, func() {})

		reqID := ctx.Request.Headers.Get(server.HeaderRequestID)
		assert.NotEmpty(t, reqID, "request ID should be set in request headers")

		respID := ctx.Response.Headers.Get(server.HeaderRequestID)
		assert.NotEmpty(t, respID, "request ID should be set in response headers")
		assert.Equal(t, reqID, respID, "response should have same request ID as request")
	})

	t.Run("should preserve existing x-request-id header in request", func(t *testing.T) {
		// Create test request with existing x-request-id header
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(server.HeaderRequestID, "existing-id")
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		middleware := NewRequestIdMiddleware()
		middleware.OnRequest(ctx, func() {})

		reqID := ctx.Request.Headers.Get(server.HeaderRequestID)
		assert.Equal(t, "existing-id", reqID, "request ID should be preserved")

		middleware.OnResponse(ctx, func() {})
		respID := ctx.Response.Headers.Get(server.HeaderRequestID)
		assert.Equal(t, "existing-id", respID, "response should have same request ID as request")
	})
}
