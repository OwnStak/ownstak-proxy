package middlewares

import (
	"encoding/json"
	"fmt"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"runtime"
	"time"
)

// Define structs with ordered fields
type SystemInfo struct {
	GoVersion       string `json:"goVersion"`
	Arch            string `json:"arch"`
	OS              string `json:"os"`
	CPUsCount       int    `json:"cpusCount"`
	GoroutinesCount int    `json:"goroutinesCount"`
	RamUsage        uint64 `json:"ramUsage"`
	RamTotal        uint64 `json:"ramTotal"`
	RamUsageString  string `json:"ramUsageString"`
	RamTotalString  string `json:"ramTotalString"`
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
	ctx.Request.Headers.Set(server.HeaderProxyVersion, constants.Version)

	// Only process requests to the internal info path
	if ctx.Request.Path != "/__internal__/info" {
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

	// Build info response with ordered fields
	info := ServerInfoResponse{
		Name:         constants.AppName,
		Version:      constants.Version,
		ID:           s.ServerId(),
		Uptime:       uptime / time.Second,
		UptimeString: uptime.String(),
		System: SystemInfo{
			GoVersion:       runtime.Version(),
			Arch:            runtime.GOARCH,
			OS:              runtime.GOOS,
			CPUsCount:       runtime.NumCPU(),
			GoroutinesCount: runtime.NumGoroutine(),
			RamUsage:        memStats.Alloc,
			RamTotal:        memStats.Sys,
			RamUsageString:  fmt.Sprintf("%d MiB", memStats.Alloc/1024/1024),
			RamTotalString:  fmt.Sprintf("%d MiB", memStats.Sys/1024/1024),
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

	// Don't call next since we've handled the request
}

// OnResponse adds version header to all responses
func (m *ServerInfoMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	ctx.Response.Headers.Set(server.HeaderProxyVersion, constants.Version)
	next()
}
