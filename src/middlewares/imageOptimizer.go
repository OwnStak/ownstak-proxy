package middlewares

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/server"
	"ownstak-proxy/src/vips"
)

/**
 * Image Optimizer Middleware
 *
 * This middleware allows customers to optimize images on the fly.
 * The Image Optimizer allows to fetch images from relative or absolute URLs on the same domain.
 * They don't need to be on OwnStak platform, so customers can fetch images from CDN cache.
 * The underlying library is libvips and it uses its own pool of threads to process images.
 *
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
const maxDimension = 3840

// Default width of image that optimizer will return.
const defaultWidth = 0  // 0 means auto
const defaultHeight = 0 // 0 means auto
const defaultFormat = "webp"
const defaultQuality = 70

// Cache control header value for optimized images.
// Optimized images should be cached "publicly" by the CDN.
const cacheControl = "public, max-age=31536000"

// Supported formats by vips.
// Can be also used as output formats.
// SVG is an exception, it will be always returned unchanged.
var supportedFormats = map[string]bool{
	"png":  true,
	"gif":  true,
	"jpg":  true,
	"jpeg": true,
	"webp": true,
	"avif": true,
}

type ImageOptimizerMiddleware struct {
	enabled bool
}

func NewImageOptimizerMiddleware() *ImageOptimizerMiddleware {
	// Initialize libvips
	enabled := true
	if err := vips.Initialize(); err != nil {
		logger.Warn("Disabling Image Optimizer middleware - Failed to initialize libvips: %v", err)
		enabled = false
	}
	return &ImageOptimizerMiddleware{
		enabled: enabled,
	}
}

func (m *ImageOptimizerMiddleware) OnRequest(ctx *server.ServerContext, next func()) {
	// Run the Image Optimizer middleware only on below path
	if ctx.Request.Path != "/__internal__/image" {
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
		ctx.Error("URL parameter is required", http.StatusBadRequest)
		return
	}

	// Validate output format
	format = strings.ToLower(format)
	if !supportedFormats[format] {
		// output formats as string, comma separated
		outputFormats := make([]string, 0, len(supportedFormats))
		for format := range supportedFormats {
			outputFormats = append(outputFormats, format)
		}
		outputFormatsString := strings.Join(outputFormats, ", ")
		ctx.Error(fmt.Sprintf("Unsupported output format: %s. Supported formats are: %s", format, outputFormatsString), http.StatusBadRequest)
		return
	}

	// Get current host
	currentHost := ctx.Request.Host
	if currentHost == "" {
		ctx.Error("Could not determine current host", http.StatusBadRequest)
		return
	}

	// Parse the target URL
	parsedURL, parseErr := url.Parse(urlStr)
	if parseErr != nil {
		ctx.Error(fmt.Sprintf("Invalid URL: %v", parseErr), http.StatusBadRequest)
		return
	}

	// Check if the URL is trying to fetch from /__internal__ path
	if strings.Contains(parsedURL.Path, "/__internal__") {
		ctx.Error("Fetching images from /__internal__ path is not allowed", http.StatusBadRequest)
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
		ctx.Error("URL must be from the same domain. Fetching images from external domains is not allowed in the production mode.", http.StatusBadRequest)
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

	// Fetch the image
	resp, err := client.Get(parsedURL.String())
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to fetch image: %v", err), http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	// Check if the response is an image
	if !strings.HasPrefix(resp.Header.Get(server.HeaderContentType), "image/") {
		ctx.Error("URL does not point to an image. \r\nServer returned content type: "+resp.Header.Get(server.HeaderContentType), http.StatusBadRequest)
		return
	}

	// Check content length
	contentLength := resp.Header.Get(server.HeaderContentLength)
	if contentLength != "" {
		// Check if the content length is too large
		contentLengthInt, err := strconv.Atoi(contentLength)
		if err == nil && contentLengthInt > maxImageSize {
			ctx.Error(fmt.Sprintf("The response content-length header exceeds maximum limit of %d bytes", maxImageSize), http.StatusBadRequest)
			return
		}
	}

	// Read the image data after we are sure it's an image and it fits into the max image size limit
	// Use LimitReader to ensure we don't read more than maxImageSize + 1 bytes
	// This protects against someone abusing the service to fetch massive files
	imageData, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to read image data: %v", err), http.StatusBadRequest)
		return
	}

	// Check if we hit the size limit
	if len(imageData) > maxImageSize {
		ctx.Error(fmt.Sprintf("Image size exceeds maximum limit of %d bytes", maxImageSize), http.StatusBadRequest)
		return
	}

	// If the Image Optimizer is disabled or the image type is an SVG,
	// just return the original image unchanged, so it still works locally even without the libvips installed.
	if !enabled || strings.Contains(resp.Header.Get(server.HeaderContentType), "svg") {
		// Set response headers and
		ctx.Response.Headers.Set(server.HeaderContentType, resp.Header.Get(server.HeaderContentType))
		ctx.Response.Headers.Set(server.HeaderCacheControl, cacheControl)
		ctx.Response.Headers.Set(server.HeaderXOwnImageOptimizer, fmt.Sprintf("enabled=%t,url=%s", enabled, urlStr))
		ctx.Response.Status = http.StatusOK
		ctx.Response.Body = imageData
		return
	}

	// Convert quality to integer
	qualityInt, err := strconv.Atoi(quality)
	if err != nil || qualityInt < 1 || qualityInt > 100 {
		ctx.Error("Quality must be a number between 1 and 100", http.StatusBadRequest)
		return
	}

	// Convert dimensions to integers
	widthInt, err := strconv.Atoi(width)
	if err != nil || widthInt < 0 {
		ctx.Error("Width must be a positive number", http.StatusBadRequest)
		return
	}

	heightInt, err := strconv.Atoi(height)
	if err != nil || heightInt < 0 {
		ctx.Error("Height must be a positive number", http.StatusBadRequest)
		return
	}

	// Load the image with vips
	image, err := vips.LoadImage(imageData)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to load image: %v", err), http.StatusBadRequest)
		return
	}

	// Get source image dimensions
	srcWidth := vips.GetImageWidth(image)
	srcHeight := vips.GetImageHeight(image)
	srcFormat := resp.Header.Get(server.HeaderContentType)
	srcFormat = strings.ToLower(srcFormat)

	// Extract the format from the content type header
	// image/webp -> webp
	if strings.HasPrefix(srcFormat, "image/") {
		srcFormat = strings.Split(srcFormat, "/")[1]
	}

	if srcWidth == 0 || srcHeight == 0 {
		ctx.Error("Failed to get image dimensions", http.StatusInternalServerError)
		return
	}

	aspectRatio := float64(srcWidth) / float64(srcHeight)

	// Calculate target dimensions while preserving aspect ratio
	targetWidth := widthInt
	targetHeight := heightInt

	if targetWidth == 0 && targetHeight == 0 {
		targetWidth = srcWidth
		targetHeight = srcHeight
	} else if targetWidth == 0 {
		targetWidth = int(float64(targetHeight) * aspectRatio)
	} else if targetHeight == 0 {
		targetHeight = int(float64(targetWidth) * aspectRatio)
	}

	// If source is larger than max, calculate target dimensions
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

	// Resize if target dimensions are different from source
	if targetWidth != srcWidth || targetHeight != srcHeight {
		// Calculate scale factor for resize
		scale := float64(targetWidth) / float64(srcWidth)
		resizedImage, err := vips.ResizeImage(image, scale)
		if err != nil {
			ctx.Error(fmt.Sprintf("Failed to resize image: %v", err), http.StatusInternalServerError)
			return
		}
		image = resizedImage
	}

	// Export with specified quality
	outputData, err := vips.SaveImage(image, format, qualityInt)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to export image: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate optimization duration
	duration := time.Since(startTime)

	// Set response headers
	ctx.Response.Headers.Set(server.HeaderContentType, "image/"+format)
	ctx.Response.Headers.Set(server.HeaderCacheControl, cacheControl)
	ctx.Response.Headers.Set(server.HeaderXOwnImageOptimizer, fmt.Sprintf("enabled=%t,url=%s,srcFormat=%s,format=%s,srcWidth=%d,width=%d,srcHeight=%d,height=%d,duration=%dms",
		enabled, urlStr, srcFormat, format, srcWidth, targetWidth, srcHeight, targetHeight, duration.Milliseconds()))
	ctx.Response.Status = http.StatusOK

	// Write the image data to the response
	ctx.Response.Body = outputData
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
