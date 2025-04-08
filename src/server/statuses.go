package server

// HTTP Status codes
const (
	StatusOK                  = 200
	StatusBadRequest          = 400
	StatusInternalServerError = 500

	// Custom status codes
	StatusInternalError          = 530
	StatusLambdaProjectError     = 531
	StatusLambdaNotFound         = 534
	StatusLambdaRequestInvalid   = 535
	StatusLambdaResponseInvalid  = 536
	StatusLambdaRequestTooLarge  = 537
	StatusLambdaResponseTooLarge = 538
	StatusLambdaTimeout          = 539
	StatusLambdaThrottled        = 540
	StatusLambdaCrashed          = 541
)
