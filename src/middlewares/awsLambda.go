package middlewares

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// API Gateway v2 JSON payload structure
type ApiGatewayV2Event struct {
	Version               string              `json:"version"`
	RouteKey              string              `json:"routeKey"`
	RawPath               string              `json:"rawPath"`
	RawQueryString        string              `json:"rawQueryString"`
	Cookies               []string            `json:"cookies,omitempty"`
	Headers               map[string]string   `json:"headers"`
	QueryStringParameters map[string]string   `json:"queryStringParameters,omitempty"`
	RequestContext        EventRequestContext `json:"requestContext"`
	Body                  string              `json:"body"`
	IsBase64Encoded       bool                `json:"isBase64Encoded"`
	PathParameters        map[string]string   `json:"pathParameters,omitempty"`
	StageVariables        map[string]string   `json:"stageVariables,omitempty"`
}

type EventRequestContext struct {
	AccountId      string          `json:"accountId"`
	ApiId          string          `json:"apiId"`
	DomainName     string          `json:"domainName"`
	DomainPrefix   string          `json:"domainPrefix"`
	Http           HttpDetails     `json:"http"`
	RequestId      string          `json:"requestId"`
	RouteKey       string          `json:"routeKey"`
	Stage          string          `json:"stage"`
	Time           string          `json:"time"`
	TimeEpoch      int64           `json:"timeEpoch"`
	Authentication *Authentication `json:"authentication,omitempty"`
}

type HttpDetails struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Protocol  string `json:"protocol"`
	SourceIp  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}

type Authentication struct {
	ClientCert *ClientCert `json:"clientCert,omitempty"`
}

type ClientCert struct {
	ClientCertPem string `json:"clientCertPem"`
	SubjectDN     string `json:"subjectDN"`
	IssuerDN      string `json:"issuerDN"`
	SerialNumber  string `json:"serialNumber"`
	Validity      struct {
		NotBefore string `json:"notBefore"`
		NotAfter  string `json:"notAfter"`
	} `json:"validity"`
}

// AWSLambdaMiddleware handles AWS Lambda invocations
type AWSLambdaMiddleware struct {
	awsConfig    *aws.Config
	lambdaClient *lambda.Client
	orgsClient   *organizations.Client
	stsClient    *sts.Client
	accountId    string
}

func NewAWSLambdaMiddleware() *AWSLambdaMiddleware {
	// Check AWS credentials before doing anything
	provider := strings.ToLower(os.Getenv(constants.EnvProvider))
	if provider != constants.ProviderAWS {
		logger.Warn(fmt.Sprintf("Disabling AWS Lambda middleware - The provider doesn't match '%s'", constants.ProviderAWS))
		return nil
	}

	// Use the AWS_ACCOUNT_ID environment variable if set
	accountId := os.Getenv(constants.EnvAWSAccountId)

	// Set default region
	region := "us-east-1"
	if envRegion := os.Getenv(constants.EnvAWSRegion); envRegion != "" {
		region = envRegion
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		logger.Error("Failed to load AWS SDK config: %v", err)
		return nil
	}

	lambdaClient := lambda.NewFromConfig(awsConfig)
	orgsClient := organizations.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	return &AWSLambdaMiddleware{
		awsConfig:    &awsConfig,
		lambdaClient: lambdaClient,
		orgsClient:   orgsClient,
		stsClient:    stsClient,
		accountId:    accountId,
	}
}

