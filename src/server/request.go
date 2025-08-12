package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"strings"
)

type Request struct {
	Method          string
	URL             string
	Path            string
	Headers         http.Header
	Body            []byte
	Query           url.Values // Query parameters (e.g. map[string][]string{"q": {"test"}})
	Host            string     // Host header (e.g. ecommerce.com) or the one from X-Own-Host header
	Protocol        string     // HTTP protocol version (e.g., HTTP/1.1, HTTP/2)
	Scheme          string     // URL scheme (e.g. http, https)
	Port            string     // Port number (e.g. 80, 443)
	RemoteAddr      string     // Client's IP address (e.g. 192.168.1.1)
	OriginalRequest *http.Request
	OriginalHost    string // Original host (the first in x-forwarded-host header, e.g: ecommerce.com)
	OriginalScheme  string // Original scheme (the first in x-forwarded-proto header, e.g: http)
	OriginalPort    string // Original port (the first in x-forwarded-port header, e.g: 80)
	OriginalURL     string // Original URL (with default ports hidden, e.g: http://ecommerce.com/api/users)
}

// NewRequest creates a new Request from an http.Request
// if provided or default empty request if not provided
func NewRequest(httpReqs ...*http.Request) (*Request, error) {
	// Return default empty request if no httpReqs provided
	if len(httpReqs) == 0 {
		return &Request{
			Headers:         make(http.Header),
			Body:            nil,
			Query:           url.Values{},
		}, nil
	}
	httpReq := httpReqs[0]

	// Handle nil request
	if httpReq == nil {
		return nil, nil
	}

	// Check if the request context is cancelled (client disconnected)
	if httpReq.Context().Err() != nil {
		logger.Debug("Request context cancelled, client disconnected: %v", httpReq.Context().Err())
		return nil, nil
	}

	// Get the host header from X-Own-Host first, then try Host header
	// NOTE: Some CDN providers such as Cloudflare doesn't allow to change the Host header to any values.
	// It often requires at least Enterprise tier but setting any other is allowed in in the free tier.
	// So we use X-Own-Host header to pass the host to the proxy and then use it in the response.
	// e.g: https://<project-slug>.<environment-slug>-<optional-deployment-id>.<cloud-backend-slug>.<organization-slug>.ownstak.link
	// e.g: https://nextjs-app-prod-123.aws-primary.my-org.ownstak.link
	receivedHost := httpReq.Host
	host := httpReq.Header.Get(HeaderXOwnHost)
	if host == "" {
		// If X-Own-Host header isn't present, use the Host header
		host = receivedHost
	}

	// Parse query parameters directly from the URL
	queryParams := httpReq.URL.Query()

	// Determine scheme (http or https)
	scheme := "http"
	if httpReq.TLS != nil || httpReq.URL.Scheme == "https" {
		scheme = "https"
	}

	// Extract port from host or use default
	port := ""
	if receivedHost != "" {
		hostPortParts := strings.Split(receivedHost, ":")
		if len(hostPortParts) > 1 {
			port = hostPortParts[1]
		}
	}
	// Use default ports based on scheme if port isn't present
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	// Get protocol version
	protocol := ""
	if httpReq.ProtoMajor == 1 {
		if httpReq.ProtoMinor == 0 {
			protocol = "HTTP/1.0"
		} else {
			protocol = "HTTP/1.1"
		}
	} else if httpReq.ProtoMajor == 2 {
		protocol = "HTTP/2.0"
	} else {
		protocol = httpReq.Proto
	}

	// Get remote address (client IP)
	remoteAddr := httpReq.RemoteAddr
	if remoteAddr != "" {
		// e.g. [2001:db8:85a3::1]:443
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			// If SplitHostPort fails, try to parse as IP directly
			if ip := net.ParseIP(remoteAddr); ip != nil {
				remoteAddr = ip.String()
			} else {
				// If all parsing fails, just use the received address
				remoteAddr = remoteAddr
			}
		} else {
			remoteAddr = host
		}
	}

	// Just copy the headers from the received request
	headers := make(http.Header)
	for key, values := range httpReq.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}

	// Set always present headers
	// NOTE: Host header is special case. Go's net/http library
	// will automatically extract it as httpReq.Host property and it's not present in httpReq.Header.
	// That's why we manually set it here.
	headers.Set(HeaderHost, host)
	headers.Set(HeaderXOwnProxy, "true")
	headers.Set(HeaderXOwnProxyVersion, constants.Version)

	// Append the received X-Forwarded-For header to the new request
	// or set the remote address if no X-Forwarded-For header is present
	if xForwardedFor := httpReq.Header.Get(HeaderXForwardedFor); xForwardedFor != "" {
		headers.Set(HeaderXForwardedFor, fmt.Sprintf("%s, %s", xForwardedFor, remoteAddr))
	} else {
		headers.Set(HeaderXForwardedFor, remoteAddr)
	}

	// Append the received X-Forwarded-Proto header to the new request
	// or set the scheme if no X-Forwarded-Proto header is present
	if xForwardedProto := httpReq.Header.Get(HeaderXForwardedProto); xForwardedProto != "" {
		headers.Set(HeaderXForwardedProto, fmt.Sprintf("%s, %s", xForwardedProto, scheme))
	} else {
		headers.Set(HeaderXForwardedProto, scheme)
	}

	// Append the received X-Forwarded-Port header to the new request
	// or set the port if no X-Forwarded-Port header is present
	if xForwardedPort := httpReq.Header.Get(HeaderXForwardedPort); xForwardedPort != "" {
		headers.Set(HeaderXForwardedPort, fmt.Sprintf("%s, %s", xForwardedPort, port))
	} else {
		headers.Set(HeaderXForwardedPort, port)
	}

	// Append the x-forwarded-host header
	// or set the received host if no x-forwarded-host header is present
	if receivedHost != "" {
		if xForwardedHost := httpReq.Header.Get(HeaderXForwardedHost); xForwardedHost != "" {
			headers.Set(HeaderXForwardedHost, fmt.Sprintf("%s, %s", xForwardedHost, receivedHost))
		} else {
			headers.Set(HeaderXForwardedHost, receivedHost)
		}
	}

	// The received host, port and scheme are not neccessary the original hosts,
	// that the client sent. They can be changed by other proxies on the way.
	// That's why we also parse them from the x-forwarded-* headers.
	// e.g. get first value from x-forwarded-host header
	originalHost := host // Default to current host
	if xForwardedHost := headers.Get(HeaderXForwardedHost); xForwardedHost != "" {
		originalHost = strings.Split(xForwardedHost, ",")[0]
	}
	originalScheme := scheme // Default to current scheme
	if xForwardedProto := headers.Get(HeaderXForwardedProto); xForwardedProto != "" {
		originalScheme = strings.Split(xForwardedProto, ",")[0]
	}
	originalPort := port // Default to current port
	if xForwardedPort := headers.Get(HeaderXForwardedPort); xForwardedPort != "" {
		originalPort = strings.Split(xForwardedPort, ",")[0]
	}
	originalURL := fmt.Sprintf("%s://%s%s", originalScheme, originalHost, httpReq.URL.Path)
	if httpReq.URL.RawQuery != "" {
		// Only add query string if it exists
		originalURL += "?" + httpReq.URL.RawQuery
	}

	// Read the body of the request
	var body []byte
	if httpReq.Body != nil {
		var err error
		body, err = io.ReadAll(httpReq.Body)
		defer httpReq.Body.Close()
		
		if err != nil {
			if err == io.EOF {
				// io.EOF signals a graceful end of input, just ignore it and keep the body empty
				body = []byte{}
			} else if err == io.ErrUnexpectedEOF {
				// io.ErrUnexpectedEOF signals that the client is gone while reading the body,
				// so we return nil for both req and err and server will stop handling the request
				return nil, nil
			} else if strings.Contains(err.Error(), "connection reset") {
				// Client disconnected, return nil for both req and err
				logger.Debug("Client disconnected during request body read: %v", err)
				return nil, nil
			} else {
				// Real error, return it
				return nil, err
			}
		}
	}

	return &Request{
		Method:          httpReq.Method,
		URL:             httpReq.URL.String(),
		Path:            httpReq.URL.Path,
		Host:            host,
		Headers:         headers,
		Body:            body,
		Query:           queryParams,
		Protocol:        protocol,
		Scheme:          scheme,
		Port:            port,
		RemoteAddr:      remoteAddr,
		OriginalRequest: httpReq,
		OriginalHost:    originalHost,
		OriginalScheme:  originalScheme,
		OriginalPort:    originalPort,
		OriginalURL:     originalURL,
	}, nil
}

// Context returns a background context for the request
func (req *Request) Context() context.Context {
	return req.OriginalRequest.Context()
}
