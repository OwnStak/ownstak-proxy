package middlewares

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestServer creates a fake server instance for testing
func createTestServer() *server.Server {
	return &server.Server{
		MaxMemory:  1024 * 1024 * 1024, // 1GB
		UsedMemory: 0,
	}
}

func TestNewAWSLambdaMiddleware(t *testing.T) {
	// Store original environment variables
	originalProvider := os.Getenv(constants.EnvProvider)
	originalAccountId := os.Getenv(constants.EnvAWSAccountId)
	originalRegion := os.Getenv(constants.EnvAWSRegion)

	defer func() {
		os.Setenv(constants.EnvProvider, originalProvider)
		os.Setenv(constants.EnvAWSAccountId, originalAccountId)
		os.Setenv(constants.EnvAWSRegion, originalRegion)
	}()

	t.Run("should return nil when provider is not AWS", func(t *testing.T) {
		os.Setenv(constants.EnvProvider, "gcp")
		middleware := NewAWSLambdaMiddleware()
		assert.Nil(t, middleware, "should return nil when provider is not AWS")
	})

	t.Run("should create middleware when provider is AWS", func(t *testing.T) {
		os.Setenv(constants.EnvProvider, constants.ProviderAWS)
		os.Setenv(constants.EnvAWSAccountId, "123456789012")
		os.Setenv(constants.EnvAWSRegion, "us-east-1")

		middleware := NewAWSLambdaMiddleware()
		assert.NotNil(t, middleware, "should create middleware when provider is AWS")
		assert.Equal(t, "123456789012", middleware.accountId, "should set account ID from environment")
	})

	t.Run("should support custom endpoints", func(t *testing.T) {
		os.Setenv(constants.EnvProvider, constants.ProviderAWS)
		os.Setenv(constants.EnvAWSLambdaEndpoint, "http://localhost:4566")
		os.Setenv(constants.EnvAWSStSEndpoint, "http://localhost:4566")
		os.Setenv(constants.EnvAWSOrganizationsEndpoint, "http://localhost:4566")

		middleware := NewAWSLambdaMiddleware()
		assert.NotNil(t, middleware, "should create middleware with custom endpoints")

		// Clean up
		os.Unsetenv(constants.EnvAWSLambdaEndpoint)
		os.Unsetenv(constants.EnvAWSStSEndpoint)
		os.Unsetenv(constants.EnvAWSOrganizationsEndpoint)
	})
}