// OnRequest processes the request to invoke Lambda if appropriate
func (m *AWSLambdaMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
	// Check the host header
	if strings.TrimSpace(ctx.Request.Host) == "" {
		errorMessage := fmt.Sprintf("The host header or %s header is required.\r\n", server.HeaderXOwnHost)
		ctx.Error(errorMessage, server.StatusBadRequest)
		return
	}

	// Parse hostname parts
	// e.g: site-125.aws-2-account.ownstak.link
	// site-125 is the AWS Lambda function readable name
	// aws-2-account is the AWS account name
	// ownstak.link is the domain name
	hostParts := strings.Split(ctx.Request.Host, ".")
	if len(hostParts) < 3 {
		errorMessage := fmt.Sprintf("Invalid hostname format '%s': ", ctx.Request.Host)
		errorMessage += "The expected format is '{project-slug}-{environment-slug}-{optional-deployment-id}.{cloudbackend-slug}.{organization-slug}.{domain-name}.'\r\n"
		errorMessage += "e.g: nextjs-app-prod-123.aws-primary.my-org.ownstak.link\r\n"
		errorMessage += "e.g: nextjs-app-prod.aws-primary.my-org.ownstak.link\r\n"
		ctx.Error(errorMessage, server.StatusBadRequest)
		return
	}

	// Parse lambda name from the host header
	// IMPORTANT:
	// The below code and logic needs to be in sync with the OwnStak Console.
	// Change it only if you're sure what you're doing and ready to face the consequences.
	// We need to do this parsing/transformation because the host/lambda name limit is 63/64 characters
	// and we need to fit deployment id into it and still keep it readable and nice looking.
	// See: https://github.com/OwnStak/ownstak-console/blob/main/api/app/services/deployments/aws_deployer.rb#L312
	lambdaHost := hostParts[0] // e.g: myproject-prod, myproject-prod-125 etc...

	// We need to extract lambda name and optional deployment id
	// from the first host segment using regex ^(.*?)(?:-(\d+))?$
	lambdaNameRegex := regexp.MustCompile(`^(.*?)(?:-(\d+))?$`)
	lambdaNameParts := lambdaNameRegex.FindStringSubmatch(lambdaHost)
	lambdaName := lambdaNameParts[1]

	// Construct the lambda name by adding "ownstak-" prefix.
	lambdaName = "ownstak-" + lambdaName

	// Get deployment id if present
	deploymentId := ""
	if len(lambdaNameParts) > 2 {
		deploymentId = lambdaNameParts[2]
	}

	// Construct the Lambda alias from the deployment id if present,
	// otherwise use "current" as the alias that points to the latest deployment.
	// NOTE: We need to do it because the Lambda alias cannot start with a number.
	lambdaAlias := "current"
	if deploymentId != "" {
		lambdaAlias = "deployment-" + deploymentId
	}

	// Get the AWS account ID from caller identity
	// if not set through AWS_ACCOUNT_ID environment variable
	if m.accountId == "" {
		// Get the AWS account ID from caller identity
		accountId, err := m.getAccountIdFromCaller(context.Background())
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to get AWS account ID: %v", err)
			ctx.Error(errorMessage, server.StatusAccountNotFound)
			return
		}

		// Store the account ID for all other invocations
		m.accountId = accountId
	}

	// Construct the Lambda ARN
	lambdaArn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s:%s", m.awsConfig.Region, m.accountId, lambdaName, lambdaAlias)

	// Create API Gateway v2 JSON event
	event, err := m.createApiGatewayEvent(ctx, m.accountId)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to create API Gateway event: %v", err)
		ctx.Error(errorMessage, server.StatusInternalServerError)
		return
	}

	// Determine if we should use sync or async mode (buffered or streaming invocation)
	asyncMode := false
	if ctx.Request.Headers.Get(server.HeaderXOwnLambdaMode) == "async" {
		asyncMode = true
	}
	// Invoke Lambda function
	invocationStartTime := time.Now()
	response, err := m.invokeLambda(context.Background(), lambdaArn, event, asyncMode)
	invocationDuration := time.Since(invocationStartTime)

	// Set Lambda invocation duration header
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "lambda-duration="+invocationDuration.String())

	// Handle invocation errors
	if err != nil {
		// Default error
		errorStatus := server.StatusInternalError
		errorMessage := fmt.Sprintf("Failed to invoke Lambda function: %v", err)

		// If the Lambda function was not found, it was probably retired.
		// In this case, we will redirect the user to the OwnStak Console with host passed as a query parameter.
		if strings.Contains(errorMessage, "ResourceNotFoundException") {
			redirectURL := fmt.Sprintf("%s/revive?host=%s", constants.ConsoleURL, ctx.Request.Host)
			ctx.Response.Headers.Set(server.HeaderLocation, redirectURL)
			ctx.Response.Status = server.StatusTemporaryRedirect
			return
		}

		ctx.Error(errorMessage, errorStatus)
		return
	}

	// Handle errors from the Lambda function payload
	if response.FunctionError != nil {
		var parsedPayload map[string]interface{}
		err = json.Unmarshal(response.Payload, &parsedPayload)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to parse Lambda function error payload: %v", err)
			ctx.Error(errorMessage, server.StatusInternalError)
			return
		}

		payloadErrorType := parsedPayload["errorType"].(string)
		payloadErrorMessage := parsedPayload["errorMessage"].(string)

		// Default error
		errorStatus := server.StatusLambdaProjectError
		errorMessage := fmt.Sprintf("Lambda function returned '%s' error: %s", payloadErrorType, payloadErrorMessage)

		// Handle known error types
		if strings.Contains(payloadErrorType, "TooManyRequests") {
			errorStatus = server.StatusLambdaThrottled
		} else if strings.Contains(payloadErrorType, "RequestTooLarge") {
			errorStatus = server.StatusLambdaRequestTooLarge
		} else if strings.Contains(payloadErrorType, "ResponseSizeTooLarge") {
			errorStatus = server.StatusLambdaResponseTooLarge
		} else if strings.Contains(payloadErrorType, "InvalidRequest") {
			errorStatus = server.StatusLambdaRequestInvalid
		} else if strings.Contains(payloadErrorType, "InvalidResponse") {
			errorStatus = server.StatusLambdaResponseInvalid
		} else if strings.Contains(payloadErrorType, "Sandbox.Timeout") {
			errorStatus = server.StatusLambdaTimeout
		} else if strings.Contains(payloadErrorType, "Runtime.ExitError") {
			errorStatus = server.StatusLambdaCrashed
		}

		ctx.Error(errorMessage, errorStatus)
		return
	}

	// Process Lambda response and set to context
	err = m.processLambdaResponse(ctx, response)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to process Lambda response: %v", err)
		ctx.Error(errorMessage, server.StatusInternalError)
		return
	}

	// Store details for the response phase
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "lambda-name="+lambdaName)
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "lambda-alias="+lambdaAlias)
	ctx.Response.AppendHeader(server.HeaderXOwnProxyDebug, "lambda-region="+m.awsConfig.Region)

	// No need to call next() as we've fully handled the request
}

