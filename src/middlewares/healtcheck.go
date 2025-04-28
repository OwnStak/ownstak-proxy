package middlewares

import (
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/server"
)

// HealthcheckMiddleware provides a healthcheck endpoint that just returns 200 OK
type HealthcheckMiddleware struct{}

func NewHealthcheckMiddleware() *HealthcheckMiddleware {
	return &HealthcheckMiddleware{}
}

func (m *HealthcheckMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	if ctx.Request.Path == constants.InternalPathPrefix+"/health" {
		ctx.Response.Status = 200
		ctx.Response.Body = []byte("OK")
		ctx.Response.Headers.Set(server.HeaderContentType, "text/plain")
		return
	}
	next()
}

func (m *HealthcheckMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	next()
}
