package logger

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	t.Run("Debug", func(t *testing.T) {
		SetLogLevel("debug")
		stdout, _ := captureOutput(func() {
			Debug("debug message")
		})
		assert.Contains(t, stdout, "debug message")
	})

	t.Run("Info", func(t *testing.T) {
		SetLogLevel("info")
		stdout, _ := captureOutput(func() {
			Info("info message")
		})
		assert.Contains(t, stdout, "info message")
	})

	t.Run("Warn", func(t *testing.T) {
		SetLogLevel("warn")
		stdout, _ := captureOutput(func() {
			Warn("warning message")
		})
		assert.Contains(t, stdout, "warning message")
	})

	t.Run("Error", func(t *testing.T) {
		SetLogLevel("error")
		_, stderr := captureOutput(func() {
			Error("error message")
		})
		assert.Contains(t, stderr, "error message")
	})

	t.Run("setLogLevel visibility", func(t *testing.T) {
		SetLogLevel("info")
		stdout, _ := captureOutput(func() {
			Debug("debug message")
		})
		assert.NotContains(t, stdout, "debug message")

		stdout, _ = captureOutput(func() {
			Info("info message")
		})
		assert.Contains(t, stdout, "info message")
	})
}

func captureOutput(f func()) (stdout, stderr string) {
	// Create buffers to capture output
	var bufOut, bufErr bytes.Buffer

	// Save current output writers
	oldStdout := currentStdout
	oldStderr := currentStderr

	// Set new output writers
	SetOutput(&bufOut, &bufErr)

	// Run the function
	f()

	// Restore original output writers
	SetOutput(oldStdout, oldStderr)

	return bufOut.String(), bufErr.String()
}