func (m *AWSLambdaMiddleware) OnResponse(ctx *server.RequestContext, next func()) {
	next()
}

// getAccountIdFromCaller retrieves the AWS account ID from the caller identity
func (m *AWSLambdaMiddleware) getAccountIdFromCaller(ctx context.Context) (string, error) {
	// Get the caller identity using STS
	input := &sts.GetCallerIdentityInput{}
	result, err := m.stsClient.GetCallerIdentity(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS caller identity: %v", err)
	}

	// Return the account ID
	accountID := *result.Account
	return accountID, nil
}

// createApiGatewayEvent creates an API Gateway v2 JSON event from the request
func (m *AWSLambdaMiddleware) createApiGatewayEvent(ctx *server.RequestContext, accountID string) ([]byte, error) {
	req := ctx.Request

	// Extract headers
	headers := make(map[string]string)
	for key, values := range req.Headers {
		// API Gateway normalizes headers to lowercase
		headers[strings.ToLower(key)] = values[0]
	}

	// Extract query parameters and handle duplicates
	queryParams := make(map[string]string)
	if strings.Contains(req.URL, "?") {
		urlParts := strings.Split(req.URL, "?")
		if len(urlParts) > 1 {
			rawQuery := urlParts[1]
			for _, param := range strings.Split(rawQuery, "&") {
				kv := strings.SplitN(param, "=", 2)
				key := kv[0]
				value := ""
				if len(kv) > 1 {
					value = kv[1]
				}
				// Remove brackets from key if present (e.g., query[] -> query)
				key = strings.TrimSuffix(key, "[]")
				if existing, found := queryParams[key]; found {
					queryParams[key] = existing + "," + value
				} else {
					queryParams[key] = value
				}
			}
		}
	}

	// Prepare request body
	var body string
	var isBase64Encoded bool

	// Determine if we should base64 encode the body (binary data)
	contentType := req.Headers.Get("Content-Type")
	if strings.Contains(contentType, "text/") ||
		strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "application/javascript") ||
		strings.Contains(contentType, "application/x-www-form-urlencoded") {
		body = string(req.Body)
		isBase64Encoded = false
	} else {
		body = base64.StdEncoding.EncodeToString(req.Body)
		isBase64Encoded = true
	}

	// Create the API Gateway event
	now := time.Now()
	event := ApiGatewayV2Event{
		Version:               "2.0",
		RouteKey:              "$default",
		RawPath:               req.Path,
		RawQueryString:        "", // Defaults to an empty string
		Cookies:               []string{},
		Headers:               headers,
		QueryStringParameters: queryParams,
		RequestContext: EventRequestContext{
			AccountId:    accountID,
			ApiId:        constants.AppName,
			DomainName:   req.Host,
			DomainPrefix: "", // Default to an empty string
			Http: HttpDetails{
				Method:    req.Method,
				Path:      req.Path,
				Protocol:  req.Scheme,
				SourceIp:  "", // Default to an empty string
				UserAgent: "", // Default to an empty string
			},
			RequestId: req.Headers.Get(server.HeaderRequestID),
			RouteKey:  "$default",
			Stage:     "$default",
			Time:      now.Format(time.RFC3339),
			TimeEpoch: now.UnixNano() / int64(time.Millisecond),
		},
		Body:            body,
		IsBase64Encoded: isBase64Encoded,
		PathParameters:  map[string]string{},
		StageVariables:  map[string]string{},
	}

	// Check if the URL contains a query string and extract it
	if strings.Contains(req.URL, "?") {
		parts := strings.Split(req.URL, "?")
		if len(parts) > 1 {
			event.RawQueryString = parts[1]
		}
	}

	// Safely extract domain prefix from the host
	if hostParts := strings.Split(req.Host, "."); len(hostParts) > 0 {
		event.RequestContext.DomainPrefix = hostParts[0]
	}

	// Safely extract source IP from the X-Forwarded-For header
	if xForwardedFor := headers[server.HeaderXForwardedFor]; xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			event.RequestContext.Http.SourceIp = strings.TrimSpace(ips[0])
		}
	}

	// Safely extract user agent from the User-Agent header
	if userAgent, exists := headers[server.HeaderUserAgent]; exists {
		event.RequestContext.Http.UserAgent = userAgent
	}

	return json.Marshal(event)
}

