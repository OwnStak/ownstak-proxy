package server

import (
	"crypto/tls"
	"net/http"
	"ownstak-proxy/src/constants"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequest(t *testing.T) {
	t.Run("NewRequest", func(t *testing.T) {
		t.Run("should create server request from basic HTTP request", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path?foo=bar", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			assert.NotNil(t, serverReq)

			assert.Equal(t, "GET", serverReq.Method)
			assert.Equal(t, "http://example.com/path?foo=bar", serverReq.URL)
			assert.Equal(t, "/path", serverReq.Path)
			assert.Equal(t, "example.com", serverReq.Host)
			assert.Equal(t, "http", serverReq.Scheme)
			assert.Equal(t, "80", serverReq.Port)
			assert.Equal(t, "192.168.1.1", serverReq.RemoteAddr)
			assert.Equal(t, "HTTP/1.1", serverReq.Protocol)
			assert.Equal(t, req, serverReq.OriginalRequest)

			// Check query parameters
			assert.Equal(t, "bar", serverReq.Query.Get("foo"))

			// Check required headers
			assert.Equal(t, "example.com", serverReq.Headers.Get(HeaderHost))
			assert.Equal(t, "true", serverReq.Headers.Get(HeaderXOwnProxy))
			assert.Equal(t, constants.Version, serverReq.Headers.Get(HeaderXOwnProxyVersion))
			assert.Equal(t, "192.168.1.1", serverReq.Headers.Get(HeaderXForwardedFor))
			assert.Equal(t, "http", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "80", serverReq.Headers.Get(HeaderXForwardedPort))
			assert.Equal(t, "example.com", serverReq.Headers.Get(HeaderXForwardedHost))
		})

		t.Run("should create request with HTTPS", func(t *testing.T) {
			req, err := http.NewRequest("GET", "https://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.TLS = &tls.ConnectionState{} // Simulate TLS connection
			req.RemoteAddr = "10.0.0.1:54321"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "https", serverReq.Scheme)
			assert.Equal(t, "443", serverReq.Port)
			assert.Equal(t, "https", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "443", serverReq.Headers.Get(HeaderXForwardedPort))
		})

		t.Run("should handle X-Own-Host header", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "original.com"
			req.Header.Set(HeaderXOwnHost, "custom.com")
			req.RemoteAddr = "127.0.0.1:8080"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Should use X-Own-Host value as the host
			assert.Equal(t, "custom.com", serverReq.Host)
			assert.Equal(t, "custom.com", serverReq.Headers.Get(HeaderHost))
			// Should still use original host for X-Forwarded-Host
			assert.Equal(t, "original.com", serverReq.Headers.Get(HeaderXForwardedHost))
		})

		t.Run("should handle request with body", func(t *testing.T) {
			body := strings.NewReader("test body content")
			req, err := http.NewRequest("POST", "http://example.com/api", body)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "POST", serverReq.Method)
			assert.Equal(t, []byte("test body content"), serverReq.Body)
		})

		t.Run("should handle request with custom port", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com:8080/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com:8080"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "8080", serverReq.Port)
			assert.Equal(t, "8080", serverReq.Headers.Get(HeaderXForwardedPort))
		})

		t.Run("should handle existing X-Forwarded headers", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			req.Header.Set(HeaderXForwardedFor, "10.0.0.1")
			req.Header.Set(HeaderXForwardedProto, "https")
			req.Header.Set(HeaderXForwardedPort, "443")
			req.Header.Set(HeaderXForwardedHost, "original.com")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Should append to existing headers
			assert.Equal(t, "10.0.0.1, 192.168.1.1", serverReq.Headers.Get(HeaderXForwardedFor))
			assert.Equal(t, "https, http", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "443, 80", serverReq.Headers.Get(HeaderXForwardedPort))
			assert.Equal(t, "original.com, example.com", serverReq.Headers.Get(HeaderXForwardedHost))
		})

		t.Run("should handle different HTTP protocols", func(t *testing.T) {
			// Test HTTP/1.0
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.ProtoMajor = 1
			req.ProtoMinor = 0
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			assert.Equal(t, "HTTP/1.0", serverReq.Protocol)

			// Test HTTP/2.0
			req, err = http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.ProtoMajor = 2
			req.ProtoMinor = 0
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err = NewRequest(req)
			assert.NoError(t, err)
			assert.Equal(t, "HTTP/2.0", serverReq.Protocol)

			// Test unknown protocol
			req, err = http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Proto = "HTTP/3.0"
			req.ProtoMajor = 3
			req.ProtoMinor = 0
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err = NewRequest(req)
			assert.NoError(t, err)
			assert.Equal(t, "HTTP/3.0", serverReq.Protocol)
		})

		t.Run("should handle empty host header", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = ""
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "", serverReq.Host)
			assert.Equal(t, "", serverReq.Headers.Get(HeaderHost))
			// X-Forwarded-Host should not be set when original host is empty
			assert.Equal(t, "", serverReq.Headers.Get(HeaderXForwardedHost))
		})

		t.Run("should handle complex query parameters", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path?a=1&b=2&a=3&c=hello%20world", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Should preserve all query parameters
			assert.Equal(t, []string{"1", "3"}, serverReq.Query["a"])
			assert.Equal(t, []string{"2"}, serverReq.Query["b"])
			assert.Equal(t, []string{"hello world"}, serverReq.Query["c"])
		})

		t.Run("should copy all original headers", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			req.Header.Set("Custom-Header", "custom-value")
			req.Header.Set("User-Agent", "test-agent")
			req.Header.Add("Multi-Value", "value1")
			req.Header.Add("Multi-Value", "value2")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Should copy all headers
			assert.Equal(t, "custom-value", serverReq.Headers.Get("Custom-Header"))
			assert.Equal(t, "test-agent", serverReq.Headers.Get("User-Agent"))
			assert.Equal(t, []string{"value1", "value2"}, serverReq.Headers["Multi-Value"])
		})
	})

	t.Run("RemoteAddr", func(t *testing.T) {
		tests := []struct {
			remoteAddr   string
			expectedAddr string
		}{
			{remoteAddr: "192.168.1.1:12345", expectedAddr: "192.168.1.1"},
			{remoteAddr: "127.0.0.1:8080", expectedAddr: "127.0.0.1"},
			{remoteAddr: "192.168.1.1", expectedAddr: "192.168.1.1"},        // No port
			{remoteAddr: "[2001:db8::1]:8080", expectedAddr: "2001:db8::1"}, // IPv6 with port
			{remoteAddr: "2001:db8::1", expectedAddr: "2001:db8::1"},        // IPv6 without port
			{remoteAddr: "[::1]:8080", expectedAddr: "::1"},                 // IPv6 localhost with port
			{remoteAddr: "::1", expectedAddr: "::1"},                        // IPv6 localhost without port
		}

		for _, test := range tests {
			t.Run(test.remoteAddr, func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = test.remoteAddr

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, test.expectedAddr, serverReq.RemoteAddr)
			})
		}
	})

	t.Run("XForwardedHeaders", func(t *testing.T) {
		t.Run("X-Forwarded-For", func(t *testing.T) {
			t.Run("should set X-Forwarded-For when not present", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "192.168.1.1", serverReq.Headers.Get(HeaderXForwardedFor))
			})

			t.Run("should append to existing X-Forwarded-For", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"
				req.Header.Set(HeaderXForwardedFor, "10.0.0.1")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "10.0.0.1, 192.168.1.1", serverReq.Headers.Get(HeaderXForwardedFor))
			})
		})

		t.Run("X-Forwarded-Proto", func(t *testing.T) {
			t.Run("should set X-Forwarded-Proto when not present", func(t *testing.T) {
				req, err := http.NewRequest("GET", "https://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "https", serverReq.Headers.Get(HeaderXForwardedProto))
			})

			t.Run("should append to existing X-Forwarded-Proto", func(t *testing.T) {
				req, err := http.NewRequest("GET", "https://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"
				req.Header.Set(HeaderXForwardedProto, "http")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "http, https", serverReq.Headers.Get(HeaderXForwardedProto))
			})
		})

		t.Run("X-Forwarded-Port", func(t *testing.T) {
			t.Run("should set X-Forwarded-Port when not present", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com:8080/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com:8080"
				req.RemoteAddr = "192.168.1.1:8080"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "8080", serverReq.Headers.Get(HeaderXForwardedPort))
			})

			t.Run("should append to existing X-Forwarded-Port", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com:8080/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com:8080"
				req.RemoteAddr = "192.168.1.1:8080"
				req.Header.Set(HeaderXForwardedPort, "443")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "443, 8080", serverReq.Headers.Get(HeaderXForwardedPort))
			})
		})

		t.Run("X-Forwarded-Host", func(t *testing.T) {
			t.Run("should set X-Forwarded-Host when not present", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "example.com", serverReq.Headers.Get(HeaderXForwardedHost))
			})

			t.Run("should append to existing X-Forwarded-Host", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/path", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.RemoteAddr = "192.168.1.1:8080"
				req.Header.Set(HeaderXForwardedHost, "original.com")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)
				assert.Equal(t, "original.com, example.com", serverReq.Headers.Get(HeaderXForwardedHost))
			})
		})
	})

	t.Run("Context", func(t *testing.T) {
		t.Run("should return context from original request", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			ctx := serverReq.Context()
			assert.NotNil(t, ctx)
			assert.Equal(t, req.Context(), ctx)
		})
	})

	t.Run("EdgeCases", func(t *testing.T) {
		t.Run("should handle nil body", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"
			req.Body = nil

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			assert.Empty(t, serverReq.Body)
		})

		t.Run("should handle body read error", func(t *testing.T) {
			req, err := http.NewRequest("POST", "http://example.com/path", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			// Create a reader that will cause an error
			req.Body = http.NoBody

			// This should not cause an error in practice, but let's test the error handling path
			serverReq, err := NewRequest(req)
			assert.NoError(t, err) // http.NoBody should not cause an error
			assert.Empty(t, serverReq.Body)
		})

		t.Run("should handle large body", func(t *testing.T) {
			largeBody := strings.Repeat("a", 10000)
			req, err := http.NewRequest("POST", "http://example.com/path", strings.NewReader(largeBody))
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			assert.Equal(t, []byte(largeBody), serverReq.Body)
		})

		t.Run("should handle URL with fragment", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/path?foo=bar#fragment", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Fragment should be preserved in URL but not affect path
			assert.Contains(t, serverReq.URL, "fragment")
			assert.Equal(t, "/path", serverReq.Path)
		})
	})

	t.Run("OriginalFields", func(t *testing.T) {
		t.Run("should set original fields from basic request with x-own-host header", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com/api/users?limit=10", nil)
			assert.NoError(t, err)
			req.Host = "example.com"
			req.Header.Set(HeaderXOwnHost, "project-123.aws-primary.org.ownstak.link")
			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			// Original fields should match current request when no X-Forwarded headers
			assert.Equal(t, "example.com", serverReq.Headers.Get(HeaderXForwardedHost))
			assert.Equal(t, "example.com", serverReq.OriginalHost)
			assert.Equal(t, "http", serverReq.OriginalScheme)
			assert.Equal(t, "80", serverReq.OriginalPort)
			assert.Equal(t, "http://example.com/api/users?limit=10", serverReq.OriginalURL)
		})

		t.Run("should set original fields from basic request without any proxy in front of it", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://project-123.aws-primary.org.ownstak.link/api/users?limit=10", nil)
			assert.NoError(t, err)
			req.Host = "project-123.aws-primary.org.ownstak.link"
			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "project-123.aws-primary.org.ownstak.link", serverReq.Headers.Get(HeaderXForwardedHost))
			assert.Equal(t, "project-123.aws-primary.org.ownstak.link", serverReq.OriginalHost)

			assert.Equal(t, "http", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "http", serverReq.OriginalScheme)

			assert.Equal(t, "80", serverReq.Headers.Get(HeaderXForwardedPort))
			assert.Equal(t, "80", serverReq.OriginalPort)

			assert.Equal(t, "http://project-123.aws-primary.org.ownstak.link/api/users?limit=10", serverReq.OriginalURL)
		})

		t.Run("should set original fields from basic request with 1 proxy in front of it", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://project-123.aws-primary.org.ownstak.link/api/users?limit=10", nil)
			assert.NoError(t, err)
			req.Host = "project-123.aws-primary.org.ownstak.link"
			req.Header.Set(HeaderXForwardedFor, "10.0.0.1")
			req.Header.Set(HeaderXForwardedProto, "https")
			req.Header.Set(HeaderXForwardedPort, "443")
			req.Header.Set(HeaderXForwardedHost, "original.com")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "original.com, project-123.aws-primary.org.ownstak.link", serverReq.Headers.Get(HeaderXForwardedHost))
			assert.Equal(t, "original.com", serverReq.OriginalHost)

			assert.Equal(t, "https, http", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "https", serverReq.OriginalScheme)

			assert.Equal(t, "443, 80", serverReq.Headers.Get(HeaderXForwardedPort))
			assert.Equal(t, "443", serverReq.OriginalPort)

			assert.Equal(t, "https://original.com/api/users?limit=10", serverReq.OriginalURL)
		})

		t.Run("should set original fields from basic request with 2 proxies in front of it", func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://project-123.aws-primary.org.ownstak.link/api/users?limit=10", nil)
			assert.NoError(t, err)
			req.Host = "project-123.aws-primary.org.ownstak.link"
			req.Header.Set(HeaderXForwardedFor, "10.0.0.1, 10.0.0.2")
			req.Header.Set(HeaderXForwardedProto, "https, http")
			req.Header.Set(HeaderXForwardedPort, "443, 80")
			req.Header.Set(HeaderXForwardedHost, "original.com, proxy-1.com")

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)

			assert.Equal(t, "original.com, proxy-1.com, project-123.aws-primary.org.ownstak.link", serverReq.Headers.Get(HeaderXForwardedHost))
			assert.Equal(t, "original.com", serverReq.OriginalHost)

			assert.Equal(t, "https, http, http", serverReq.Headers.Get(HeaderXForwardedProto))
			assert.Equal(t, "https", serverReq.OriginalScheme)

			assert.Equal(t, "443, 80, 80", serverReq.Headers.Get(HeaderXForwardedPort))
			assert.Equal(t, "443", serverReq.OriginalPort)

			assert.Equal(t, "https://original.com/api/users?limit=10", serverReq.OriginalURL)
		})

	})
}
