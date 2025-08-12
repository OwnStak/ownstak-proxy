package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	// Setup test environment
	setupTestEnv := func() {
		os.Clearenv()
		os.Setenv("PROVIDER", "aws")
		// Create test cert directory
		err := os.MkdirAll("/tmp/certs", 0755)
		if err != nil {
			t.Fatalf("Failed to create test cert directory: %v", err)
		}
	}

	// Cleanup test environment
	cleanupTestEnv := func() {
		os.RemoveAll("/tmp/certs")
	}

	// Helper function to create a test request and response
	createTestRequest := func() (*httptest.ResponseRecorder, *http.Request) {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		req.Host = "example.com"
		req.RemoteAddr = "127.0.0.1:8080"
		res := httptest.NewRecorder()
		return res, req
	}

	t.Run("NewServer", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should create server with default values", func(t *testing.T) {
			server := NewServer()
			assert.NotNil(t, server)
			assert.Equal(t, "0.0.0.0", server.host)
			assert.Equal(t, "80", server.httpPort)
			assert.Equal(t, "443", server.httpsPort)
			assert.Equal(t, "/tmp/certs/cert.pem", server.certFile)
			assert.Equal(t, "/tmp/certs/key.pem", server.keyFile)
			assert.Equal(t, "/tmp/certs/ca.pem", server.caFile)
			assert.Equal(t, 2*time.Minute, server.reqReadTimeout)
			assert.Equal(t, 2*time.Hour, server.resWriteTimeout)
			assert.Equal(t, 60*time.Second, server.reqIdleTimeout)
			assert.Equal(t, 64*1024, server.reqMaxHeadersSize)
			assert.Equal(t, 10*1024*1024, server.reqMaxBodySize)
			assert.NotEmpty(t, server.serverId)
			assert.NotNil(t, server.MiddlewaresChain)
		})

		t.Run("should create server with custom values from environment", func(t *testing.T) {
			os.Setenv("HOST", "127.0.0.1")
			os.Setenv("HTTP_PORT", "8080")
			os.Setenv("HTTPS_PORT", "8443")
			os.Setenv("REQ_READ_TIMEOUT", "30s")
			os.Setenv("RES_WRITE_TIMEOUT", "1h")
			os.Setenv("REQ_IDLE_TIMEOUT", "30s")
			os.Setenv("REQ_MAX_HEADERS_SIZE", "2048")
			os.Setenv("REQ_MAX_BODY_SIZE", "5242880")

			server := NewServer()
			assert.NotNil(t, server)
			assert.Equal(t, "127.0.0.1", server.host)
			assert.Equal(t, "8080", server.httpPort)
			assert.Equal(t, "8443", server.httpsPort)
			assert.Equal(t, 30*time.Second, server.reqReadTimeout)
			assert.Equal(t, time.Hour, server.resWriteTimeout)
			assert.Equal(t, 30*time.Second, server.reqIdleTimeout)
			assert.Equal(t, 2048, server.reqMaxHeadersSize)
			assert.Equal(t, 5242880, server.reqMaxBodySize)
		})

		t.Run("should handle invalid environment values", func(t *testing.T) {
			os.Setenv("REQ_READ_TIMEOUT", "invalid")
			os.Setenv("RES_WRITE_TIMEOUT", "invalid")
			os.Setenv("REQ_IDLE_TIMEOUT", "invalid")
			os.Setenv("REQ_MAX_HEADERS_SIZE", "invalid")
			os.Setenv("REQ_MAX_BODY_SIZE", "invalid")

			server := NewServer()
			assert.NotNil(t, server)
			assert.Equal(t, 2*time.Minute, server.reqReadTimeout)
			assert.Equal(t, 2*time.Hour, server.resWriteTimeout)
			assert.Equal(t, 60*time.Second, server.reqIdleTimeout)
			assert.Equal(t, 64*1024, server.reqMaxHeadersSize)
			assert.Equal(t, 10*1024*1024, server.reqMaxBodySize)
		})
	})

	t.Run("MaxBodySize", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should limit request body size", func(t *testing.T) {
			// Set a small max body size for testing
			os.Setenv("REQ_MAX_BODY_SIZE", "1024")
			server := NewServer()
			
			// Create a request with a body larger than the limit
			largeBody := make([]byte, 2048) // 2KB body
			req := httptest.NewRequest("POST", "http://example.com", nil)
			req.Body = io.NopCloser(bytes.NewReader(largeBody))
			req.ContentLength = int64(len(largeBody))
			req.Host = "example.com"
			req.RemoteAddr = "127.0.0.1:8080"
			
			res := httptest.NewRecorder()
			
			// This should trigger the max body size limit
			server.HandleRequest(res, req)
			
			// The request should fail due to body size limit
			assert.Equal(t, http.StatusRequestEntityTooLarge, res.Code)
		})

		t.Run("should accept request within body size limit", func(t *testing.T) {
			// Set a reasonable max body size
			os.Setenv("REQ_MAX_BODY_SIZE", "1024")
			server := NewServer()
			
			// Create a request with a body within the limit
			smallBody := make([]byte, 512) // 512B body
			req := httptest.NewRequest("POST", "http://example.com", nil)
			req.Body = io.NopCloser(bytes.NewReader(smallBody))
			req.ContentLength = int64(len(smallBody))
			req.Host = "example.com"
			req.RemoteAddr = "127.0.0.1:8080"
			
			res := httptest.NewRecorder()
			
			// This should work fine
			server.HandleRequest(res, req)
			
			// The request should be processed normally
			assert.Equal(t, http.StatusOK, res.Code)
		})

		t.Run("should handle client disconnection gracefully", func(t *testing.T) {
			server := NewServer()
			
			// Create a request with a context that's already cancelled
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately to simulate client disconnection
			
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req = req.WithContext(ctx)
			req.Host = "example.com"
			req.RemoteAddr = "127.0.0.1:8080"
			
			res := httptest.NewRecorder()
			
			// This should handle the cancelled context gracefully
			server.HandleRequest(res, req)
			
			// Should not return an error status, just handle gracefully
			// The response might be empty or have a specific status
			assert.NotEqual(t, http.StatusInternalServerError, res.Code)
		})
	})

	t.Run("Use", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should add middleware to chain", func(t *testing.T) {
			server := NewServer()
			middleware := &MockMiddleware{Name: "test", ShouldCallNext: true}

			server.Use(middleware)

			assert.Len(t, server.MiddlewaresChain.middlewares, 1)
			assert.Equal(t, middleware, server.MiddlewaresChain.middlewares[0])
		})

		t.Run("should ignore nil middleware", func(t *testing.T) {
			server := NewServer()

			server.Use(nil)

			assert.Empty(t, server.MiddlewaresChain.middlewares)
		})

		t.Run("should support chaining middleware", func(t *testing.T) {
			server := NewServer()
			middleware1 := &MockMiddleware{Name: "test1", ShouldCallNext: true}
			middleware2 := &MockMiddleware{Name: "test2", ShouldCallNext: true}

			server.Use(middleware1).Use(middleware2)

			assert.Len(t, server.MiddlewaresChain.middlewares, 2)
			assert.Equal(t, middleware1, server.MiddlewaresChain.middlewares[0])
			assert.Equal(t, middleware2, server.MiddlewaresChain.middlewares[1])
		})
	})

	t.Run("HandleRequest", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should handle basic request", func(t *testing.T) {
			server := NewServer()
			res, req := createTestRequest()

			server.HandleRequest(res, req)

			assert.Equal(t, http.StatusOK, res.Code)
		})

		t.Run("should execute middleware chain", func(t *testing.T) {
			server := NewServer()
			middleware := &MockMiddleware{
				ShouldCallNext: true,
				Name:           "test",
			}
			server.Use(middleware)

			res, req := createTestRequest()
			server.HandleRequest(res, req)

			assert.True(t, middleware.OnRequestCalled, "OnRequest should be called")
			assert.True(t, middleware.OnResponseCalled, "OnResponse should be called")
		})

		t.Run("should return error when no provider is set", func(t *testing.T) {
			// Unset provider before creating server and request
			os.Unsetenv("PROVIDER")
			server := NewServer()
			res, req := createTestRequest()
			server.HandleRequest(res, req)
			assert.Equal(t, http.StatusServiceUnavailable, res.Code)
		})


		t.Run("should handle client disconnection during body read", func(t *testing.T) {
			server := NewServer()
			
			// Create a request with a body that will fail to read
			req := httptest.NewRequest("POST", "http://example.com", nil)
			req.Host = "example.com"
			req.RemoteAddr = "127.0.0.1:8080"
			
			// Create a mock body that simulates client disconnection
			req.Body = io.NopCloser(&mockDisconnectingBody{})
			
			res := httptest.NewRecorder()
			
			// This should handle the disconnection gracefully
			server.HandleRequest(res, req)
			
			// Should not return an error status, just handle gracefully
			assert.NotEqual(t, http.StatusInternalServerError, res.Code)
		})
	})

	t.Run("ServerInfo", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should return server start time", func(t *testing.T) {
			server := NewServer()
			startTime := server.StartTime()

			assert.NotZero(t, startTime)
			assert.True(t, time.Since(startTime) < time.Second)
		})

		t.Run("should return server ID", func(t *testing.T) {
			server := NewServer()
			serverId := server.ServerId()

			assert.NotEmpty(t, serverId)
		})
	})

	t.Run("Certificate", func(t *testing.T) {
		setupTestEnv()
		defer cleanupTestEnv()

		t.Run("should generate self-signed certificate", func(t *testing.T) {
			server := NewServer()
			server.certFile = filepath.Join("/tmp/certs", "cert.pem")
			server.keyFile = filepath.Join("/tmp/certs", "key.pem")
			server.caFile = filepath.Join("/tmp/certs", "ca.pem")

			server.generateSelfSignedCert()

			// Verify certificate files were created
			_, err := os.Stat(server.certFile)
			assert.NoError(t, err)
			_, err = os.Stat(server.keyFile)
			assert.NoError(t, err)

			// Verify certificate can be loaded
			cert := server.loadCertificate()
			assert.NotNil(t, cert)
			assert.NotNil(t, cert.Leaf)
		})

		t.Run("should load existing certificate", func(t *testing.T) {
			server := NewServer()
			server.certFile = filepath.Join("/tmp/certs", "cert.pem")
			server.keyFile = filepath.Join("/tmp/certs", "key.pem")
			server.caFile = filepath.Join("/tmp/certs", "ca.pem")

			// Generate certificate first
			server.generateSelfSignedCert()

			// Load the certificate
			cert := server.loadCertificate()
			assert.NotNil(t, cert)
			assert.NotNil(t, cert.Leaf)
		})
	})
}

// Mock types for testing client disconnection scenarios

type mockDisconnectingBody struct{}

func (m *mockDisconnectingBody) Read(p []byte) (n int, err error) {
	// Simulate client disconnection by returning a connection error
	return 0, fmt.Errorf("connection reset by peer")
}

type mockErrorBody struct{}

func (m *mockErrorBody) Read(p []byte) (n int, err error) {
	// Simulate a real error (not client disconnection)
	return 0, fmt.Errorf("disk full")
}

type mockConnectionError struct{}

func (m *mockConnectionError) Read(p []byte) (n int, err error) {
	// Simulate various connection-related errors
	return 0, fmt.Errorf("broken pipe")
}