// invokeLambdaAsync invokes the specified Lambda function with the given payload using streaming mode
func (m *AWSLambdaMiddleware) invokeLambdaAsync(ctx context.Context, lambdaArn string, payload []byte) (*lambda.InvokeOutput, error) {
	// Use streaming mode for Lambda invocation
	input := &lambda.InvokeWithResponseStreamInput{
		FunctionName: aws.String(lambdaArn),
		Payload:      payload,
	}

	logger.Debug("Invoking Lambda function in streaming mode: %s", lambdaArn)
	streamOutput, err := m.lambdaClient.InvokeWithResponseStream(ctx, input)
	if err != nil {
		return nil, err
	}

	// Get the event stream and ensure it's closed when done
	eventStream := streamOutput.GetStream()
	defer eventStream.Close()

	// Define a maximum response size limit (20MB - AWS Lambda's maximum)
	const maxResponseSize = 20 * 1024 * 1024
	var responseSize int
	var responsePayload []byte
	var errorInfo *types.InvokeWithResponseStreamResponseEventMemberInvokeComplete

	// Read the streaming response
	for event := range eventStream.Events() {
		// Type switch on the event to handle different event types
		switch e := event.(type) {
		case *types.InvokeWithResponseStreamResponseEventMemberPayloadChunk:
			// Check if adding this chunk would exceed our limit
			chunkSize := len(e.Value.Payload)
			responseSize += chunkSize

			if responseSize > maxResponseSize {
				// Stop processing and return an error
				logger.Error("Lambda response exceeds size limit of %d bytes", maxResponseSize)
				return nil, fmt.Errorf("lambda response too large: exceeds %d bytes", maxResponseSize)
			}

			responsePayload = append(responsePayload, e.Value.Payload...)
		case *types.InvokeWithResponseStreamResponseEventMemberInvokeComplete:
			// Store the completion info - contains error details if any
			errorInfo = e
			// Check for function error
			if e.Value.ErrorCode != nil && *e.Value.ErrorCode != "" {
				logger.Error("Lambda function error: %s", *e.Value.ErrorCode)
				// Create an error that matches the format expected by the caller
				var functionError string
				if e.Value.ErrorCode != nil {
					functionError = *e.Value.ErrorCode
				}

				errorMessage := "Lambda function execution failed"
				if e.Value.ErrorDetails != nil {
					errorMessage = *e.Value.ErrorDetails
				}

				return &lambda.InvokeOutput{
					FunctionError: &functionError,
					Payload:       []byte(fmt.Sprintf(`{"errorType":"%s","errorMessage":"%s"}`, *e.Value.ErrorCode, errorMessage)),
					StatusCode:    streamOutput.StatusCode,
				}, nil
			}
		}
	}

	// Check for any error from the stream
	if err := eventStream.Err(); err != nil {
		logger.Error("Stream error: %v", err)
		return nil, fmt.Errorf("stream error: %v", err)
	}

	// Handle the case where execution exceeds Lambda execution time
	if responsePayload == nil && errorInfo == nil {
		logger.Error("Lambda execution timed out or did not return a response")
		return nil, fmt.Errorf("lambda execution timed out or did not return a response")
	}

	// Construct the InvokeOutput
	invokeOutput := &lambda.InvokeOutput{
		Payload:    responsePayload,
		StatusCode: streamOutput.StatusCode,
	}

	return invokeOutput, nil
}

