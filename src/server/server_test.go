package server

import (
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
			assert.Equal(t, 2*time.Minute, server.readTimeout)
			assert.Equal(t, 2*time.Hour, server.writeTimeout)
			assert.Equal(t, 60*time.Second, server.idleTimeout)
			assert.Equal(t, 1024*1024, server.maxHeaderBytes)
			assert.NotEmpty(t, server.serverId)
			assert.NotNil(t, server.MiddlewaresChain)
		})

		t.Run("should create server with custom values from environment", func(t *testing.T) {
			os.Setenv("HOST", "127.0.0.1")
			os.Setenv("HTTP_PORT", "8080")
			os.Setenv("HTTPS_PORT", "8443")
			os.Setenv("READ_TIMEOUT", "30s")
			os.Setenv("WRITE_TIMEOUT", "1h")
			os.Setenv("IDLE_TIMEOUT", "30s")
			os.Setenv("MAX_HEADER_BYTES", "2048")

			server := NewServer()
			assert.NotNil(t, server)
			assert.Equal(t, "127.0.0.1", server.host)
			assert.Equal(t, "8080", server.httpPort)
			assert.Equal(t, "8443", server.httpsPort)
			assert.Equal(t, 30*time.Second, server.readTimeout)
			assert.Equal(t, time.Hour, server.writeTimeout)
			assert.Equal(t, 30*time.Second, server.idleTimeout)
			assert.Equal(t, 2048, server.maxHeaderBytes)
		})

		t.Run("should handle invalid environment values", func(t *testing.T) {
			os.Setenv("READ_TIMEOUT", "invalid")
			os.Setenv("WRITE_TIMEOUT", "invalid")
			os.Setenv("IDLE_TIMEOUT", "invalid")
			os.Setenv("MAX_HEADER_BYTES", "invalid")

			server := NewServer()
			assert.NotNil(t, server)
			assert.Equal(t, 2*time.Minute, server.readTimeout)
			assert.Equal(t, 2*time.Hour, server.writeTimeout)
			assert.Equal(t, 60*time.Second, server.idleTimeout)
			assert.Equal(t, 1024*1024, server.maxHeaderBytes)
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
