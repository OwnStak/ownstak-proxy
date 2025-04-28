package middlewares

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"net/url"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
)

// Define structs with ordered fields
type SystemInfo struct {
	GoVersion         string  `json:"goVersion"`
	Arch              string  `json:"arch"`
	OS                string  `json:"os"`
	CPUsCount         int     `json:"cpusCount"`
	GoroutinesCount   int     `json:"goroutinesCount"`
	MemoryUsage       uint64  `json:"memoryUsage"`
	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsageString string  `json:"memoryUsageString"`
	MemoryTotalString string  `json:"memoryTotalString"`
	MemoryUsagePct    float64 `json:"memoryUsagePct"`
	// System memory info
	SystemTotalMemory    uint64  `json:"systemTotalMemory"`
	SystemFreeMemory     uint64  `json:"systemFreeMemory"`
	SystemUsedMemory     uint64  `json:"systemUsedMemory"`
	SystemTotalMemoryStr string  `json:"systemTotalMemoryStr"`
	SystemFreeMemoryStr  string  `json:"systemFreeMemoryStr"`
	SystemUsedMemoryStr  string  `json:"systemUsedMemoryStr"`
	SystemMemoryUsagePct float64 `json:"systemMemoryUsagePct"`
}

type CacheInfo struct {
	Size     int `json:"size"`
	Count    int `json:"count"`
	MaxSize  int `json:"maxSize"`
	Sections int `json:"sections"`
}

type ServerInfoResponse struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Uptime       time.Duration `json:"uptime"`
	UptimeString string        `json:"uptimeString"`
	System       SystemInfo    `json:"system"`
	Cache        CacheInfo     `json:"cache"`
}

// ServerInfoMiddleware provides information about the server
type ServerInfoMiddleware struct{}

func NewServerInfoMiddleware() *ServerInfoMiddleware {
	return &ServerInfoMiddleware{}
}

// OnRequest handles the request phase
func (m *ServerInfoMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	ctx.Request.Headers.Set(server.HeaderXOwnProxyVersion, constants.Version)

	// Handle pprof endpoints under /__internal__/debug/pprof/
	if constants.Mode == "development" && strings.HasPrefix(ctx.Request.Path, "/__internal__/debug/pprof/") {
		// Remove the /__internal__ prefix to match pprof's expected paths
		path := strings.TrimPrefix(ctx.Request.Path, "/__internal__")

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
		return
	}

	// Only process requests to the internal info path
	if ctx.Request.Path != constants.InternalPathPrefix+"/info" {
		// Not our path, continue to the next middleware
		next()
		return
	}

	// Get server reference from context
	s := ctx.Server

	// Calculate uptime
	uptime := time.Since(s.StartTime())

	// Read memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get system memory info
	virtualMemory, err := mem.VirtualMemory()
	if err != nil {
		logger.Error("Failed to get system memory info: %v", err)
	}

	// Build info response with ordered fields
	info := ServerInfoResponse{
		Name:         constants.AppName,
		Version:      constants.Version,
		ID:           s.ServerId(),
		Uptime:       uptime / time.Second,
		UptimeString: uptime.String(),
		System: SystemInfo{
			GoVersion:         runtime.Version(),
			Arch:              runtime.GOARCH,
			OS:                runtime.GOOS,
			CPUsCount:         runtime.NumCPU(),
			GoroutinesCount:   runtime.NumGoroutine(),
			MemoryUsage:       memStats.Alloc,
			MemoryTotal:       memStats.Sys,
			MemoryUsageString: utils.FormatBytes(memStats.Alloc),
			MemoryTotalString: utils.FormatBytes(memStats.Sys),
			MemoryUsagePct:    float64(memStats.Alloc) / float64(memStats.Sys) * 100,
			// System memory info
			SystemTotalMemory:    virtualMemory.Total,
			SystemFreeMemory:     virtualMemory.Free,
			SystemUsedMemory:     virtualMemory.Used,
			SystemTotalMemoryStr: utils.FormatBytes(virtualMemory.Total),
			SystemFreeMemoryStr:  utils.FormatBytes(virtualMemory.Free),
			SystemUsedMemoryStr:  utils.FormatBytes(virtualMemory.Used),
			SystemMemoryUsagePct: virtualMemory.UsedPercent,
		},
		Cache: CacheInfo{
			Size:     s.CacheSize(),
			Count:    s.CacheCount(),
			MaxSize:  s.CacheMaxSize(),
			Sections: s.CacheSectionCount(),
		},
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal server info: %v", err)
		ctx.Response.Status = 500
		ctx.Response.Body = []byte("Error generating server info")
		return
	}

	// Return the info as JSON
	ctx.Response.Headers.Set(server.HeaderContentType, server.ContentTypeJSON)
	ctx.Response.Body = jsonData
}

// OnResponse adds version header to all responses
func (m *ServerInfoMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	ctx.Response.Headers.Set(server.HeaderXOwnProxyVersion, constants.Version)
	next()
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
