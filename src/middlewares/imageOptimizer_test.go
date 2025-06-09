package middlewares

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jarcoal/httpmock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ownstak-proxy/src/server"
)

func TestImageOptimizerMiddleware(t *testing.T) {
	cleanupImageOptimizer := setupImageOptimizer(t)
	defer cleanupImageOptimizer()

	cleanupMockClient := setupImageOptimizerMockClient(t)
	defer cleanupMockClient()

	// Create middleware instance and inject mock client
	middleware := NewImageOptimizerMiddleware()
	middleware.client = http.DefaultClient // Inject the mocked client

	t.Run("Basic functionality", func(t *testing.T) {
		t.Run("should require url parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "URL parameter is required")
		})

		t.Run("should work with url parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "srcFormat=webp")
		})

		t.Run("should work with width parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&width=100", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "width=100")
		})

		t.Run("should work with w parameter (alias for width)", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&w=100", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "width=100")
		})

		t.Run("should work with height parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&height=150", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "height=150")
		})

		t.Run("should work with h parameter (alias for height)", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&h=150", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "height=150")
		})

		t.Run("should work with quality parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&quality=80", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "quality=80")
		})

		t.Run("should work with q parameter (alias for quality)", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&q=80", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "quality=80")
		})

		t.Run("should work with format parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&format=jpeg", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/jpeg", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "format=jpeg")
		})

		t.Run("should work with f parameter (alias for format)", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&f=jpeg", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/jpeg", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "format=jpeg")
		})

		t.Run("should work with enabled parameter", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&enabled=true", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "enabled=true")
		})

		t.Run("should work with e parameter (alias for enabled)", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&e=true", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "enabled=true")
		})

		t.Run("should work with all parameters together", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&w=100&h=150&q=80&f=jpeg&e=true", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/jpeg", ctx.Response.Headers.Get("Content-Type"))
			optimizerHeader := ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer)
			assert.Contains(t, optimizerHeader, "width=100")
			assert.Contains(t, optimizerHeader, "height=150")
			assert.Contains(t, optimizerHeader, "quality=80")
			assert.Contains(t, optimizerHeader, "format=jpeg")
			assert.Contains(t, optimizerHeader, "enabled=true")
		})

		t.Run("should validate quality range", func(t *testing.T) {
			// Create test request with invalid quality
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&q=0", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Quality must be a number between 1 and 100")
		})

		t.Run("should validate width range", func(t *testing.T) {
			// Create test request with invalid width
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&w=-1", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Width must be a positive number")
		})

		t.Run("should validate height range", func(t *testing.T) {
			// Create test request with invalid height
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&h=-1", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Height must be a positive number")
		})

		t.Run("should validate output format", func(t *testing.T) {
			// Create test request with invalid format
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&f=bmp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Unsupported output format")
		})

		t.Run("should prevent fetching from internal endpoints", func(t *testing.T) {
			// Create test request with internal path
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/__ownstak__/health", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Fetching images from /__ownstak__ path is not allowed")
		})

		t.Run("should allow to fetch from relative path", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Body to string
			body := string(ctx.Response.Body)
			assert.Equal(t, "", body)

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
		})

		t.Run("should allow to fetch from absolute path on same host", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=http://localhost:8080/image.webp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Body to string
			body := string(ctx.Response.Body)
			assert.Equal(t, "", body)

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
		})

		t.Run("should handle non-image content type", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/robots.txt", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "does not point to an image")
		})

		t.Run("should handle SVG images without optimization", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.svg", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/svg+xml", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "enabled=false")
		})

		t.Run("should handle disabled optimization", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&enabled=false", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer), "enabled=false")
		})

		t.Run("should limit max width to image original size", func(t *testing.T) {
			// Create test request with dimensions exceeding maxDimension (2560)
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&w=5000", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			optimizerHeader := ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer)
			assert.Contains(t, optimizerHeader, "width=100")
		})

		t.Run("should limit height to image original size", func(t *testing.T) {
			// Create test request with dimensions exceeding maxDimension (2560)
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&h=5000", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			optimizerHeader := ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer)
			assert.Contains(t, optimizerHeader, "height=150")
		})

		t.Run("should handle aspect ratio preservation", func(t *testing.T) {
			// Create test request with only width specified
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp&w=100", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			optimizerHeader := ctx.Response.Headers.Get(server.HeaderXOwnImageOptimizer)
			assert.Contains(t, optimizerHeader, "width=100")
			// Height should be calculated to preserve aspect ratio
			assert.Contains(t, optimizerHeader, "height=")
		})

		t.Run("should handle cache control headers", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/image.webp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusOK, ctx.Response.Status)
			assert.Equal(t, "image/webp", ctx.Response.Headers.Get("Content-Type"))
			assert.Contains(t, ctx.Response.Headers.Get("Cache-Control"), "public")
			assert.Contains(t, ctx.Response.Headers.Get("Cache-Control"), "max-age=")
		})

		t.Run("should limit max size of fetched image to 6MB", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/__ownstak__/image?url=/large-image.webp", nil)
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, nil)

			// Create and run middleware
			middleware.OnRequest(ctx, func() {})

			// Verify response
			assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "content-length header exceeds maximum limit")
		})
	})
}

func setupImageOptimizerMockClient(t *testing.T) func() {
	httpmock.Activate(t)

	img, err := os.Open("src/vips/vips_test.webp")
	require.NoError(t, err)

	// Read the image into a byte slice
	imgBytes, err := io.ReadAll(img)
	require.NoError(t, err)

	httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/image.webp" {
			resp := httpmock.NewBytesResponse(200, imgBytes)
			resp.Header.Set("Content-Type", "image/webp") // Set correct MIME type
			resp.Header.Set("Content-Length", strconv.Itoa(len(imgBytes)))
			return resp, nil
		}
		if req.URL.Path == "/large-image.webp" {
			imgBytes = make([]byte, 12*1024*1024) // 12MB
			for i := range imgBytes {
				imgBytes[i] = byte(i % 256)
			}
			resp := httpmock.NewBytesResponse(200, imgBytes)
			resp.Header.Set("Content-Type", "image/webp") // Set correct MIME type
			resp.Header.Set("Content-Length", strconv.Itoa(len(imgBytes)))
			return resp, nil
		}
		if req.URL.Path == "/image.svg" {
			resp := httpmock.NewStringResponse(200, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`)
			resp.Header.Set("Content-Type", "image/svg+xml") // Set correct MIME type
			return resp, nil
		}
		if req.URL.Path == "/robots.txt" {
			resp := httpmock.NewStringResponse(200, "User-agent: *\nDisallow: /")
			resp.Header.Set("Content-Type", "text/plain")
			return resp, nil
		}

		resp := httpmock.NewStringResponse(404, "Not Found")
		return resp, nil
	})

	return func() {
		httpmock.DeactivateAndReset()
	}
}

func setupImageOptimizer(t *testing.T) func() {
	// Get current working directory and project structure
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Calculate project root (go up from src/vips to project root)
	projectRoot := filepath.Dir(filepath.Dir(wd))

	// Change working directory to project root so lib/ path resolves correctly
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(projectRoot)
	require.NoError(t, err)

	// Return cleanup function
	return func() {
		os.Chdir(originalWd) // Restore original working directory
	}
}
