package middlewares

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	server.DefaultMiddleware

	awsConfig    *aws.Config
	lambdaClient *lambda.Client
	orgsClient   *organizations.Client
	stsClient    *sts.Client
	accountId    string

	highPriorityQueue              chan struct{}
	highPriorityQueueConcurrency   int
	mediumPriorityQueue            chan struct{}
	mediumPriorityQueueConcurrency int
	lowPriorityQueue               chan struct{}
	lowPriorityQueueConcurrency    int

	streamingMode bool
}

const (
	defaultHighPriorityQueueConcurrency   = 1000
	defaultMediumPriorityQueueConcurrency = 20
	defaultLowPriorityQueueConcurrency    = 10
)

var (
	// IMPORTANT:
	// Do not change this variable without knowing what you're doing.
	// This needs to be in sync and backward compatible with all released ownstak-cli versions.
	// 8 null bytes indicates the end of response headers part and start of the body in streaming mode.
	// This special marker cannot appear anywhere in the res headers.
	// e.g: "\x00\x00\x00\x00\x00\x00\x00\x00"
	//
	// NOTE: This could be any sequence that's unlikely to appear in the response headers (can be in the body),
	// but if we use the same delimiter as AWS Lambda uses for Function URLs,
	// it will be easier for us to debug and troubleshoot issues.
	// See: https://github.com/aws/aws-lambda-nodejs-runtime-interface-client/blob/main/src/HttpResponseStream.js#L12
	//
	// Buffered lambda response example:
	// {
	//   "statusCode": 200,
	//   "headers": {
	//     "Content-Type": "text/html"
	//   },
	//   "body": "Hello from OwnStak Lambda!",
	//   "isBase64Encoded": false
	// }
	//
	// Streaming lambda response example:
	// {
	//   "statusCode": 200,
	//   "headers": {
	//     "Content-Type": "text/html"
	//   },
	//   "isBase64Encoded": false
	// }
	// \x00\x00\x00\x00\x00\x00\x00\x00
	// Hello from
	// OwnStak Lambda!
	//
	StreamingBodyDelimiter = strings.Repeat("\x00", 8)
)

