package server

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// ServerContext encapsulates the request, response, and shared resources
// for a single HTTP request lifecycle
type ServerContext struct {
	Request     *ServerRequest
	Response    *ServerResponse
	Cache       *Cache
	Server      *Server
	ErrorMesage string
	ErrorStatus int
}

// NewServerContext creates a new context for a request/response pair
func NewServerContext(req *ServerRequest, res *ServerResponse, s *Server) *ServerContext {
	return &ServerContext{
		Request:     req,
		Response:    res,
		Cache:       s.cache,
		Server:      s,
		ErrorMesage: "",
		ErrorStatus: 0,
	}
}

// Error sets an error response with the given message and status code to the current context and returns the response.
func (ctx *ServerContext) Error(errorMessage string, errorStatus int) {
	// Set error information to context
	ctx.ErrorMesage = errorMessage
	ctx.ErrorStatus = errorStatus

	// Clear the current response and write the new error response
	ctx.Response.Clear()
	ctx.Response.Status = errorStatus

	// Get Accept header from request to determine response format
	accept := ctx.Request.Headers.Get(HeaderAccept)
	if accept == "" {
		accept = ContentTypeJSON
	}

	// If client accepts HTML, return HTML error
	// Otherwise, return JSON error
	if strings.Contains(accept, ContentTypeHTML) {
		ctx.Response.Body = []byte(ToHtmlErrorBody(errorMessage, errorStatus))
	} else {
		ctx.Response.Body = []byte(ToJsonErrorBody(errorMessage, errorStatus))
	}
}

func ToHtmlErrorBody(errorMessage string, errorCode int) string {
	// Escape special characters to prevent XSS from the messages/header values.
	errorMessage = html.EscapeString(errorMessage)
	errorMessage = strings.ReplaceAll(errorMessage, "\r\n", "<br>")

	// Return the HTML error body
	return fmt.Sprintf(`
		<html>
			<head>
				<title>Error %d</title>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
				<style>
					body {
						font-family: Arial, sans-serif;
						background-color: #f4f4f9;
						color: #333;
						margin: 0;
						padding: 0;
						display: flex;
						justify-content: center;
						align-items: center;
						height: 100vh;
						white-space: pre-wrap;
					}
					.container {
						text-align: center;
						max-width: 800px;
						padding: 20px;
					}
					h1 {
						font-size: 48px;
						margin: 0;
						color: #e74c3c;
					}
					p {
						font-size: 20px;
						margin: 10px 0;
						line-height: 1.5;
					}
				</style>
			</head>
			<body>
				<div class="container">
					<h1>Error %d</h1>
					<p>%s</p>
				</div>
			</body>
		</html>
	`, errorCode, errorCode, errorMessage)
}

func ToJsonErrorBody(errorMessage string, errorCode int) string {
	jsonData, err := json.MarshalIndent(map[string]interface{}{
		"errorStatus":  errorCode,
		"errorMessage": errorMessage,
	}, "", "  ") // Use 2 spaces for indentation

	if err != nil {
		// Handle error case - return simple string in worst case
		return fmt.Sprintf(`{"errorStatus":%d,"errorMessage":"Error marshaling JSON: %s"}`,
			errorCode, err.Error())
	}

	return string(jsonData)
}
