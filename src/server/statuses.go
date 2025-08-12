package server

// HTTP Status codes
const (
	StatusOK                  = 200
	StatusBadRequest          = 400
	StatusRequestEntityTooLarge = 413
	StatusInternalServerError = 500
	StatusServiceUnavailable  = 503
	StatusPermanentRedirect   = 301
	StatusTemporaryRedirect   = 302

	// Custom status codes
	StatusInternalError          = 530
	StatusAccountNotFound        = 531
	StatusLambdaProjectError     = 534
	StatusLambdaRequestInvalid   = 535
	StatusLambdaResponseInvalid  = 536
	StatusLambdaRequestTooLarge  = 537
	StatusLambdaResponseTooLarge = 538
	StatusLambdaTimeout          = 539
	StatusLambdaThrottled        = 540
	StatusLambdaCrashed          = 541
)
