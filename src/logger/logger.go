package logger

import (
	"fmt"
	"log"
	"os"
	"ownstak-proxy/src/constants"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	traceLogger = log.New(os.Stdout, "", 0)
	debugLogger = log.New(os.Stdout, "", 0)
	infoLogger  = log.New(os.Stdout, "", 0)
	warnLogger  = log.New(os.Stdout, "", 0)
	errorLogger = log.New(os.Stderr, "", 0)
	fatalLogger = log.New(os.Stderr, "", 0)
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
	colorWhite  = "\033[97m"
	colorYellow = "\033[33m"
)

// Define log levels
const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	FATAL
)

// Current log level
var currentLogLevel = INFO

func init() {
	// Load .env file if it exists
	godotenv.Load(".env", ".env.local")

	// Get log level from environment variable
	logLevel := os.Getenv(constants.EnvLogLevel)
	switch strings.ToLower(logLevel) {
	case "debug":
		currentLogLevel = DEBUG
	case "info":
		currentLogLevel = INFO
	case "warn":
		currentLogLevel = WARN
	case "error":
		currentLogLevel = ERROR
	case "fatal":
		currentLogLevel = FATAL
	default:
		currentLogLevel = INFO
	}
}

// Log formats and logs the message with the given level and color
func Log(logger *log.Logger, level string, color string, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	logger.Printf("%s[%s] %s: %s%s\n", color, timestamp, level, message, colorReset)
}

// Info logs informational messages
func Info(format string, args ...interface{}) {
	if currentLogLevel <= INFO {
		Log(infoLogger, "INFO", colorWhite, format, args...)
	}
}

// Trace logs trace messages
func Trace(format string, args ...interface{}) {
	if currentLogLevel <= DEBUG {
		Log(traceLogger, "TRACE", colorGray, format, args...)
	}
}

// Debug logs debug messages
func Debug(format string, args ...interface{}) {
	if currentLogLevel <= DEBUG {
		Log(debugLogger, "DEBUG", colorGray, format, args...)
	}
}

// Warn logs warning messages
func Warn(format string, args ...interface{}) {
	if currentLogLevel <= WARN {
		Log(warnLogger, "WARN", colorYellow, format, args...)
	}
}

// Error logs error messages
func Error(format string, args ...interface{}) {
	if currentLogLevel <= ERROR {
		Log(errorLogger, "ERROR", colorRed, format, args...)
	}
}

// Fatal logs fatal error messages and exits the program
func Fatal(format string, args ...interface{}) {
	if currentLogLevel <= FATAL {
		Log(fatalLogger, "FATAL", colorRed, format, args...)
		os.Exit(1)
	}
}
