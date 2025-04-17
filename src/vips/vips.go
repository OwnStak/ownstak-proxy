package vips

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"ownstak-proxy/src/logger"

	"github.com/ebitengine/purego"
)

/**
 * READ BEFORE USE (it might prevent you from having a libvips headache)
 * This package is a libvips wrapper for golang that exposes the libvips low level C/C++ API to our go code.
 * The libvips is a image proccesing library that allows optimizations, such as resizing, rotating, cropping, and more.
 * Currently it is the most efficient library for this purpose that is also used by popular NodeJS library SHARP.
 * According to benchmarks, it is 4-8x faster than other libraries like imagemagick and way more efficient than native golang image/webp/png/jpeg libraries, cwebp, etc.
 *
 * The official bindings for golang are provided by the https://github.com/davidbyttow/govips package.
 * However, it uses CGO and requires to have all libvips.so including all .h files locally, including all the depencies such as pkg-config,libobject, libglib, libiconv, etc...
 * This makes cross platform compilations and local testing way more complicated.
 * For example just libvips-dev package for ubuntu has hundreds of other dev dependencies.
 *
 * This our own vips.go implementation is based on https://github.com/davidbyttow/govips but uses purego to dynamically load the correct library file
 * at runtime and then calls c/c++ functions directly. Unlike govips, it exposes only very little subset of functions/structures that we actually need
 * and you don't even need to have libvips-dev installed locally to run dev.sh script.
 *
 * This implementation is compatible with libvips 8.x.
 * See https://github.com/libvips/libvips/blob/v8.16.0/libvips/include/vips/vips.h for the full API.
 */

// Package level variables
var (
	libvips uintptr         // Handle to the libvips shared library
	debug   bool    = false // Debug mode flag
)

// Initialize debug mode from environment variables
func init() {
	// Enable debug output if DEBUG_VIPS is set
	if os.Getenv("DEBUG_VIPS") != "" {
		debug = true
		logger.Info("VIPS debug mode enabled")
	}
}

// Image represents a VIPS image object
type Image struct {
	ptr unsafe.Pointer
}

// VipsError represents an error from the libvips library
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("vips error: %s", e.Message)
}

// Initialize loads the libvips library and initializes it.
// This must be called before any other function in this package.
//
// Example:
//
//	if err := vips.Initialize(); err != nil {
//	    log.Fatalf("Failed to initialize VIPS: %v", err)
//	}
//	defer vips.Shutdown()
func Initialize() error {
	var err error

	// First try to load platform-specific libraries from ./lib
	osName := runtime.GOOS     // e.g., "linux", "darwin", "windows"
	archName := runtime.GOARCH // e.g., "amd64", "arm64"

	// Paths to try in order of preference
	var libPaths []string

	// Determine the directory of the current executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)

	// Add the executable directory to the list of paths to try first
	// This is to make the binary self-contained and avoid having to install libvips-dev locally.
	// 1. By default, we will try to load the libvips library from the executable directory.
	libPaths = append([]string{
		// Current working directory
		filepath.Join("lib", fmt.Sprintf("libvips-%s-%s.so", osName, archName)),
		filepath.Join("lib", fmt.Sprintf("libvips-%s-%s.dylib", osName, archName)),
		filepath.Join("lib", fmt.Sprintf("libvips-%s-%s.dll", osName, archName)),
		// Executable directory
		filepath.Join(execDir, "lib", fmt.Sprintf("libvips-%s-%s.so", osName, archName)),
		filepath.Join(execDir, "lib", fmt.Sprintf("libvips-%s-%s.dylib", osName, archName)),
		filepath.Join(execDir, "lib", fmt.Sprintf("libvips-%s-%s.dll", osName, archName)),
	}, libPaths...)

	// 2. Try standard platform-specific system paths as fallback
	switch osName {
	case "linux":
		libPaths = append(libPaths, "libvips.so.42", "libvips.so")
	case "darwin":
		libPaths = append(libPaths, "libvips.42.dylib", "libvips.dylib")
	case "windows":
		libPaths = append(libPaths, "libvips-42.dll", "libvips.dll")
	default:
		return fmt.Errorf("unsupported platform: %s/%s", osName, archName)
	}

	// Try each path in order
	var lastErr error
	for _, path := range libPaths {
		if debug {
			logger.Debug("Trying to load libvips from: %s", path)
		}

		libvips, err = purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			if debug {
				logger.Debug("Successfully loaded libvips from: %s", path)
			}
			break
		}
		if debug {
			logger.Debug("Failed to load libvips from: %s", path)
		}
	}

	// If all paths failed
	if lastErr != nil {
		return fmt.Errorf("failed to load libvips. Please ensure libvips is either in ./lib or installed on your system")
	}

	// Initialize VIPS
	initVips, err := purego.Dlsym(libvips, "vips_init")
	if err != nil {
		return fmt.Errorf("failed to find vips_init: %v", err)
	}

	// Create a function pointer with the correct signature
	var initFunc func(string) int
	purego.RegisterFunc(&initFunc, initVips)

	if initFunc("") != 0 {
		return getError()
	}

	if debug {
		logger.Info("VIPS initialized successfully")
	}

	return nil
}

