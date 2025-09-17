package server

import (
	"fmt"
	"net/http"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"strconv"
	"strings"
)

type Response struct {
	Status           int
	Headers          http.Header
	Body             []byte
	Ended            bool
	Streaming        bool
	StreamingStarted bool
	ResponseWriter   http.ResponseWriter
}

// NewResponse creates a new Response with default values
func NewResponse(responseWriter ...http.ResponseWriter) *Response {
	headers := make(http.Header)

	res := &Response{
		Status:           http.StatusOK,
		Headers:          headers,
		Body:             []byte{},
		Ended:            false,
		Streaming:        false,
		StreamingStarted: false,
	}

	// Clear response, so default values are set
	res.Clear()

	// Set response writer if provided
	if len(responseWriter) > 0 && responseWriter[0] != nil {
		res.ResponseWriter = responseWriter[0]
	}

	return res
}

// SetResponseWriter allows setting the response writer after creation
func (res *Response) SetResponseWriter(rw http.ResponseWriter) {
	res.ResponseWriter = rw
}

// EnableStreaming enables streaming mode for this response
// @param value - Optional boolean value to set the streaming mode to. Default: true
func (res *Response) EnableStreaming(value ...bool) {
	if len(value) > 0 {
		res.Streaming = value[0]
	} else {
		res.Streaming = true
	}
}

// Writes body chunks to the response writer
func (res *Response) Write(chunk []byte) (int, error) {
	if res.Ended {
		return 0, fmt.Errorf("response already ended")
	}

	// If streaming is disabled, just accumulate in body
	// and the body will be sent when End() is called
	if !res.Streaming {
		res.Body = append(res.Body, chunk...)
		return len(chunk), nil
	}

	if res.ResponseWriter == nil {
		logger.Warn("Attempted to stream response with nil ResponseWriter")
		res.Body = append(res.Body, chunk...)
		return len(chunk), nil
	}

	if !res.StreamingStarted {
		// Add chunked transfer encoding if not already present
		// and content-length header is not present.
		// NOTE: They cannot be used together.
		if res.Headers.Get(HeaderTransferEncoding) == "" && res.Headers.Get(HeaderContentLength) == "" {
			res.Headers.Add(HeaderTransferEncoding, "chunked")
		}

		res.WriteHead(res.Status)
	}

	n, err := res.ResponseWriter.Write(chunk)
	if err != nil {
		logger.Debug("Failed to stream the response. Client is gone: %v", err)
	}

	// Flush if the writer supports it
	if flusher, ok := res.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}

	return n, err
}

func (res *Response) Clear() {
	res.Status = http.StatusOK
	res.Ended = false
	res.Streaming = false
	res.StreamingStarted = false
	res.ClearHeaders(true)
	res.ClearBody()
}

func (res *Response) ClearHeaders(preserveInternalHeaders ...bool) {
	internalHeaders := make(http.Header)
	for k, v := range res.Headers {
		if res.IsInternalHeader(k) {
			internalHeaders[k] = v
		}
	}

	res.Headers = make(http.Header)
	// Preserve our internal headers if preserveInternalHeaders is true
	if len(preserveInternalHeaders) > 0 && preserveInternalHeaders[0] {
		res.Headers = internalHeaders
	}

	// Always set default content type and proxy version headers
	res.Headers.Set(HeaderContentType, ContentTypePlain)
	res.Headers.Set(HeaderXOwnProxyVersion, constants.Version)
}

func (res *Response) ClearBody() {
	res.Body = []byte{}
}

// Finishes the response and sends it to the client
// if it wasn't already streamed
func (res *Response) End() bool {
	if res.Ended {
		return false
	}

	// If we're already streaming or have no response writer, don't do anything more.
	// Go net/http will automatically finish the stream when main handler exits.
	if res.StreamingStarted || res.ResponseWriter == nil {
		return false
	}

	// Remove transfer-encoding header if it's present
	// and update the content-length header when we have the whole response buffered in memory.
	if res.Headers.Get(HeaderTransferEncoding) != "" {
		res.Headers.Del(HeaderTransferEncoding)
	}
	if res.Headers.Get(HeaderContentLength) == "" {
		res.Headers.Set(HeaderContentLength, strconv.Itoa(len(res.Body)))
	}

	// Write status and headers
	res.WriteHead(res.Status)

	// Write body
	res.ResponseWriter.Write(res.Body)
	if flusher, ok := res.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}

	res.Ended = true
	return true
}

func (res *Response) AppendHeader(key, value string) {
	existingValues := res.Headers.Get(key)
	if existingValues != "" {
		res.Headers.Set(key, existingValues+","+value)
	} else {
		res.Headers.Set(key, value)
	}
}

// For compatibility with http.ResponseWriter
func (res *Response) WriteHead(status int) {
	if res.Ended || res.StreamingStarted {
		return
	}

	if status == 0 {
		status = http.StatusOK
	}

	res.Status = status
	res.StreamingStarted = true

	// Set headers that cannot be overriden
	res.Headers.Set(HeaderXOwnProxyVersion, constants.Version)
	res.Headers.Set(HeaderServer, constants.AppName)

	if res.ResponseWriter == nil {
		return
	}

	// Set all headers before starting to write
	for key, values := range res.Headers {
		for _, value := range values {
			res.ResponseWriter.Header().Add(key, value)
		}
	}

	res.ResponseWriter.WriteHeader(res.Status)
}

func (res *Response) IsInternalHeader(key string) bool {
	key = strings.ToLower(key)

	if strings.HasPrefix(key, strings.ToLower(HeaderXOwnPrefix)) {
		return true
	}

	// Also preserved protected headers
	// such as:
	// x-request-id
	if key == strings.ToLower(HeaderRequestID) {
		return true
	}

	return false
}
