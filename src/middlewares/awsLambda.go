package middlewares

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"ownstack-proxy/src/constants"
	"ownstack-proxy/src/logger"
	"ownstack-proxy/src/server"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
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
	accountCache map[string]string // Cache of account name to ID
}

func NewAWSLambdaMiddleware() *AWSLambdaMiddleware {
	// Check AWS credentials before doing anything
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		logger.Warn("Skipping AWS Lambda middleware - AWS credentials not found.")
		return nil
	}

	// Set default region
	region := "us-east-1"
	if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
		region = envRegion
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		logger.Error("Failed to load AWS SDK config: %v", err)
		return nil
	}

	lambdaClient := lambda.NewFromConfig(awsConfig)
	orgsClient := organizations.NewFromConfig(awsConfig)

	return &AWSLambdaMiddleware{
		awsConfig:    &awsConfig,
		lambdaClient: lambdaClient,
		orgsClient:   orgsClient,
		accountCache: make(map[string]string),
	}
}

// OnRequest processes the request to invoke Lambda if appropriate
func (m *AWSLambdaMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	// Parse hostname parts
	// e.g: site-125.aws-2-account.ownstack.link
	// site-125 is the AWS Lambda function readable name
	// aws-2-account is the AWS account name
	// ownstack.link is the domain name
	hostParts := strings.Split(ctx.Request.Host, ".")
	if len(hostParts) < 2 {
		errorMessage := fmt.Sprintf("Invalid hostname format '%s'.\r\n", ctx.Request.Host)
		errorMessage += "The expected format is '{lambda-name}.{aws-account-name}.{domain-name}.'\r\n"
		errorMessage += "e.g: site-125.aws-2-account.ownstack.link\r\n"
		ctx.Error(errorMessage, server.StatusBadRequest)
		return
	}

	// The maximum length of the AWS Lambda name is 64 characters.
	// That's why we hash the readable name by sha256 to make sure it's always 64 characters at max and unique.
	lambdaRedableName := hostParts[0] // e.g: site-125
	lambdaName := m.getLambdaName(lambdaRedableName)
	accountName := hostParts[1]

	// Get AWS account ID
	accountID, err := m.getAccountID(context.Background(), accountName)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to get AWS account ID: %v", err)
		ctx.Error(errorMessage, server.StatusAccountNotFound)
		return
	}

	// Construct the Lambda ARN
	lambdaArn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", m.awsConfig.Region, accountID, lambdaName)

	// Create API Gateway v2 JSON event
	event, err := m.createApiGatewayEvent(ctx, accountID)
	if err != nil {
		errorMessage := fmt.Sprintf("Failed to create API Gateway event: %v", err)
		ctx.Error(errorMessage, server.StatusInternalServerError)
		return
	}

	// Invoke Lambda function
	response, err := m.invokeLambda(context.Background(), lambdaArn, event)
	// Handle invocation errors
	if err != nil {
		// Default error
		errorStatus := server.StatusInternalError
		errorMessage := fmt.Sprintf("Failed to invoke Lambda function: %v", err)

		// If the Lambda function was not found, it was probably retired.
		// In this case, we will redirect the user to the OwnStack Console with host passed as a query parameter.
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
	ctx.Response.Headers.Set(server.HeaderLambdaName, lambdaName)
	ctx.Response.Headers.Set(server.HeaderLambdaRegion, m.awsConfig.Region)

	// No need to call next() as we've fully handled the request
}

// OnResponse adds Lambda headers to the response
func (m *AWSLambdaMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	ctx.Response.Headers.Set(server.HeaderLambdaRegion, m.awsConfig.Region)
	next()
}

// getLambdaName returns a 64 characters sha256 hash of the lambda readable name
func (m *AWSLambdaMiddleware) getLambdaName(lambdaReadableName string) string {
	hash := sha256.Sum256([]byte(lambdaReadableName))
	return hex.EncodeToString(hash[:])
}

// getAccountID retrieves an AWS account ID by account name
func (m *AWSLambdaMiddleware) getAccountID(ctx context.Context, accountName string) (string, error) {
	// Check cache first
	if id, exists := m.accountCache[accountName]; exists {
		return id, nil
	}

	// If we couldn't extract an ID, try to look it up via Organizations API
	// Note: This requires proper permissions
	input := &organizations.ListAccountsInput{}
	paginator := organizations.NewListAccountsPaginator(m.orgsClient, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list AWS accounts: %v", err)
		}

		for _, account := range output.Accounts {
			if strings.EqualFold(accountName, *account.Name) {
				m.accountCache[accountName] = *account.Id
				return *account.Id, nil
			}
		}
	}

	return "", fmt.Errorf("could not find AWS account ID for name '%s'", accountName)
}

// createApiGatewayEvent creates an API Gateway v2 JSON event from the request
func (m *AWSLambdaMiddleware) createApiGatewayEvent(ctx *server.ServerContext, accountID string) ([]byte, error) {
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
			ApiId:        "ownstack-proxy",
			DomainName:   req.Host,
			DomainPrefix: "", // Default to an empty string
			Http: HttpDetails{
				Method:    req.Method,
				Path:      req.Path,
				Protocol:  "HTTP/1.1",
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
	if xForwardedFor := headers["x-forwarded-for"]; xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			event.RequestContext.Http.SourceIp = strings.TrimSpace(ips[0])
		}
	}

	// Safely extract user agent from the User-Agent header
	if userAgent, exists := headers["user-agent"]; exists {
		event.RequestContext.Http.UserAgent = userAgent
	}

	return json.Marshal(event)
}

// invokeLambda invokes the specified Lambda function with the given payload
func (m *AWSLambdaMiddleware) invokeLambda(ctx context.Context, lambdaArn string, payload []byte) (*lambda.InvokeOutput, error) {
	input := &lambda.InvokeInput{
		FunctionName: aws.String(lambdaArn),
		Payload:      payload,
	}

	logger.Debug("Invoking Lambda function: %s", lambdaArn)
	return m.lambdaClient.Invoke(ctx, input)
}

// processLambdaResponse processes the Lambda response and updates the ServerContext
func (m *AWSLambdaMiddleware) processLambdaResponse(ctx *server.ServerContext, lambdaResponse *lambda.InvokeOutput) error {
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
