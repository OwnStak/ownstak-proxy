package constants

var (
	// These are default placeholder values for the app name and version in the dev mode.
	// During the build process, these values are overridden with the actual version and app name.
	// See: scripts/build.sh
	AppName = "OwnStak Proxy DEV"
	Version = "0.0.0"
	Mode    = "development" // "development" or "production" (boolean would be better, but linker doesn't seem to support it with -X flag)
	// The prefix all our internal endpoints.
	// E.g. /__ownstak__/health
	InternalPathPrefix = "/__ownstak__"
)

// The names of the accepted ENV variables
const (
	// General
	EnvConsoleURL           = "CONSOLE_URL"            // e.g. https://console.ownstak.com
	EnvSupportURL           = "SUPPORT_URL"            // e.g. https://ownstak.com/support
	EnvProvider             = "PROVIDER"               // aws
	EnvLogLevel             = "LOG_LEVEL"              // debug, info, warn, error
	EnvHost                 = "HOST"                   // e.g. 0.0.0.0
	EnvHttpPort             = "HTTP_PORT"              // e.g. 80
	EnvHttpsPort            = "HTTPS_PORT"             // e.g. 443
	EnvHttpsCert            = "HTTPS_CERT"             // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.pem
	EnvHttpsCertKey         = "HTTPS_CERT_KEY"         // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.key
	EnvHttpsCertCa          = "HTTPS_CERT_CA"          // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.ca
	EnvResWriteTimeout      = "RES_WRITE_TIMEOUT"      // max waiting time for client to receive the response
	EnvReqReadTimeout       = "REQ_READ_TIMEOUT"       // max waiting time for client to send the request
	EnvReqIdleTimeout       = "REQ_IDLE_TIMEOUT"       // max waiting time for client to send anything
	EnvReqMaxHeadersSize    = "REQ_MAX_HEADERS_SIZE"   // the max total size of accepted request headers in bytes
	EnvReqMaxBodySize       = "REQ_MAX_BODY_SIZE"      // the max size of the request body in bytes
	EnvMaxMemory            = "MAX_MEMORY"             // max memory in bytes that the proxy server can use
	EnvLambdaFunctionPrefix = "LAMBDA_FUNCTION_PREFIX" // unique prefix for each cloud backend. e.g. "ownstak-1skda"
	EnvLambdaStreamingMode  = "LAMBDA_STREAMING_MODE"  // true by default, set to false to invoke lambda in legacy buffered mode

	// Go GC
	EnvGoMemLimit = "GOMEMLIMIT" // e.g. 1024MiB, heap allocated memory size that Golang garbage collector will try to reach if possible

	// AWS Lambda middleware
	EnvAWSAccountId             = "AWS_ACCOUNT_ID"
	EnvAWSRegion                = "AWS_REGION"
	EnvAWSLambdaEndpoint        = "AWS_LAMBDA_ENDPOINT"
	EnvAWSOrganizationsEndpoint = "AWS_ORGANIZATIONS_ENDPOINT"
	EnvAWSStSEndpoint           = "AWS_STS_ENDPOINT"

	// VIPS
	EnvVipsDebug        = "VIPS_DEBUG"
	EnvMallocArenaMax   = "MALLOC_ARENA_MAX"
	EnvVipsConcurrency  = "VIPS_CONCURRENCY"
	EnvVipsMaxCacheSize = "VIPS_MAX_CACHE_SIZE"
	EnvVipsMaxCacheMem  = "VIPS_MAX_CACHE_MEM"
	EnvVipsLeak         = "VIPS_LEAK"
	EnvVipsTrace        = "VIPS_TRACE"
)

// Accepted providers
const (
	ProviderAWS = "aws"
)
