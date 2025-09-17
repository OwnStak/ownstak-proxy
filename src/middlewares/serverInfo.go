package middlewares

import (
	"encoding/json"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
	"runtime"
	"time"
)

// Define structs with ordered fields
type SystemInfo struct {
	GoVersion           string  `json:"goVersion"`
	Arch                string  `json:"arch"`
	OS                  string  `json:"os"`
	CPUsCount           int     `json:"cpusCount"`
	GoroutinesCount     int     `json:"goroutinesCount"`
	GoMemoryUsage       uint64  `json:"goMemoryUsage"`
	GoMemoryTotal       uint64  `json:"goMemoryTotal"`
	GoMemoryUsageString string  `json:"goMemoryUsageString"`
	GoMemoryTotalString string  `json:"goMemoryTotalString"`
	GoMemoryUsagePct    float64 `json:"goMemoryUsagePct"`
	// System memory info
	TotalMemory    uint64  `json:"totalMemory"`
	FreeMemory     uint64  `json:"freeMemory"`
	UsedMemory     uint64  `json:"usedMemory"`
	TotalMemoryStr string  `json:"totalMemoryStr"`
	FreeMemoryStr  string  `json:"freeMemoryStr"`
	UsedMemoryStr  string  `json:"usedMemoryStr"`
	MemoryUsagePct float64 `json:"memoryUsagePct"`
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
type ServerInfoMiddleware struct {
	server.DefaultMiddleware
}

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
	uptime := time.Since(s.StartTime)

	// Read memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalMemory, _ := utils.GetAvailableMemory()
	totalMemoryStr := utils.FormatBytes(totalMemory)

	usedMemory, _ := utils.GetUsedMemory()
	usedMemoryStr := utils.FormatBytes(usedMemory)

	freeMemory := totalMemory - usedMemory
	freeMemoryStr := utils.FormatBytes(freeMemory)

	// Build info response with ordered fields
	info := ServerInfoResponse{
		Name:         constants.AppName,
		Version:      constants.Version,
		ID:           s.ServerId,
		Uptime:       uptime / time.Second,
		UptimeString: uptime.String(),
		System: SystemInfo{
			GoVersion: runtime.Version(),
			Arch:      runtime.GOARCH,
			OS:        runtime.GOOS,
			CPUsCount: runtime.NumCPU(),
			// Go GC memory info
			GoroutinesCount:     runtime.NumGoroutine(),
			GoMemoryUsage:       memStats.Alloc,
			GoMemoryTotal:       memStats.Sys,
			GoMemoryUsageString: utils.FormatBytes(memStats.Alloc),
			GoMemoryTotalString: utils.FormatBytes(memStats.Sys),
			GoMemoryUsagePct:    float64(memStats.Alloc) / float64(memStats.Sys) * 100,
			// System memory info
			TotalMemory:    totalMemory,
			FreeMemory:     freeMemory,
			UsedMemory:     usedMemory,
			TotalMemoryStr: totalMemoryStr,
			FreeMemoryStr:  freeMemoryStr,
			UsedMemoryStr:  usedMemoryStr,
			MemoryUsagePct: float64(usedMemory) / float64(totalMemory) * 100,
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
