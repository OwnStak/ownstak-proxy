package middlewares

import (
	"ownstack-proxy/src/server"

	"github.com/google/uuid"
)

// RequestIdMiddleware provides request ID generation and tracking
type RequestIdMiddleware struct{}

func NewRequestIdMiddleware() *RequestIdMiddleware {
	return &RequestIdMiddleware{}
}

// OnRequest checks for an existing X-Request-ID header and generates a new one if missing
func (m *RequestIdMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	requestID := ctx.Request.Headers.Get(server.HeaderRequestID)
	if requestID == "" {
		requestID = uuid.New().String()
		ctx.Request.Headers.Set(server.HeaderRequestID, requestID)
	}
	next()
}

func (m *RequestIdMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	ctx.Response.Headers.Set(server.HeaderRequestID, ctx.Request.Headers.Get(server.HeaderRequestID))
	next()
}
