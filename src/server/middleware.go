package server

// Middleware interface defines the contract for all middleware components
type Middleware interface {
	// OnRequest is called right after the request is received,
	// and before onResponse is called.
	OnRequest(ctx *RequestContext, next func())

	// OnResponse is called after all other OnRequest middlewares have been executed
	// and right before the response is sent to the client, so it can modify the response.
	OnResponse(ctx *RequestContext, next func())
}

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

// Execute runs all middlewares in the order they were added
func (mc *MiddlewaresChain) Execute(ctx *RequestContext) {
	// Execute OnRequest middlewares
	mc.executeChainOnRequest(ctx)

	// Execute OnResponse middlewares
	mc.executeChainOnResponse(ctx)
}

// executeChainOnRequest executes OnRequest middlewares in the chain
// NOTE: This could be recursion, but debugging for with index is way easier
// in profiler
func (mc *MiddlewaresChain) executeChainOnRequest(ctx *RequestContext) {
	for index := 0; index < len(mc.middlewares); index++ {
		currentMiddleware := mc.middlewares[index]

		// Execute OnRequest with next middleware
		stop := true
		currentMiddleware.OnRequest(ctx, func() {
			// Continue to next middleware
			stop = false
		})

		// If stop flag is set or there's an error, exit the loop
		if stop || ctx.ErrorStatus != 0 {
			break
		}
	}
}

// executeChainOnResponse executes OnResponse middlewares in the chain
func (mc *MiddlewaresChain) executeChainOnResponse(ctx *RequestContext) {
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
