package utils

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// GetUsedMemory returns the current memory usage of this process in bytes
// Priority: Docker cgroup > Process memory > System memory
func GetUsedMemory() (uint64, error) {
	// Try to get current memory usage from cgroup (container environment)
	if usage, err := GetCgroupUsedMemory(); err == nil && usage > 0 {
		return usage, nil
	}

	// Try to get current process memory usage first
	if usage, err := GetProcessUsedMemory(); err == nil && usage > 0 {
		return usage, nil
	}

	// Fallback to system memory usage
	return GetSystemUsedMemory()
}

// GetCgroupCurrentMemory returns the current memory usage from cgroup in bytes
func GetCgroupUsedMemory() (uint64, error) {
	// Try different cgroup paths for current memory usage
	cgroupPaths := []string{
		"/sys/fs/cgroup/memory/memory.usage_in_bytes", // cgroup v1
		"/sys/fs/cgroup/memory.current",               // cgroup v2
		"/sys/fs/cgroup/memory/memory.usage",          // alternative cgroup v1
	}

	for _, path := range cgroupPaths {
		if data, err := os.ReadFile(path); err == nil {
			// Remove newline and whitespace
			usageStr := strings.TrimSpace(string(data))

			// Parse the usage
			if usage, err := strconv.ParseUint(usageStr, 10, 64); err == nil {
				return usage, nil
			}
		}
	}

	return 0, fmt.Errorf("no valid cgroup memory usage found")
}

// GetSystemCurrentMemory returns the current system memory usage in bytes
func GetSystemUsedMemory() (uint64, error) {
	// Try reading from /proc/meminfo first (Linux)
	if memInfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available uint64

		scanner := bufio.NewScanner(strings.NewReader(string(memInfo)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						total = kb * 1024
					}
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						available = kb * 1024
					}
				}
			}
		}

		if total > 0 && available > 0 {
			return total - available, nil
		}
	}

	return 0, fmt.Errorf("unable to determine system memory usage from /proc/meminfo")
}

// GetProcessUsedMemory returns the current memory usage of this process in bytes
func GetProcessUsedMemory() (uint64, error) {
	// Try reading from /proc/self/status (Linux)
	if statusData, err := os.ReadFile("/proc/self/status"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(statusData)))
		for scanner.Scan() {
			line := scanner.Text()
			// Look for VmRSS (Resident Set Size) - physical memory currently used
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						return kb * 1024, nil // Convert KB to bytes
					}
				}
			}
		}
	}

	// Fallback: try /proc/self/statm
	if statmData, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(statmData))
		if len(fields) >= 2 {
			// First field is total program size in pages
			if pages, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
				// Page size is typically 4096 bytes on Linux
				pageSizeBytes := uint64(4096)
				return pages * pageSizeBytes, nil
			}
		}
	}

	return 0, fmt.Errorf("unable to determine process memory usage")
}

// GetAvailableMemory returns the first available memory limit in bytes
// Priority: Docker cgroup > System memory
func GetAvailableMemory() (uint64, error) {
	// Try Docker cgroup first
	if limit, err := GetCgroupAvailableMemory(); err == nil && limit > 0 {
		return limit, nil
	}

	// Fallback to system memory
	return GetSystemAvailableMemory()
}

// GetCgroupMaxMemory returns the first cgroup memory limit in bytes from Docker
func GetCgroupAvailableMemory() (uint64, error) {
	// Try different cgroup paths for Docker
	cgroupPaths := []string{
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory.current",
	}

	for _, path := range cgroupPaths {
		if data, err := os.ReadFile(path); err == nil {
			// Remove newline and whitespace
			limitStr := strings.TrimSpace(string(data))

			// Check if it's unlimited (-1)
			if limitStr == "-1" || limitStr == "max" {
				return 0, nil // 0 indicates unlimited
			}

			// Parse the limit
			if limit, err := strconv.ParseUint(limitStr, 10, 64); err == nil {
				return limit, nil
			}
		}
	}

	return 0, fmt.Errorf("no valid cgroup memory limit found")
}

// GetSystemMaxMemory returns the total system memory in bytes
func GetSystemAvailableMemory() (uint64, error) {
	// Try reading from /proc/meminfo first (Linux)
	if memInfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(memInfo)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						return kb * 1024, nil // Convert KB to bytes
					}
				}
			}
		}
	}

	// Fallback to syscall for other systems (only on supported platforms)
	// Note: syscall.Sysinfo is not available on all platforms
	return 0, fmt.Errorf("unable to determine system memory limit from /proc/meminfo")
}

// ParseMemorySize parses a memory size string with units (e.g., "512MB", "1GB", "1024KB")
// Returns the size in bytes
func ParseMemorySize(sizeStr string) (uint64, error) {
	// Remove any whitespace
	sizeStr = strings.TrimSpace(sizeStr)

	// Check if it's just a number (bytes)
	if size, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
		return size, nil
	}

	// Check for units
	units := map[string]uint64{
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
		"K":  1024,
		"M":  1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
	}

	for unit, multiplier := range units {
		if strings.HasSuffix(strings.ToUpper(sizeStr), unit) {
			// Extract the numeric part
			numericPart := strings.TrimSuffix(strings.ToUpper(sizeStr), unit)
			numericPart = strings.TrimSpace(numericPart)

			if size, err := strconv.ParseUint(numericPart, 10, 64); err == nil {
				return size * multiplier, nil
			}
			return 0, fmt.Errorf("invalid numeric part in memory size: %s", numericPart)
		}
	}

	return 0, fmt.Errorf("invalid memory size format: %s (supported units: B, KB/K, MB/M, GB/G, TB/T)", sizeStr)
}