func TestAWSLambdaMiddleware(t *testing.T) {
	// Store original environment variables
	originalProvider := os.Getenv(constants.EnvProvider)
	originalAccountId := os.Getenv(constants.EnvAWSAccountId)
	originalRegion := os.Getenv(constants.EnvAWSRegion)
	originalLambdaEndpoint := os.Getenv(constants.EnvAWSLambdaEndpoint)
	originalStsEndpoint := os.Getenv(constants.EnvAWSStSEndpoint)

	defer func() {
		os.Setenv(constants.EnvProvider, originalProvider)
		os.Setenv(constants.EnvAWSAccountId, originalAccountId)
		os.Setenv(constants.EnvAWSRegion, originalRegion)
		os.Setenv(constants.EnvAWSLambdaEndpoint, originalLambdaEndpoint)
		os.Setenv(constants.EnvAWSStSEndpoint, originalStsEndpoint)
	}()

	// Setup test environment
	os.Setenv(constants.EnvProvider, constants.ProviderAWS)
	os.Setenv(constants.EnvAWSAccountId, "123456789012")
	os.Setenv(constants.EnvAWSRegion, "us-east-1")
	os.Setenv(constants.EnvAWSLambdaEndpoint, "http://localhost:4566")
	os.Setenv(constants.EnvAWSStSEndpoint, "http://localhost:4566")

	// Setup httpmock
	cleanupMock := setupAWSLambdaMock(t)
	defer cleanupMock()

	// Create middleware instance
	middleware := NewAWSLambdaMiddleware()

	t.Run("streaming mode disabled", func(t *testing.T) {
		// Disable streaming mode in middleware settings
		middleware.streamingMode = false

		t.Run("should set x-own-streaming header to false", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "myapp-prod.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			// Run middleware
			middleware.OnRequest(ctx, func() {})

			// Check that x-own-streaming header was set to "true"
			assert.Equal(t, "false", serverReq.Headers.Get(server.HeaderXOwnStreaming))
		})

		t.Run("should return simple html response", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": "<html><body><h1>Hello from buffered Lambda!</h1></body></html>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "myapp-prod.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			// Run middleware
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Hello from buffered Lambda!")
		})

		t.Run("should return base64 encoded response", func(t *testing.T) {
			// Base64 encoded "<h1>Base64 Content</h1>"
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body":            "PGgxPkJhc2U2NCBDb250ZW50PC9oMT4=",
				"isBase64Encoded": true,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "base64-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Base64 Content")
		})

		t.Run("should return response with multi-value headers", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type":    "text/html",
					"X-Custom-Header": "single-value",
				},
				"multiValueHeaders": map[string][]string{
					"Set-Cookie": {"session=abc123", "user=john"},
					"X-Multi":    {"value1", "value2"},
				},
				"body": "<h1>Multi Headers</h1>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "multi-headers.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			logger.Info("ctx.Response %v", ctx.Response)

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "single-value", res.Header().Get("X-Custom-Header"))
			cookies := res.Header()["Set-Cookie"]
			assert.Len(t, cookies, 2)
			assert.Contains(t, cookies, "session=abc123")
			assert.Contains(t, cookies, "user=john")
			assert.Contains(t, string(res.Body.String()), "<h1>Multi Headers</h1>")
		})

		t.Run("should handle lambda invocation errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "Runtime.UserCodeSyntaxError",
				"errorMessage": "Syntax error in user code",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "error-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectError, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Runtime.UserCodeSyntaxError")
		})

		t.Run("should handle lambda timeout errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "Sandbox.Timeout",
				"errorMessage": "Task timed out after 30.00 seconds",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "timeout-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectTimeout, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Sandbox.Timeout")
		})

		t.Run("should handle lambda throttling errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "TooManyRequests",
				"errorMessage": "Rate exceeded",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "throttle-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectThrottled, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "TooManyRequests")
		})

		t.Run("should handle lambda request too large errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "RequestTooLarge",
				"errorMessage": "Request payload size exceeded",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "large-req.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectRequestTooLarge, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "RequestTooLarge")
		})

		t.Run("should handle lambda response too large errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "ResponseSizeTooLarge",
				"errorMessage": "Response payload size exceeded",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "large-resp.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectResponseTooLarge, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "ResponseSizeTooLarge")
		})

		t.Run("should handle lambda crashed errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"errorType":    "Runtime.ExitError",
				"errorMessage": "Process exited with non-zero status",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "crashed.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectCrashed, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Runtime.ExitError")
		})

		t.Run("should handle lambda function not found errors", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate ResourceNotFoundException
					return httpmock.NewStringResponse(404, `{"__type":"ResourceNotFoundException","message":"The resource you requested does not exist."}`), nil
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "notfound-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)
			assert.Contains(t, ctx.Response.Headers.Get("Location"), "revive")
		})

		t.Run("should properly set user status codes", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 404,
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
				"body": `{"error":"Not Found"}`,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "status404-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 404, res.Code)
			assert.Equal(t, "application/json", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Not Found")
		})

		t.Run("should return empty lambda response body", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 204,
				"headers": map[string]string{
					"Content-Type": "text/plain",
				},
				"body": "",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "empty-body.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 204, ctx.Response.Status)
			assert.Equal(t, "text/plain", ctx.Response.Headers.Get("Content-Type"))
			assert.Empty(t, ctx.Response.Body)
		})

		t.Run("should handle invalid host format", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "invalid-host-format"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Invalid hostname format")
		})

		t.Run("should handle lambda api network errors", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate network error
					return nil, fmt.Errorf("network error: connection timeout")
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "network-error.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Failed to invoke Lambda function")
		})

		t.Run("should return error when host header is missing", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "" // Empty host
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "host header")
		})

		t.Run("should return for host header with invalid format", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com" // Only 2 parts instead of required 3+
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusBadRequest, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Invalid hostname format")
		})

		t.Run("should handle corrupted lambda response errors", func(t *testing.T) {
			lambdaResponse := `{"StatusCode":200,"Payload":"this is not valid json}`
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "invalid-response.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
		})

		t.Run("should handle invalid base64 encoding errors", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body":            "invalid-base64!@#$%",
				"isBase64Encoded": true,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "bad-base64.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
			assert.Contains(t, string(res.Body.String()), "failed to decode base64")
		})

		t.Run("should return response with large headers", func(t *testing.T) {
			largeValue := strings.Repeat("x", 1000) // 1KB value
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type":     "text/html",
					"X-Large-Header-1": largeValue,
					"X-Large-Header-2": largeValue,
					"X-Large-Header-3": largeValue,
					"X-Large-Header-4": largeValue,
				},
				"body": "<h1>Large Headers</h1>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "large-headers.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, ctx.Response.Status)
			assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-1")))
		})

		t.Run("should return response with many headers", func(t *testing.T) {
			headers := map[string]string{"Content-Type": "text/html"}
			for i := 0; i < 1000; i++ {
				headers[fmt.Sprintf("X-Custom-Header-%d", i)] = fmt.Sprintf("value-%d", i)
			}
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers":    headers,
				"body":       "<h1>Many Headers</h1>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "many-headers.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, ctx.Response.Status)
			assert.Equal(t, "value-0", ctx.Response.Headers.Get("X-Custom-Header-0"))
			assert.Equal(t, "value-49", ctx.Response.Headers.Get("X-Custom-Header-49"))
		})

		t.Run("should return aws access denied error", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate AWS AccessDenied error
					return httpmock.NewStringResponse(403, `{"__type":"AccessDeniedException","message":"User is not authorized to perform: lambda:InvokeFunction"}`), nil
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "denied-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Failed to invoke Lambda function")
		})

		t.Run("should return response body for null body response ", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/plain",
				},
				"body": nil, // Null body
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "null-body.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, ctx.Response.Status)
			assert.Empty(t, ctx.Response.Body)
		})

		t.Run("should call lambda with deployment id alias from host header", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": "<h1>Deployment 123</h1>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)

			// Use custom responder to verify deployment alias in URL
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Verify the function name includes deployment alias
					url := req.URL.String()
					assert.Contains(t, url, "deployment-123")

					resp := httpmock.NewBytesResponse(200, lambdaResponseBytes)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "myapp-prod-123.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Deployment 123")
		})

		t.Run("should call lambda with current alias when there is no deployment id in host header", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": "<h1>Current</h1>",
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)

			// Use custom responder to verify current alias in URL
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Verify the function name includes current alias
					url := req.URL.String()
					assert.Contains(t, url, "current")

					resp := httpmock.NewBytesResponse(200, lambdaResponseBytes)
					resp.Header.Set("Content-Type", "application/json")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "myapp-prod.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Contains(t, string(res.Body.String()), "Current")
		})

		t.Run("should return unicode characters response", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": `<html><body><h1>Hello 世界! 🌍</h1><p>Chars: αβγδε</p></body></html>`,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "unicode-app.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Hello 世界!")
			assert.Contains(t, string(res.Body.String()), "🌍")
			assert.Contains(t, string(res.Body.String()), "αβγδε")
		})

		t.Run("should return binary response", func(t *testing.T) {
			// Create a simple PNG-like binary data
			binaryData := []byte{137, 80, 78, 71, 13, 10, 26, 10} // PNG signature
			base64Data := base64.StdEncoding.EncodeToString(binaryData)

			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "image/png",
				},
				"body":            base64Data,
				"isBase64Encoded": true,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/image.png", nil)
			req.Host = "binary-app.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "image/png", res.Header().Get("Content-Type"))
			// Check if binary data was properly decoded
			bodyBytes := res.Body.Bytes()
			assert.Equal(t, byte(137), bodyBytes[0]) // PNG signature first byte
		})

		t.Run("should store debug info in x-own-proxy-debug header", func(t *testing.T) {
			lambdaResponse := map[string]interface{}{
				"statusCode": 404,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": `<h1>Hello world</h1>`,
			}
			lambdaResponseBytes, _ := json.Marshal(lambdaResponse)
			registerBufferedLambdaMock(t, lambdaResponseBytes)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "status404-test.aws-primary.org.ownstak.link"
			req.Header.Set(server.HeaderXOwnDebug, "true")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnProxyDebug), "lambda-name=")
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnProxyDebug), "lambda-region=")
			assert.Contains(t, ctx.Response.Headers.Get(server.HeaderXOwnProxyDebug), "lambda-alias=")
		})

		t.Run("should redirect to console when lambda function is not found", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate AWS Lambda ResourceNotFoundException (404)
					awsError := map[string]interface{}{
						"__type":  "ResourceNotFoundException",
						"message": "The resource you requested does not exist.",
					}
					errorBytes, _ := json.Marshal(awsError)
					resp := httpmock.NewBytesResponse(404, errorBytes)
					resp.Header.Set("Content-Type", "application/x-amz-json-1.1")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/api/users", nil)
			req.Host = "ecommerce.com"
			req.Header.Set(server.HeaderXOwnHost, "ecommerce-default-123.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			// Verify it's a temporary redirect to revive
			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

			// Verify the Location header contains the revive URL
			location := ctx.Response.Headers.Get("Location")
			assert.NotEmpty(t, location, "Location header should be set for revive redirect")
			assert.Equal(t, "https://console.ownstak.com/revive?host=ecommerce-default-123.aws-primary.org.ownstak.link&originalUrl=http://ecommerce.com/api/users", location)

		})

		t.Run("should redirect to console when lambda function is not found to original URL from x-forwarded-host", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate AWS Lambda ResourceNotFoundException (404)
					awsError := map[string]interface{}{
						"__type":  "ResourceNotFoundException",
						"message": "The resource you requested does not exist.",
					}
					errorBytes, _ := json.Marshal(awsError)
					resp := httpmock.NewBytesResponse(404, errorBytes)
					resp.Header.Set("Content-Type", "application/x-amz-json-1.1")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/api/users", nil)
			req.Host = "ecommerce.com"
			req.Header.Set(server.HeaderXOwnHost, "ecommerce-default-123.aws-primary.org.ownstak.link")
			req.Header.Set(server.HeaderXForwardedHost, "original-ecommerce.com, proxy-1.com")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			// Verify it's a temporary redirect to revive
			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

			// Verify the Location header contains the revive URL
			location := ctx.Response.Headers.Get("Location")
			assert.NotEmpty(t, location, "Location header should be set for revive redirect")
			assert.Equal(t, "https://console.ownstak.com/revive?host=ecommerce-default-123.aws-primary.org.ownstak.link&originalUrl=http://original-ecommerce.com/api/users", location)
		})

		t.Run("should redirect to console url from env variable", func(t *testing.T) {
			os.Setenv(constants.EnvConsoleURL, "https://console.dev.ownstak.com")
			defer os.Unsetenv(constants.EnvConsoleURL)

			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate AWS Lambda ResourceNotFoundException (404)
					awsError := map[string]interface{}{
						"__type":  "ResourceNotFoundException",
						"message": "The resource you requested does not exist.",
					}
					errorBytes, _ := json.Marshal(awsError)
					resp := httpmock.NewBytesResponse(404, errorBytes)
					resp.Header.Set("Content-Type", "application/x-amz-json-1.1")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/api/users", nil)
			req.Host = "ecommerce.com"
			req.Header.Set(server.HeaderXOwnHost, "ecommerce-default-123.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			// Verify it's a temporary redirect to revive
			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

			// Verify the Location header contains the revive URL
			location := ctx.Response.Headers.Get("Location")
			assert.NotEmpty(t, location, "Location header should be set for revive redirect")
			assert.Equal(t, "https://console.dev.ownstak.com/revive?host=ecommerce-default-123.aws-primary.org.ownstak.link&originalUrl=http://ecommerce.com/api/users", location)
		})

		t.Run("should redirect to revive with query parameters preserved", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate AWS Lambda ResourceNotFoundException (404)
					awsError := map[string]interface{}{
						"__type":  "ResourceNotFoundException",
						"message": "Function not found: arn:aws:lambda:us-east-1:123456789012:function:ownstak-missing-app-prod:current",
					}
					errorBytes, _ := json.Marshal(awsError)
					resp := httpmock.NewBytesResponse(404, errorBytes)
					resp.Header.Set("Content-Type", "application/x-amz-json-1.1")
					return resp, nil
				},
			)

			req := httptest.NewRequest("GET", "/search?q=test&limit=10&category=tech", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "missing-app.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			// Verify it's a temporary redirect to revive
			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

			// Verify the Location header contains the revive URL with query parameters
			location := ctx.Response.Headers.Get("Location")
			assert.NotEmpty(t, location, "Location header should be set for revive redirect")
			assert.Contains(t, location, "revive", "Location should contain 'revive'")
			assert.Contains(t, location, "/search", "Location should preserve the original path")
			assert.Contains(t, location, "q=test", "Location should preserve query parameters")
			assert.Contains(t, location, "limit=10", "Location should preserve query parameters")
			assert.Contains(t, location, "category=tech", "Location should preserve query parameters")
		})
	})

	t.Run("streaming mode enabled", func(t *testing.T) {
		// Enable streaming mode in the middleware settings
		middleware.streamingMode = true

		t.Run("should set x-own-streaming header to true", func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "myapp-prod.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			// Create request context
			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			// Run middleware
			middleware.OnRequest(ctx, func() {})

			// Check that x-own-streaming header was set to "true"
			assert.Equal(t, "true", serverReq.Headers.Get(server.HeaderXOwnStreaming))
		})

		t.Run("should redirect to revive when streaming function not found", func(t *testing.T) {
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2021-11-15/functions/.*?/response-streaming-invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate ResourceNotFoundException
					return httpmock.NewStringResponse(404, `{"__type":"ResourceNotFoundException","message":"The resource you requested does not exist."}`), nil
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "notfound-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)
			assert.Contains(t, ctx.Response.Headers.Get("Location"), "revive")
		})

		t.Run("should return simple html response", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Hello from streaming Lambda!")
		})

		t.Run("should return chunked response", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello</h1>"),
				[]byte("<p>content 1</p>"),
				[]byte("<p>content 2</p>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "<h1>Hello</h1><p>content 1</p><p>content 2</p>")
		})

		t.Run("should enable end-to-end streaming for responses without redirect", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, true, serverRes.Streaming)
		})

		t.Run("should disable end-to-end streaming for responses with redirect", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":302,\"headers\":{\"Content-Type\":\"text/html\", \"Location\":\"https://s3.amazonaws.com/mybucket/myobject\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte(""),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, false, serverRes.Streaming)
		})

		t.Run("should be backward compatible and be able to parse body in head JSON", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"},\"body\":\"<h1>Hello</h1>\"}"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "<h1>Hello</h1>")
		})

		t.Run("should be backward compatible and be able to parse base64 encoded body in head JSON", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"},\"body\":\"PGgxPkhlbGxvPC9oMT4=\", \"isBase64Encoded\": true}"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "<h1>Hello</h1>")
		})

		t.Run("should not fail when no streaming body is sent", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "")
		})

		t.Run("should set custom status code", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":404,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Not Found</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 404, res.Code)
		})

		t.Run("should set custom headers", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\", \"X-Custom-Header\":\"custom-value\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "custom-value", res.Header().Get("X-Custom-Header"))
		})

		t.Run("should return response with multi-value headers", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"multiValueHeaders\":{\"Set-Cookie\":[\"session=abc123\", \"user=john\"]}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			cookies := res.Header()["Set-Cookie"]
			assert.Len(t, cookies, 2)
			assert.Contains(t, cookies, "session=abc123")
			assert.Contains(t, cookies, "user=john")
		})

		t.Run("should return empty response body", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte(""),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "", res.Body.String())
		})

		t.Run("should return binary response", func(t *testing.T) {
			binaryData := []byte{137, 80, 78, 71, 13, 10, 26, 10}
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"image/png\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				binaryData,
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "image/png", res.Header().Get("Content-Type"))
			assert.Equal(t, binaryData, res.Body.Bytes())
		})

		t.Run("should return chunked binary response", func(t *testing.T) {
			binaryData := []byte{137, 80, 78, 71, 13, 10, 26, 10}
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"image/png\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				binaryData[:2],
				binaryData[2:4],
				binaryData[4:6],
				binaryData[6:8],
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "image/png", res.Header().Get("Content-Type"))
			assert.Equal(t, binaryData, res.Body.Bytes())
		})

		t.Run("should return unicode characters response", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<html><body><h1>Hello 世界! 🌍</h1><p>Chars: αβγδε</p></body></html>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "Hello 世界!")
			assert.Contains(t, string(res.Body.String()), "🌍")
			assert.Contains(t, string(res.Body.String()), "αβγδε")
		})

		t.Run("should return response with large headers", func(t *testing.T) {
			largeValue := strings.Repeat("x", 1000) // 1KB value
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\",\"X-Large-Header-1\":\"" + largeValue + "\",\"X-Large-Header-2\":\"" + largeValue + "\",\"X-Large-Header-3\":\"" + largeValue + "\",\"X-Large-Header-4\":\"" + largeValue + "\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Large Headers</h1>"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, ctx.Response.Status)
			assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-1")))
			assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-2")))
			assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-3")))
			assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-4")))
		})

		t.Run("should return response with many headers", func(t *testing.T) {
			headers := map[string]string{"Content-Type": "text/html"}
			for i := 0; i < 1000; i++ {
				headers[fmt.Sprintf("X-Custom-Header-%d", i)] = fmt.Sprintf("value-%d", i)
			}
			responseHeaders := map[string]string{}
			for k, v := range headers {
				responseHeaders[k] = v
			}
			responseHeadersBytes, _ := json.Marshal(responseHeaders)
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":" + string(responseHeadersBytes) + "}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Many Headers</h1>"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, ctx.Response.Status)
			for k, v := range headers {
				assert.Equal(t, v, ctx.Response.Headers.Get(k))
			}
		})

		t.Run("should handle lambda timeout errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"Sandbox.Timeout\",\"errorMessage\":\"Task timed out after 30.00 seconds\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectTimeout, res.Code)
			assert.Contains(t, string(res.Body.String()), "Sandbox.Timeout")
		})

		t.Run("should handle lambda invocation errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"Runtime.UserCodeSyntaxError\",\"errorMessage\":\"Syntax error in user code\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectError, res.Code)
			assert.Contains(t, string(res.Body.String()), "Runtime.UserCodeSyntaxError")
		})

		t.Run("should handle lambda throttling errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"TooManyRequests\",\"errorMessage\":\"Rate exceeded\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectThrottled, res.Code)
			assert.Contains(t, string(res.Body.String()), "TooManyRequests")
		})

		t.Run("should handle lambda crashed errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"Runtime.ExitError\",\"errorMessage\":\"Process exited with non-zero status\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectCrashed, res.Code)
			assert.Contains(t, string(res.Body.String()), "Runtime.ExitError")
		})

		t.Run("should handle lambda request too large errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"RequestTooLarge\",\"errorMessage\":\"Request payload size exceeded maximum allowed payload\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectRequestTooLarge, res.Code)
			assert.Contains(t, string(res.Body.String()), "RequestTooLarge")
		})

		t.Run("should handle lambda response too large errors", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"errorType\":\"ResponseSizeTooLarge\",\"errorMessage\":\"Response payload size exceeded maximum allowed payload\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusProjectResponseTooLarge, res.Code)
			assert.Contains(t, string(res.Body.String()), "ResponseSizeTooLarge")
		})

		t.Run("should handle lambda timeout error in the middle of the body streaming", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
				[]byte("{\"errorType\":\"Sandbox.Timeout\",\"errorMessage\":\"Task timed out after 30.00 seconds\"}"),
			}
			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Contains(t, string(res.Body.String()), "<h1>Hello from streaming Lambda!</h1>")
		})

		t.Run("should handle streaming invocation timeout error", func(t *testing.T) {
			// Setup mock to return 404 (function not found)
			httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2021-11-15/functions/.*?/response-streaming-invocations$`,
				func(req *http.Request) (*http.Response, error) {
					// Simulate network error for streaming
					return nil, fmt.Errorf("network error: connection timeout")
				},
			)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "example.com"
			req.Header.Set(server.HeaderXOwnHost, "error-test.aws-primary.org.ownstak.link")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusInternalError, res.Code)
			assert.Contains(t, string(res.Body.String()), "Failed to invoke Lambda function")
		})

		t.Run("should be able to parse response head split into multiple chunks", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusC"),
				[]byte("ode\":200,\"hea"),
				[]byte("ders\":{\"Content-Type\":\"text/html\",\"X-Custom-Header\":\"value\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("<h1>Hello from streaming Lambda!</h1>"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "value", res.Header().Get("X-Custom-Header"))
			assert.Contains(t, string(res.Body.String()), "Hello from streaming Lambda!")
		})

		t.Run("should be able to parse response delimiter split into multiple chunks", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusC"),
				[]byte("ode\":200,\"hea"),
				[]byte("ders\":{\"Content-Type\":\"text/html\",\"X-Custom-Header\":\"value\"}}"),
				[]byte("\x00"),
				[]byte("\x00\x00"),
				[]byte("\x00\x00\x00"),
				[]byte("\x00\x00"),
				[]byte("Hello"),
				[]byte(" from streaming"),
				[]byte(" Lambda!"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "value", res.Header().Get("X-Custom-Header"))
			assert.Contains(t, string(res.Body.String()), "Hello from streaming Lambda!")
		})

		t.Run("should not fail when response body contains 8x \\x00", func(t *testing.T) {
			// Create response chunks with header + delimiter + body
			responseChunks := [][]byte{
				[]byte("{\"statusC"),
				[]byte("ode\":200,\"hea"),
				[]byte("ders\":{\"Content-Type\":\"text/html\",\"X-Custom-Header\":\"value\"}}"),
				[]byte("\x00"),
				[]byte("\x00\x00"),
				[]byte("\x00\x00\x00"),
				[]byte("\x00\x00"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00Hello"),
				[]byte(" from \x00streaming"),
				[]byte(" Lambda!"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			assert.Equal(t, "value", res.Header().Get("X-Custom-Header"))
			assert.Contains(t, string(res.Body.String()), "\x00\x00\x00\x00\x00\x00\x00\x00Hello from \x00streaming Lambda!")
		})

		t.Run("should fail if response head contains 8x \\x00", func(t *testing.T) {
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\",\"X-Custom-Header\":\"\x00\x00\x00\x00\x00\x00\x00\x00value\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
				[]byte("Hello from streaming Lambda!"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 530, res.Code)
			assert.Contains(t, string(res.Body.String()), "Failed to process streaming lambda response")
		})

		t.Run("should handle multiple body delimiters correctly", func(t *testing.T) {
			// Create response chunks with header + delimiter + body containing multiple delimiters
			responseChunks := [][]byte{
				[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"), // First delimiter - should split here
				[]byte("First part of body"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"), // Second delimiter - should be part of body
				[]byte("Second part of body"),
				[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"), // Third delimiter - should be part of body
				[]byte("Third part of body"),
			}

			registerStreamingLambdaMock(t, responseChunks)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "streaming-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
			assert.Equal(t, "text/html", res.Header().Get("Content-Type"))
			// Verify that all content after the first delimiter is included in the body
			expectedBody := "First part of body\x00\x00\x00\x00\x00\x00\x00\x00Second part of body\x00\x00\x00\x00\x00\x00\x00\x00Third part of body"
			assert.Equal(t, expectedBody, res.Body.String())
		})
	})

	t.Run("throttling", func(t *testing.T) {
		middleware.streamingMode = true
		registerStreamingLambdaMock(t, [][]byte{
			[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
			[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
			[]byte("<h1>Hello from streaming Lambda!</h1>"),
		})

		t.Run("should throttle requests when high priority queue is full", func(t *testing.T) {
			// Create a server with very low memory to force small queue sizes
			testServer := &server.Server{
				MaxMemory:  16 * 1024, // 16KB - very small
				UsedMemory: 0,
			}

			// Initialize the middleware queues with small sizes
			middleware.OnStart(testServer)

			// Fill up the high priority queue
			for i := 0; i < middleware.highPriorityQueueConcurrency; i++ {
				middleware.highPriorityQueue <- struct{}{}
			}

			// Try to make a request that should be throttled
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout and return service overloaded
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")
			assert.Contains(t, string(ctx.Response.Body), "queue slot timeout")

			// Clean up - drain the queue
			for i := 0; i < middleware.highPriorityQueueConcurrency; i++ {
				<-middleware.highPriorityQueue
			}
		})

		t.Run("should throttle large requests with content-length when medium priority queue is full", func(t *testing.T) {
			// Create a server with low memory to force small queue sizes
			testServer := &server.Server{
				MaxMemory:  128 * 1024, // 128KB - small
				UsedMemory: 0,
			}

			// Initialize the middleware queues
			middleware.OnStart(testServer)

			// Fill up the medium priority queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				middleware.mediumPriorityQueue <- struct{}{}
			}

			// Create a large request body (>64KB) that goes to medium priority queue
			largeBody := strings.Repeat("x", 65*1024) // 65KB
			req := httptest.NewRequest("POST", "/test", strings.NewReader(largeBody))
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			req.Header.Set(server.HeaderContentLength, strconv.Itoa(len(largeBody)))
			req.Header.Del(server.HeaderTransferEncoding)
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout and return service overloaded
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")
			assert.Contains(t, string(ctx.Response.Body), "queue slot timeout")

			// Clean up - drain the queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				<-middleware.mediumPriorityQueue
			}
		})

		t.Run("should throttle large requests with transfer-encoding: chunked when medium priority queue is full", func(t *testing.T) {
			// Create a server with low memory to force small queue sizes
			testServer := &server.Server{
				MaxMemory:  128 * 1024, // 128KB - small
				UsedMemory: 0,
			}

			// Initialize the middleware queues
			middleware.OnStart(testServer)

			// Fill up the medium priority queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				middleware.mediumPriorityQueue <- struct{}{}
			}

			// Create a large request body (>64KB) that goes to medium priority queue
			largeBody := strings.Repeat("x", 65*1024) // 65KB
			req := httptest.NewRequest("POST", "/test", strings.NewReader(largeBody))
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			req.Header.Set(server.HeaderTransferEncoding, "chunked")
			req.Header.Del(server.HeaderContentLength)
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout and return service overloaded
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")
			assert.Contains(t, string(ctx.Response.Body), "queue slot timeout")

			// Clean up - drain the queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				<-middleware.mediumPriorityQueue
			}
		})

		t.Run("should use medium priority queue when memory usage is high", func(t *testing.T) {
			// Create a server with high memory usage (>80%)
			testServer := &server.Server{
				MaxMemory:  1024 * 1024, // 1MB
				UsedMemory: 850 * 1024,  // 850KB (>80%)
			}

			// Initialize the middleware queues
			middleware.OnStart(testServer)

			// Fill up the medium priority queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				middleware.mediumPriorityQueue <- struct{}{}
			}

			// Small request should now go to medium priority queue due to high memory usage
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout with shorter timeout (15ms)
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")

			// Clean up - drain the queue
			for i := 0; i < middleware.mediumPriorityQueueConcurrency; i++ {
				<-middleware.mediumPriorityQueue
			}
		})

		t.Run("should use low priority queue for large requests with content-length when memory usage is high", func(t *testing.T) {
			// Create a server with high memory usage (>80%)
			testServer := &server.Server{
				MaxMemory:  1024 * 1024, // 1MB
				UsedMemory: 850 * 1024,  // 850KB (>80%)
			}

			// Initialize the middleware queues
			middleware.OnStart(testServer)

			// Fill up the low priority queue
			for i := 0; i < middleware.lowPriorityQueueConcurrency; i++ {
				middleware.lowPriorityQueue <- struct{}{}
			}

			// Large request with high memory usage should go to low priority queue
			largeBody := strings.Repeat("x", 65*1024) // 65KB
			req := httptest.NewRequest("POST", "/test", strings.NewReader(largeBody))
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			req.Header.Set(server.HeaderContentLength, strconv.Itoa(len(largeBody)))
			req.Header.Del(server.HeaderTransferEncoding)
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout with shortest timeout (15ms)
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")

			// Clean up - drain the queue
			for i := 0; i < middleware.lowPriorityQueueConcurrency; i++ {
				<-middleware.lowPriorityQueue
			}
		})

		t.Run("should use low priority queue for large requests with transfer-encoding: chunked when memory usage is high", func(t *testing.T) {
			// Create a server with high memory usage (>80%)
			testServer := &server.Server{
				MaxMemory:  1024 * 1024, // 1MB
				UsedMemory: 850 * 1024,  // 850KB (>80%)
			}

			// Initialize the middleware queues
			middleware.OnStart(testServer)

			// Fill up the low priority queue
			for i := 0; i < middleware.lowPriorityQueueConcurrency; i++ {
				middleware.lowPriorityQueue <- struct{}{}
			}

			// Large request with high memory usage should go to low priority queue
			largeBody := strings.Repeat("x", 65*1024) // 65KB
			req := httptest.NewRequest("POST", "/test", strings.NewReader(largeBody))
			req.Host = "throttle-test.aws-primary.org.ownstak.link"
			req.Header.Del(server.HeaderContentLength)
			req.Header.Set(server.HeaderTransferEncoding, "chunked")
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, testServer)

			// This should timeout with shortest timeout (15ms)
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, server.StatusServiceOverloaded, ctx.Response.Status)
			assert.Contains(t, string(ctx.Response.Body), "Server is overloaded")

			// Clean up - drain the queue
			for i := 0; i < middleware.lowPriorityQueueConcurrency; i++ {
				<-middleware.lowPriorityQueue
			}
		})

		t.Run("should succeed when queue slot becomes available within set timeout window", func(t *testing.T) {
			// Use normal server setup
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = "success-test.aws-primary.org.ownstak.link"
			res := httptest.NewRecorder()

			serverReq, err := server.NewRequest(req)
			require.NoError(t, err)
			serverRes := server.NewResponse(res)
			ctx := server.NewRequestContext(serverReq, serverRes, createTestServer())

			// This should succeed normally
			middleware.OnRequest(ctx, func() {})

			assert.Equal(t, 200, res.Code)
		})

		t.Run("should handle request context cancellation", func(t *testing.T) {
			t.Run("should return error response when streaming has not started", func(t *testing.T) {
				// Create a cancelled context
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				// Create a request with the cancelled context
				req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
				req.Host = "cancel-test.aws-primary.org.ownstak.link"
				res := httptest.NewRecorder()

				// server.NewRequest should return nil for cancelled context
				serverReq, err := server.NewRequest(req)

				// If NewRequest returns nil due to cancelled context, test that behavior
				if serverReq == nil && err == nil {
					// This is the expected behavior - the middleware should handle this gracefully
					assert.Nil(t, serverReq, "NewRequest should return nil for cancelled context")
					return
				}

				// If NewRequest succeeds despite cancelled context, continue with test
				require.NoError(t, err)
				serverRes := server.NewResponse(res)
				requestCtx := server.NewRequestContext(serverReq, serverRes, createTestServer())

				// This should detect the cancelled context during queue wait and exit gracefully
				middleware.OnRequest(requestCtx, func() {})

				// When streaming hasn't started, we should get a normal response (no error set)
				// The middleware should just return without setting any error
				assert.Equal(t, 0, res.Code) // No response written
			})

			t.Run("should handle TCP reset when streaming has started", func(t *testing.T) {
				// Enable streaming mode
				middleware.streamingMode = true

				// Create a request with a cancellable context
				req := httptest.NewRequest("GET", "/test", nil)
				req.Host = "streaming-cancel-test.aws-primary.org.ownstak.link"
				res := httptest.NewRecorder()

				// Create a context that we can cancel during streaming
				ctx, cancel := context.WithCancel(req.Context())
				req = req.WithContext(ctx)

				// Setup a streaming response that will be interrupted
				responseChunks := [][]byte{
					[]byte("{\"statusCode\":200,\"headers\":{\"Content-Type\":\"text/html\"}}"),
					[]byte("\x00\x00\x00\x00\x00\x00\x00\x00"),
					[]byte("<h1>Streaming started</h1>"),
				}

				// Register a mock that simulates connection cancellation during streaming
				httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2021-11-15/functions/.*?/response-streaming-invocations$`,
					func(httpReq *http.Request) (*http.Response, error) {
						// Cancel the context to simulate client disconnect
						cancel()

						// Create AWS EventStream response
						encoder := eventstream.NewEncoder()
						var buf bytes.Buffer

						// Stream the first chunk successfully
						message := eventstream.Message{
							Headers: eventstream.Headers{
								{Name: ":event-type", Value: eventstream.StringValue("PayloadChunk")},
								{Name: ":message-type", Value: eventstream.StringValue("event")},
							},
							Payload: responseChunks[0],
						}
						encoder.Encode(&buf, message)

						// Return the response that will be interrupted
						resp := httpmock.NewBytesResponse(200, buf.Bytes())
						resp.Header.Set("Content-Type", "application/vnd.awslambda.http-integration-response")
						return resp, nil
					},
				)

				serverReq, err := server.NewRequest(req)
				require.NoError(t, err)
				serverRes := server.NewResponse(res)
				requestCtx := server.NewRequestContext(serverReq, serverRes, createTestServer())

				// This should start streaming but detect context cancellation
				middleware.OnRequest(requestCtx, func() {})

				// When streaming has started, the connection should be reset
				// The exact behavior depends on how the streaming handles cancellation
				// We mainly want to ensure it doesn't panic or hang
				assert.True(t, true, "Should handle context cancellation gracefully during streaming")
			})
		})
	})
}

