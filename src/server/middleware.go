package server

// Middleware interface defines the contract for all middleware components
type Middleware interface {
	// OnStart is called when the server starts
	OnStart(server *Server)

	// OnStop is called when the server stops
	OnStop(server *Server)

	// OnRequest is called right after the request is received,
	// and before onResponse is called.
	OnRequest(ctx *RequestContext, next func())

	// OnResponse is called after all other OnRequest middlewares have been executed
	// and right before the response is sent to the client, so it can modify the response.
	OnResponse(ctx *RequestContext, next func())
}

type DefaultMiddleware struct{}

func (m *DefaultMiddleware) OnStart(server *Server)                      {}
func (m *DefaultMiddleware) OnStop(server *Server)                       {}
func (m *DefaultMiddleware) OnRequest(ctx *RequestContext, next func())  { next() }
func (m *DefaultMiddleware) OnResponse(ctx *RequestContext, next func()) { next() }

// MiddlewaresChain holds ordered lists of middleware for request and response phases
type MiddlewaresChain struct {
	middlewares []Middleware
}

// NewMiddlewaresChain creates a new middleware chain
func NewMiddlewaresChain() *MiddlewaresChain {
	return &MiddlewaresChain{
		middlewares: []Middleware{},
	}
}

// Adds a middleware to the processing chain
func (mc *MiddlewaresChain) Add(mw Middleware) {
	mc.middlewares = append(mc.middlewares, mw)
}

// Count returns the number of middlewares in the chain
func (mc *MiddlewaresChain) Count() int {
	return len(mc.middlewares)
}

// GetMiddleware returns the middleware at the specified index
func (mc *MiddlewaresChain) GetMiddleware(index int) Middleware {
	if index < 0 || index >= len(mc.middlewares) {
		return nil
	}
	return mc.middlewares[index]
}

// Execute runs all middlewares in the order they were added
func (mc *MiddlewaresChain) ExecuteOnStart(server *Server) {
	for index := 0; index < len(mc.middlewares); index++ {
		currentMiddleware := mc.middlewares[index]
		currentMiddleware.OnStart(server)
	}
}

// ExecuteOnStop executes OnStop middlewares in the chain
func (mc *MiddlewaresChain) ExecuteOnStop(server *Server) {
	for index := 0; index < len(mc.middlewares); index++ {
		currentMiddleware := mc.middlewares[index]
		currentMiddleware.OnStop(server)
	}
}

// executeChainOnRequest executes OnRequest middlewares in the chain
// NOTE: This could be recursion, but debugging for with index is way easier
// in profiler
func (mc *MiddlewaresChain) ExecuteOnRequest(ctx *RequestContext) {
	var executeMiddleware func(int)

	executeMiddleware = func(index int) {
		if index >= len(mc.middlewares) || ctx.ErrorStatus != 0 {
			return
		}

		currentMiddleware := mc.middlewares[index]
		currentMiddleware.OnRequest(ctx, func() {
			executeMiddleware(index + 1)
		})
	}

	executeMiddleware(0)
}

// executeChainOnResponse executes OnResponse middlewares in the chain
func (mc *MiddlewaresChain) ExecuteOnResponse(ctx *RequestContext) {
	var executeMiddleware func(int)

	executeMiddleware = func(index int) {
		if index >= len(mc.middlewares) {
			return
		}

		currentMiddleware := mc.middlewares[index]
		currentMiddleware.OnResponse(ctx, func() {
			executeMiddleware(index + 1)
		})
	}

	executeMiddleware(0)
}
