package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	res.Ended = false
	res.Streaming = false
	res.StreamingStarted = false
}

func (res *ServerResponse) ClearHeaders() {
	res.Headers = make(http.Header)
}

func (res *ServerResponse) ClearBody() {
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
	if flusher, ok := res.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
	res.Clear()
	return true
}

// For compatibility with http.ResponseWriter
func (res *ServerResponse) Header() http.Header {
	return res.Headers
}

func (res *ServerResponse) AppendHeader(key, value string) {
	existingValues := res.Headers.Get(key)
	if existingValues != "" {
		res.Headers.Set(key, existingValues+","+value)
	} else {
		res.Headers.Set(key, value)
	}
}

// For compatibility with http.ResponseWriter
func (res *ServerResponse) WriteHeader(status int) {
	if status == 0 {
		status = http.StatusOK
	}
	res.Status = status
}

func (res *ServerResponse) Serialize() string {
	serialized := make(map[string]interface{})
	serialized["status"] = res.Status
	serialized["headers"] = res.Headers

	// Convert body to base64 string for JSON serialization
	serialized["body"] = base64.StdEncoding.EncodeToString(res.Body)

	data, err := json.Marshal(serialized)
	if err != nil {
		// Return empty byte slice on error
		return ""
	}
	return string(data)
}

func DeserializeServerResponse(data string) (*ServerResponse, error) {
	if data == "" {
		return nil, fmt.Errorf("empty data provided for deserialization")
	}

	var serialized map[string]interface{}
	if err := json.Unmarshal([]byte(data), &serialized); err != nil {
		return nil, fmt.Errorf("invalid JSON data: %v", err)
	}

	res := NewServerResponse()

	// Handle status
	if status, ok := serialized["status"].(float64); ok {
		res.Status = int(status)
	} else {
		return nil, fmt.Errorf("invalid or missing status field")
	}

	// Handle headers
	if headers, ok := serialized["headers"].(map[string]interface{}); ok {
		res.Headers = make(http.Header)
		for key, value := range headers {
			if values, ok := value.([]interface{}); ok {
				for _, v := range values {
					if str, ok := v.(string); ok {
						res.Headers.Add(key, str)
					}
				}
			} else if str, ok := value.(string); ok {
				res.Headers.Set(key, str)
			}
		}
	} else {
		return nil, fmt.Errorf("invalid or missing headers field")
	}

	// Handle body
	if bodyStr, ok := serialized["body"].(string); ok {
		body, err := base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode body: %v", err)
		}
		res.Body = body
	} else {
		return nil, fmt.Errorf("invalid or missing body field")
	}

	return res, nil
}
