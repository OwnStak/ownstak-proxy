package middlewares

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/server"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.NotNil(t, middleware, "should create middleware when provider is AWS")

	t.Run("should invoke lambda in sync mode", func(t *testing.T) {
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "myapp-prod.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		// Create request context
		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		// Run middleware
		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, "text/html", ctx.Response.Headers.Get("Content-Type"))
		assert.Contains(t, string(ctx.Response.Body), "Hello from Lambda!")
	})

	t.Run("should handle lambda response with base64 encoded body", func(t *testing.T) {
		// Setup special mock for this test
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Base64 encoded "<h1>Base64 Content</h1>"
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/html",
					},
					"body":            "PGgxPkJhc2U2NCBDb250ZW50PC9oMT4=",
					"isBase64Encoded": true,
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "base64-test.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, "text/html", ctx.Response.Headers.Get("Content-Type"))
		assert.Contains(t, string(ctx.Response.Body), "Base64 Content")
	})

	t.Run("should handle lambda response with multi-value headers", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
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
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "multi-headers.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, "text/html", ctx.Response.Headers.Get("Content-Type"))
		assert.Equal(t, "single-value", ctx.Response.Headers.Get("X-Custom-Header"))
		cookies := ctx.Response.Headers["Set-Cookie"]
		assert.Len(t, cookies, 2)
		assert.Contains(t, cookies, "session=abc123")
		assert.Contains(t, cookies, "user=john")
	})

	t.Run("should handle lambda invocation errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// AWS Lambda error response format with proper headers
				errorPayload := map[string]interface{}{
					"errorType":    "Runtime.UserCodeSyntaxError",
					"errorMessage": "Syntax error in user code",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "error-test.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaProjectError, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Runtime.UserCodeSyntaxError")
	})

	t.Run("should handle lambda timeout errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// AWS Lambda timeout error response format with proper headers
				errorPayload := map[string]interface{}{
					"errorType":    "Sandbox.Timeout",
					"errorMessage": "Task timed out after 30.00 seconds",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "timeout-test.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaTimeout, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Sandbox.Timeout")
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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)
		assert.Contains(t, ctx.Response.Headers.Get("Location"), "revive")
	})

	t.Run("should properly set user status codes", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				lambdaResponse := map[string]interface{}{
					"statusCode": 404,
					"headers": map[string]string{
						"Content-Type": "application/json",
					},
					"body": `{"error":"Not Found"}`,
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "status404-test.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 404, ctx.Response.Status)
		assert.Equal(t, "application/json", ctx.Response.Headers.Get("Content-Type"))
		assert.Contains(t, string(ctx.Response.Body), "Not Found")
	})

	t.Run("should return empty lambda response body", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				lambdaResponse := map[string]interface{}{
					"statusCode": 204,
					"headers": map[string]string{
						"Content-Type": "text/plain",
					},
					"body": "",
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "empty-body.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusBadRequest, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Invalid hostname format")
	})

	t.Run("should handle invalid lambda response errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Return invalid JSON that cannot be parsed as a lambda response
				resp := httpmock.NewStringResponse(200, `{"StatusCode":200,"Payload":"this is not valid json}`)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "invalid-response.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Failed to process Lambda response")
	})

	t.Run("should handle invalid base64 encoding errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/html",
					},
					"body":            "invalid-base64!@#$%",
					"isBase64Encoded": true,
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "bad-base64.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Failed to process Lambda response")
	})

	t.Run("should handle lambda throttling errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				errorPayload := map[string]interface{}{
					"errorType":    "TooManyRequests",
					"errorMessage": "Rate exceeded",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "throttle-test.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaThrottled, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "TooManyRequests")
	})

	t.Run("should handle lambda request too large errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				errorPayload := map[string]interface{}{
					"errorType":    "RequestTooLarge",
					"errorMessage": "Request payload size exceeded",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "large-req.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaRequestTooLarge, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "RequestTooLarge")
	})

	t.Run("should handle lambda response too large errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				errorPayload := map[string]interface{}{
					"errorType":    "ResponseSizeTooLarge",
					"errorMessage": "Response payload size exceeded",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "large-resp.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaResponseTooLarge, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "ResponseSizeTooLarge")
	})

	t.Run("should handle lambda crashed errors", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				errorPayload := map[string]interface{}{
					"errorType":    "Runtime.ExitError",
					"errorMessage": "Process exited with non-zero status",
				}
				errorPayloadBytes, _ := json.Marshal(errorPayload)

				resp := httpmock.NewBytesResponse(200, errorPayloadBytes)
				resp.Header.Set("Content-Type", "application/json")
				resp.Header.Set("X-Amz-Function-Error", "Unhandled")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "crashed.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusLambdaCrashed, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Runtime.ExitError")
	})

	t.Run("should call lambda with deployment id alias from host header", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Verify the function name includes deployment alias
				url := req.URL.String()
				assert.Contains(t, url, "deployment-123")

				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/html",
					},
					"body": "<h1>Deployment 123</h1>",
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "myapp-prod-123.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Deployment 123")
	})

	t.Run("should call lambda with current alias when there is no deployment id in host header", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Verify the function name includes deployment alias
				url := req.URL.String()
				assert.Contains(t, url, "current")

				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/html",
					},
					"body": "<h1>Current</h1>",
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "myapp-prod.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Current")
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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Failed to invoke Lambda function")
	})

	t.Run("should correctly return binary base64 encoded response from lambda", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
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
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/image.png", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "binary-app.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, "image/png", ctx.Response.Headers.Get("Content-Type"))
		// Check if binary data was properly decoded
		assert.Equal(t, byte(137), ctx.Response.Body[0]) // PNG signature first byte
	})

	t.Run("should return error when host header is missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "" // Empty host
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusBadRequest, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Invalid hostname format")
	})

	t.Run("should correctly handle special characters in response", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/html; charset=utf-8",
						"X-Custom":     "special-chars-αβγ-🚀",
					},
					"body": `<html><body><h1>Hello 世界! 🌍</h1><p>Special chars: αβγδε</p></body></html>`,
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "unicode-app.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Hello 世界!")
		assert.Contains(t, string(ctx.Response.Body), "🌍")
		assert.Equal(t, "special-chars-αβγ-🚀", ctx.Response.Headers.Get("X-Custom"))
	})

	t.Run("should return response with large headers", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Create headers with combined size near AWS limits
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
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "large-headers.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Equal(t, 1000, len(ctx.Response.Headers.Get("X-Large-Header-1")))
	})

	t.Run("should return response with max number of headers", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// Create response with many headers (close to AWS limit of ~100)
				headers := map[string]string{"Content-Type": "text/html"}
				for i := 0; i < 50; i++ {
					headers[fmt.Sprintf("X-Custom-Header-%d", i)] = fmt.Sprintf("value-%d", i)
				}
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers":    headers,
					"body":       "<h1>Many Headers</h1>",
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "many-headers.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, server.StatusInternalError, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "Failed to invoke Lambda function")
	})

	t.Run("should return error when lambda returns malformed json in payload", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				// AWS Lambda response with invalid JSON in body field
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "application/json",
					},
					"body": `{"key": "value" "missing_comma": true}`, // Invalid JSON
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "bad-json.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		// Should still pass through the response even if JSON is malformed
		assert.Equal(t, 200, ctx.Response.Status)
		assert.Contains(t, string(ctx.Response.Body), "missing_comma")
	})

	t.Run("should return null response body", func(t *testing.T) {
		httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
			func(req *http.Request) (*http.Response, error) {
				lambdaResponse := map[string]interface{}{
					"statusCode": 200,
					"headers": map[string]string{
						"Content-Type": "text/plain",
					},
					"body": nil, // Null body
				}
				responseBytes, _ := json.Marshal(lambdaResponse)
				resp := httpmock.NewBytesResponse(200, responseBytes)
				resp.Header.Set("Content-Type", "application/json")
				return resp, nil
			},
		)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "null-body.aws-primary.org.ownstak.link")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		assert.Equal(t, 200, ctx.Response.Status)
		assert.Empty(t, ctx.Response.Body)
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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		// Verify it's a temporary redirect to revive
		assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

		// Verify the Location header contains the revive URL
		location := ctx.Response.Headers.Get("Location")
		assert.NotEmpty(t, location, "Location header should be set for revive redirect")
		assert.Contains(t, location, "revive", "Location should contain 'revive'")
		assert.Contains(t, location, "host=ecommerce-default-123.aws-primary.org.ownstak.link", "Location should contain the host from host/x-own-host header")
		assert.Contains(t, location, "originalHost=ecommerce.com", "Location should contain the original host from x-forwarded-host/host header")
		assert.Contains(t, location, "originalUrl=http://ecommerce.com/api/users", "Location should contain the url with original host")

		// Verify that the redirect preserves the original path
		assert.Contains(t, location, "/api/users", "Location should preserve the original request path")
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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		// Verify it's a temporary redirect to revive
		assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

		// Verify the Location header contains the revive URL
		location := ctx.Response.Headers.Get("Location")
		assert.NotEmpty(t, location, "Location header should be set for revive redirect")
		assert.Contains(t, location, "revive", "Location should contain 'revive'")
		assert.Contains(t, location, "host=ecommerce-default-123.aws-primary.org.ownstak.link", "Location should contain the host from host/x-own-host header")
		assert.Contains(t, location, "originalHost=original-ecommerce.com", "Location should contain the original host from x-forwarded-host/host header")
		assert.Contains(t, location, "originalUrl=http://original-ecommerce.com/api/users", "Location should contain the url with original host")

		// Verify that the redirect preserves the original path
		assert.Contains(t, location, "/api/users", "Location should preserve the original request path")
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
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

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

	t.Run("should redirect to revive with POST method and preserve request details", func(t *testing.T) {
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

		req := httptest.NewRequest("POST", "/api/create", strings.NewReader(`{"name":"test","data":"value"}`))
		req.Host = "example.com"
		req.Header.Set(server.HeaderXOwnHost, "deleted-app.aws-primary.org.ownstak.link")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer token123")
		res := httptest.NewRecorder()

		serverReq, err := server.NewRequest(req)
		require.NoError(t, err)
		serverRes := server.NewResponse(res)
		ctx := server.NewRequestContext(serverReq, serverRes, nil)

		middleware.OnRequest(ctx, func() {})

		// Verify it's a temporary redirect to revive
		assert.Equal(t, server.StatusTemporaryRedirect, ctx.Response.Status)

		// Verify the Location header contains the revive URL
		location := ctx.Response.Headers.Get("Location")
		assert.NotEmpty(t, location, "Location header should be set for revive redirect")
		assert.Contains(t, location, "revive", "Location should contain 'revive'")
		assert.Contains(t, location, "deleted-app", "Location should contain the app name")
		assert.Contains(t, location, "/api/create", "Location should preserve the original path")

		// The revive redirect should preserve the original request method in the redirect URL
		// so the revive system can handle the POST appropriately
		assert.Contains(t, location, "aws-primary.org.ownstak.link", "Location should contain the original host")
	})
}

func setupAWSLambdaMock(t *testing.T) func() {
	// Activate httpmock
	httpmock.Activate(t)

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

	// Mock Lambda Invoke - use exact URL pattern
	httpmock.RegisterResponder("POST", `=~^http://localhost:4566/2015-03-31/functions/.*?/invocations$`,
		func(req *http.Request) (*http.Response, error) {
			name := req.URL.String()
			t.Logf("Lambda invoke mock called for URL: %s", name)
			lambdaResponse := map[string]interface{}{
				"statusCode": 200,
				"headers": map[string]string{
					"Content-Type": "text/html",
				},
				"body": "<html><body><h1>Hello from Lambda!</h1></body></html>",
			}
			responseBytes, _ := json.Marshal(lambdaResponse)
			t.Logf("Lambda API response: %s", string(responseBytes))

			resp := httpmock.NewBytesResponse(200, responseBytes)
			resp.Header.Set("Content-Type", "application/json")
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