// invokeLambdaSync invokes the specified Lambda function with the given payload using standard synchronous mode
func (m *AWSLambdaMiddleware) invokeLambdaSync(ctx context.Context, lambdaArn string, payload []byte) (*lambda.InvokeOutput, error) {
	// Use standard synchronous Lambda invocation
	input := &lambda.InvokeInput{
		FunctionName: aws.String(lambdaArn),
		Payload:      payload,
	}

	logger.Debug("Invoking Lambda function in synchronous mode: %s", lambdaArn)

	// Invoke the function synchronously
	return m.lambdaClient.Invoke(ctx, input)
}

// invokeLambda determines which invocation method to use based on the payload size and other factors
func (m *AWSLambdaMiddleware) invokeLambda(ctx context.Context, lambdaArn string, payload []byte, asyncMode bool) (*lambda.InvokeOutput, error) {
	if asyncMode {
		return m.invokeLambdaAsync(ctx, lambdaArn, payload)
	} else {
		return m.invokeLambdaSync(ctx, lambdaArn, payload)
	}
}

// processLambdaResponse processes the Lambda response and updates the RequestContext
func (m *AWSLambdaMiddleware) processLambdaResponse(ctx *server.RequestContext, lambdaResponse *lambda.InvokeOutput) error {
	if lambdaResponse.FunctionError != nil {
		return fmt.Errorf("lambda function returned error: %s", *lambdaResponse.FunctionError)
	}

	// Parse the API Gateway response format
	var response struct {
		StatusCode        int                 `json:"statusCode"`
		Headers           map[string]string   `json:"headers,omitempty"`
		MultiValueHeaders map[string][]string `json:"multiValueHeaders,omitempty"`
		Body              string              `json:"body,omitempty"`
		IsBase64Encoded   bool                `json:"isBase64Encoded"`
	}

	err := json.Unmarshal(lambdaResponse.Payload, &response)
	if err != nil {
		return fmt.Errorf("failed to parse lambda response: %v", err)
	}

	// Set status code
	// e.g: 200
	if response.StatusCode != 0 {
		ctx.Response.Status = response.StatusCode
	} else {
		ctx.Response.Status = http.StatusOK // Default to 200 OK
	}

	// Set single-value headers if present
	// e.g: "Content-Type" -> "text/html"
	if response.Headers != nil {
		for key, value := range response.Headers {
			ctx.Response.Headers.Set(key, value)
		}
	}

	// Set multi-value headers if present
	// e.g: "Set-Cookie" -> ["cookie1=value1", "cookie2=value2"]
	if response.MultiValueHeaders != nil {
		for key, values := range response.MultiValueHeaders {
			for _, value := range values {
				ctx.Response.Headers.Add(key, value)
			}
		}
	}

	// Set body if present
	// e.g raw string: "<html><body><h1>Hello, World!</h1></body></html>"
	// e.g base64 encoded: "PGh0bWw+PGJvZHk+PGgxPkV4YW1wbGUgYm9keTwvaDE+PC9ib2R5PjwvaHRtbD4="
	if response.Body != "" {
		if response.IsBase64Encoded {
			bodyBytes, err := base64.StdEncoding.DecodeString(response.Body)
			if err != nil {
				return fmt.Errorf("failed to decode base64 response body: %v", err)
			}
			ctx.Response.Body = bodyBytes
		} else {
			ctx.Response.Body = []byte(response.Body)
		}
	} else {
		ctx.Response.Body = []byte{}
	}

	return nil
}
