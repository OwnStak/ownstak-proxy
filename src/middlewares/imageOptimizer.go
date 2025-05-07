package middlewares

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/utils"
	"ownstak-proxy/src/vips"

	"github.com/google/uuid"
)

/**
 * Image Optimizer Middleware
 *
 * This middleware allows customers to optimize images on the fly.
 * The Image Optimizer allows to fetch images from relative or absolute URLs on the same domain.
 * They don't need to be on OwnStak platform, so customers can fetch images from CDN cache.
 * The underlying library is libvips and it uses its own pool of threads to process images.
 *
 * Throttling:
 * All image optimization requests are put into a queue and processed in FIFO order with configured concurrency based on VIPS_CONCURRENCY environment variable.
 * This is by default half of the available CPUs/threads to make sure it doesn't put too much load on the system and proxy has enough resources to handle standard IO requests.
 * If the request is in the queue too long, the client close/reset the connection during the waiting, the image optimization task is skipped.
 * Each concurrent thread that processes the image consumes around 100MB of RAM memory depending on image size, format etc...
 *
 * Usage:
 * The Image Optimizer is enabled by default as long as the libvips library was found on the system.
 * It has the same syntax as image optimizer from Next.js, so it works as drop-in replacement.
 *
 * For example:
 * https://example.com/__internal__/image?url=/image.jpg&w=100&h=100&q=80
 * https://example.com/__internal__/image?url=https://example.com/image.png&w=100&h=100&f=png
 *
 * The Image Optimizer supports the following query params:
 * - url: The relative or absolute URL of the image to optimize.
 * - width (or just w): The width of the image.
 * - height (or just h): The height of the image.
 * - format (or just f): The format of the image.
 * - quality (or just q): The quality of the image.
 * - enabled (or just e): Whether the Image Optimizer is enabled or not.
 *
 * The Image Optimizer will return the optimized image and set the following headers:
 * - Content-Type: The content type of the output image.
 * - Cache-Control: The cache control header value for optimized images.
 * - X-Own-Image-Optimizer: The X-Own-Image-Optimizer header value.
 *
 * The Image Optimizer will return the original image unchanged if:
 * - The libvips library is not found on the system.
 * - The image is an SVG.
 * - The "enabled" query param is set to "false".
 *
 * The Image Optimizer will return a 400 error if:
 * - The "url" query param is not provided.
 * - The "url" query param is not a valid URL.
 * - The "url" query param is not from the same domain.
 * - The "url" param points back to /__internal__/ path.
 */

// Define limits and defaults
// Maximum accepted image size in bytes. Exceeding this limit will result in a 400 error.
const maxImageSize = 6 * 1024 * 1024 // 6MB

// Maximum width or height in pixels of image that optimizer will return.
// Exceeding this limit won't throw an error, but the image will be resized to the limit.
const maxDimension = 2560

// Default width of image that optimizer will return.
const defaultWidth = 0  // 0 means auto
const defaultHeight = 0 // 0 means auto
const defaultFormat = "webp"
const defaultQuality = 60

// Cache control header value for optimized images.
// Optimized images should be cached "publicly" by the CDN.
const defaultCacheControl = "public, max-age=86400, s-maxage=31536000"

// Supported formats by vips.
// Can be also used as output formats.
// SVG is an exception, it will be always returned unchanged.
var supportedOutputFormats = map[string]bool{
	"gif":  true,
	"jpg":  true,
	"jpeg": true,
	"webp": true,
}

type ImageOptimizerMiddleware struct {
	enabled bool
	// Channel to control concurrent image processing and fetching
	fetchQueue   chan struct{}
	processQueue chan struct{}
}

func NewImageOptimizerMiddleware() *ImageOptimizerMiddleware {
	// Initialize libvips
	enabled := true
	processConcurrency := 1
	fetchConcurrency := 10

	if err := vips.Initialize(); err != nil {
		logger.Warn("Disabling Image Optimizer middleware - Failed to initialize libvips: %v", err)
		enabled = false
	} else {
		// Defines how many concurrent requests to Image Optimizer can start to fetch and processat the same time.
		// The actual processing will be done by VIPS workers with VIPS_CONCURRENCY threads.
		// Each VIPS concurrent thread consumes around 100MB of RAM memory depending on image size, format etc...
		processConcurrency = vips.GetConcurrency()
		fetchConcurrency = processConcurrency * 10
	}

	// Create a buffered channel to control concurrency
	processQueue := make(chan struct{}, processConcurrency)
	fetchQueue := make(chan struct{}, fetchConcurrency)

	return &ImageOptimizerMiddleware{
		enabled:      enabled,
		fetchQueue:   fetchQueue,
		processQueue: processQueue,
	}
}

