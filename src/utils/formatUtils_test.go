package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatBytes(t *testing.T) {
	t.Run("zero bytes", func(t *testing.T) {
		result := FormatBytes(0)
		assert.Equal(t, "0 B", result)
	})

	t.Run("less than 1KB", func(t *testing.T) {
		result := FormatBytes(500)
		assert.Equal(t, "500 B", result)
	})

	t.Run("exactly 1KB", func(t *testing.T) {
		result := FormatBytes(1024)
		assert.Equal(t, "1.0 KB", result)
	})

	t.Run("less than 1MB", func(t *testing.T) {
		result := FormatBytes(1024 * 500)
		assert.Equal(t, "500.0 KB", result)
	})

	t.Run("exactly 1MB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024)
		assert.Equal(t, "1.0 MB", result)
	})

	t.Run("less than 1GB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 500)
		assert.Equal(t, "500.0 MB", result)
	})

	t.Run("exactly 1GB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024)
		assert.Equal(t, "1.0 GB", result)
	})

	t.Run("less than 1TB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 500)
		assert.Equal(t, "500.0 GB", result)
	})

	t.Run("exactly 1TB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 1024)
		assert.Equal(t, "1.0 TB", result)
	})

	t.Run("less than 1PB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 1024 * 500)
		assert.Equal(t, "500.0 TB", result)
	})

	t.Run("exactly 1PB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 1024 * 1024)
		assert.Equal(t, "1.0 PB", result)
	})

	t.Run("less than 1EB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 1024 * 1024 * 500)
		assert.Equal(t, "500.0 PB", result)
	})

	t.Run("exactly 1EB", func(t *testing.T) {
		result := FormatBytes(1024 * 1024 * 1024 * 1024 * 1024 * 1024)
		assert.Equal(t, "1.0 EB", result)
	})
}
