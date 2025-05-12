package vips

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/utils"

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
	libvips uintptr // Handle to the libvips shared library
	libc    uintptr // Handle to the libc shared library

	mutex       sync.Mutex
	errorMutex  sync.Mutex // New mutex for error buffer access
	initialized bool
	debug       bool = false // Debug mode flag

	// libvips functions
	vipsImageNewFromBuffer     func(unsafe.Pointer, int, string, unsafe.Pointer) unsafe.Pointer
	vipsImageWriteToBuffer     func(unsafe.Pointer, *byte, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int
	vipsImageNewFromFile       func(string, unsafe.Pointer) unsafe.Pointer
	vipsInit                   func(string) int
	vipsVersionString          func() string
	vipsTrackedGetMem          func() int64
	vipsTrackedGetMemHighwater func() int64
	vipsTrackedGetAllocs       func() int64
	vipsTrackedGetFiles        func() int64
	vipsCacheGetSize           func() int64
	vipsCacheGetMax            func() int64
	vipsConcurrencyGet         func() int
	vipsErrorBuffer            func() string
	vipsErrorClear             func()
	vipsObjectPrintAll         func()
	vipsObjectUnrefOutputs     func(unsafe.Pointer)
	vipsThreadShutdown         func()
	vipsCacheDropAll           func()
	vipsShutdown               func()
	vipsResize                 func(unsafe.Pointer, unsafe.Pointer, float64, unsafe.Pointer) int
	imageGetBlob               func(unsafe.Pointer, *byte, unsafe.Pointer) uintptr
	vipsCacheSetMax            func(int)
	vipsCacheSetMaxMem         func(int)
	vipsConcurrencySet         func(int)
	vipsLeakSet                func(int)
	vipsImageGetWidth          func(unsafe.Pointer) int
	vipsImageGetHeight         func(unsafe.Pointer) int
	vipsCacheGetMaxMem         func() int64
	vipsCacheGetMaxFiles       func() int64
	vipsJpegLoadBuffer         func(unsafe.Pointer, int, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsJpegSaveBuffer         func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, string, int, string, int, unsafe.Pointer) int
	vipsWebpLoadBuffer         func(unsafe.Pointer, int, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsWebpSaveBuffer         func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, string, int, string, int, unsafe.Pointer) int
	vipsGifLoadBuffer          func(unsafe.Pointer, int, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsGifSaveBuffer          func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int
	vipsJpegLoad               func(*byte, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsJpegSave               func(unsafe.Pointer, *byte, *byte, int, *byte, int, unsafe.Pointer) int
	vipsWebpLoad               func(*byte, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsWebpSave               func(unsafe.Pointer, *byte, *byte, int, *byte, int, unsafe.Pointer) int
	vipsGifLoad                func(*byte, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsGifSave                func(unsafe.Pointer, *byte, *byte, int, *byte, int, unsafe.Pointer) int
	vipsTiffLoad               func(*byte, unsafe.Pointer, *byte, int, unsafe.Pointer) int
	vipsTiffSave               func(unsafe.Pointer, *byte, *byte, int, *byte, int, unsafe.Pointer) int

	// libglib functions
	gFree        func(ptr unsafe.Pointer)
	gObjectUnref func(ptr unsafe.Pointer)
	mallocTrim   func() int
)

const (
	VIPS_ACCESS_RANDOM = 1
)

// Image represents a VIPS image object
type VipsImage struct {
	ptr         unsafe.Pointer
	ImageFormat ImageFormat
}

// VipsError represents an error from the libvips library
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("vips error: %s", e.Message)
}

type Stats struct {
	Version     string // VIPS version
	Mem         int64  // Current allocated memory in bytes
	MemHigh     int64  // High water mark of allocated memory
	Allocs      int64  // Number of active allocations
	Files       int64  // Number of open files
	CacheSize   int64  // Current cache size in bytes
	CacheMax    int64  // Maximum cache size in bytes
	Concurrency int    // Number of concurrent operations
}

// ImageFormat represents an image type value.
type ImageFormat string

const (
	// UNKNOWN represents an unknow image type value.
	UNKNOWN ImageFormat = ""
	// JPEG represents the JPEG image type.
	JPEG ImageFormat = "jpeg"
	// WEBP represents the WEBP image type.
	WEBP ImageFormat = "webp"
	// PNG represents the PNG image type.
	PNG ImageFormat = "png"
	// TIFF represents the TIFF image type.
	TIFF ImageFormat = "tiff"
	// GIF represents the GIF image type.
	GIF ImageFormat = "gif"
	// PDF represents the PDF type.
	PDF ImageFormat = "pdf"
	// SVG represents the SVG image type.
	SVG ImageFormat = "svg"
	// HEIF represents the HEIC/HEIF/HVEC image type
	HEIF ImageFormat = "heif"
	// AVIF represents the AVIF image type.
	AVIF ImageFormat = "avif"
	// JXL represents the JPEG XL image type.
	JXL ImageFormat = "jxl"
)

// ImageFormats stores as pairs of image types supported and its alias names.
var ImageFormats = map[ImageFormat]string{
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

// Initialize loads the libvips library and initializes it.
// This must be called before any other function in this package.
//
// Example:
//
//	if err := vips.Initialize(); err != nil {
//	    log.Fatalf("Failed to initialize VIPS: %v", err)
//	}
func Initialize() error {
	mutex.Lock()
	runtime.LockOSThread()
	defer mutex.Unlock()
	defer runtime.UnlockOSThread()

	if initialized {
		return nil
	}

	var err error
	// Initialize debug mode from environment variables
	debug = os.Getenv(constants.EnvVipsDebug) == "true"
	if debug {
		logger.Info("VIPS debug mode enabled")
	}

	// Set default MALLOC_ARENA_MAX before loading libvips
	if os.Getenv(constants.EnvMallocArenaMax) == "" {
		os.Setenv(constants.EnvMallocArenaMax, "2")
	}

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

	// Register all libvips functions
	purego.RegisterLibFunc(&vipsInit, libvips, "vips_init")
	purego.RegisterLibFunc(&vipsImageNewFromBuffer, libvips, "vips_image_new_from_buffer")
	purego.RegisterLibFunc(&vipsImageWriteToBuffer, libvips, "vips_image_write_to_buffer")
	purego.RegisterLibFunc(&vipsVersionString, libvips, "vips_version_string")
	purego.RegisterLibFunc(&vipsTrackedGetMem, libvips, "vips_tracked_get_mem")
	purego.RegisterLibFunc(&vipsTrackedGetMemHighwater, libvips, "vips_tracked_get_mem_highwater")
	purego.RegisterLibFunc(&vipsTrackedGetAllocs, libvips, "vips_tracked_get_allocs")
	purego.RegisterLibFunc(&vipsTrackedGetFiles, libvips, "vips_tracked_get_files")
	purego.RegisterLibFunc(&vipsCacheGetSize, libvips, "vips_cache_get_size")
	purego.RegisterLibFunc(&vipsCacheGetMax, libvips, "vips_cache_get_max")
	purego.RegisterLibFunc(&vipsConcurrencyGet, libvips, "vips_concurrency_get")
	purego.RegisterLibFunc(&vipsErrorBuffer, libvips, "vips_error_buffer")
	purego.RegisterLibFunc(&vipsErrorClear, libvips, "vips_error_clear")
	purego.RegisterLibFunc(&vipsObjectPrintAll, libvips, "vips_object_print_all")
	purego.RegisterLibFunc(&vipsThreadShutdown, libvips, "vips_thread_shutdown")
	purego.RegisterLibFunc(&vipsCacheDropAll, libvips, "vips_cache_drop_all")
	purego.RegisterLibFunc(&vipsShutdown, libvips, "vips_shutdown")
	purego.RegisterLibFunc(&vipsResize, libvips, "vips_resize")
	purego.RegisterLibFunc(&vipsObjectUnrefOutputs, libvips, "vips_object_unref_outputs")
	purego.RegisterLibFunc(&imageGetBlob, libvips, "vips_image_get_blob")
	purego.RegisterLibFunc(&vipsCacheSetMax, libvips, "vips_cache_set_max")
	purego.RegisterLibFunc(&vipsCacheSetMaxMem, libvips, "vips_cache_set_max_mem")
	purego.RegisterLibFunc(&vipsConcurrencySet, libvips, "vips_concurrency_set")
	purego.RegisterLibFunc(&vipsJpegLoadBuffer, libvips, "vips_jpegload_buffer")
	purego.RegisterLibFunc(&vipsJpegSaveBuffer, libvips, "vips_jpegsave_buffer")
	purego.RegisterLibFunc(&vipsWebpLoadBuffer, libvips, "vips_webpload_buffer")
	purego.RegisterLibFunc(&vipsWebpSaveBuffer, libvips, "vips_webpsave_buffer")
	purego.RegisterLibFunc(&vipsGifLoadBuffer, libvips, "vips_gifload_buffer")
	purego.RegisterLibFunc(&vipsGifSaveBuffer, libvips, "vips_gifsave_buffer")
	purego.RegisterLibFunc(&vipsJpegLoad, libvips, "vips_jpegload")
	purego.RegisterLibFunc(&vipsJpegSave, libvips, "vips_jpegsave")
	purego.RegisterLibFunc(&vipsWebpLoad, libvips, "vips_webpload")
	purego.RegisterLibFunc(&vipsWebpSave, libvips, "vips_webpsave")
	purego.RegisterLibFunc(&vipsGifLoad, libvips, "vips_gifload")
	purego.RegisterLibFunc(&vipsGifSave, libvips, "vips_gifsave")
	purego.RegisterLibFunc(&vipsTiffLoad, libvips, "vips_tiffload")
	purego.RegisterLibFunc(&vipsTiffSave, libvips, "vips_tiffsave")
	purego.RegisterLibFunc(&vipsLeakSet, libvips, "vips_leak_set")
	purego.RegisterLibFunc(&vipsImageGetWidth, libvips, "vips_image_get_width")
	purego.RegisterLibFunc(&vipsImageGetHeight, libvips, "vips_image_get_height")
	purego.RegisterLibFunc(&vipsImageNewFromFile, libvips, "vips_image_new_from_file")
	purego.RegisterLibFunc(&vipsCacheGetMaxMem, libvips, "vips_cache_get_max_mem")
	purego.RegisterLibFunc(&vipsCacheGetMaxFiles, libvips, "vips_cache_get_max_files")

	// Register libglib functions
	purego.RegisterLibFunc(&gFree, libvips, "g_free")
	purego.RegisterLibFunc(&gObjectUnref, libvips, "g_object_unref")

	// Initialize libvips
	initCode := vipsInit("vips2")
	if initCode != 0 {
		return fmt.Errorf("failed to initialize VIPS: %d", initCode)
	}

	// Set concurrency to half of the available CPUs by default
	cpus := runtime.NumCPU()
	concurrency := cpus / 2
	if concurrency < 1 {
		concurrency = 1
	}
	if os.Getenv("VIPS_CONCURRENCY") != "" {
		envConcurrency, err := strconv.Atoi(os.Getenv("VIPS_CONCURRENCY"))
		if err != nil {
			return fmt.Errorf("failed to parse VIPS_CONCURRENCY: %v", err)
		}
		if envConcurrency > 0 {
			concurrency = envConcurrency
		}
	}
	vipsConcurrencySet(concurrency)

	maxCacheSize := 0 // disable operations cache
	if os.Getenv(constants.EnvVipsMaxCacheSize) != "" {
		envMaxCacheSize, err := strconv.Atoi(os.Getenv(constants.EnvVipsMaxCacheSize))
		if err != nil {
			return fmt.Errorf("failed to parse VIPS_MAX_CACHE_SIZE: %v", err)
		}
		maxCacheSize = envMaxCacheSize
	}
	vipsCacheSetMax(maxCacheSize)

	maxCacheMem := 0 // disable memory cache
	if os.Getenv(constants.EnvVipsMaxCacheMem) != "" {
		envMaxCacheMem, err := strconv.Atoi(os.Getenv(constants.EnvVipsMaxCacheMem))
		if err != nil {
			return fmt.Errorf("failed to parse VIPS_MAX_CACHE_MEM: %v", err)
		}
		maxCacheMem = envMaxCacheMem
	}
	vipsCacheSetMaxMem(maxCacheMem)

	leak := 0 // do not try to trace leaks by default, as it will slow down the library
	if os.Getenv(constants.EnvVipsLeak) != "" {
		envLeak, err := strconv.Atoi(os.Getenv(constants.EnvVipsLeak))
		if err != nil {
			return fmt.Errorf("failed to parse VIPS_LEAK: %v", err)
		}
		leak = envLeak
	}
	vipsLeakSet(leak)

	// Get info about the libvips
	vipsVersionStr := vipsVersionString()
	vipsConcurrency := vipsConcurrencyGet()

	maxCacheMemHuman := utils.FormatBytes(uint64(maxCacheMem))
	logger.Info("VIPS %s initialized successfully (concurrency: %d, max cache size: %d, max cache mem: %s)", vipsVersionStr, vipsConcurrency, maxCacheSize, maxCacheMemHuman)
	initialized = true

	// Import libc. This is needed for malloc_trim to work on all platforms.
	// This is a workaround to ensure malloc_trim is available on all platforms including minimal alpine distros.
	libcPaths := []string{
		"libgcompat.so.0",
		"libgcompat.so.0",
		"libc.musl-x86_64.so.1",
		"libc.musl-aarch64.so.1",
		"libc.so.6",
		"libc.so",
		"libSystem.B.dylib",
	}

	for _, path := range libcPaths {
		logger.Debug("Trying to load libc from: %s", path)
		libc, err := purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			continue
		}
		logger.Debug("Successfully loaded libc from: %s", path)
		purego.RegisterLibFunc(&mallocTrim, libc, "malloc_trim")
		return nil
	}

	// if malloc_trim is not found (darwin platforms),
	// just use a stub implementation
	logger.Warn("The malloc_trim func was not found in any libc implementation. The VIPS memory usage might be higher. This is expected for Darwin/Windows platforms.")
	mallocTrim = func() int {
		return 0
	}

	return nil
}

// malloc_trim returns heap memory back to OS after it is no longer needed
// and glib still holds it for future use.
func MallocTrim() {
	mallocTrim()
}

// getErrorMessage gets the last error message from VIPS
func getErrorMessage() string {
	errorMutex.Lock()
	defer errorMutex.Unlock()
	return vipsErrorBuffer()
}

// clearVipsError clears the VIPS error buffer
func clearError() {
	errorMutex.Lock()
	defer errorMutex.Unlock()
	vipsErrorClear()
}

// getError gets the error from VIPS and wraps it in our Error type
func getError() error {
	errorMutex.Lock()
	defer errorMutex.Unlock()

	message := vipsErrorBuffer()
	vipsErrorClear()

	if message == "" {
		return fmt.Errorf("unknown vips error (error buffer is empty)")
	}

	return &Error{Message: message}
}

// ReadVipsMemStats returns various memory statistics such as allocated memory and open files.
// See: https://www.libvips.org/API/current/VipsOperation.html#vips-cache-operation-buildp
func ReadVipsMemStats(stats *Stats) error {
	if stats == nil {
		return fmt.Errorf("stats pointer is nil")
	}

	// Get version
	stats.Version = vipsVersionString()

	// Get tracked memory
	stats.Mem = vipsTrackedGetMem()

	// Get high water mark
	stats.MemHigh = vipsTrackedGetMemHighwater()

	// Get number of allocations
	stats.Allocs = vipsTrackedGetAllocs()

	// Get number of open files
	stats.Files = vipsTrackedGetFiles()

	// Get cache size
	stats.CacheSize = vipsCacheGetSize()

	// Get maximum cache size
	stats.CacheMax = vipsCacheGetMax()

	// Get concurrency
	stats.Concurrency = vipsConcurrencyGet()

	return nil
}

// LogVipsMemStats logs the current memory statistics
func LogVipsStats() {
	var stats Stats
	if err := ReadVipsMemStats(&stats); err != nil {
		logger.Error("Failed to read VIPS stats: %v", err)
		return
	}

	logger.Info("VIPS stats: version=%s, mem=%s, high=%s, allocs=%d, files=%d, cache_size=%d, cache_max=%s, concurrency=%d",
		stats.Version,
		utils.FormatBytes(uint64(stats.Mem)),
		utils.FormatBytes(uint64(stats.MemHigh)),
		stats.Allocs,
		stats.Files,
		stats.CacheSize,
		utils.FormatBytes(uint64(stats.CacheMax)),
		stats.Concurrency)
}

func LogObjectReport() {
	logger.Info("\n=======================================\nvips live objects:...\n")
	vipsObjectPrintAll()
	logger.Info("=======================================\n\n")
}

func ThreadShutdown() {
	vipsThreadShutdown()
}

func CacheDrop() {
	vipsCacheDropAll()
}

func Shutdown() {
	mutex.Lock()
	defer mutex.Unlock()

	if !initialized {
		return
	}

	vipsShutdown()
	initialized = false
}

// LoadImageFromBuffer loads an image from memory (byte slice).
//
// Example:
//
//	data, err := ioutil.ReadFile("image.jpg")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	img, err := vips.LoadImageFromBuffer(data)
//	if err != nil {
//	    log.Fatalf("Failed to load image: %v", err)
//	}
func LoadImageFromBuffer(data []byte) (*VipsImage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("invalid empty image data")
	}

	clearError()

	imageFormat := GetImageFormat(data)
	ptr := vipsImageNewFromBuffer(unsafe.Pointer(&data[0]), len(data), "", nil)
	if ptr == nil {
		return nil, getError()
	}

	image := &VipsImage{
		ptr:         ptr,
		ImageFormat: imageFormat,
	}

	return image, nil
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
func SaveImageToBuffer(img *VipsImage, format string, quality int) ([]byte, error) {
	// Default quality if not specified
	if quality <= 0 {
		quality = 80 // Default quality
	}

	// Ensure quality is within reasonable limits
	if quality > 100 {
		quality = 100
	}

	// Clear any previous errors
	clearError()

	// Normalize format (lowercase and remove dot if present)
	format = strings.ToLower(strings.TrimPrefix(format, "."))

	// Variables for save function result
	var data []byte
	var result int

	// Format-specific save functions
	switch format {
	case "webp":
		result, data = SaveWebpImageToBuffer(img, quality)
	case "jpeg":
		result, data = SaveJpegImageToBuffer(img, quality)
	case "gif":
		result, data = SaveGifImageToBuffer(img)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	// If format-specific functions worked, return the result
	if result == 0 && data != nil {
		return data, nil
	}

	return nil, getError()
}

// SaveWebpImage saves an image in WebP format
func SaveWebpImageToBuffer(img *VipsImage, quality int) (int, []byte) {
	if img == nil || img.ptr == nil {
		return -1, nil
	}

	var ptr unsafe.Pointer
	var size int

	// WebP uses the same quality scale as JPEG (1-100)
	result := vipsWebpSaveBuffer(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"Q", quality,
		"lossless", 0, // Use lossy compression
		nil)

	if result != 0 || ptr == nil || size == 0 {
		if debug {
			logger.Debug("WebP save failed: result=%d, ptr=%v, size=%d", result, ptr, size)
		}
		return result, nil
	}

	// Copy the data before freeing the pointer
	data := make([]byte, size)
	copy(data, (*[1 << 30]byte)(ptr)[:size:size])

	// Free the libvips-allocated memory directly with gFree
	gFree(ptr)
	return result, data
}

// SaveJpegImage saves an image in JPEG format
func SaveJpegImageToBuffer(img *VipsImage, quality int) (int, []byte) {
	if img == nil || img.ptr == nil {
		return -1, nil
	}

	var ptr unsafe.Pointer
	var size int

	// Set Q parameter for JPEG quality (1-100) and optimize for progressive display
	result := vipsJpegSaveBuffer(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		"Q", quality,
		"optimize_coding", 1,
		nil)

	if result != 0 || ptr == nil || size == 0 {
		if debug {
			logger.Debug("JPEG save failed: result=%d, ptr=%v, size=%d", result, ptr, size)
		}
		return result, nil
	}

	// Copy the data before freeing the pointer
	data := make([]byte, size)
	copy(data, (*[1 << 30]byte)(ptr)[:size:size])

	// Free the libvips-allocated memory directly with gFree
	gFree(ptr)

	return result, data
}

// SaveGifImageToBuffer saves an image in GIF format
func SaveGifImageToBuffer(img *VipsImage) (int, []byte) {
	if img == nil || img.ptr == nil {
		return -1, nil
	}

	var ptr unsafe.Pointer
	var size int

	// Call vips_gifsave_buffer
	result := vipsGifSaveBuffer(
		img.ptr,
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&size),
		nil,
	)

	if result != 0 || ptr == nil || size == 0 {
		if debug {
			logger.Debug("GIF save failed: result=%d, ptr=%v, size=%d", result, ptr, size)
		}
		return result, nil
	}

	// Copy the data before freeing the pointer
	data := make([]byte, size)
	copy(data, (*[1 << 30]byte)(ptr)[:size:size])

	// Free the libvips-allocated memory directly with gFree
	gFree(ptr)

	return result, data
}

// GetImageWidth returns the width of the image in pixels.
func GetImageWidth(img *VipsImage) int {
	if img == nil || img.ptr == nil {
		return 0
	}

	width := vipsImageGetWidth(img.ptr)
	if width <= 0 && debug {
		logger.Warn("Invalid image width: %d", width)
	}

	return width
}

// GetImageHeight returns the height of the image in pixels.
func GetImageHeight(img *VipsImage) int {
	if img == nil || img.ptr == nil {
		return 0
	}

	height := vipsImageGetHeight(img.ptr)
	if height <= 0 && debug {
		logger.Warn("Invalid image height: %d", height)
	}

	return height
}

// Free releases all resources associated with the image, including the parent operation
// and associated source object. This method should be called when you're done with an image
// to prevent memory leaks. It follows this order:
// 1. If the image has a parent operation (like VipsForeignLoadJpegBuffer), free it
// 2. Free the image itself
// 3. Return memory to the OS
func (img *VipsImage) Free() {
	if img.ptr == nil {
		return
	}

	// Get the parent operation (load operation like VipsForeignLoadJpegBuffer)
	parent := vipsImageGetParent(img.ptr)
	// Unreference the parent operation
	Unref(parent)

	// Free the image itself
	Unref(unsafe.Pointer(img.ptr))
}

// vipsImageGetParent gets the parent operation of a VipsImage
func vipsImageGetParent(ptr unsafe.Pointer) unsafe.Pointer {
	if ptr == nil {
		return nil
	}

	// Get the parent field from the VipsImage struct
	// The parent field is at offset 0 in the VipsImage struct
	parent := *(*unsafe.Pointer)(ptr)
	return parent
}

func Free(ptr unsafe.Pointer) {
	gFree(ptr)
}

func Unref(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	gObjectUnref(ptr)
}

func UnrefOutputs(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	vipsObjectUnrefOutputs(ptr)
}

func StringToFormat(format string) ImageFormat {
	switch format {
	case "jpg", "jpeg":
		return JPEG
	case "webp":
		return WEBP
	case "gif":
		return GIF
	case "png":
		return PNG
	case "tiff":
		return TIFF
	}
	return UNKNOWN
}

func GetImageFormat(buf []byte) ImageFormat {
	if len(buf) < 12 {
		return UNKNOWN
	}
	if buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
		return JPEG
	}
	if buf[0] == 0x47 && buf[1] == 0x49 && buf[2] == 0x46 {
		return GIF
	}
	if buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47 {
		return PNG
	}
	if (buf[0] == 0x49 && buf[1] == 0x49 && buf[2] == 0x2A && buf[3] == 0x0) ||
		(buf[0] == 0x4D && buf[1] == 0x4D && buf[2] == 0x0 && buf[3] == 0x2A) {
		return TIFF
	}
	if buf[8] == 0x57 && buf[9] == 0x45 && buf[10] == 0x42 && buf[11] == 0x50 {
		return WEBP
	}
	if buf[0] == 0xFF && buf[1] == 0x0A {
		// This is naked jxl file header
		return JXL
	}
	if buf[0] == 0x0 && buf[1] == 0x0 && buf[2] == 0x0 && buf[3] == 0x0C &&
		buf[4] == 0x4A && buf[5] == 0x58 && buf[6] == 0x4C && buf[7] == 0x20 &&
		buf[8] == 0x0D && buf[9] == 0x0A && buf[10] == 0x87 && buf[11] == 0x0A {
		// This is an ISOBMFF-based container
		return JXL
	}

	return UNKNOWN
}

func GetConcurrency() int {
	return vipsConcurrencyGet()
}

func GetMaxCacheSize() int64 {
	return vipsCacheGetSize()
}

func GetMaxCacheMem() int64 {
	return vipsCacheGetMaxMem()
}

func GetMaxCacheFiles() int64 {
	return vipsCacheGetMaxFiles()
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
func ResizeImage(img *VipsImage, scale float64) (*VipsImage, error) {
	if img == nil || img.ptr == nil {
		return nil, fmt.Errorf("invalid image pointer")
	}

	if scale <= 0 {
		return nil, fmt.Errorf("invalid scale factor: %f (must be positive)", scale)
	}

	// Clear any previous errors
	clearError()

	var out unsafe.Pointer
	result := vipsResize(img.ptr, unsafe.Pointer(&out), scale, nil)
	if result != 0 || out == nil {
		return nil, getError()
	}

	imageFormat := img.ImageFormat
	image := &VipsImage{
		ptr:         out,
		ImageFormat: imageFormat,
	}

	return image, nil
}

// LoadImageFromFile loads an image from a file path.
// Supported formats: JPEG, WebP
//
// Example:
//
//	img, err := vips2.LoadImageFromFile("image.jpg")
//	if err != nil {
//	    log.Fatalf("Failed to load image: %v", err)
//	}
func LoadImageFromFile(filePath string) (*VipsImage, error) {
	if filePath == "" {
		return nil, fmt.Errorf("invalid empty file path")
	}

	// Check if file exists and is readable
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("error accessing file: %w", err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	// Clear any previous errors
	clearError()

	// Determine image type from file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return nil, fmt.Errorf("file has no extension, cannot determine image type")
	}
	ext = ext[1:] // Remove leading dot

	imageFormat := StringToFormat(ext)

	imagePtr := vipsImageNewFromFile(filePath, nil)
	if imagePtr == nil {
		return nil, getError()
	}

	image := &VipsImage{
		ptr:         imagePtr,
		ImageFormat: imageFormat,
	}

	return image, nil
}

// SaveImageToFile saves an image to a file path.
// Supported formats: JPEG, WebP
//
// Example:
//
//	err := vips2.SaveImageToFile(img, "output.jpg", 80)
//	if err != nil {
//	    log.Fatalf("Failed to save image: %v", err)
//	}
func SaveImageToFile(image *VipsImage, filePath string, quality int) error {
	if image == nil || image.ptr == nil {
		return fmt.Errorf("invalid image pointer")
	}

	if filePath == "" {
		return fmt.Errorf("invalid empty file path")
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
	clearError()

	// Determine format from file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return fmt.Errorf("file has no extension, cannot determine image format")
	}
	ext = ext[1:] // Remove leading dot

	// Convert file path to C string (null-terminated)
	cFilePath := append([]byte(filePath), 0)

	var code int
	switch ext {
	case "jpg", "jpeg":
		cQuality := append([]byte("Q"), 0)
		code = vipsJpegSave(image.ptr, &cFilePath[0], &cQuality[0], quality, nil, 0, nil)
	case "webp":
		// Set Q for quality and lossless=0 for lossy compression
		cQuality := append([]byte("Q"), 0)
		cLossless := append([]byte("lossless"), 0)
		code = vipsWebpSave(
			image.ptr,
			&cFilePath[0],
			&cQuality[0], quality,
			&cLossless[0], 0,
			nil)
	case "gif":
		cQuality := append([]byte("Q"), 0)
		code = vipsGifSave(image.ptr, &cFilePath[0], &cQuality[0], quality, nil, 0, nil)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}

	if code != 0 {
		return getError()
	}

	return nil
}
