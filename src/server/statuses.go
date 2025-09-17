package server

// HTTP Status codes
const (
	StatusOK = 200

	StatusMovedPermanently  = 301
	StatusFound             = 302
	StatusNotModified       = 304
	StatusTemporaryRedirect = 307
	StatusPermanentRedirect = 308

	StatusBadRequest         = 400
	StatusUnauthorized       = 401
	StatusForbidden          = 403
	StatusNotFound           = 404
	StatusMethodNotAllowed   = 405
	StatusRequestTimeout     = 408
	StatusConflict           = 409
	StatusPreconditionFailed = 412
	StatusContentTooLarge    = 413
	StatusTooManyRequests    = 429

	StatusInternalServerError     = 500
	StatusNotImplemented          = 501
	StatusBadGateway              = 502
	StatusServiceUnavailable      = 503
	StatusGatewayTimeout          = 504
	StatusHTTPVersionNotSupported = 505

	// Custom status codes
	StatusServiceOverloaded     = 529
	StatusInternalError         = 530
	StatusRequestRecursionError = 531

	StatusProjectError            = 540
	StatusProjectRequestInvalid   = 541
	StatusProjectResponseInvalid  = 542
	StatusProjectRequestTooLarge  = 543
	StatusProjectResponseTooLarge = 544
	StatusProjectTimeout          = 545
	StatusProjectThrottled        = 546
	StatusProjectCrashed          = 547
)
