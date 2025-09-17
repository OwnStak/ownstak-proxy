package server

import (
	"bytes"
	"crypto/tls"
	"io"
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
			responseBody, err := serverReq.Body()
			assert.NoError(t, err)
			assert.Equal(t, []byte("test body content"), responseBody)
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
			body, err := serverReq.Body()
			assert.NoError(t, err)
			assert.Empty(t, body)
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
			body, err := serverReq.Body()
			assert.NoError(t, err)
			assert.Empty(t, body)
		})

		t.Run("should handle large body", func(t *testing.T) {
			largeBody := strings.Repeat("a", 10000)
			req, err := http.NewRequest("POST", "http://example.com/path", strings.NewReader(largeBody))
			assert.NoError(t, err)
			req.Host = "example.com"
			req.RemoteAddr = "192.168.1.1:12345"

			serverReq, err := NewRequest(req)
			assert.NoError(t, err)
			body, err := serverReq.Body()
			assert.NoError(t, err)
			assert.Equal(t, []byte(largeBody), body)
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

	t.Run("BodyReading", func(t *testing.T) {
		t.Run("Body", func(t *testing.T) {
			t.Run("should read simple body", func(t *testing.T) {
				bodyContent := "Hello, World!"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body)
			})

			t.Run("should read empty body", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/api", nil)
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Empty(t, body)
			})

			t.Run("should read large body", func(t *testing.T) {
				largeBodyContent := strings.Repeat("a", 100000) // 100KB
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(largeBodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(largeBodyContent), body)
			})

			t.Run("should cache body after first read", func(t *testing.T) {
				bodyContent := "Test body"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				// First read
				body1, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body1)

				// Second read should return cached body
				body2, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body2)
				assert.Equal(t, body1, body2)
			})

			t.Run("should handle JSON body", func(t *testing.T) {
				jsonBody := `{"name":"John","age":30}`
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(jsonBody))
				assert.NoError(t, err)
				req.Host = "example.com"
				req.Header.Set("Content-Type", "application/json")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(jsonBody), body)
			})

			t.Run("should handle binary body", func(t *testing.T) {
				binaryData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
				req, err := http.NewRequest("POST", "http://example.com/upload", bytes.NewReader(binaryData))
				assert.NoError(t, err)
				req.Host = "example.com"
				req.Header.Set("Content-Type", "image/png")

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, binaryData, body)
			})
		})

		t.Run("BodyReader", func(t *testing.T) {
			t.Run("should return reader for simple body", func(t *testing.T) {
				bodyContent := "Hello, World!"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				reader := serverReq.BodyReader()
				defer reader.Close()

				body, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body)
			})

			t.Run("should return reader for empty body", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/api", nil)
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				reader := serverReq.BodyReader()
				defer reader.Close()

				body, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Empty(t, body)
			})

			t.Run("should return reader for large body", func(t *testing.T) {
				largeBodyContent := strings.Repeat("b", 50000) // 50KB
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(largeBodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				reader := serverReq.BodyReader()
				defer reader.Close()

				body, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, []byte(largeBodyContent), body)
			})

			t.Run("should work after Body() has been called", func(t *testing.T) {
				bodyContent := "Test content"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				// First read with Body()
				body1, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body1)

				// Then read with BodyReader()
				reader := serverReq.BodyReader()
				defer reader.Close()

				body2, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body2)
			})

			t.Run("should work with partial reads", func(t *testing.T) {
				bodyContent := "This is a longer test content for partial reading"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				reader := serverReq.BodyReader()
				defer reader.Close()

				// Read in chunks
				chunk1 := make([]byte, 10)
				n1, err := reader.Read(chunk1)
				assert.NoError(t, err)
				assert.Equal(t, 10, n1)
				assert.Equal(t, []byte("This is a "), chunk1)

				chunk2 := make([]byte, 20)
				n2, err := reader.Read(chunk2)
				assert.NoError(t, err)
				assert.Equal(t, 20, n2)
				assert.Equal(t, []byte("longer test content "), chunk2)

				// Read rest
				remaining, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Equal(t, []byte("for partial reading"), remaining)
			})
		})

		t.Run("SetBody", func(t *testing.T) {
			t.Run("should set body content", func(t *testing.T) {
				req, err := http.NewRequest("GET", "http://example.com/api", nil)
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				newBody := []byte("New body content")
				serverReq.SetBody(newBody)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, newBody, body)
			})

			t.Run("should override original body", func(t *testing.T) {
				originalBody := "Original content"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(originalBody))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				newBody := []byte("Overridden content")
				serverReq.SetBody(newBody)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, newBody, body)
			})
		})

		t.Run("ClearBody", func(t *testing.T) {
			t.Run("should clear buffered body", func(t *testing.T) {
				bodyContent := "Content to clear"
				req, err := http.NewRequest("POST", "http://example.com/api", strings.NewReader(bodyContent))
				assert.NoError(t, err)
				req.Host = "example.com"

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				// First read body to buffer it
				body1, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Equal(t, []byte(bodyContent), body1)

				// Clear the body
				serverReq.ClearBody()

				// Now body should be empty since original reader was consumed
				body2, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Empty(t, body2)
			})
		})

		t.Run("EdgeCases", func(t *testing.T) {
			t.Run("should handle nil body reader", func(t *testing.T) {
				req, err := http.NewRequest("POST", "http://example.com/api", nil)
				assert.NoError(t, err)
				req.Host = "example.com"
				req.Body = nil

				serverReq, err := NewRequest(req)
				assert.NoError(t, err)

				body, err := serverReq.Body()
				assert.NoError(t, err)
				assert.Empty(t, body)

				reader := serverReq.BodyReader()
				defer reader.Close()
				readerBody, err := io.ReadAll(reader)
				assert.NoError(t, err)
				assert.Empty(t, readerBody)
			})
		})
	})
}