func (m *ImageOptimizerMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	// Run the Image Optimizer middleware only on below path
	if ctx.Request.Path != constants.InternalPathPrefix+"/image" {
		next()
		return
	}

	// Start timing the optimization process
	startTime := time.Now()

	// Extract query parameters and set defaults
	urlStr := GetQueryParam(ctx.Request.Query, "url", "", "")
	quality := GetQueryParam(ctx.Request.Query, "quality", "q", strconv.Itoa(defaultQuality))
	width := GetQueryParam(ctx.Request.Query, "width", "w", strconv.Itoa(defaultWidth))
	height := GetQueryParam(ctx.Request.Query, "height", "h", strconv.Itoa(defaultHeight))
	format := GetQueryParam(ctx.Request.Query, "format", "f", defaultFormat)

	// Check if the Image Optimizer can and should be applied to the image
	enabled := m.enabled
	// Disable the Image Optimizer if the query param "enabled" is set to "false"
	if GetQueryParam(ctx.Request.Query, "enabled", "e", "") == "false" {
		enabled = false
	}

	if urlStr == "" {
		ctx.Error("Image Optimizer failed: URL parameter is required", http.StatusBadRequest)
		return
	}

	// Validate output format
	format = strings.ToLower(format)
	if !supportedOutputFormats[format] {
		// output formats as string, comma separated
		outputFormats := make([]string, 0, len(supportedOutputFormats))
		for format := range supportedOutputFormats {
			outputFormats = append(outputFormats, format)
		}
		outputFormatsString := strings.Join(outputFormats, ", ")
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Unsupported output format: %s. Supported formats are: %s", format, outputFormatsString), http.StatusBadRequest)
		return
	}

	// Get current host
	currentHost := ctx.Request.Host
	if currentHost == "" {
		ctx.Error("Image Optimizer failed: Could not determine current host", http.StatusBadRequest)
		return
	}

	// Parse the target URL
	parsedURL, parseErr := url.Parse(urlStr)
	if parseErr != nil {
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Invalid URL: %v", parseErr), http.StatusBadRequest)
		return
	}

	// Check if the URL is trying to fetch from /__internal__ path
	if strings.Contains(parsedURL.Path, constants.InternalPathPrefix) {
		ctx.Error("Image Optimizer failed: Fetching images from "+constants.InternalPathPrefix+" path is not allowed", http.StatusBadRequest)
		return
	}

	// Handle relative URLs
	if !parsedURL.IsAbs() {
		// For relative URLs, use the current host and scheme
		parsedURL.Scheme = ctx.Request.Scheme
		parsedURL.Host = currentHost
	}

	// Don't allow fetching images from other domains or even subdomains,
	// so people can't use this proxy/image optimizer to fetch images from other sites.
	// For example by setting DNS records images.example.com to your server and example.com to this proxy.
	// This feature is allowed in development mode, so we can easily test it locally when runnning ./scripts/dev.sh.
	if parsedURL.Host != ctx.Request.Host && constants.Mode != "development" {
		ctx.Error("Image Optimizer failed: URL must be from the same domain. Fetching images from external domains is not allowed in the production mode.", http.StatusBadRequest)
		return
	}

	// Create an HTTP client that can handle both HTTP and HTTPS
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Allow self-signed certificates
			},
		},
	}

	// Wait for an available slot in the fetch queue
	// before we try to fetch the image.
	select {
	case m.fetchQueue <- struct{}{}:
		// Got a slot, fetch the image
	case <-ctx.Request.Context().Done():
		// Request was cancelled or connection was closed
		// Just return without error as the client is no longer waiting for the response
		return
	}

	// Fetch the image
	fetchStartTime := time.Now()
	logger.Debug("Image Optimizer - Fetching image from %s", parsedURL.String())
	resp, err := client.Get(parsedURL.String())
	if err != nil {
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Failed to fetch image: %v", err), http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()
	fetchDuration := time.Since(fetchStartTime)

	// Release the fetch slot
	func() { <-m.fetchQueue }()

	// Check if the response is an image
	if !strings.HasPrefix(resp.Header.Get(server.HeaderContentType), "image/") {
		ctx.Error("Image Optimizer failed: URL does not point to an image. \r\nServer returned content type: "+resp.Header.Get(server.HeaderContentType), http.StatusBadRequest)
		return
	}

	// Use cache-control header from the original image if it exists
	// otherwise use the default cache-control header for optimized images.
	// This allows the customers to have different cache-control headers for each image.
	cacheControl := resp.Header.Get(server.HeaderCacheControl)
	if cacheControl == "" {
		cacheControl = defaultCacheControl
	}

	// Check content length
	contentLength := resp.Header.Get(server.HeaderContentLength)
	if contentLength != "" {
		// Check if the content length is too large
		contentLengthInt, err := strconv.Atoi(contentLength)
		if err == nil && contentLengthInt > maxImageSize {
			ctx.Error(fmt.Sprintf("Image Optimizer failed: The response content-length header exceeds maximum limit of %s", utils.FormatBytes(uint64(maxImageSize))), http.StatusBadRequest)
			return
		}
	}

	// Return error if the resp was cancelled or connection was closed
	// Read the image data after we are sure it's an image and it fits into the max image size limit
	// Use LimitReader to ensure we don't read more than maxImageSize + 1 bytes
	// This protects against someone abusing the service to fetch massive files
	limitedReader := io.LimitReader(resp.Body, maxImageSize+1)

	// If the Image Optimizer is disabled or the image type is an SVG,
	// just return the original image unchanged, so it still works locally even without the libvips installed.
	if !enabled || strings.Contains(resp.Header.Get(server.HeaderContentType), "svg") {
		// Set response headers and
		ctx.Response.Headers.Set(server.HeaderContentType, resp.Header.Get(server.HeaderContentType))
		ctx.Response.Headers.Set(server.HeaderCacheControl, cacheControl)
		ctx.Response.Headers.Set(server.HeaderXOwnImageOptimizer, fmt.Sprintf("enabled=%t,url=%s,fetchDuration=%dms", enabled, urlStr, fetchDuration.Milliseconds()))
		ctx.Response.Status = http.StatusOK

		// Enable streaming for the response
		ctx.Response.EnableStreaming()

		// Stream the image data directly to the response
		if _, err := io.Copy(ctx.Response, limitedReader); err != nil {
			ctx.Error(fmt.Sprintf("Image Optimizer failed: Failed to stream image: %v", err), http.StatusInternalServerError)
			return
		}
		return
	}

	// Convert quality to integer
	qualityInt, err := strconv.Atoi(quality)
	if err != nil || qualityInt < 1 || qualityInt > 100 {
		ctx.Error("Image Optimizer failed: Quality must be a number between 1 and 100", http.StatusBadRequest)
		return
	}

	// Convert dimensions to integers
	widthInt, err := strconv.Atoi(width)
	if err != nil || widthInt < 0 {
		ctx.Error("Image Optimizer failed: Width must be a positive number", http.StatusBadRequest)
		return
	}

	heightInt, err := strconv.Atoi(height)
	if err != nil || heightInt < 0 {
		ctx.Error("Image Optimizer failed: Height must be a positive number", http.StatusBadRequest)
		return
	}

	// Wait for an available slot in the process queue
	// before we start to process the image.
	select {
	case m.processQueue <- struct{}{}:
		// Got a slot, process the image
		defer func() { <-m.processQueue }() // Release the slot when we are done
	case <-ctx.Request.Context().Done():
		// Request was cancelled or connection was closed
		// Just return without error as the client is no longer waiting for the response
		return
	}

	// We stream the fetched image directly to tmp file, so we dont need to hold it whole in memory
	// until VIPS worker picks up the task.
	srcImageFilename := fmt.Sprintf("/tmp/image-optimizer-src-%s.%s", uuid.New().String(), format)
	srcImageFile, err := os.Create(srcImageFilename)
	if err != nil {
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Failed to create temporary srcImageFile: %v", err), http.StatusInternalServerError)
		return
	}
	// Always remove the tmp file after we are done
	defer os.Remove(srcImageFilename)

	// Run VIPS thread shutdown when we are done or if we get an error
	defer vips.ThreadShutdown()
	defer vips.MallocTrim()

	// Stream the image to the tmp file
	if _, err := io.Copy(srcImageFile, limitedReader); err != nil {
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Failed to stream image: %v", err), http.StatusInternalServerError)
		return
	}
	srcImageFile.Close()

	// Stream the image data directly to libvips tmp file
	// instead of loading it whole into memory.
	srcImage, err := vips.LoadImageFromFile(srcImageFilename)
	if err != nil {
		ctx.Error(fmt.Sprintf("Image Optimizer failed: Failed to load image: %v", err), http.StatusBadRequest)
		return
	}
	// Ensure the image is freed when we're done.
	// libvips memory is managed manually by dummy glib reference counting and Golang garbage collector doesn't know about it.
	// If we don't free the image, it will stay in memory forever and cause memory leaks.
	defer srcImage.Free()

	// Get source image dimensions
	srcWidth := vips.GetImageWidth(srcImage)
	srcHeight := vips.GetImageHeight(srcImage)
	srcFormat := resp.Header.Get(server.HeaderContentType)
	srcFormat = strings.ToLower(srcFormat)

	// Extract the format from the content type header
	// image/webp -> webp
	if strings.HasPrefix(srcFormat, "image/") {
		srcFormat = strings.Split(srcFormat, "/")[1]
	}

	if srcWidth == 0 || srcHeight == 0 {
		ctx.Error("Image Optimizer failed: Failed to get image dimensions", http.StatusInternalServerError)
		return
	}

	aspectRatio := float64(srcWidth) / float64(srcHeight)

	// Start with the target dimensions set to provided width and height
	targetWidth := widthInt
	targetHeight := heightInt

	// Make sure target dimensions are smaller or equal to source dimensions
	if targetWidth > srcWidth {
		targetWidth = srcWidth
	}
	if targetHeight > srcHeight {
		targetHeight = srcHeight
	}

	// Calculate missing dimensions while preserving aspect ratio
	// if any of the target dimensions are not provided
	if targetWidth == 0 && targetHeight == 0 {
		targetWidth = srcWidth
		targetHeight = srcHeight
	} else if targetWidth == 0 {
		targetWidth = int(float64(targetHeight) * aspectRatio)
	} else if targetHeight == 0 {
		targetHeight = int(float64(targetWidth) * aspectRatio)
	}

	// If source is larger than max allowed dimension,
	// set target dimensions to max allowed dimension
	// while preserving aspect ratio
	if targetHeight > maxDimension || targetWidth > maxDimension {
		if targetWidth > targetHeight {
			// Width is larger, limit it to maxDimension
			targetWidth = maxDimension
			targetHeight = int(float64(maxDimension) / aspectRatio)
		} else {
			// Height is larger, limit it to maxDimension
			targetHeight = maxDimension
			targetWidth = int(float64(maxDimension) * aspectRatio)
		}
	}

	// Call resize if target dimensions are different from source
	if targetWidth != srcWidth || targetHeight != srcHeight {
		// Calculate scale factor for resize
		scale := float64(targetWidth) / float64(srcWidth)
		resizedImage, err := vips.ResizeImage(srcImage, scale)
		if err != nil {
			ctx.Error(fmt.Sprintf("Failed to resize image: %v", err), http.StatusInternalServerError)
			return
		}
		defer resizedImage.Free()
		srcImage = resizedImage
	}

	// Export image to the specified format
	ctx.Response.Headers.Set(server.HeaderContentType, "image/"+format)
	ctx.Response.Headers.Set(server.HeaderCacheControl, cacheControl)
	ctx.Response.Headers.Set(server.HeaderXOwnImageOptimizer, fmt.Sprintf("enabled=%t,url=%s,srcFormat=%s,format=%s,srcWidth=%d,width=%d,srcHeight=%d,height=%d,fetchDuration=%dms,duration=%dms",
		enabled, urlStr, srcFormat, format, srcWidth, targetWidth, srcHeight, targetHeight, fetchDuration.Milliseconds(), time.Since(startTime).Milliseconds()))
	ctx.Response.Status = http.StatusOK

	// Enable streaming for the response
	ctx.Response.EnableStreaming()

	outImageFilename := fmt.Sprintf("/tmp/image-optimizer-out-%s.%s", uuid.New().String(), format)
	err = vips.SaveImageToFile(srcImage, outImageFilename, qualityInt)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to save image: %v", err), http.StatusInternalServerError)
		return
	}

	// Start streaming the image from tmp file to client
	outImageFile, err := os.Open(outImageFilename)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to open image: %v", err), http.StatusInternalServerError)
		return
	}
	defer outImageFile.Close()
	defer os.Remove(outImageFilename)
	io.Copy(ctx.Response, outImageFile)

	runtime.GC()
}

func (m *ImageOptimizerMiddleware) OnResponse(ctx *server.ServerContext, next func()) {
	next()
}

func GetQueryParam(query map[string][]string, param1, param2, defaultValue string) string {
	if values, ok := query[param1]; ok && len(values) > 0 {
		return values[0]
	}
	if values, ok := query[param2]; ok && len(values) > 0 {
		return values[0]
	}
	return defaultValue
}
