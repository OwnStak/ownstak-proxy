package server

import (
	"encoding/json"
	"fmt"
	"html"
	"ownstak-proxy/src/constants"
	"strings"
)

// RequestContext encapsulates the request, response, and shared resources
// for a single HTTP request lifecycle
type RequestContext struct {
	Request     *Request
	Response    *Response
	Server      *Server
	ErrorMesage string
	ErrorStatus int
}

// NewRequestContext creates a new context for a request/response pair
func NewRequestContext(req *Request, res *Response, server *Server) *RequestContext {
	return &RequestContext{
		Request:     req,
		Response:    res,
		Server:      server,
		ErrorMesage: "",
		ErrorStatus: 0,
	}
}

// Error sets an error response with the given message and status code to the current context and returns the response.
func (ctx *RequestContext) Error(errorMessage string, errorStatus int) {
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

	requestId := ctx.Request.Headers.Get(HeaderRequestID)

	// If client accepts HTML, return HTML error
	// Otherwise, return JSON error
	if strings.Contains(accept, ContentTypeHTML) {
		ctx.Response.Headers.Set(HeaderContentType, ContentTypeHTML)
		ctx.Response.Body = []byte(ToHtmlErrorBody(errorMessage, errorStatus, requestId))
	} else {
		ctx.Response.Headers.Set(HeaderContentType, ContentTypeJSON)
		ctx.Response.Body = []byte(ToJsonErrorBody(errorMessage, errorStatus, requestId))
	}
}