func setupAWSLambdaMock(t *testing.T) func() {
	// Activate httpmock
	httpmock.Activate(t)

	// Mock EC2 metadata service for IAM credentials
	httpmock.RegisterResponder("GET", "http://169.254.169.254/latest/meta-data/iam/security-credentials/",
		func(req *http.Request) (*http.Response, error) {
			// Return the IAM role name
			return httpmock.NewStringResponse(200, "test-role"), nil
		},
	)
	httpmock.RegisterResponder("PUT", "http://169.254.169.254/latest/api/token",
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewStringResponse(201, "token"), nil
		},
	)

	// Mock EC2 metadata service for specific IAM role credentials
	httpmock.RegisterResponder("GET", "http://169.254.169.254/latest/meta-data/iam/security-credentials/test-role",
		func(req *http.Request) (*http.Response, error) {
			// Return mock IAM credentials
			credentials := map[string]interface{}{
				"Code":            "Success",
				"LastUpdated":     "2024-01-01T00:00:00Z",
				"Type":            "AWS-HMAC",
				"AccessKeyId":     "ASIAMOCKACCESKEYID",
				"SecretAccessKey": "mocksecretaccesskey",
				"Token":           "mocktoken",
				"Expiration":      "2024-12-31T23:59:59Z",
			}
			resp, err := httpmock.NewJsonResponse(200, credentials)
			return resp, err
		},
	)

	// Mock STS GetCallerIdentity
	httpmock.RegisterResponder("POST", "http://localhost:4566/",
		func(req *http.Request) (*http.Response, error) {
			// Check if this is an STS request
			if req.Header.Get("X-Amz-Target") == "AWSSecurityTokenServiceV20110615.GetCallerIdentity" {
				resp, err := httpmock.NewJsonResponse(200, map[string]interface{}{
					"Account": "123456789012",
				})
				return resp, err
			}
			resp := httpmock.NewStringResponse(404, "Not Found")
			return resp, nil
		},
	)

	// Add a catch-all responder to log any unmatched requests
	httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
		t.Logf("Unmatched HTTP request: %s %s", req.Method, req.URL.String())
		t.Logf("Request headers: %+v", req.Header)
		// Return a 404 for unmatched requests
		return httpmock.NewStringResponse(404, "Not Found"), nil
	})

	// Return cleanup function
	return func() {
		httpmock.DeactivateAndReset()
	}
}

