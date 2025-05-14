package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"ownstak-proxy/src/constants"
	"strings"
)

type Request struct {
	Method          string
	URL             string
	Path            string
	Headers         http.Header
	Host            string
	Body            []byte
	Query           url.Values
	Protocol        string // HTTP protocol version (e.g., HTTP/1.1, HTTP/2)
	Scheme          string // URL scheme (http, https)
	Port            string // Port number
	RemoteAddr      string // Client's IP address and port
	OriginalRequest *http.Request
}

// NewRequest creates a new Request from an http.Request
func NewRequest(httpReq *http.Request) (*Request, error) {
	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		return nil, err
	}
	defer httpReq.Body.Close()

	// Get the host header from X-Own-Host first, then try Host header
	// NOTE: Some CDN providers such as Cloudflare doesn't allow to change the Host header to any values.
	// It often requires at least Enterprise tier but setting any other is allowed in in the free tier.
	// So we use X-Own-Host header to pass the host to the proxy and then use it in the response.
	// e.g: https://<project-slug>.<environment-slug>-<optional-deployment-id>.<cloud-backend-slug>.<organization-slug>.ownstak.link
	// e.g: https://nextjs-app-prod-123.aws-primary.my-org.ownstak.link
	host := httpReq.Header.Get(HeaderXOwnHost)
	if host == "" {
		host = httpReq.Header.Get(HeaderHost)
	}
	if host == "" {
		host = httpReq.Host
	}

	// Parse query parameters directly from the URL
	queryParams := httpReq.URL.Query()

	// Determine scheme (http or https)
	scheme := "http"
	if httpReq.TLS != nil {
		scheme = "https"
	} else if forwardedProto := httpReq.Header.Get(HeaderXForwardedProto); forwardedProto != "" {
		// If there are multiple forwarded protocols, use the last one
		if commaIdx := strings.LastIndex(forwardedProto, ","); commaIdx != -1 {
			scheme = strings.TrimSpace(forwardedProto[commaIdx+1:])
		} else {
			scheme = forwardedProto
		}
	}

	// Extract port from host or use default
	port := ""
	if hostPort := httpReq.Header.Get(HeaderXForwardedPort); hostPort != "" {
		// If there are multiple forwarded ports, use the last one
		if commaIdx := strings.LastIndex(hostPort, ","); commaIdx != -1 {
			port = strings.TrimSpace(hostPort[commaIdx+1:])
		} else {
			port = hostPort
		}
	} else {
		// Use default ports based on scheme
		hostPort := httpReq.Host
		if hostPort != "" {
			hostPortParts := strings.Split(hostPort, ":")
			if len(hostPortParts) > 1 {
				port = hostPortParts[1]
			}
		} else {
			if scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
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
	if xForwardedFor := httpReq.Header.Get(HeaderXForwardedFor); xForwardedFor != "" {
		// Use the leftmost IP which is the original client
		remoteAddr = strings.Split(xForwardedFor, ",")[0]
		remoteAddr = strings.TrimSpace(remoteAddr)
	}
	// Remove port from remoteAddr if present
	remoteAddr = strings.Split(remoteAddr, ":")[0]

	headers := make(http.Header)
	for key, values := range httpReq.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	headers.Set(HeaderHost, host)
	headers.Set(HeaderXOwnProxy, "true")
	headers.Set(HeaderXOwnProxyVersion, constants.Version)

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
	}, nil
}

// Context returns a background context for the request
func (req *Request) Context() context.Context {
	return req.OriginalRequest.Context()
}
