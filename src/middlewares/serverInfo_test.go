package middlewares

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/server"

	"github.com/stretchr/testify/assert"
)

func TestServerInfoMiddleware(t *testing.T) {
	t.Run("should call next middleware for non-info paths", func(t *testing.T) {
		// Set required environment variable
		os.Setenv(constants.EnvProvider, "test")
		defer os.Unsetenv(constants.EnvProvider)

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
		middleware := NewServerInfoMiddleware()
		middleware.OnRequest(ctx, next)
		assert.True(t, nextCalled, "next() should be called for non-info paths")
	})

	t.Run("should return server info for info endpoint", func(t *testing.T) {
		// Set required environment variable
		os.Setenv(constants.EnvProvider, "test")
		defer os.Unsetenv(constants.EnvProvider)

		// Create test request to info endpoint
		req := httptest.NewRequest("GET", "/__ownstak__/info", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		// Create a mock server for the context
		srv := server.NewServer()
		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
			Server:   srv,
		}

		middleware := NewServerInfoMiddleware()
		middleware.OnRequest(ctx, func() {})

		// Verify response
		assert.Equal(t, "application/json", ctx.Response.Headers.Get(server.HeaderContentType))
		assert.NotEmpty(t, ctx.Response.Body, "response body should not be empty")

		// Parse JSON response
		var info ServerInfoResponse
		err := json.Unmarshal(ctx.Response.Body, &info)
		assert.NoError(t, err, "response should be valid JSON")

		// Check required fields
		assert.Equal(t, constants.AppName, info.Name)
		assert.Equal(t, constants.Version, info.Version)
		assert.NotEmpty(t, info.ID, "server ID should be set")
		assert.Greater(t, info.System.CPUsCount, 0, "CPU count should be positive")
		assert.Greater(t, info.System.GoroutinesCount, 0, "goroutines count should be positive")
	})

	t.Run("should not call next for info endpoint", func(t *testing.T) {
		// Set required environment variable
		os.Setenv(constants.EnvProvider, "test")
		defer os.Unsetenv(constants.EnvProvider)

		// Create test request to info endpoint
		req := httptest.NewRequest("GET", "/__ownstak__/info", nil)
		res := httptest.NewRecorder()

		// Create request context
		serverReq, _ := server.NewRequest(req)
		serverRes := server.NewResponse(res)

		// Create a mock server for the context
		srv := server.NewServer()
		ctx := &server.RequestContext{
			Request:  serverReq,
			Response: serverRes,
			Server:   srv,
		}

		// Fake next middleware
		nextCalled := false
		next := func() {
			nextCalled = true
		}

		middleware := NewServerInfoMiddleware()
		middleware.OnRequest(ctx, next)

		assert.False(t, nextCalled, "next() should not be called for info endpoint")
	})
}
