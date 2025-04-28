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

	HeaderXOwnFollowedRedirectUrl    = "X-Own-Followed-Redirect-Url"    // Present in res headers when the proxy follows a redirect
	HeaderXOwnFollowedRedirectStatus = "X-Own-Followed-Redirect-Status" // Present in res headers when the proxy follows a redirect
	HeaderXOwnLambdaName             = "X-Own-Lambda-Name"              // Present in res when the request is proxied to a Lambda function
	HeaderXOwnLambdaRegion           = "X-Own-Lambda-Region"            // Present in res when the request is proxied to a Lambda function
	HeaderXOwnLambdaDuration         = "X-Own-Lambda-Duration"          // Present in res when the request is proxied to a Lambda function
	HeaderXOwnImageOptimizer         = "X-Own-Image-Optimizer"          // Present in res when the request is handled by the image optimizer
	HeaderXOwnImageOptimizError      = "X-Own-Image-Optimizer-Error"    // Present in res when the request is handled by the image optimizer and there is an error
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
