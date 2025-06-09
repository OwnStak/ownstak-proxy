package middlewares

import (
	"encoding/json"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
	"runtime"
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

type ServerInfoResponse struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Uptime       time.Duration `json:"uptime"`
	UptimeString string        `json:"uptimeString"`
	System       SystemInfo    `json:"system"`
}

// ServerInfoMiddleware provides information about the server
type ServerInfoMiddleware struct{}

func NewServerInfoMiddleware() *ServerInfoMiddleware {
	return &ServerInfoMiddleware{}
}

// OnRequest handles the request phase
func (m *ServerInfoMiddleware) OnRequest(ctx *server.RequestContext, next func()) {
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

func (m *ServerInfoMiddleware) OnResponse(ctx *server.RequestContext, next func()) {
	next()
}