// Shutdown cleans up VIPS resources.
// This should be called when you're done using VIPS, typically with defer.
func Shutdown() {
	if libvips != 0 {
		shutdownVips, _ := purego.Dlsym(libvips, "vips_shutdown")
		var shutdownFunc func()
		purego.RegisterFunc(&shutdownFunc, shutdownVips)
		shutdownFunc()

		if debug {
			logger.Info("VIPS shutdown completed")
		}
	}
}

// LoadImage loads an image from memory (byte slice).
//
// Example:
//
//	data, err := ioutil.ReadFile("image.jpg")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	img, err := vips.LoadImage(data)
//	if err != nil {
//	    log.Fatalf("Failed to load image: %v", err)
//	}
func LoadImage(data []byte) (*Image, error) {
	if data == nil || len(data) == 0 {
		return nil, fmt.Errorf("invalid empty image data")
	}

	// Clear any previous errors
	clearVipsError()

	loadImage, err := purego.Dlsym(libvips, "vips_image_new_from_buffer")
	if err != nil {
		return nil, fmt.Errorf("failed to find vips_image_new_from_buffer: %v", err)
	}

	var loadFunc func(unsafe.Pointer, int, string, unsafe.Pointer) unsafe.Pointer
	purego.RegisterFunc(&loadFunc, loadImage)

	ptr := loadFunc(unsafe.Pointer(&data[0]), len(data), "", nil)
	if ptr == nil {
		return nil, getError()
	}

	return &Image{ptr: ptr}, nil
}

// ResizeImage resizes an image with the given scale factor.
// A scale of 1.0 represents original size, 0.5 is half size, 2.0 is double size.
//
// Example:
//
//	// Resize to 50% of original size
//	resizedImg, err := vips.ResizeImage(img, 0.5)
//	if err != nil {
//	    log.Fatalf("Failed to resize image: %v", err)
//	}
func ResizeImage(img *Image, scale float64) (*Image, error) {
	if img == nil || img.ptr == nil {
		return nil, fmt.Errorf("invalid image pointer")
	}

	if scale <= 0 {
		return nil, fmt.Errorf("invalid scale factor: %f (must be positive)", scale)
	}

	// Clear any previous errors
	clearVipsError()

	resizeImage, err := purego.Dlsym(libvips, "vips_resize")
	if err != nil {
		return nil, fmt.Errorf("failed to find vips_resize: %v", err)
	}

	// int vips_resize(VipsImage *in, VipsImage **out, double scale, ...);
	var resizeFunc func(unsafe.Pointer, unsafe.Pointer, float64, unsafe.Pointer) int
	purego.RegisterFunc(&resizeFunc, resizeImage)

	var out unsafe.Pointer
	result := resizeFunc(img.ptr, unsafe.Pointer(&out), scale, nil)
	if result != 0 || out == nil {
		return nil, getError()
	}

	return &Image{ptr: out}, nil
}

// GetImageWidth returns the width of the image in pixels.
func GetImageWidth(img *Image) int {
	if img == nil || img.ptr == nil {
		return 0
	}

	getWidth, err := purego.Dlsym(libvips, "vips_image_get_width")
	if err != nil {
		if debug {
			logger.Error("Error finding vips_image_get_width: %v", err)
		}
		return 0
	}

	var getWidthFunc func(unsafe.Pointer) int
	purego.RegisterFunc(&getWidthFunc, getWidth)

	width := getWidthFunc(img.ptr)
	if width <= 0 && debug {
		logger.Warn("Invalid image width: %d", width)
	}

	return width
}

// GetImageHeight returns the height of the image in pixels.
func GetImageHeight(img *Image) int {
	if img == nil || img.ptr == nil {
		return 0
	}

	getHeight, err := purego.Dlsym(libvips, "vips_image_get_height")
	if err != nil {
		if debug {
			logger.Error("Error finding vips_image_get_height: %v", err)
		}
		return 0
	}

	var getHeightFunc func(unsafe.Pointer) int
	purego.RegisterFunc(&getHeightFunc, getHeight)

	height := getHeightFunc(img.ptr)
	if height <= 0 && debug {
		logger.Warn("Invalid image height: %d", height)
	}

	return height
}