func NewAWSLambdaMiddleware() *AWSLambdaMiddleware {
	// Check AWS credentials before doing anything
	provider := utils.GetEnv(constants.EnvProvider)
	if provider != constants.ProviderAWS {
		logger.Warn("Disabling AWS Lambda middleware - The provider doesn't match '%s'", constants.ProviderAWS)
		return nil
	}

	// Use the AWS_ACCOUNT_ID environment variable if set
	accountId := utils.GetEnv(constants.EnvAWSAccountId)
	// Set default region
	region := utils.GetEnvWithDefault(constants.EnvAWSRegion, "us-east-1")

	// Configure AWS options for potential endpoint overrides
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithHTTPClient(http.DefaultClient),
		// Disable retries
		config.WithRetryMode(aws.RetryModeAdaptive),
		config.WithRetryMaxAttempts(1),
		// Uncomment to troubleshoot AWS SDK API errors
		//config.WithClientLogMode(aws.LogRequestWithBody | aws.LogResponseWithBody | aws.LogRequestEventMessage | aws.LogResponseEventMessage | aws.LogSigning),
	}

	// Add endpoint resolver for testing/mocking if custom endpoints are set
	lambdaEndpoint := utils.GetEnv(constants.EnvAWSLambdaEndpoint)
	stsEndpoint := utils.GetEnv(constants.EnvAWSStSEndpoint)
	orgsEndpoint := utils.GetEnv(constants.EnvAWSOrganizationsEndpoint)

	// Use custom mock endpoints if set
	if lambdaEndpoint != "" || stsEndpoint != "" || orgsEndpoint != "" {
		configOptions = append(configOptions, config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				if service == "Lambda" && lambdaEndpoint != "" {
					logger.Debug("Calling custom Lambda endpoint: %s", lambdaEndpoint)
					return aws.Endpoint{
						URL:           lambdaEndpoint,
						SigningRegion: region,
					}, nil
				}
				if service == "STS" && stsEndpoint != "" {
					logger.Debug("Calling custom STS endpoint: %s", stsEndpoint)
					return aws.Endpoint{
						URL:           stsEndpoint,
						SigningRegion: region,
					}, nil
				}
				if service == "Organizations" && orgsEndpoint != "" {
					logger.Debug("Calling custom Organizations endpoint: %s", orgsEndpoint)
					return aws.Endpoint{
						URL:           orgsEndpoint,
						SigningRegion: region,
					}, nil
				}
				// Return empty endpoint to use default AWS endpoint for this service
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}),
		))
	}

	// Load AWS configuration with custom options
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		configOptions...,
	)

	if err != nil {
		logger.Error("Failed to load AWS SDK config: %v", err)
		return nil
	}

	// Create service clients with potential endpoint overrides
	lambdaClient := lambda.NewFromConfig(awsConfig)
	orgsClient := organizations.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	// This controls the used lambda invocation mode (default: true)
	// - true - sync invocation with streaming response
	// - false - sync invocation with buffered response
	//
	// By default we always use streaming invocation mode as it's way more efficient
	// and allows us to directly stream the response in 32KB chunks without buffering it in memory.
	// The invocation event can also be discarded immediately after we invoke the lambda function.
	// In the buffered invocation mode, we need to hold it the whole time we're waiting for the response.
	//
	// The CLI controls whatever is streams the response end to end to client
	// or it buffers it inside the lambda function and then streams the whole response to proxy.
	//
	// This option is controlled only by the LAMBDA_STREAMING_MODE environment variable and should never by controlled by the req header,
	// because that would potentially could open DoS attack vector.
	// NOTE: This option and whole logic should be completely removed in the future.
	// There's no reason for setting the streamingMode to false except for debugging and troubleshooting in production.
	streamingMode := utils.GetEnvWithDefault(constants.EnvLambdaStreamingMode, "true") == "true"

	return &AWSLambdaMiddleware{
		awsConfig:                      &awsConfig,
		lambdaClient:                   lambdaClient,
		orgsClient:                     orgsClient,
		stsClient:                      stsClient,
		accountId:                      accountId,
		highPriorityQueueConcurrency:   defaultHighPriorityQueueConcurrency,
		mediumPriorityQueueConcurrency: defaultMediumPriorityQueueConcurrency,
		lowPriorityQueueConcurrency:    defaultLowPriorityQueueConcurrency,
		highPriorityQueue:              make(chan struct{}, defaultHighPriorityQueueConcurrency),
		mediumPriorityQueue:            make(chan struct{}, defaultMediumPriorityQueueConcurrency),
		lowPriorityQueue:               make(chan struct{}, defaultLowPriorityQueueConcurrency),
		streamingMode:                  streamingMode,
	}
}

// OnStart is called when the server starts
func (m *AWSLambdaMiddleware) OnStart(server *server.Server) {
	// Calculate available concurrency based on available memory
	// to keep memory usage under control when we need to buffer the whole request body in memory
	// to calculate AWS HTTP Signature v4 of req and send it to AWS API.
	// Unlike other traditional proxies, it cannot be streamed chunk by chunk and needs to be fully buffered in memory.
	//
	// IMPORTANT: Account for JSON marshaling overhead (base64 encoding + metadata) and associated buffers.
	// AVG memory usage is ~4x the request size due to:
	// - Base64 encoding (33% overhead), 6MiB => 8MiB
	// - JSON structure metadata
	// - Response buffering
	// - Concurrent processing overhead
	// - io.read/write buffers
	// - string, headers copies
	//
	// Example: MAX_MEMORY=1024MiB results in invocation concurrency (high: 8192, medium: 85, low: 42)
	// NOTE: invocation concurrency != requests/sec, req/sec can be higher
	m.highPriorityQueueConcurrency = int(server.MaxMemory / (4 * 32 * 1024))         // reserve 4x 32KiB per request (standard queue - AVG 16KiB req + res headers + body + buffers)
	m.mediumPriorityQueueConcurrency = int(server.MaxMemory / (4 * 3 * 1024 * 1024)) // reserve 4x 3MB per request (optimistic queue for large requests - 64KiB-3MiB req body)
	m.lowPriorityQueueConcurrency = int(server.MaxMemory / (4 * 6 * 1024 * 1024))    // reserve 4x 6MB per request (pessimistic queue for large requests - MAX 3MiB-6MiB req body)

	// Ensure minimum concurrency values if the calculated values are too low
	if m.highPriorityQueueConcurrency < 1 {
		m.highPriorityQueueConcurrency = defaultHighPriorityQueueConcurrency
	}
	if m.mediumPriorityQueueConcurrency < 1 {
		m.mediumPriorityQueueConcurrency = defaultMediumPriorityQueueConcurrency
	}
	if m.lowPriorityQueueConcurrency < 1 {
		m.lowPriorityQueueConcurrency = defaultLowPriorityQueueConcurrency
	}

	m.highPriorityQueue = make(chan struct{}, m.highPriorityQueueConcurrency)
	m.mediumPriorityQueue = make(chan struct{}, m.mediumPriorityQueueConcurrency)
	m.lowPriorityQueue = make(chan struct{}, m.lowPriorityQueueConcurrency)

	logger.Info("AWS Lambda middleware initialized with throttling concurrency (high: %d, medium: %d, low: %d)", m.highPriorityQueueConcurrency, m.mediumPriorityQueueConcurrency, m.lowPriorityQueueConcurrency)
}

