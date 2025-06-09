package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestContext(t *testing.T) {
	t.Run("NewRequestContext", func(t *testing.T) {
		t.Run("should create context with request and response", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			serverRes := NewResponse()

			ctx := NewRequestContext(serverReq, serverRes, nil)
			assert.NotNil(t, ctx)
			assert.Equal(t, serverReq, ctx.Request)
			assert.Equal(t, serverRes, ctx.Response)
			assert.Equal(t, "", ctx.ErrorMesage)
			assert.Equal(t, 0, ctx.ErrorStatus)
		})
	})

	t.Run("Error", func(t *testing.T) {
		t.Run("should set error message and status", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			serverResp := NewResponse()

			ctx := NewRequestContext(serverReq, serverResp, nil)
			ctx.Error("Test error", http.StatusBadRequest)

			assert.Equal(t, "Test error", ctx.ErrorMesage)
			assert.Equal(t, http.StatusBadRequest, ctx.ErrorStatus)
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
		})
	})

	t.Run("ErrorResponse", func(t *testing.T) {
		t.Run("should return HTML error when Accept header is HTML", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			req.Header.Set("Accept", "text/html")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			serverResp := NewResponse()

			ctx := NewRequestContext(serverReq, serverResp, nil)
			ctx.Error("HTML error message", http.StatusInternalServerError)

			assert.Contains(t, string(ctx.Response.Body), "<html")
			assert.Contains(t, string(ctx.Response.Body), "HTML error message")
			assert.Equal(t, "text/html", ctx.Response.Headers.Get("Content-Type"))
		})

		t.Run("should return JSON error when Accept header is not HTML", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			req.Header.Set("Accept", "application/json")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			serverResp := NewResponse()

			ctx := NewRequestContext(serverReq, serverResp, nil)
			ctx.Error("JSON error message", http.StatusInternalServerError)

			assert.True(t, strings.HasPrefix(string(ctx.Response.Body), "{"))
			assert.Contains(t, string(ctx.Response.Body), "JSON error message")
			assert.Equal(t, "application/json", ctx.Response.Headers.Get("Content-Type"))
		})
	})
}
