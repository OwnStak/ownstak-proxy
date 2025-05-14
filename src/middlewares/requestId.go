package middlewares

import (
	"ownstak-proxy/src/server"

	"github.com/google/uuid"
)

// RequestIdMiddleware provides request ID generation and tracking
type RequestIdMiddleware struct{}

func NewRequestIdMiddleware() *RequestIdMiddleware {
	return &RequestIdMiddleware{}
}

// OnRequest checks for an existing X-Request-ID header and generates a new one if missing
func (m *RequestIdMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
	requestID := ctx.Request.Headers.Get(server.HeaderRequestID)
	if requestID == "" {
		requestID = uuid.New().String()
		ctx.Request.Headers.Set(server.HeaderRequestID, requestID)
	}
	// Set request ID to res in the request handle phase, so other middlewares can break chain, exit early
	// and stream the response directly to the client without waiting for the response handle phase
	ctx.Response.Headers.Set(server.HeaderRequestID, requestID)
	next()
}

func (m *RequestIdMiddleware) OnResponse(ctx *server.RequestContext, next func()) {
	// Set request ID to res in the response send phase for middlewares that overrides the whole response
	ctx.Response.Headers.Set(server.HeaderRequestID, ctx.Request.Headers.Get(server.HeaderRequestID))
	next()
}
