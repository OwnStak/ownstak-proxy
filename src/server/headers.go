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

	// Custom Lambda Proxy headers
	HeaderProxyVersion   = "X-Proxy-Version"
	HeaderLambdaName     = "X-Lambda-Name"
	HeaderLambdaRegion   = "X-Lambda-Region"
	HeaderLambdaTime     = "X-Lambda-Time"
	HeaderFollowRedirect = "X-Follow-Redirect"
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
