package vips

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to initialize VIPS for tests
func setupVips(t *testing.T) func() {
	// Get current working directory and project structure
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Calculate project root (go up from src/vips to project root)
	projectRoot := filepath.Dir(filepath.Dir(wd))

	// Change working directory to project root so lib/ path resolves correctly
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(projectRoot)
	require.NoError(t, err)

	// Initialize VIPS
	err = Initialize()
	if err != nil {
		t.Skipf("VIPS not available: %v", err)
	}

	// Return cleanup function
	return func() {
		Shutdown()
		os.Chdir(originalWd) // Restore original working directory
	}
}

// Test image data for various formats
var (
	// Minimal valid JPEG header
	jpegData = []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xD9,
	}

	// Minimal valid PNG header
	pngData = []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00,
	}

	// WebP header
	webpData = []byte{
		'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P',
	}

	// GIF header - corrected to match actual GIF87a format
	gifData = []byte{
		0x47, 0x49, 0x46, 0x38, 0x37, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
)

func TestVipsInitialization(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should initialize successfully", func(t *testing.T) {
		// If we got here, VIPS initialized successfully (setupVips would have skipped otherwise)
		assert.NotNil(t, libvips, "libvips should be loaded")
	})

	t.Run("should handle environment variables", func(t *testing.T) {
		// Check that MALLOC_ARENA_MAX was set
		value := os.Getenv("MALLOC_ARENA_MAX")
		assert.Equal(t, "2", value)
	})
}

func TestImageFormatDetection(t *testing.T) {
	t.Run("should detect JPEG format", func(t *testing.T) {
		format := GetImageFormat(jpegData)
		assert.Equal(t, JPEG, format)
	})

	t.Run("should detect PNG format", func(t *testing.T) {
		format := GetImageFormat(pngData)
		assert.Equal(t, PNG, format)
	})

	t.Run("should detect WebP format", func(t *testing.T) {
		format := GetImageFormat(webpData)
		assert.Equal(t, WEBP, format)
	})

	t.Run("should detect GIF format", func(t *testing.T) {
		format := GetImageFormat(gifData)
		assert.Equal(t, GIF, format)
	})

	t.Run("should return UNKNOWN for invalid data", func(t *testing.T) {
		format := GetImageFormat([]byte{0x00, 0x01, 0x02})
		assert.Equal(t, UNKNOWN, format)
	})

	t.Run("should handle empty data", func(t *testing.T) {
		format := GetImageFormat([]byte{})
		assert.Equal(t, UNKNOWN, format)
	})
}

func TestStringToFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected ImageFormat
	}{
		// StringToFormat is case-sensitive and only handles lowercase
		{"jpeg", JPEG},
		{"jpg", JPEG},
		{"png", PNG},
		{"webp", WEBP},
		{"gif", GIF},
		{"tiff", TIFF},
		{"unknown", UNKNOWN},
		{"", UNKNOWN},
		// Test that it's case-sensitive (uppercase should return UNKNOWN)
		{"JPEG", UNKNOWN},
		{"JPG", UNKNOWN},
		{"PNG", UNKNOWN},
		{"WEBP", UNKNOWN},
		{"GIF", UNKNOWN},
		{"TIFF", UNKNOWN},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := StringToFormat(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestStringToFormatCaseInsensitive(t *testing.T) {
	// Test a case-insensitive wrapper if needed
	stringToFormatInsensitive := func(format string) ImageFormat {
		return StringToFormat(strings.ToLower(format))
	}

	tests := []struct {
		input    string
		expected ImageFormat
	}{
		{"JPEG", JPEG},
		{"JPG", JPEG},
		{"PNG", PNG},
		{"WEBP", WEBP},
		{"GIF", GIF},
		{"TIFF", TIFF},
		{"jpeg", JPEG},
		{"jpg", JPEG},
		{"png", PNG},
		{"webp", WEBP},
		{"gif", GIF},
		{"tiff", TIFF},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := stringToFormatInsensitive(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestImageOperations(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	// Test with a real image file if available
	testImagePath := "src/vips/vips_test.webp"
	if _, err := os.Stat(testImagePath); os.IsNotExist(err) {
		t.Skipf("Test image not found, skipping image operations tests")
		return
	}

	t.Run("should load image from file", func(t *testing.T) {
		img, err := LoadImageFromFile(testImagePath)
		assert.NoError(t, err, "should load image from file without error")
		defer img.Free()

		assert.NotNil(t, img)
		assert.NotNil(t, img.ptr)

		// Check dimensions
		width := GetImageWidth(img)
		height := GetImageHeight(img)
		assert.Greater(t, width, 0)
		assert.Greater(t, height, 0)
		t.Logf("Image dimensions: %dx%d", width, height)
	})

	t.Run("should save and load image buffer", func(t *testing.T) {
		img, err := LoadImageFromFile(testImagePath)
		assert.NoError(t, err, "should load image from file without error")
		defer img.Free()

		// Save to buffer
		buffer, err := SaveImageToBuffer(img, "webp", 80)
		assert.NoError(t, err, "should save image to buffer without error")
		assert.Greater(t, len(buffer), 0)

		// Load from buffer
		img2, err := LoadImageFromBuffer(buffer)
		assert.NoError(t, err, "should load image from buffer without error")
		defer img2.Free()

		assert.NotNil(t, img2)
		assert.NotNil(t, img2.ptr)
	})

	t.Run("should resize image", func(t *testing.T) {
		img, err := LoadImageFromFile(testImagePath)
		assert.NoError(t, err, "should load image from file without error")
		defer img.Free()

		originalWidth := GetImageWidth(img)
		originalHeight := GetImageHeight(img)

		// Resize to 50%
		resized, err := ResizeImage(img, 0.5)
		assert.NoError(t, err, "should resize image without error")
		defer resized.Free()

		newWidth := GetImageWidth(resized)
		newHeight := GetImageHeight(resized)

		// Check that dimensions are approximately halved
		assert.InDelta(t, float64(originalWidth)/2, float64(newWidth), 2)
		assert.InDelta(t, float64(originalHeight)/2, float64(newHeight), 2)
		t.Logf("Original: %dx%d, Resized: %dx%d", originalWidth, originalHeight, newWidth, newHeight)
	})

	t.Run("should handle invalid file path", func(t *testing.T) {
		_, err := LoadImageFromFile("nonexistent.jpg")
		assert.Error(t, err, "should handle invalid file path")
	})

	t.Run("should handle invalid buffer data", func(t *testing.T) {
		_, err := LoadImageFromBuffer([]byte{0x00, 0x01, 0x02})
		assert.Error(t, err, "should handle invalid buffer data")
	})

	t.Run("should save image to file", func(t *testing.T) {
		img, err := LoadImageFromFile(testImagePath)
		assert.NoError(t, err, "should load image from file without error")
		defer img.Free()

		// Test saving as JPEG
		jpegPath := "test_output.jpg"
		defer os.Remove(jpegPath) // Clean up

		err = SaveImageToFile(img, jpegPath, 80)
		assert.NoError(t, err, "should save image as JPEG without error")

		// Verify file was created and has content
		info, err := os.Stat(jpegPath)
		assert.NoError(t, err, "saved JPEG file should exist")
		assert.Greater(t, info.Size(), int64(0), "saved JPEG file should have content")

		// Test saving as WebP
		webpPath := "test_output.webp"
		defer os.Remove(webpPath) // Clean up

		err = SaveImageToFile(img, webpPath, 90)
		assert.NoError(t, err, "should save image as WebP without error")

		// Verify file was created and has content
		info, err = os.Stat(webpPath)
		assert.NoError(t, err, "saved WebP file should exist")
		assert.Greater(t, info.Size(), int64(0), "saved WebP file should have content")
	})
}

func TestMemoryManagement(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should provide memory stats", func(t *testing.T) {
		var stats Stats
		err := ReadVipsMemStats(&stats)
		assert.NoError(t, err)

		assert.NotEmpty(t, stats.Version)
		assert.GreaterOrEqual(t, stats.Mem, int64(0))
		assert.GreaterOrEqual(t, stats.MemHigh, int64(0))
		assert.GreaterOrEqual(t, stats.Allocs, int64(0))
		assert.GreaterOrEqual(t, stats.Files, int64(0))

		t.Logf("VIPS Stats: Version=%s, Mem=%d, Allocs=%d",
			stats.Version, stats.Mem, stats.Allocs)
	})

	t.Run("should manage memory operations", func(t *testing.T) {
		// Test memory operations don't crash
		assert.NotPanics(t, func() {
			MallocTrim()
			ThreadShutdown()
			CacheDrop()
		})
	})
}

func TestCacheOperations(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should provide cache information", func(t *testing.T) {
		concurrency := GetConcurrency()
		assert.Greater(t, concurrency, 0)

		maxCacheSize := GetMaxCacheSize()
		assert.GreaterOrEqual(t, maxCacheSize, int64(0))

		maxCacheMem := GetMaxCacheMem()
		assert.GreaterOrEqual(t, maxCacheMem, int64(0))

		maxCacheFiles := GetMaxCacheFiles()
		assert.GreaterOrEqual(t, maxCacheFiles, int64(0))

		t.Logf("Cache Info: Concurrency=%d, MaxSize=%d, MaxMem=%d, MaxFiles=%d",
			concurrency, maxCacheSize, maxCacheMem, maxCacheFiles)
	})
}

func TestErrorHandling(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should handle errors gracefully", func(t *testing.T) {
		// Try to load an invalid image
		_, err := LoadImageFromBuffer([]byte("invalid image data"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vips error")
	})
}

func TestImageFormats(t *testing.T) {
	t.Run("should have correct format mappings", func(t *testing.T) {
		assert.Equal(t, "jpeg", ImageFormats[JPEG])
		assert.Equal(t, "png", ImageFormats[PNG])
		assert.Equal(t, "webp", ImageFormats[WEBP])
		assert.Equal(t, "gif", ImageFormats[GIF])
		assert.Equal(t, "tiff", ImageFormats[TIFF])
		assert.Equal(t, "", ImageFormats[UNKNOWN])
	})
}

func TestUtilityFunctions(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should test Unref and UnrefOutputs", func(t *testing.T) {
		// These should not panic or crash with nil pointers
		assert.NotPanics(t, func() {
			Unref(nil)
			UnrefOutputs(nil)
		})
	})

	t.Run("should test Free function", func(t *testing.T) {
		// Free should not panic with nil pointer
		assert.NotPanics(t, func() {
			Free(nil)
		})
	})

	t.Run("should test MallocTrim", func(t *testing.T) {
		// MallocTrim should not panic
		assert.NotPanics(t, func() {
			MallocTrim()
		})
	})

	t.Run("should test LogObjectReport", func(t *testing.T) {
		// LogObjectReport should not panic
		assert.NotPanics(t, func() {
			LogObjectReport()
		})
	})

	t.Run("should test LogVipsStats", func(t *testing.T) {
		// LogVipsStats should not panic
		assert.NotPanics(t, func() {
			LogVipsStats()
		})
	})
}