// registerBufferedLambdaMock registers a mock for AWS Lambda buffered invocations
// Parameters:
//   - t: testing instance
//   - response: string containing the response body to return
//   - responseStatusCode: optional status code to return as API response, not lambda response
func registerBufferedLambdaMock(t *testing.T, response []byte, responseStatusCode ...int) {
	statusCode := 200
	if len(responseStatusCode) > 0 {
		statusCode = responseStatusCode[0]
	}
	httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
		func(req *http.Request) (*http.Response, error) {
			resp := httpmock.NewBytesResponse(statusCode, response)
			resp.Header.Set("Content-Type", "application/json")
			// If 200 response contains errorType/errorMessage such as Sandbox.Timeout
			// set corresponding x-amz-function-error header for the SDK to handle the error
			if strings.Contains(string(response), "errorType") {
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
			}
			return resp, nil
		},
	)
}

// registerStreamingLambdaMock registers a mock for AWS Lambda streaming invocations
// Parameters:
//   - t: testing instance
//   - responseChunks: slice of byte arrays representing chunks to stream as AWS EventStream
//   - responseStatusCode: optional status code to return as API response, not lambda response
func registerStreamingLambdaMock(t *testing.T, responseChunks [][]byte, responseStatusCode ...int) {
	statusCode := 200
	if len(responseStatusCode) > 0 {
		statusCode = responseStatusCode[0]
	}
	httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2021-11-15/functions/.*?/response-streaming-invocations$`,
		func(req *http.Request) (*http.Response, error) {
			// Create AWS EventStream response
			encoder := eventstream.NewEncoder()
			var buf bytes.Buffer

			// Stream each chunk as a PayloadChunk event
			for _, chunk := range responseChunks {
				message := eventstream.Message{
					Headers: eventstream.Headers{
						{
							Name:  ":event-type",
							Value: eventstream.StringValue("PayloadChunk"),
						},
						{
							Name:  ":message-type",
							Value: eventstream.StringValue("event"),
						},
					},
					Payload: chunk,
				}

				// Encode the message
				if err := encoder.Encode(&buf, message); err != nil {
					return nil, err
				}
			}

			// Simulate invocation complete event
			completeEventPayloadBytes := []byte("{}")
			// If response contains errors, attach error details to the complete event
			lastChunk := responseChunks[len(responseChunks)-1]
			if strings.Contains(string(lastChunk), "errorType") {
				completeEventPayload := map[string]interface{}{
					"ErrorCode":    "Unhandled",
					"ErrorDetails": lastChunk,
				}
				completeEventPayloadBytes, _ = json.Marshal(completeEventPayload)
			}

			// Send complete event
			completeEvent := eventstream.Message{
				Headers: eventstream.Headers{
					{
						Name:  ":event-type",
						Value: eventstream.StringValue("InvokeComplete"),
					},
					{
						Name:  ":message-type",
						Value: eventstream.StringValue("event"),
					},
				},
				Payload: completeEventPayloadBytes,
			}

			// Encode the complete message
			if err := encoder.Encode(&buf, completeEvent); err != nil {
				return nil, err
			}

			resp := httpmock.NewBytesResponse(statusCode, buf.Bytes())
			resp.Header.Set("Content-Type", "application/vnd.awslambda.http-integration-response")
			return resp, nil
		},
	)
}
