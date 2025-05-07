package server

// HTTP header constants
const (
	// Standard HTTP headers
	HeaderContentType        = "Content-Type"
	HeaderContentLength      = "Content-Length"
	HeaderContentEncoding    = "Content-Encoding"
	HeaderContentDisposition = "Content-Disposition"
	HeaderCacheControl       = "Cache-Control"
	HeaderLocation           = "Location"
	HeaderHost               = "Host"
	HeaderAccept             = "Accept"
	HeaderRequestID          = "X-Request-ID"
	HeaderUserAgent          = "User-Agent"
	HeaderXForwardedHost     = "X-Forwarded-Host"
	HeaderXForwardedFor      = "X-Forwarded-For"
	HeaderXForwardedProto    = "X-Forwarded-Proto"
	HeaderXForwardedPort     = "X-Forwarded-Port"
	HeaderTransferEncoding   = "Transfer-Encoding"
	HeaderAcceptRanges       = "Accept-Ranges"
	HeaderContentRange       = "Content-Range"
	HeaderETag               = "ETag"
	HeaderLastModified       = "Last-Modified"
	HeaderExpires            = "Expires"

	// Custom OwnStak Proxy headers
	HeaderXOwnPrefix         = "X-Own-"
	HeaderXOwnProxy          = "X-Own-Proxy"           // Present in req then is proxied
	HeaderXOwnProxyVersion   = "X-Own-Proxy-Version"   // Present in req/res headers when the request is proxied
	HeaderXOwnHost           = "X-Own-Host"            // Works as replacement for Host header and preffered way of specifying the host for the proxy in the req
	HeaderXOwnLambdaMode     = "X-Own-Lambda-Mode"     // When detected in the req, the proxy will use sync/async mode to invoke the Lambda function
	HeaderXOwnMergeHeaders   = "X-Own-Merge-Headers"   // When present in the req, the proxy will merge the headers from the original headers when following a redirect
	HeaderXOwnFollowRedirect = "X-Own-Follow-Redirect" // When detected in the res from lambda, the proxy will follow the redirect

	HeaderXOwnImageOptimizer = "X-Own-Image-Optimizer" // Present in res when the request is handled by the image optimizer
	HeaderXOwnProxyDebug     = "X-Own-Proxy-Debug"     // Present in res, contains the debug information from the proxy middlewares
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
