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
	mc.executeChain(mc.middlewares, 0, ctx, func(middleware Middleware, ctx *ServerContext, next func()) {
		middleware.OnRequest(ctx, next)
	})

	mc.executeChain(mc.middlewares, 0, ctx, func(middleware Middleware, ctx *ServerContext, next func()) {
		middleware.OnResponse(ctx, next)
	})
}

// executeChain executes a chain of middlewares starting at the given index
func (mc *MiddlewareChain) executeChain(
	chain []Middleware,
	index int,
	ctx *ServerContext,
	executor func(Middleware, *ServerContext, func())) {

	// If we've reached the end of the chain, return
	if index >= len(chain) {
		return
	}

	// Get the current middleware
	current := chain[index]

	// Call the middleware with a next function that calls the next middleware in the chain
	executor(current, ctx, func() {
		mc.executeChain(chain, index+1, ctx, executor)
	})
}
