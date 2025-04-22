package server

// HTTP header constants
const (
	// Standard HTTP headers
	HeaderContentType      = "Content-Type"
	HeaderContentLength    = "Content-Length"
	HeaderContentEncoding  = "Content-Encoding"
	HeaderCacheControl     = "Cache-Control"
	HeaderLocation         = "Location"
	HeaderHost             = "Host"
	HeaderAccept           = "Accept"
	HeaderRequestID        = "X-Request-ID"
	HeaderUserAgent        = "User-Agent"
	HeaderXForwardedHost   = "X-Forwarded-Host"
	HeaderXForwardedFor    = "X-Forwarded-For"
	HeaderXForwardedProto  = "X-Forwarded-Proto"
	HeaderXForwardedPort   = "X-Forwarded-Port"
	HeaderTransferEncoding = "Transfer-Encoding"

	// Custom OwnStak Proxy headers
	HeaderXOwnPrefix            = "X-Own-"
	HeaderXOwnProxy             = HeaderXOwnPrefix + "Proxy"
	HeaderXOwnProxyVersion      = HeaderXOwnPrefix + "Proxy-Version"
	HeaderXOwnFollowRedirect    = HeaderXOwnPrefix + "Follow-Redirect"
	HeaderXOwnLambdaName        = HeaderXOwnPrefix + "Lambda-Name"
	HeaderXOwnLambdaRegion      = HeaderXOwnPrefix + "Lambda-Region"
	HeaderXOwnLambdaTime        = HeaderXOwnPrefix + "Lambda-Time"
	HeaderXOwnImageOptimizer    = HeaderXOwnPrefix + "Image-Optimizer"
	HeaderXOwnImageOptimizError = HeaderXOwnPrefix + "Image-Optimizer-Error"
	HeaderXOwnLambdaMode        = HeaderXOwnPrefix + "Lambda-Mode" // sync or async
)

// Content type constants
const (
	ContentTypeJSON        = "application/json"
	ContentTypeXML         = "application/xml"
	ContentTypeFormURL     = "application/x-www-form-urlencoded"
	ContentTypeHTML        = "text/html"
	ContentTypePlain       = "text/plain"
	ContentTypeMultipart   = "multipart/form-data"
	ContentTypeOctetStream = "application/octet-stream"
)
