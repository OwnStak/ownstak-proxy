package server

import (
	"net/http"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
)

type ServerResponse struct {
	Status           int
	Headers          http.Header
	Body             []byte
	Ended            bool
	Streaming        bool
	StreamingStarted bool
	ResponseWriter   http.ResponseWriter
}

// NewServerResponse creates a new ServerResponse with default values
func NewServerResponse(responseWriter ...http.ResponseWriter) *ServerResponse {
	headers := make(http.Header)
	headers.Set(HeaderContentType, ContentTypePlain)
	headers.Set(HeaderXOwnProxyVersion, constants.Version)

	sr := &ServerResponse{
		Status:           http.StatusOK,
		Headers:          headers,
		Body:             []byte{},
		Ended:            false,
		Streaming:        false,
		StreamingStarted: false,
	}

	// Set response writer if provided
	if len(responseWriter) > 0 && responseWriter[0] != nil {
		sr.ResponseWriter = responseWriter[0]
	}

	return sr
}

// SetResponseWriter allows setting the response writer after creation
func (res *ServerResponse) SetResponseWriter(rw http.ResponseWriter) {
	res.ResponseWriter = rw
}

// EnableStreaming enables streaming mode for this response
func (res *ServerResponse) EnableStreaming() {
	res.Streaming = true
}

// DisableStreaming disables streaming mode for this response
func (res *ServerResponse) DisableStreaming() {
	res.Streaming = false
}

// Writes body chunks to the response writer
func (res *ServerResponse) Write(chunk []byte) (int, error) {
	if res.Streaming {
		if res.ResponseWriter == nil {
			logger.Warn("Attempted to stream response with nil ResponseWriter")
			res.Body = append(res.Body, chunk...)
			return len(chunk), nil
		}

		if !res.StreamingStarted {
			res.StreamingStarted = true
			// Set all headers before starting to write
			for key, values := range res.Headers {
				for _, value := range values {
					res.ResponseWriter.Header().Add(key, value)
				}
			}
			// Add chunked transfer encoding if not already present
			if res.Headers.Get(HeaderTransferEncoding) == "" {
				res.Headers.Add(HeaderTransferEncoding, "chunked")
			}
			res.ResponseWriter.WriteHeader(res.Status)
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

	// If streaming is disabled, just accumulate in body
	// and the body will be sent when End() is called
	res.Body = append(res.Body, chunk...)
	return len(chunk), nil
}

func (res *ServerResponse) Clear() {
	res.Status = http.StatusOK
	res.Headers = make(http.Header)
	res.Body = []byte{}
}

// Finishes the response and sends it to the client
// if it wasn't already streamed
func (res *ServerResponse) End() bool {
	if res.Ended {
		return false
	}
	res.Ended = true

	// If we're already streaming or have no response writer, don't do anything more
	if res.StreamingStarted || res.ResponseWriter == nil {
		return false
	}

	// Set headers
	for key, values := range res.Headers {
		for _, value := range values {
			res.ResponseWriter.Header().Add(key, value)
		}
	}

	// Write status and body
	res.ResponseWriter.WriteHeader(res.Status)
	res.ResponseWriter.Write(res.Body)
	return true
}
