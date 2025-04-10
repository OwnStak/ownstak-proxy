package server

// HTTP header constants
const (
	// Standard HTTP headers
	HeaderContentType     = "Content-Type"
	HeaderContentLength   = "Content-Length"
	HeaderContentEncoding = "Content-Encoding"
	HeaderLocation        = "Location"
	HeaderHost            = "Host"
	HeaderAccept          = "Accept"
	HeaderRequestID       = "X-Request-ID"
	HeaderForwardedHost   = "X-Forwarded-Host"
	HeaderForwardedFor    = "X-Forwarded-For"
	HeaderForwardedProto  = "X-Forwarded-Proto"
	HeaderForwardedPort   = "X-Forwarded-Port"

	// Custom OwnStak Proxy headers
	HeaderProxyVersion   = "X-Own-Proxy-Version"
	HeaderFollowRedirect = "X-Own-Follow-Redirect"
	HeaderLambdaName     = "X-Own-Lambda-Name"
	HeaderLambdaRegion   = "X-Own-Lambda-Region"
	HeaderLambdaTime     = "X-Own-Lambda-Time"
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
