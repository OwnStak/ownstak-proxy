package server

// Middleware interface defines the contract for all middleware components
type Middleware interface {
	// OnRequest is called right after the request is received,
	// and before onResponse is called.
	OnRequest(ctx *ServerContext, next func())

	// OnResponse is called after all other OnRequest middlewares have been executed
	// and right before the response is sent to the client, so it can modify the response.
	OnResponse(ctx *ServerContext, next func())
}

// MiddlewareChain holds ordered lists of middleware for request and response phases
type MiddlewareChain struct {
	middlewares []Middleware
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: []Middleware{},
	}
}

// Use adds a middleware to the processing chain
func (mc *MiddlewareChain) Use(mw Middleware) {
	mc.middlewares = append(mc.middlewares, mw)
}

// Execute runs all middlewares in the order they were added
func (mc *MiddlewareChain) Execute(ctx *ServerContext) {
	// Execute OnRequest middlewares
	mc.executeChainOnRequest(ctx)

	// Execute OnResponse middlewares
	mc.executeChainOnResponse(ctx)
}

// executeChainOnRequest executes OnRequest middlewares in the chain
// NOTE: This could be recursion, but debugging for with index is way easier
// in profiler
func (mc *MiddlewareChain) executeChainOnRequest(ctx *ServerContext) {
	for index := 0; index < len(mc.middlewares); index++ {
		currentMiddleware := mc.middlewares[index]

		// Execute OnRequest with next middleware
		stop := true
		currentMiddleware.OnRequest(ctx, func() {
			// Continue to next middleware
			stop = false
		})

		// If stop flag is set, exit the loop
		if stop {
			break
		}
	}
}

// executeChainOnResponse executes OnResponse middlewares in the chain
func (mc *MiddlewareChain) executeChainOnResponse(ctx *ServerContext) {
	for index := 0; index < len(mc.middlewares); index++ {
		currentMiddleware := mc.middlewares[index]

		// Execute OnResponse with next middleware
		stop := true
		currentMiddleware.OnResponse(ctx, func() {
			// Continue to next middleware
			stop = false
		})

		// If stop flag is set, exit the loop
		if stop {
			break
		}
	}
}