// SaveImage saves an image in the specified format with the given quality.
// Supported formats: jpeg/jpg, png, webp, avif, gif
// Quality (1-100) applies to lossy formats like jpeg, webp, and avif.
//
// Example:
//
//	// Save as JPEG with 80% quality
//	jpegData, err := vips.SaveImage(img, "jpeg", 80)
//	if err != nil {
//	    log.Fatalf("Failed to save as JPEG: %v", err)
//	}
//
//	// Save as WebP with 90% quality
//	webpData, err := vips.SaveImage(img, "webp", 90)
func SaveImage(img *Image, format string, quality int) ([]byte, error) {
	if img == nil || img.ptr == nil {
		return nil, fmt.Errorf("invalid image pointer")
	}

	// Default quality if not specified
	if quality <= 0 {
		quality = 80 // Default quality
	}

	// Ensure quality is within reasonable limits
	if quality > 100 {
		quality = 100
	}

	// Clear any previous errors
	clearVipsError()

	// Get the g_free function for memory cleanup
	freeFunc := getMemoryFreeFunction()
	if freeFunc == nil {
		return nil, fmt.Errorf("failed to find memory free function")
	}

	// Normalize format (lowercase and remove dot if present)
	format = strings.ToLower(strings.TrimPrefix(format, "."))

	// Variables for save function result
	var ptr unsafe.Pointer
	var size int
	var result int

	// Format-specific save functions
	switch format {
	case "jpeg", "jpg":
		result, ptr, size = saveJpeg(img, quality)
	case "png":
		result, ptr, size = savePng(img, quality)
	case "webp":
		result, ptr, size = saveWebp(img, quality)
	case "avif":
		result, ptr, size = saveAvif(img, quality)
	case "gif":
		result, ptr, size = saveGif(img)
	}

	// If format-specific functions worked, return the result
	if result == 0 && ptr != nil && size > 0 {
		// Copy the data before freeing the pointer
		data := make([]byte, size)
		copy(data, (*[1 << 30]byte)(ptr)[:size])

		// Free the memory allocated by libvips
		freeFunc(ptr)

		return data, nil
	}

	// Try the generic function as a fallback
	return saveGeneric(img, format, freeFunc)
}

// Internal utility functions for SaveImage

// saveJpeg saves an image in JPEG format
func saveJpeg(img *Image, quality int) (int, unsafe.Pointer, int) {
	jpegSave, err := purego.Dlsym(libvips, "vips_jpegsave_buffer")
	if err != nil {
		return -1, nil, 0
	}

	var jpegSaveFunc func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, string, int, string, int, unsafe.Pointer) int
	purego.RegisterFunc(&jpegSaveFunc, jpegSave)

	var ptr unsafe.Pointer
	var size int

	// Set Q parameter for JPEG quality (1-100) and optimize for progressive display
	result := jpegSaveFunc(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"Q", quality,
		"optimize_coding", 1,
		nil)

	if debug {
		logger.Debug("JPEG save result=%d, ptr=%v, size=%d, quality=%d",
			result, ptr, size, quality)
	}

	return result, ptr, size
}

// savePng saves an image in PNG format
func savePng(img *Image, quality int) (int, unsafe.Pointer, int) {
	pngSave, err := purego.Dlsym(libvips, "vips_pngsave_buffer")
	if err != nil {
		return -1, nil, 0
	}

	var pngSaveFunc func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, string, int, string, int, string, int, unsafe.Pointer) int
	purego.RegisterFunc(&pngSaveFunc, pngSave)

	var ptr unsafe.Pointer
	var size int

	// Convert quality (1-100) to compression level (9-0)
	// PNG compression is inverted: 9 is max compression (lowest quality), 0 is no compression (highest quality)
	compressionLevel := 9 - (quality / 11)
	if compressionLevel < 0 {
		compressionLevel = 0
	} else if compressionLevel > 9 {
		compressionLevel = 9
	}

	result := pngSaveFunc(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"compression", compressionLevel,
		"interlace", 0,
		"filter", 0, // No filter
		nil)

	if debug {
		logger.Debug("PNG save result=%d, ptr=%v, size=%d, compression=%d",
			result, ptr, size, compressionLevel)
	}

	return result, ptr, size
}

