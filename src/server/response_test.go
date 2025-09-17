package server

import (
	"net/http"
	"net/http/httptest"
	"ownstak-proxy/src/constants"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResponse(t *testing.T) {
	t.Run("should create default empty response", func(t *testing.T) {
		resp := NewResponse()

		assert.Equal(t, http.StatusOK, resp.Status)
		assert.NotNil(t, resp.Headers)
		assert.Equal(t, ContentTypePlain, resp.Headers.Get(HeaderContentType))
		assert.Equal(t, constants.Version, resp.Headers.Get(HeaderXOwnProxyVersion))
		assert.Empty(t, resp.Body)
		assert.False(t, resp.Ended)
		assert.False(t, resp.Streaming)
		assert.False(t, resp.StreamingStarted)
		assert.Nil(t, resp.ResponseWriter)
	})

	t.Run("should create response with provided ResponseWriter", func(t *testing.T) {
		rw := httptest.NewRecorder()
		resp := NewResponse(rw)

		assert.Equal(t, rw, resp.ResponseWriter)
	})

	t.Run("should handle nil ResponseWriter", func(t *testing.T) {
		resp := NewResponse(nil)
		assert.Nil(t, resp.ResponseWriter)
	})
}

func TestResponseMethods(t *testing.T) {
	t.Run("SetResponseWriter should set the writer", func(t *testing.T) {
		resp := NewResponse()
		rw := httptest.NewRecorder()

		resp.SetResponseWriter(rw)
		assert.Equal(t, rw, resp.ResponseWriter)
	})

	t.Run("streaming mode should be disabled by default", func(t *testing.T) {
		resp := NewResponse()
		assert.False(t, resp.Streaming)
	})

	t.Run("EnableStreaming(true) should enable streaming mode by default", func(t *testing.T) {
		resp := NewResponse()
		resp.EnableStreaming()
		assert.True(t, resp.Streaming)
		resp.EnableStreaming(true)
		assert.True(t, resp.Streaming)
	})

	t.Run("EnableStreaming(false) should disable streaming mode", func(t *testing.T) {
		resp := NewResponse()
		resp.EnableStreaming(false)
		assert.False(t, resp.Streaming)
	})

	t.Run("WriteHead should set status", func(t *testing.T) {
		resp := NewResponse()
		resp.WriteHead(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("WriteHead should default to 200 for zero status", func(t *testing.T) {
		resp := NewResponse()
		resp.WriteHead(0)
		assert.Equal(t, http.StatusOK, resp.Status)
	})
}

func TestResponseWrite(t *testing.T) {
	t.Run("should write to buffer when streaming disabled", func(t *testing.T) {
		resp := NewResponse()
		data := []byte("test data")

		n, err := resp.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, resp.Body)
	})

	t.Run("should append to buffer when streaming disabled", func(t *testing.T) {
		resp := NewResponse()

		resp.Write([]byte("first"))
		resp.Write([]byte("second"))

		assert.Equal(t, []byte("firstsecond"), resp.Body)
	})

	t.Run("should stream when streaming enabled with ResponseWriter", func(t *testing.T) {
		rw := httptest.NewRecorder()
		resp := NewResponse(rw)
		resp.EnableStreaming()
		resp.Headers.Set("Custom-Header", "custom-value")

		data := []byte("streaming data")
		n, err := resp.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.True(t, resp.StreamingStarted)
		assert.Equal(t, string(data), rw.Body.String())
		assert.Equal(t, "custom-value", rw.Header().Get("Custom-Header"))
	})

	t.Run("should fall back to buffer when streaming enabled but no ResponseWriter", func(t *testing.T) {
		resp := NewResponse()
		resp.EnableStreaming()
		data := []byte("test data")

		n, err := resp.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, data, resp.Body)
		assert.False(t, resp.StreamingStarted)
	})

	t.Run("should continue streaming after first write", func(t *testing.T) {
		rw := httptest.NewRecorder()
		resp := NewResponse(rw)
		resp.EnableStreaming()

		resp.Write([]byte("first"))
		resp.Write([]byte("second"))

		assert.Equal(t, "firstsecond", rw.Body.String())
		assert.True(t, resp.StreamingStarted)
	})
}

func TestResponseClear(t *testing.T) {
	t.Run("Clear should reset all values", func(t *testing.T) {
		resp := NewResponse()
		resp.Status = http.StatusNotFound
		resp.Ended = true
		resp.Streaming = true
		resp.StreamingStarted = true
		resp.Body = []byte("test")
		resp.Headers.Set("Custom-Header", "custom-value")

		resp.Clear()

		assert.Equal(t, http.StatusOK, resp.Status)
		assert.False(t, resp.Ended)
		assert.False(t, resp.Streaming)
		assert.False(t, resp.StreamingStarted)
		assert.Empty(t, resp.Body)
		assert.Equal(t, ContentTypePlain, resp.Headers.Get(HeaderContentType))
		assert.Equal(t, "", resp.Headers.Get("Custom-Header"))
	})

	t.Run("ClearHeaders should reset headers to defaults", func(t *testing.T) {
		resp := NewResponse()
		resp.Headers.Set("Custom-Header", "custom-value")

		resp.ClearHeaders()

		assert.Equal(t, ContentTypePlain, resp.Headers.Get(HeaderContentType))
		assert.Equal(t, constants.Version, resp.Headers.Get(HeaderXOwnProxyVersion))
		assert.Equal(t, "", resp.Headers.Get("Custom-Header"))
	})

	t.Run("ClearBody should reset body", func(t *testing.T) {
		resp := NewResponse()
		resp.Body = []byte("test data")

		resp.ClearBody()

		assert.Empty(t, resp.Body)
	})
}

func TestResponseEnd(t *testing.T) {
	t.Run("should return false when already ended", func(t *testing.T) {
		resp := NewResponse()
		resp.Ended = true

		result := resp.End()
		assert.False(t, result)
	})
	t.Run("should return false when streaming is enabled", func(t *testing.T) {
		resp := NewResponse()
		resp.EnableStreaming()

		result := resp.End()
		assert.False(t, result)
	})
	t.Run("should return false when there is no response writer", func(t *testing.T) {
		resp := NewResponse()
		resp.ResponseWriter = nil

		result := resp.End()
		assert.False(t, result)
	})
}

func TestResponseAppendHeader(t *testing.T) {
	t.Run("should set header when not exists", func(t *testing.T) {
		resp := NewResponse()
		resp.AppendHeader("Test-Header", "value1")

		assert.Equal(t, "value1", resp.Headers.Get("Test-Header"))
	})

	t.Run("should append to existing header", func(t *testing.T) {
		resp := NewResponse()
		resp.Headers.Set("Test-Header", "value1")
		resp.AppendHeader("Test-Header", "value2")

		assert.Equal(t, "value1,value2", resp.Headers.Get("Test-Header"))
	})

	t.Run("should handle multiple appends", func(t *testing.T) {
		resp := NewResponse()
		resp.AppendHeader("Test-Header", "value1")
		resp.AppendHeader("Test-Header", "value2")
		resp.AppendHeader("Test-Header", "value3")

		assert.Equal(t, "value1,value2,value3", resp.Headers.Get("Test-Header"))
	})
}