// OnRequest processes the request to invoke Lambda if appropriate
func (m *AWSLambdaMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
	transferEncoding := ctx.Request.Headers.Get(server.HeaderTransferEncoding)
	contentLength, _ := ctx.Request.ContentLength()

	// By default all small GET, POST... requests go into the high priority queue.
	// If server memory usage is over 80% of the configured max memory,
	// move all requests to the medium priority queue.
	// We also need to set acceptable timeout for waiting in the queue,
	// so we don't just accumulate traffic we likely won't be able to handle
	// and to keep AVG response time under load low.
	// NOTE: These numbers are tuned from load testing. Do not adjust without re-testing..
	reqQueue := m.highPriorityQueue
	reqQueueTimeout := time.Millisecond * 250

	if ctx.Server.UsedMemory >= ctx.Server.MaxMemory*80/100 {
		reqQueue = m.mediumPriorityQueue
		reqQueueTimeout = time.Millisecond * 15
	}

	// If the POST/PUT/PATCH/DELETE request has body with 64KiB or more or transfer-encoding: chunked header,
	// it will be processed in the medium or low priority queue based on the current memory usage.
	// This is to prevent the server from being overloaded with large buffered request bodies (5.9MiB - 6MiB)
	// and to keep memory usage under control.
	if contentLength >= 64*1024 || transferEncoding != "" {
		reqQueue = m.mediumPriorityQueue
		reqQueueTimeout = time.Millisecond * 15

		if ctx.Server.UsedMemory >= ctx.Server.MaxMemory*80/100 || contentLength >= 3*1024*1024 {
			reqQueue = m.lowPriorityQueue
			reqQueueTimeout = time.Millisecond * 15
		}
	}

	// Wait for an available slot in the target queue
	// before we try to invoke the Lambda function to sure we keep memory usage under control
	// while loading large req bodies and signing them
	enqueuedAt := time.Now()
	select {
	case reqQueue <- struct{}{}:
		// Got a slot, continue with the request
	case <-ctx.Request.Context().Done():
		// Request was cancelled or connection was closed while client was waiting for a slot in the queue.
		logger.Debug("Request context cancelled, exiting queue")
		return
	case <-time.After(reqQueueTimeout):
		// Request waited for too long to get a slot in the queue.
		ctx.Error(fmt.Sprintf("Server is overloaded: OwnStak proxy server couldn't enqueue the request in time because of high load. Please try again later. (queue slot timeout: %s)", reqQueueTimeout.String()), server.StatusServiceOverloaded)
		return
	}

	queueSlotReleased := false
	releaseQueueSlot := func() {
		if !queueSlotReleased {
			queueSlotReleased = true
			<-reqQueue
		}
	}
	// Always release the queue slot when we are done with the request processing
	defer releaseQueueSlot()

	queueWaitDuration := time.Since(enqueuedAt)
	if queueWaitDuration < time.Millisecond {
		queueWaitDuration = time.Millisecond
	}
	ctx.Debug("lambda-queue-duration=" + queueWaitDuration.String())

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

	// Construct the lambda name by adding prefix from environment variable or default to "ownstak"
	lambdaPrefix := utils.GetEnv(constants.EnvLambdaFunctionPrefix)
	if lambdaPrefix == "" {
		lambdaPrefix = "ownstak"
	}
	lambdaName = lambdaPrefix + "-" + lambdaName

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
		accountId, err := m.getAccountIdFromCaller(ctx)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to get AWS account ID: %v", err)
			ctx.Error(errorMessage, server.StatusInternalError)
			return
		}

		// Store the account ID for all other invocations
		m.accountId = accountId
	}

	// Construct the Lambda ARN
	lambdaArn := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s:%s", m.awsConfig.Region, m.accountId, lambdaName, lambdaAlias)

	// Store debug information about the lambda invocation
	ctx.Debug("lambda-name=" + lambdaName)
	ctx.Debug("lambda-alias=" + lambdaAlias)
	ctx.Debug("lambda-region=" + m.awsConfig.Region)
	ctx.Debug("lambda-streaming-mode=" + strconv.FormatBool(m.streamingMode))

	// Set x-own-streaming header to the request if not set yet.
	// This header tells the ownstak-cli that the used proxy version and invocation mode
	// supports the streaming and it can return response in streaming format.
	// Older proxy versions don't send this header => ownstak-cli handler will return response in buffered mode.
	ctx.Request.Headers.Set(server.HeaderXOwnStreaming, strconv.FormatBool(m.streamingMode))

	// NOTE: AWS Lambda invocation operation is quite memory intensive.
	// The issue is that the whole invocation is sync blocking operation,
	// so we need to hold the whole req payload including up to 6MB body
	// in memory until we receive the response from Lambda even though it's needed only for the actual invocation.
	invocationErr := m.invokeLambda(ctx, lambdaArn, releaseQueueSlot)

	// Handle invocation errors
	if invocationErr != nil {
		// Default error
		errorStatus := server.StatusInternalError
		errorMessage := fmt.Sprintf("Failed to invoke Lambda function: %v", invocationErr)

		// If the Lambda function was not found, it was probably retired.
		// In this case, we will redirect the user to the OwnStak Console with host passed as a query parameter.
		if strings.Contains(errorMessage, "ResourceNotFoundException") {
			consoleUrl := utils.GetEnvWithDefault(constants.EnvConsoleURL, "https://console.ownstak.com")
			originalUrl := ctx.Request.OriginalURL // e.g: https://ecommerce.com/products/123
			host := ctx.Request.Host               // e.g: ecommerce-default-123.aws-primary.org.ownstak.link

			redirectURL := fmt.Sprintf("%s/revive?host=%s&originalUrl=%s", consoleUrl, host, originalUrl)
			ctx.Response.Headers.Set(server.HeaderLocation, redirectURL)
			ctx.Response.Status = server.StatusTemporaryRedirect
			return
		}

		ctx.Error(errorMessage, errorStatus)
		return
	}

	// No need to call next() as we've fully handled the request
}