func TestMoreImageFormats(t *testing.T) {
	t.Run("should detect TIFF format", func(t *testing.T) {
		// TIFF format starts with "II*\0" (little-endian) or "MM\0*" (big-endian)
		// Need at least 12 bytes for GetImageFormat to work
		tiffDataLE := make([]byte, 12)
		tiffDataLE[0] = 0x49 // I
		tiffDataLE[1] = 0x49 // I
		tiffDataLE[2] = 0x2A // *
		tiffDataLE[3] = 0x00 // \0
		format := GetImageFormat(tiffDataLE)
		assert.Equal(t, TIFF, format)

		tiffDataBE := make([]byte, 12)
		tiffDataBE[0] = 0x4D // M
		tiffDataBE[1] = 0x4D // M
		tiffDataBE[2] = 0x00 // \0
		tiffDataBE[3] = 0x2A // *
		format = GetImageFormat(tiffDataBE)
		assert.Equal(t, TIFF, format)
	})

	t.Run("should detect JXL format", func(t *testing.T) {
		// JXL format can start with various headers
		// Need at least 12 bytes for GetImageFormat to work
		jxlData1 := make([]byte, 12)
		jxlData1[0] = 0xFF // JXL codestream header
		jxlData1[1] = 0x0A
		format := GetImageFormat(jxlData1)
		assert.Equal(t, JXL, format)

		jxlData2 := []byte{0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A} // JXL container header
		format = GetImageFormat(jxlData2)
		assert.Equal(t, JXL, format)
	})

	t.Run("should handle very short buffers", func(t *testing.T) {
		// Test with buffers shorter than 12 bytes
		shortBuffer := []byte{0xFF}
		format := GetImageFormat(shortBuffer)
		assert.Equal(t, UNKNOWN, format)

		// Buffer with 11 bytes should still return UNKNOWN
		elevenBytes := make([]byte, 11)
		elevenBytes[0] = 0xFF
		elevenBytes[1] = 0xD8
		elevenBytes[2] = 0xFF
		format = GetImageFormat(elevenBytes)
		assert.Equal(t, UNKNOWN, format)
	})

	t.Run("should handle large buffers", func(t *testing.T) {
		// Test with larger buffer that contains valid header but more data
		largeJpegBuffer := make([]byte, 1000)
		largeJpegBuffer[0] = 0xFF
		largeJpegBuffer[1] = 0xD8
		largeJpegBuffer[2] = 0xFF
		format := GetImageFormat(largeJpegBuffer)
		assert.Equal(t, JPEG, format)
	})
}

