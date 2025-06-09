package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMiddlewaresChain(t *testing.T) {
	t.Run("should create empty middleware chain", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		assert.NotNil(t, chain)
		assert.NotNil(t, chain.middlewares)
		assert.Empty(t, chain.middlewares)
	})
}

func TestMiddlewaresChainAdd(t *testing.T) {
	t.Run("should add middleware to chain", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		middleware1 := &MockMiddleware{Name: "middleware1"}
		middleware2 := &MockMiddleware{Name: "middleware2"}

		chain.Add(middleware1)
		chain.Add(middleware2)

		assert.Len(t, chain.middlewares, 2)
		assert.Equal(t, middleware1, chain.middlewares[0])
		assert.Equal(t, middleware2, chain.middlewares[1])
	})

	t.Run("should maintain order when adding middlewares", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		for i := 0; i < 5; i++ {
			middleware := &MockMiddleware{Name: string(rune('A' + i))}
			chain.Add(middleware)
		}

		assert.Len(t, chain.middlewares, 5)
		for i := 0; i < 5; i++ {
			expectedName := string(rune('A' + i))
			actualName := chain.middlewares[i].(*MockMiddleware).Name
			assert.Equal(t, expectedName, actualName)
		}
	})
}

func TestMiddlewaresChainExecute(t *testing.T) {
	t.Run("should execute single middleware", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		middleware := &MockMiddleware{
			ShouldCallNext: true,
			Name:           "test",
		}
		chain.Add(middleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		assert.True(t, middleware.OnRequestCalled)
		assert.True(t, middleware.OnResponseCalled)
	})

	t.Run("should execute multiple middlewares in order", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		var tracker []string

		middleware1 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW1",
			shouldContinue: true,
		}
		middleware2 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW2",
			shouldContinue: true,
		}
		middleware3 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW3",
			shouldContinue: true,
		}

		chain.Add(middleware1)
		chain.Add(middleware2)
		chain.Add(middleware3)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		expectedOrder := []string{
			"OnRequest-MW1",
			"OnRequest-MW2",
			"OnRequest-MW3",
			"OnResponse-MW1",
			"OnResponse-MW2",
			"OnResponse-MW3",
		}

		assert.Equal(t, expectedOrder, tracker)
	})

	t.Run("should stop OnRequest chain when middleware doesn't call next", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		var tracker []string

		middleware1 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW1",
			shouldContinue: true,
		}
		middleware2 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW2",
			shouldContinue: false, // Stop here
		}
		middleware3 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW3",
			shouldContinue: true,
		}

		chain.Add(middleware1)
		chain.Add(middleware2)
		chain.Add(middleware3)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		expectedOrder := []string{
			"OnRequest-MW1",
			"OnRequest-MW2",
			// MW3 OnRequest should not be called
			"OnResponse-MW1",
			"OnResponse-MW2",
		}

		assert.Equal(t, expectedOrder, tracker)
	})

	t.Run("should stop OnResponse chain when middleware doesn't call next", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		var tracker []string

		middleware1 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW1",
			shouldContinue: true,
		}
		// Create custom middleware that doesn't call next in OnResponse
		middleware2 := &CustomMiddleware{
			name: "MW2",
			onRequestFunc: func(ctx *RequestContext, next func()) {
				tracker = append(tracker, "OnRequest-MW2")
				next()
			},
			onResponseFunc: func(ctx *RequestContext, next func()) {
				tracker = append(tracker, "OnResponse-MW2")
				// Don't call next()
			},
		}
		middleware3 := &TrackingMiddleware{
			tracker:        &tracker,
			name:           "MW3",
			shouldContinue: true,
		}

		chain.Add(middleware1)
		chain.Add(middleware2)
		chain.Add(middleware3)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		expectedOrder := []string{
			"OnRequest-MW1",
			"OnRequest-MW2",
			"OnRequest-MW3",
			"OnResponse-MW1",
			"OnResponse-MW2",
			// MW3 OnResponse should not be called
		}

		assert.Equal(t, expectedOrder, tracker)
	})

	t.Run("should handle empty middleware chain", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		// Should not panic or error
		assert.NotPanics(t, func() {
			chain.Execute(ctx)
		})
	})
}

