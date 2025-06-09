package middlewares

import (
	"net/http/httptest"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/server"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerProfilerMiddleware(t *testing.T) {
	// Store original mode and restore after tests
	originalMode := constants.Mode
	defer func() {
		constants.Mode = originalMode
	}()

	t.Run("should call next middleware in production mode", func(t *testing.T) {
		// Set mode to production
		constants.Mode = "production"

		// Create test request
		req := httptest.NewRequest("GET", "/__ownstak__/debug/pprof/", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Track next middleware call
		nextCalled := false
		next := func() {
			nextCalled = true
		}

		middleware := NewServerProfilerMiddleware()
		middleware.OnRequest(ctx, next)

		assert.True(t, nextCalled, "next() should be called when not in development mode")
	})

	t.Run("should call next middleware when path does not match pprof prefix", func(t *testing.T) {
		// Set mode to development
		constants.Mode = "development"

		// Create test request with non-pprof path
		req := httptest.NewRequest("GET", "/regular/path", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
		}

		// Track next middleware call
		nextCalled := false
		next := func() {
			nextCalled = true
		}

		middleware := NewServerProfilerMiddleware()
		middleware.OnRequest(ctx, next)

		assert.True(t, nextCalled, "next() should be called when path doesn't match pprof prefix")
	})
}