// getAccountIdFromCaller retrieves the AWS account ID from the caller identity
func (m *AWSLambdaMiddleware) getAccountIdFromCaller(ctx *server.RequestContext) (string, error) {
	// Get the caller identity using STS
	input := &sts.GetCallerIdentityInput{}
	result, err := m.stsClient.GetCallerIdentity(ctx.Request.Context(), input)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS caller identity: %v", err)
	}

	// Return the account ID
	accountID := *result.Account
	return accountID, nil
}

// createInvocationEvent creates an API Gateway v2 JSON compatible event from the request
func (m *AWSLambdaMiddleware) createInvocationEvent(ctx *server.RequestContext, accountID string) ([]byte, error) {
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
	var bodyBytes []byte
	var bodyErr error
	var isBase64Encoded bool

	// Read the request body only for POST, PUT, PATCH, DELETE methods
	// with content length header or transfer encoding header
	transferEncoding := req.Headers.Get(server.HeaderTransferEncoding)
	contentLength, _ := req.ContentLength()

	if contentLength > 0 || transferEncoding != "" {
		bodyBytes, bodyErr = req.Body()
		if bodyErr != nil {
			return nil, fmt.Errorf("failed to read request body: %w", bodyErr)
		}
	}

	// Determine if we should base64 encode the body (text vs binary data)
	// - it's way faster and results in smaller event to send pure text as it is, e.g.: 6MiB => 6MiB
	// - binary data have to be encoded to base64, which comes with about 33% overhead. e.g: 6MiB => 8MiB
	// - we cannot do this based on Content-Type header because it's not reliable and can be spoofed.
	// If attacker sends binary data with Content-Type:text/plain, the resulting text is even bigger than the base64 representation.
	// e.g: 6MiB => 22MiB
	if utf8.Valid(bodyBytes) {
		isBase64Encoded = false
		body = string(bodyBytes)
	} else {
		body = base64.StdEncoding.EncodeToString(bodyBytes)
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

	eventStr, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	return eventStr, nil
}

// invokeLambda determines which invocation method to use based on the payload size and other factors
func (m *AWSLambdaMiddleware) invokeLambda(ctx *server.RequestContext, lambdaArn string, releaseQueueSlot func()) error {
	if m.streamingMode {
		return m.invokeLambdaInStreamingMode(ctx, lambdaArn, releaseQueueSlot)
	} else {
		return m.invokeLambdaInBufferedMode(ctx, lambdaArn, releaseQueueSlot)
	}
}

// invokeLambdaInStreamingMode invokes the specified Lambda function with the given payload using streaming mode
func (m *AWSLambdaMiddleware) invokeLambdaInStreamingMode(ctx *server.RequestContext, lambdaArn string, releaseQueueSlot func()) error {
	// Create API Gateway v2 JSON event
	event, eventErr := m.createInvocationEvent(ctx, m.accountId)
	// Free the request body from memory immediately after creating the event
	ctx.Request.ClearBody()

	if eventErr != nil {
		errorMessage := fmt.Sprintf("Failed to create API Gateway event: %v", eventErr)
		ctx.Error(errorMessage, server.StatusInternalError)
		return errors.New(errorMessage)
	}

	// Use streaming mode for Lambda invocation
	input := &lambda.InvokeWithResponseStreamInput{
		FunctionName: aws.String(lambdaArn),
		Payload:      event,
	}

	logger.Debug("Invoking Lambda function in streaming mode: %s", lambdaArn)
	streamOutput, streamOutputErr := m.lambdaClient.InvokeWithResponseStream(ctx.Request.Context(), input)

	// Free the event from memory and release the queue slot immediately after the invocation
	event = nil
	input = nil
	releaseQueueSlot()

	if streamOutputErr != nil {
		return streamOutputErr
	}

	// Get the event stream and ensure it's closed when done
	eventStream := streamOutput.GetStream()
	defer eventStream.Close()

	// Define a maximum response size limit (200MB - AWS Lambda's maximum)
	const maxResponseSize = 200 * 1024 * 1024 // 200MB
	var responseSize int
	var responseHead []byte
	var responseHeadReceived bool
	var responseLastChunk []byte

	// Read the streaming response
	for event := range eventStream.Events() {
		// Check if context was cancelled (client disconnected)
		select {
		case <-ctx.Request.Context().Done():
			logger.Debug("Client disconnected, stopping Lambda stream processing")
			eventStream.Close()
			return nil
		default:
			// Continue processing
		}

		// Type switch on the event to handle different event types
		switch e := event.(type) {
		case *types.InvokeWithResponseStreamResponseEventMemberPayloadChunk:
			// Check if adding this chunk would exceed our limit
			chunk := e.Value.Payload
			chunkSize := len(chunk)
			responseSize += chunkSize

			// AWS Lambda at this moment allows returning up to 200MB of response body in streaming mode.
			// The error from AWS API should kick in before we reach this limit,
			// but we still have this artificial limit in place just in case the limit is increased in the future
			// and for cases where response with corrupted body delimiter is returned,
			// so we just don't blindly buffer it into memory.
			if responseSize > maxResponseSize {
				errorMessage := fmt.Sprintf("Lambda response exceeds size limit of %d bytes", maxResponseSize)
				ctx.Error(errorMessage, server.StatusProjectResponseTooLarge)
				return errors.New(errorMessage)
			}

			// Store last chunk for error handling but don't accumulate all chunks
			// and limit responseLastChunk to 64KiB
			responseLastChunk = chunk
			if len(responseLastChunk) > 64*1024 {
				responseLastChunk = responseLastChunk[0 : 64*1024]
			}

			if !responseHeadReceived {
				responseHead = append(responseHead, chunk...)

				// Use more efficient bytes.Index instead of string conversion to find delimiter
				delimiterBytes := []byte(StreamingBodyDelimiter)
				delimiterIndex := bytes.Index(responseHead, delimiterBytes)

				if delimiterIndex >= 0 {
					// Split chunk into head and body parts by the first found body delimiter
					headPart := make([]byte, delimiterIndex)
					copy(headPart, responseHead[:delimiterIndex])

					bodyStart := delimiterIndex + len(delimiterBytes)
					bodyPart := make([]byte, len(responseHead)-bodyStart)
					copy(bodyPart, responseHead[bodyStart:])

					responseHeadReceived = true

					responsePayload := &lambda.InvokeOutput{
						Payload:    headPart,
						StatusCode: streamOutput.StatusCode,
					}

					logger.Debug("Lambda response head: %s", string(headPart))

					// Process Lambda response and set to context
					invocationResponseErr := m.processLambdaResponse(ctx, responsePayload)
					if invocationResponseErr != nil {
						errorMessage := fmt.Sprintf("Failed to process streaming lambda response: %v", invocationResponseErr)
						ctx.Error(errorMessage, server.StatusInternalError)
						return invocationResponseErr
					}

					// Free memory immediately after processing
					responseHead = nil

					// Write the body part and release memory immediately
					ctx.Response.Write(bodyPart)
					continue
				}
			} else {
				// All remaining chunks are part of the response body
				// Stream them directly to the client and don't accumulate in memory
				ctx.Response.Write(chunk)
				// chunk is automatically garbage collected as it goes out of scope
			}
		case *types.InvokeWithResponseStreamResponseEventMemberInvokeComplete:
			// Check for function error
			if e.Value.ErrorCode != nil && *e.Value.ErrorCode != "" {
				// Create an error that matches the format expected by the caller
				var functionError string
				if e.Value.ErrorCode != nil {
					functionError = *e.Value.ErrorCode
				}

				errorDetails := fmt.Sprintf("{\"errorType\":\"%s\",\"errorMessage\":\"%s\"}", *e.Value.ErrorCode, "Lambda function execution failed")
				if e.Value.ErrorDetails != nil && *e.Value.ErrorDetails != "" {
					errorDetails = *e.Value.ErrorDetails
				}

				// Reconstruct the error response from lambda from the last chunk
				invocationResponse := &lambda.InvokeOutput{
					FunctionError: &functionError,
					Payload:       []byte(errorDetails),
					StatusCode:    streamOutput.StatusCode,
				}

				// If lambda returned some response, use it instead of the error payload
				if *e.Value.ErrorCode == "Unhandled" && len(responseLastChunk) > 0 {
					invocationResponse.Payload = responseLastChunk
				}

				logger.Debug("Processing streaming lambda error response")
				invocationResponseErr := m.processLambdaResponse(ctx, invocationResponse)
				if invocationResponseErr != nil {
					errorMessage := fmt.Sprintf("Failed to process streaming lambda response: %v", invocationResponseErr)
					ctx.Error(errorMessage, server.StatusInternalError)
					return invocationResponseErr
				}
				return nil
			}
		}
	}

	// Check for any error from the stream
	if streamErr := eventStream.Err(); streamErr != nil {
		// Check if this is just a context cancellation (client disconnected)
		if errors.Is(streamErr, context.Canceled) {
			logger.Debug("Lambda stream cancelled by client disconnect")
			return nil
		}

		logger.Error("lambda stream error: %v", streamErr)
		return fmt.Errorf("lambda stream error: %v", streamErr)
	}

	if !responseHeadReceived {
		responsePayload := &lambda.InvokeOutput{
			Payload:    responseHead,
			StatusCode: streamOutput.StatusCode,
		}

		// Process Lambda response and set to context
		//logger.Debug("Processing streaming lambda response")
		invocationResponseErr := m.processLambdaResponse(ctx, responsePayload)
		if invocationResponseErr != nil {
			errorMessage := fmt.Sprintf("Failed to process streaming lambda response: %v", invocationResponseErr)
			ctx.Error(errorMessage, server.StatusInternalError)
			return invocationResponseErr
		}
	}

	return nil
}

// invokeLambdaSync invokes the specified Lambda function with the given payload using standard synchronous mode
func (m *AWSLambdaMiddleware) invokeLambdaInBufferedMode(ctx *server.RequestContext, lambdaArn string, releaseQueueSlot func()) error {
	// Create API Gateway v2 JSON event
	event, eventErr := m.createInvocationEvent(ctx, m.accountId)
	if eventErr != nil {
		errorMessage := fmt.Sprintf("Failed to create API Gateway event: %v", eventErr)
		ctx.Error(errorMessage, server.StatusInternalError)
		return errors.New(errorMessage)
	}

	// Use standard synchronous Lambda invocation
	input := &lambda.InvokeInput{
		FunctionName: aws.String(lambdaArn),
		Payload:      event,
	}

	logger.Debug("Invoking Lambda function in buffered mode: %s", lambdaArn)
	invocationResponse, invocationErr := m.lambdaClient.Invoke(ctx.Request.Context(), input)

	// Free event, req body from memory when we finish invocation and release the queue slot
	ctx.Request.ClearBody()
	event = nil
	releaseQueueSlot()

	if invocationErr != nil {
		return invocationErr
	}

	// Process Lambda response and set to context
	invocationResponseErr := m.processLambdaResponse(ctx, invocationResponse)
	if invocationResponseErr != nil {
		errorMessage := fmt.Sprintf("Failed to process buffered lambda response: %v", invocationResponseErr)
		ctx.Error(errorMessage, server.StatusInternalError)
		return invocationResponseErr
	}

	return nil
}

// processLambdaResponse processes the Lambda response and updates the RequestContext
func (m *AWSLambdaMiddleware) processLambdaResponse(ctx *server.RequestContext, lambdaResponse *lambda.InvokeOutput) error {
	// Handle errors from the Lambda function payload
	if lambdaResponse.FunctionError != nil {
		var parsedPayload map[string]interface{}
		err := json.Unmarshal([]byte(lambdaResponse.Payload), &parsedPayload)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to parse Lambda function error payload: %v, payload: %s", err, string(lambdaResponse.Payload))
			ctx.Error(errorMessage, server.StatusInternalError)
			return nil
		}

		payloadErrorType := parsedPayload["errorType"].(string)
		payloadErrorMessage := parsedPayload["errorMessage"].(string)

		// Default error
		errorStatus := server.StatusProjectError
		errorMessage := fmt.Sprintf("Lambda function returned '%s' error: %s", payloadErrorType, payloadErrorMessage)

		// Handle known error types
		if strings.Contains(payloadErrorType, "TooManyRequests") {
			errorStatus = server.StatusProjectThrottled
		} else if strings.Contains(payloadErrorType, "RequestTooLarge") {
			errorStatus = server.StatusProjectRequestTooLarge
		} else if strings.Contains(payloadErrorType, "ResponseSizeTooLarge") {
			errorStatus = server.StatusProjectResponseTooLarge
		} else if strings.Contains(payloadErrorType, "InvalidRequest") {
			errorStatus = server.StatusProjectRequestInvalid
		} else if strings.Contains(payloadErrorType, "InvalidResponse") {
			errorStatus = server.StatusProjectResponseInvalid
		} else if strings.Contains(payloadErrorType, "Sandbox.Timeout") {
			errorStatus = server.StatusProjectTimeout
		} else if strings.Contains(payloadErrorType, "Runtime.ExitError") {
			errorStatus = server.StatusProjectCrashed
		}

		ctx.Error(errorMessage, errorStatus)
		return nil
	}

	// Parse the API Gateway response format
	var response struct {
		StatusCode        int                 `json:"statusCode,omitempty"`
		Headers           map[string]string   `json:"headers,omitempty"`
		MultiValueHeaders map[string][]string `json:"multiValueHeaders,omitempty"`
		Body              string              `json:"body,omitempty"`
		IsBase64Encoded   bool                `json:"isBase64Encoded,omitempty"`
	}

	err := json.Unmarshal(lambdaResponse.Payload, &response)
	if err != nil {
		return fmt.Errorf("failed to parse lambda response: %v, response: %s", err, string(lambdaResponse.Payload))
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

	// If response contains redirect, turn off streaming mode, so next middleware can follow it.
	// Otherwise, stream the response directly to the client.
	ctx.Response.EnableStreaming(ctx.Response.Headers.Get(server.HeaderLocation) == "")

	// Set body if present
	// e.g raw string: "<html><body><h1>Hello, World!</h1></body></html>"
	// e.g base64 encoded: "PGh0bWw+PGJvZHk+PGgxPkV4YW1wbGUgYm9keTwvaDE+PC9ib2R5PjwvaHRtbD4="
	if response.Body != "" {
		if response.IsBase64Encoded {
			bodyBytes, err := base64.StdEncoding.DecodeString(response.Body)
			if err != nil {
				return fmt.Errorf("failed to decode base64 response body: %v", err)
			}
			ctx.Response.Write(bodyBytes)
		} else {
			ctx.Response.Write([]byte(response.Body))
		}
	}

	return nil
}
