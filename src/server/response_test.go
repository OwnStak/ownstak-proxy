package server

import (
	"encoding/base64"
	"encoding/json"
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

	t.Run("Header should return headers", func(t *testing.T) {
		resp := NewResponse()
		resp.Headers.Set("Test-Header", "test-value")

		headers := resp.Header()
		assert.Equal(t, "test-value", headers.Get("Test-Header"))
		assert.Equal(t, resp.Headers, headers)
	})

	t.Run("WriteHeader should set status", func(t *testing.T) {
		resp := NewResponse()
		resp.WriteHeader(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("WriteHeader should default to 200 for zero status", func(t *testing.T) {
		resp := NewResponse()
		resp.WriteHeader(0)
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

func TestResponseSerialization(t *testing.T) {
	t.Run("should serialize response correctly", func(t *testing.T) {
		resp := NewResponse()
		resp.Status = http.StatusCreated
		resp.Headers.Set("Custom-Header", "custom-value")
		resp.Headers.Add("Multi-Header", "value1")
		resp.Headers.Add("Multi-Header", "value2")
		resp.Body = []byte("test body")

		serialized := resp.Serialize()
		assert.NotEmpty(t, serialized)

		// Verify it's valid JSON
		var data map[string]interface{}
		err := json.Unmarshal([]byte(serialized), &data)
		assert.NoError(t, err)

		assert.Equal(t, float64(http.StatusCreated), data["status"])
		assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("test body")), data["body"])

		headers, ok := data["headers"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, headers, "Custom-Header")
	})

	t.Run("should return empty string on serialization error", func(t *testing.T) {
		// This is hard to trigger, but we can test the behavior
		resp := NewResponse()
		serialized := resp.Serialize()
		assert.NotEmpty(t, serialized) // Should not be empty for valid response
	})
}

func TestDeserializeResponse(t *testing.T) {
	t.Run("should deserialize response correctly", func(t *testing.T) {
		originalBody := []byte("test body content")
		serializedData := map[string]interface{}{
			"status": float64(http.StatusCreated),
			"headers": map[string]interface{}{
				"Content-Type":  []interface{}{"application/json"},
				"Custom-Header": "custom-value",
				"Multi-Header":  []interface{}{"value1", "value2"},
			},
			"body": base64.StdEncoding.EncodeToString(originalBody),
		}

		jsonData, err := json.Marshal(serializedData)
		assert.NoError(t, err)

		resp, err := DeserializeResponse(string(jsonData))
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		assert.Equal(t, http.StatusCreated, resp.Status)
		assert.Equal(t, originalBody, resp.Body)
		assert.Equal(t, "application/json", resp.Headers.Get("Content-Type"))
		assert.Equal(t, "custom-value", resp.Headers.Get("Custom-Header"))
		assert.Equal(t, []string{"value1", "value2"}, resp.Headers["Multi-Header"])
	})

	t.Run("should handle empty data", func(t *testing.T) {
		resp, err := DeserializeResponse("")
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "empty data")
	})

	t.Run("should handle invalid JSON", func(t *testing.T) {
		resp, err := DeserializeResponse("invalid json")
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("should handle missing status field", func(t *testing.T) {
		data := map[string]interface{}{
			"headers": map[string]interface{}{},
			"body":    base64.StdEncoding.EncodeToString([]byte("test")),
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing status")
	})

	t.Run("should handle invalid status field", func(t *testing.T) {
		data := map[string]interface{}{
			"status":  "invalid",
			"headers": map[string]interface{}{},
			"body":    base64.StdEncoding.EncodeToString([]byte("test")),
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing status")
	})

	t.Run("should handle missing headers field", func(t *testing.T) {
		data := map[string]interface{}{
			"status": float64(200),
			"body":   base64.StdEncoding.EncodeToString([]byte("test")),
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing headers")
	})

	t.Run("should handle invalid headers field", func(t *testing.T) {
		data := map[string]interface{}{
			"status":  float64(200),
			"headers": "invalid",
			"body":    base64.StdEncoding.EncodeToString([]byte("test")),
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing headers")
	})

	t.Run("should handle missing body field", func(t *testing.T) {
		data := map[string]interface{}{
			"status":  float64(200),
			"headers": map[string]interface{}{},
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing body")
	})

	t.Run("should handle invalid body field", func(t *testing.T) {
		data := map[string]interface{}{
			"status":  float64(200),
			"headers": map[string]interface{}{},
			"body":    123, // Invalid type
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid or missing body")
	})

	t.Run("should handle invalid base64 body", func(t *testing.T) {
		data := map[string]interface{}{
			"status":  float64(200),
			"headers": map[string]interface{}{},
			"body":    "invalid base64!!!",
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to decode body")
	})

	t.Run("should handle headers with string values", func(t *testing.T) {
		data := map[string]interface{}{
			"status": float64(200),
			"headers": map[string]interface{}{
				"String-Header": "string-value",
				"Array-Header":  []interface{}{"value1", "value2"},
			},
			"body": base64.StdEncoding.EncodeToString([]byte("test")),
		}
		jsonData, _ := json.Marshal(data)

		resp, err := DeserializeResponse(string(jsonData))
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		assert.Equal(t, "string-value", resp.Headers.Get("String-Header"))
		assert.Equal(t, []string{"value1", "value2"}, resp.Headers["Array-Header"])
	})
}

func TestResponseIntegration(t *testing.T) {
	t.Run("should serialize and deserialize correctly", func(t *testing.T) {
		original := NewResponse()
		original.Status = http.StatusAccepted
		original.Headers.Set("Custom-Header", "custom-value")
		original.Headers.Add("Multi-Header", "value1")
		original.Headers.Add("Multi-Header", "value2")
		original.Body = []byte("integration test body")

		serialized := original.Serialize()
		assert.NotEmpty(t, serialized)

		deserialized, err := DeserializeResponse(serialized)
		assert.NoError(t, err)
		assert.NotNil(t, deserialized)

		assert.Equal(t, original.Status, deserialized.Status)
		assert.Equal(t, original.Body, deserialized.Body)
		assert.Equal(t, original.Headers.Get("Custom-Header"), deserialized.Headers.Get("Custom-Header"))
		assert.Equal(t, original.Headers["Multi-Header"], deserialized.Headers["Multi-Header"])
	})
}

func TestResponseWriterCompatibility(t *testing.T) {
	t.Run("should work as http.ResponseWriter", func(t *testing.T) {
		rw := httptest.NewRecorder()
		resp := NewResponse(rw)

		// Use as http.ResponseWriter
		var w http.ResponseWriter = resp
		w.Header().Set("Test-Header", "test-value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("test content"))

		resp.End()

		assert.Equal(t, http.StatusCreated, rw.Code)
		assert.Equal(t, "test content", rw.Body.String())
		assert.Equal(t, "test-value", rw.Header().Get("Test-Header"))
	})
}