func ToHtmlErrorBody(errorMessage string, errorCode int, requestId string) string {
	// Escape special characters to prevent XSS from the messages/header values.
	errorSegments := strings.Split(errorMessage, ": ")
	errorMessage = errorSegments[0]
	errorStackTrace := strings.Join(errorSegments[1:], ": ")

	errorMessage = html.EscapeString(errorMessage)
	errorStackTrace = html.EscapeString(errorStackTrace)

	// Return the HTML error body
	return fmt.Sprintf(`
		<html>
			<head>
				<title>Error %d</title>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
				<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Poppins:wght@400;700&display=swap">
				<style>
					html, body, div, span, applet, object, iframe,
					h1, h2, h3, h4, h5, h6, p, blockquote, pre,
					a, abbr, acronym, address, big, cite, code,
					del, dfn, em, img, ins, kbd, q, s, samp,
					small, strike, strong, sub, sup, tt, var,
					b, u, i, center,
					dl, dt, dd, ol, ul, li,
					fieldset, form, label, legend,
					table, caption, tbody, tfoot, thead, tr, th, td,
					article, aside, canvas, details, embed, 
					figure, figcaption, footer, header, hgroup, 
					menu, nav, output, ruby, section, summary,
					time, mark, audio, video {
						margin: 0;
						padding: 0;
						border: 0;
						font-size: 1;
						font: inherit;
						vertical-align: baseline;
					}
					article, aside, details, figcaption, figure, 
					footer, header, hgroup, menu, nav, section {
						display: block;
					}
					body {
						font-family: 'Poppins', sans-serif;
						background:
							radial-gradient(ellipse 60%% 40%% at 0%% 100%%, rgba(231, 76, 60, 0.07) 0%%, rgba(231, 76, 60, 0.09) 60%%, rgba(255,255,255,0) 100%%),
							radial-gradient(ellipse 40%% 30%% at 100%% 0%%, rgba(231, 76, 60, 0.07) 0%%, rgba(231, 76, 60, 0.05) 60%%, rgba(255,255,255,0) 100%%),
							#fff;
						color: #222;
						margin: 0;
						padding: 0;
						display: flex;
						justify-content: center;
						align-items: center;
						height: 100vh;
						white-space: pre-wrap;
						text-align: center;
					}
					.container {
						max-width: 600px;
						padding: 1em 2em;
						margin: 0 auto;
					}
					h1 {
						font-size: 10em;
						font-weight: 900;
						margin: 0;
						color: #e74c3c;
						background: linear-gradient(45deg, rgba(231, 76, 60, 0.5) 0%%, rgba(231, 76, 60, 1) 50%%, rgba(231, 76, 60, 1) 60%%, rgba(231, 76, 60, 0.5) 100%%);
						-webkit-background-clip: text;
						background-clip: text;
						color: transparent;
					}
					h2 {
						font-size: 1.15em;
						line-height: 1.6;
						font-weight: 700;
						margin: -1.25em 0 2.5em 0;
						color: rgba(0, 0, 0, 0.8);
					}
					a {
						color: rgba(231, 76, 60, 0.8);
						text-decoration: none;
					}
					a:hover {
						color: rgba(231, 76, 60, 1);
					}
					p {
						font-size: 1.2em;
						margin: 0.5em 0;
						line-height: 1.5;

					}
					footer {
						font-size: 0.9em;
						margin: 1em auto;
						padding-top: 2em;
						color: rgba(0, 0, 0, 0.75);
						position: relative;
						max-width: 400px;
					}
					footer::before {
						content: "";
						width:60%%;
						position: absolute;
						top: 0;
						left: 50%%;
						transform: translate(-50%%, 0);
						height:1px;
						background-color: rgba(0, 0, 0, 0.35);
						display: block;
					}

					/* Error Table/Card Styles */
					.error-table {
						display: flex;
						flex-direction: column;
						align-items: stretch;
						margin: 0 auto 1.5em auto;
						max-width: 100%%;
						border-radius: 0.7em;
						box-shadow: 0 2px 12px 0 rgba(231,76,60,0.08);
						border: 1px solid rgba(0,0,0,0.3);
						background: rgba(255,255,255,0.5);
						backdrop-filter: blur(5px);
						overflow: hidden;
					}
					.error-table-header {
						font-weight: 400;
						padding: 0.7em 1.2em 0.6em 1.2em;
						font-size: 1.1em;
						border-bottom: 1px solid rgba(0,0,0,0.15);
						text-align: left;
						color: rgba(0, 0, 0, 0.8);
						background: rgba(255,255,255,0.95);
					}
					.error-table-header i {
						color: #e74c3c;
					}
					.error-table-body {
						padding: 1.2em;
						background: none;
					}
					.error-table-body code {
						background: none;
						border: none;
						padding: 0;
						font-size: 1em;
						color: rgba(0, 0, 0, 0.8);
						font-family: 'Courier New', Courier, monospace;
						white-space: wrap;
						word-break: break-word;
						display: block;
						max-height: 95px;
						overflow-y: auto;
						overflow-x: hidden;
						text-align: left;
					}
					.error-table-footer {
						padding: 0.7em 1.2em;
						background: rgba(255,255,255,0.95);
						border-top: 1px solid rgba(0,0,0,0.13);
						color: rgba(0, 0, 0, 0.6);
						font-size: 0.95em;
						text-align: left;
						white-space: wrap;
						word-break: break-word;
						line-height: 1.5;
					}
				</style>
			</head>
			<body>
				<div class="container">
					<h1>%d</h1><h2>Oops! This site is experiencing problems serving your request. If you are the site administrator, please see the error details below or contact <a href="%s">OwnStak support</a> for assistance.</h2><div class="error-table"><div class="error-table-header"><i>Error:</i> %s</div><div class="error-table-body"><code>%s</code></div><div class="error-table-footer">Request ID: %s<br>Version: %s %s</div></div>
				</div>
			</body>
		</html>
	`, errorCode, errorCode, constants.SupportURL, errorMessage, errorStackTrace, requestId, constants.AppName, constants.Version)
}

func ToJsonErrorBody(errorMessage string, errorCode int, requestId string) string {
	jsonData, err := json.MarshalIndent(map[string]interface{}{
		"errorStatus":  errorCode,
		"errorMessage": errorMessage,
		"requestId":    requestId,
	}, "", "  ") // Use 2 spaces for indentation

	if err != nil {
		// Handle error case - return simple string in worst case
		return fmt.Sprintf(`{"errorStatus":%d,"errorMessage":"Error marshaling JSON: %s"}`,
			errorCode, err.Error())
	}

	return string(jsonData)
}

// Stores the debug info for given request context.
// The value is outputed in the response header x-own-proxy-debug when requested.
// Returns true if the header was appended, false otherwise
// @example: ctx.Debug("lambda-duration="+invocationDuration.String())
func (ctx *RequestContext) Debug(value string) bool {
	if ctx.Request.Headers.Get(HeaderXOwnDebug) == "" && ctx.Request.Headers.Get(HeaderXOwnProxyDebug) == "" {
		return false
	}

	ctx.Response.AppendHeader(HeaderXOwnProxyDebug, value)
	return true
}