func TestShutdownAndCleanup(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should handle ThreadShutdown", func(t *testing.T) {
		assert.NotPanics(t, func() {
			ThreadShutdown()
		})
	})

	t.Run("should handle CacheDrop", func(t *testing.T) {
		assert.NotPanics(t, func() {
			CacheDrop()
		})
	})

	t.Run("should handle Shutdown", func(t *testing.T) {
		// Note: We don't actually call Shutdown() here as it would
		// interfere with other tests. Just test that the function exists.
		assert.NotNil(t, Shutdown, "Shutdown function should exist")
	})
}

func TestAdditionalFormats(t *testing.T) {
	t.Run("should have all format constants defined", func(t *testing.T) {
		// Test that all format constants are properly defined
		assert.Equal(t, ImageFormat(""), UNKNOWN)
		assert.Equal(t, ImageFormat("jpeg"), JPEG)
		assert.Equal(t, ImageFormat("png"), PNG)
		assert.Equal(t, ImageFormat("webp"), WEBP)
		assert.Equal(t, ImageFormat("gif"), GIF)
		assert.Equal(t, ImageFormat("tiff"), TIFF)
		assert.Equal(t, ImageFormat("pdf"), PDF)
		assert.Equal(t, ImageFormat("svg"), SVG)
		assert.Equal(t, ImageFormat("heif"), HEIF)
		assert.Equal(t, ImageFormat("avif"), AVIF)
		assert.Equal(t, ImageFormat("jxl"), JXL)
	})

	t.Run("should have complete ImageFormats map", func(t *testing.T) {
		// Test that ImageFormats map contains all expected entries
		expectedFormats := map[ImageFormat]string{
			UNKNOWN: "",
			JPEG:    "jpeg",
			PNG:     "png",
			WEBP:    "webp",
			TIFF:    "tiff",
			GIF:     "gif",
			PDF:     "pdf",
			SVG:     "svg",
			HEIF:    "heif",
			AVIF:    "avif",
			JXL:     "jxl",
		}

		for format, expected := range expectedFormats {
			actual, exists := ImageFormats[format]
			assert.True(t, exists, "Format %s should exist in ImageFormats map", format)
			assert.Equal(t, expected, actual, "Format %s should map to %s", format, expected)
		}
	})
}

func TestErrorType(t *testing.T) {
	t.Run("should create and format error correctly", func(t *testing.T) {
		err := &Error{Message: "test error message"}
		assert.Equal(t, "vips error: test error message", err.Error())
	})

	t.Run("should handle empty error message", func(t *testing.T) {
		err := &Error{Message: ""}
		assert.Equal(t, "vips error: ", err.Error())
	})
}

func TestLoadImageFromFileEdgeCases(t *testing.T) {
	cleanup := setupVips(t)
	defer cleanup()

	t.Run("should handle invalid file paths", func(t *testing.T) {
		// Test with empty file path
		_, err := LoadImageFromFile("")
		assert.Error(t, err, "should error with empty file path")
		assert.Contains(t, err.Error(), "invalid empty file path", "error should mention empty file path")

		// Test with non-existent file
		_, err = LoadImageFromFile("non_existent_file.jpg")
		assert.Error(t, err, "should error with non-existent file")
		assert.Contains(t, err.Error(), "error accessing file", "error should mention file access error")

		// Test with directory instead of file
		_, err = LoadImageFromFile("src")
		assert.Error(t, err, "should error when path is directory")
		assert.Contains(t, err.Error(), "path is a directory", "error should mention directory")

		// Test with file without extension
		// Create a temporary file without extension
		tmpFile, err := os.CreateTemp("", "test_no_ext")
		assert.NoError(t, err, "should create temp file")
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		_, err = LoadImageFromFile(tmpFile.Name())
		assert.Error(t, err, "should error with file without extension")
		assert.Contains(t, err.Error(), "no extension", "error should mention no extension")
	})
}
