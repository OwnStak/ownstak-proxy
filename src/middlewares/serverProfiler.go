package middlewares

import (
	"bytes"
	"net/http"
	"net/http/pprof"
	"net/url"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/server"
	"strings"
)

// ServerProfilerMiddleware allows to profile the memory in development mode
type ServerProfilerMiddleware struct {
	server.DefaultMiddleware
}

func NewServerProfilerMiddleware() *ServerProfilerMiddleware {
	return &ServerProfilerMiddleware{}
}

// OnRequest handles the request phase
func (m *ServerProfilerMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
	// Run only in development mode and if the path is /__ownstak__/debug/pprof/
	serverProfilerPath := constants.InternalPathPrefix + "/debug/pprof/"
	if !strings.HasPrefix(ctx.Request.Path, serverProfilerPath) || constants.Mode != "development" {
		next()
		return
	}

	// Remove the /__ownstak__ prefix to match pprof's expected paths
	path := strings.TrimPrefix(ctx.Request.Path, constants.InternalPathPrefix)

	// Create a response writer that captures the output
	rw := &responseWriter{
		headers: make(http.Header),
		body:    new(bytes.Buffer),
	}

	// Create a request with the modified path
	req := &http.Request{
		Method: ctx.Request.Method,
		URL: &url.URL{
			Path: path,
		},
	}

	// Route to the appropriate pprof handler
	switch path {
	case "/debug/pprof/":
		pprof.Index(rw, req)
	case "/debug/pprof/cmdline":
		pprof.Cmdline(rw, req)
	case "/debug/pprof/profile":
		pprof.Profile(rw, req)
	case "/debug/pprof/symbol":
		pprof.Symbol(rw, req)
	case "/debug/pprof/trace":
		pprof.Trace(rw, req)
	default:
		// Handle goroutine, heap, threadcreate, etc.
		if strings.HasPrefix(path, "/debug/pprof/") {
			pprof.Handler(strings.TrimPrefix(path, "/debug/pprof/")).ServeHTTP(rw, req)
		} else {
			ctx.Error("Not Found", http.StatusNotFound)
			return
		}
	}

	// Copy the response
	ctx.Response.Status = rw.status
	for k, v := range rw.headers {
		ctx.Response.Headers.Set(k, v[0])
	}
	ctx.Response.Body = rw.body.Bytes()
}

// responseWriter implements http.ResponseWriter to capture pprof output
type responseWriter struct {
	headers http.Header
	body    *bytes.Buffer
	status  int
}

func (rw *responseWriter) Header() http.Header {
	return rw.headers
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// If no status code has been set, default to 200
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.body.Write(b)
}

func (rw *responseWriter) WriteHeader(status int) {
	// Ensure we never set a status code of 0
	if status == 0 {
		status = http.StatusOK
	}
	rw.status = status
}
