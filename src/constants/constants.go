package constants

var (
	// These are default placeholder values for the app name and version in the dev mode.
	// During the build process, these values are overridden with the actual version and app name.
	// See: scripts/build.sh
	AppName    = "OwnStak Proxy DEV"
	Version    = "0.0.0"
	ConsoleURL = "https://console.dev.ownstak.com"
	SupportURL = "https://ownstak.com/support"
	Mode       = "development" // "development" or "production" (boolean would be better, but linker doesn't seem to support it with -X flag)
	// The prefix all our internal endpoints.
	// E.g. /__ownstak__/health
	InternalPathPrefix = "/__ownstak__"
)

// The names of the accepted ENV variables
const (
	// General
	EnvProvider             = "PROVIDER"               // aws
	EnvLogLevel             = "LOG_LEVEL"              // debug, info, warn, error
	EnvHost                 = "HOST"                   // e.g. 0.0.0.0
	EnvHttpPort             = "HTTP_PORT"              // e.g. 80
	EnvHttpsPort            = "HTTPS_PORT"             // e.g. 443
	EnvHttpsCert            = "HTTPS_CERT"             // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.pem
	EnvHttpsCertKey         = "HTTPS_CERT_KEY"         // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.key
	EnvHttpsCertCa          = "HTTPS_CERT_CA"          // e.g. /etc/certs/ownstak.com/wildcard-ownstak-link.ca
	EnvReadTimeout          = "READ_TIMEOUT"           // max waiting time for client to send the request
	EnvWriteTimeout         = "WRITE_TIMEOUT"          // max waiting time for client to receive the response
	EnvIdleTimeout          = "IDLE_TIMEOUT"           // max waiting time for client to send anything
	EnvMaxHeaderBytes       = "MAX_HEADER_BYTES"       // the max total size of accepted request headers in bytes
	EnvCacheMaxSize         = "CACHE_MAX_SIZE"         // e.g. 100 for 100MiB
	EnvLambdaFunctionPrefix = "LAMBDA_FUNCTION_PREFIX" // unique prefix for each cloud backend. e.g. "ownstak-1skda"

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