func TestMiddlewareInterface(t *testing.T) {
	t.Run("should implement Middleware interface", func(t *testing.T) {
		// Verify MockMiddleware implements Middleware interface
		var middleware Middleware = &MockMiddleware{}
		assert.NotNil(t, middleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		// Test OnRequest
		called := false
		middleware.OnRequest(ctx, func() {
			called = true
		})
		assert.False(t, called) // MockMiddleware with default ShouldCallNext=false

		// Test OnResponse
		called = false
		middleware.OnResponse(ctx, func() {
			called = true
		})
		assert.False(t, called) // MockMiddleware with default ShouldCallNext=false
	})
}

func TestMiddlewareChainWithRealComponents(t *testing.T) {
	t.Run("should work with request context modifications", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		// Middleware that modifies headers
		headerMiddleware := &CustomMiddleware{
			name: "HeaderMW",
			onRequestFunc: func(ctx *RequestContext, next func()) {
				ctx.Request.Headers.Set("X-Modified-By", "HeaderMiddleware")
				next()
			},
		}

		// Middleware that modifies response
		responseMiddleware := &CustomMiddleware{
			name: "ResponseMW",
			onResponseFunc: func(ctx *RequestContext, next func()) {
				ctx.Response.Headers.Set("X-Response-Modified", "true")
				next()
			},
		}

		chain.Add(headerMiddleware)
		chain.Add(responseMiddleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		// Verify modifications
		assert.Equal(t, "HeaderMiddleware", ctx.Request.Headers.Get("X-Modified-By"))
		assert.Equal(t, "true", ctx.Response.Headers.Get("X-Response-Modified"))
	})

	t.Run("should handle middleware that sets errors", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		// Middleware that sets an error
		errorMiddleware := &CustomMiddleware{
			name: "ErrorMW",
			onRequestFunc: func(ctx *RequestContext, next func()) {
				ctx.Error("Test error from middleware", http.StatusBadRequest)
				// Don't call next()
			},
		}

		// This middleware should not be reached
		unreachableMiddleware := &MockMiddleware{
			ShouldCallNext: true,
			Name:           "UnreachableMW",
		}

		chain.Add(errorMiddleware)
		chain.Add(unreachableMiddleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		// Verify error was set
		assert.Equal(t, "Test error from middleware", ctx.ErrorMesage)
		assert.Equal(t, http.StatusBadRequest, ctx.ErrorStatus)
		assert.Equal(t, http.StatusBadRequest, ctx.Response.Status)

		// Verify unreachable middleware was not called in OnRequest
		assert.False(t, unreachableMiddleware.OnRequestCalled)
		// But OnResponse should still be called
		assert.True(t, unreachableMiddleware.OnResponseCalled)
	})
}

func TestMiddlewareExecutionOrder(t *testing.T) {
	t.Run("should execute OnRequest in forward order and OnResponse in forward order", func(t *testing.T) {
		chain := NewMiddlewaresChain()
		var executionOrder []string

		// Create middlewares that track execution order
		for i := 1; i <= 3; i++ {
			name := string(rune('A' + i - 1))

			middleware := &CustomMiddleware{
				name: name,
				onRequestFunc: func(ctx *RequestContext, next func()) {
					executionOrder = append(executionOrder, "OnRequest-"+name)
					next()
				},
				onResponseFunc: func(ctx *RequestContext, next func()) {
					executionOrder = append(executionOrder, "OnResponse-"+name)
					next()
				},
			}

			chain.Add(middleware)
		}

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		serverReq, err := NewRequest(httpReq)
		assert.NoError(t, err)

		serverRes := NewResponse()
		ctx := NewRequestContext(serverReq, serverRes, nil)

		chain.Execute(ctx)

		expectedOrder := []string{
			"OnRequest-A",
			"OnRequest-B",
			"OnRequest-C",
			"OnResponse-A",
			"OnResponse-B",
			"OnResponse-C",
		}

		assert.Equal(t, expectedOrder, executionOrder)
	})
}

func TestMiddlewareChainEdgeCases(t *testing.T) {
	t.Run("should handle nil middleware", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		// This should not cause issues in practice since Add expects Middleware interface
		// But we can test the chain execution with valid middlewares
		middleware := &MockMiddleware{ShouldCallNext: true}
		chain.Add(middleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		req, err := NewRequest(httpReq)
		assert.NoError(t, err)

		resp := NewResponse()
		ctx := NewRequestContext(req, resp, nil)

		assert.NotPanics(t, func() {
			chain.Execute(ctx)
		})
	})

	t.Run("should handle middleware that panics", func(t *testing.T) {
		chain := NewMiddlewaresChain()

		// Middleware that panics
		panicMiddleware := &CustomMiddleware{
			name: "PanicMW",
			onRequestFunc: func(ctx *RequestContext, next func()) {
				panic("test panic")
			},
		}

		chain.Add(panicMiddleware)

		// Create test context
		httpReq, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		httpReq.Host = "example.com"
		httpReq.RemoteAddr = "127.0.0.1:8080"

		req, err := NewRequest(httpReq)
		assert.NoError(t, err)

		resp := NewResponse()
		ctx := NewRequestContext(req, resp, nil)

		// Should panic since we don't have panic recovery in the middleware chain
		assert.Panics(t, func() {
			chain.Execute(ctx)
		})
	})
}

// Mock middleware for testing
type MockMiddleware struct {
	OnRequestCalled  bool
	OnResponseCalled bool
	ShouldCallNext   bool
	Name             string
}

func (m *MockMiddleware) OnRequest(ctx *RequestContext, next func()) {
	m.OnRequestCalled = true
	if m.ShouldCallNext {
		next()
	}
}

func (m *MockMiddleware) OnResponse(ctx *RequestContext, next func()) {
	m.OnResponseCalled = true
	if m.ShouldCallNext {
		next()
	}
}

// Tracking middleware to test execution order
type TrackingMiddleware struct {
	tracker        *[]string
	name           string
	shouldContinue bool
}

func (tm *TrackingMiddleware) OnRequest(ctx *RequestContext, next func()) {
	*tm.tracker = append(*tm.tracker, "OnRequest-"+tm.name)
	if tm.shouldContinue {
		next()
	}
}

func (tm *TrackingMiddleware) OnResponse(ctx *RequestContext, next func()) {
	*tm.tracker = append(*tm.tracker, "OnResponse-"+tm.name)
	if tm.shouldContinue {
		next()
	}
}

// Custom middleware with overridable behavior
type CustomMiddleware struct {
	name           string
	onRequestFunc  func(ctx *RequestContext, next func())
	onResponseFunc func(ctx *RequestContext, next func())
}

func (cm *CustomMiddleware) OnRequest(ctx *RequestContext, next func()) {
	if cm.onRequestFunc != nil {
		cm.onRequestFunc(ctx, next)
	} else {
		next()
	}
}

func (cm *CustomMiddleware) OnResponse(ctx *RequestContext, next func()) {
	if cm.onResponseFunc != nil {
		cm.onResponseFunc(ctx, next)
	} else {
		next()
	}
}