// saveWebp saves an image in WebP format
func saveWebp(img *Image, quality int) (int, unsafe.Pointer, int) {
	webpSave, err := purego.Dlsym(libvips, "vips_webpsave_buffer")
	if err != nil {
		return -1, nil, 0
	}

	var webpSaveFunc func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer,
		string, int, string, int, unsafe.Pointer) int
	purego.RegisterFunc(&webpSaveFunc, webpSave)

	var ptr unsafe.Pointer
	var size int

	// WebP uses the same quality scale as JPEG (1-100)
	result := webpSaveFunc(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"Q", quality,
		"lossy", 0, // Use lossy compression
		nil)

	if debug {
		logger.Debug("WebP save result=%d, ptr=%v, size=%d, quality=%d",
			result, ptr, size, quality)
	}

	return result, ptr, size
}

// saveAvif saves an image in AVIF format
func saveAvif(img *Image, quality int) (int, unsafe.Pointer, int) {
	avifSave, err := purego.Dlsym(libvips, "vips_heifsave_buffer")
	if err != nil {
		return -1, nil, 0
	}

	var avifSaveFunc func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer,
		string, int, string, int, string, int, unsafe.Pointer) int
	purego.RegisterFunc(&avifSaveFunc, avifSave)

	var ptr unsafe.Pointer
	var size int

	// AVIF uses the same quality scale as JPEG (1-100)
	result := avifSaveFunc(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"Q", quality,
		"compression", 10, // HEVC compression for AVIF
		"effort", 4, // Medium encoding effort
		nil)

	if debug {
		logger.Debug("AVIF save result=%d, ptr=%v, size=%d, quality=%d",
			result, ptr, size, quality)
	}

	return result, ptr, size
}

// saveGif saves an image in GIF format
func saveGif(img *Image) (int, unsafe.Pointer, int) {
	gifSave, err := purego.Dlsym(libvips, "vips_gifsave_buffer")
	if err != nil {
		return -1, nil, 0
	}

	var gifSaveFunc func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int
	purego.RegisterFunc(&gifSaveFunc, gifSave)

	var ptr unsafe.Pointer
	var size int

	// GIF doesn't support quality parameter directly
	result := gifSaveFunc(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		nil)

	if debug {
		logger.Debug("GIF save result=%d, ptr=%v, size=%d",
			result, ptr, size)
	}

	return result, ptr, size
}

// saveGeneric tries to save using the generic buffer save function
func saveGeneric(img *Image, format string, freeFunc func(unsafe.Pointer)) ([]byte, error) {
	saveImage, err := purego.Dlsym(libvips, "vips_image_write_to_buffer")
	if err != nil {
		return nil, fmt.Errorf("failed to find vips_image_write_to_buffer: %v", err)
	}

	var saveFunc func(unsafe.Pointer, string, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int
	purego.RegisterFunc(&saveFunc, saveImage)

	// Clear previous errors before trying
	clearVipsError()

	// Use generic save function with format suffix
	var ptr unsafe.Pointer
	var size int
	result := saveFunc(img.ptr, "."+format, unsafe.Pointer(&ptr), unsafe.Pointer(&size), nil)

	if result != 0 || ptr == nil || size == 0 {
		message := getErrorMessage()
		if message != "" {
			return nil, &Error{Message: message}
		}
		return nil, fmt.Errorf("failed to save image in %s format (no error message from libvips)", format)
	}

	// Copy the data before freeing the pointer
	data := make([]byte, size)
	copy(data, (*[1 << 30]byte)(ptr)[:size])

	// Free the memory allocated by libvips
	freeFunc(ptr)

	return data, nil
}

// Helper functions for error handling and memory management

// getMemoryFreeFunction gets the g_free function for memory cleanup
func getMemoryFreeFunction() func(unsafe.Pointer) {
	freePointer, err := purego.Dlsym(libvips, "g_free")
	if err != nil {
		if debug {
			logger.Error("Failed to find g_free: %v", err)
		}
		return nil
	}

	var freeFunc func(unsafe.Pointer)
	purego.RegisterFunc(&freeFunc, freePointer)
	return freeFunc
}

// getErrorMessage gets the last error message from VIPS
func getErrorMessage() string {
	errorBuffer, err := purego.Dlsym(libvips, "vips_error_buffer")
	if err != nil {
		return ""
	}

	var errorBufferFunc func() string
	purego.RegisterFunc(&errorBufferFunc, errorBuffer)
	return errorBufferFunc()
}

// clearVipsError clears the VIPS error buffer
func clearVipsError() {
	clearError, err := purego.Dlsym(libvips, "vips_error_clear")
	if err != nil {
		return
	}

	var clearErrorFunc func()
	purego.RegisterFunc(&clearErrorFunc, clearError)
	clearErrorFunc()
}

// getError gets the error from VIPS and wraps it in our Error type
func getError() error {
	message := getErrorMessage()

	// Clear the error after reading it
	clearVipsError()

	if message == "" {
		return fmt.Errorf("unknown vips error (error buffer is empty)")
	}

	return &Error{Message: message}
}
