package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"ownstak-proxy/src/constants"
	"strings"
)

type ServerRequest struct {
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

// NewServerRequest creates a new serverRequest from an http.Request
func NewServerRequest(r *http.Request) (*ServerRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// Get the host header from X-Own-Host first, then try Host header
	// NOTE: Some CDN providers such as Cloudflare doesn't allow to change the Host header to any values.
	// It often requires at least Enterprise tier but setting any other is allowed in in the free tier.
	// So we use X-Own-Host header to pass the host to the proxy and then use it in the response.
	// e.g: https://<project-slug>.<environment-slug>-<optional-deployment-id>.<cloud-backend-slug>.<organization-slug>.ownstak.link
	// e.g: https://nextjs-app-prod-123.aws-primary.my-org.ownstak.link
	host := r.Header.Get(HeaderXOwnHost)
	if host == "" {
		host = r.Header.Get(HeaderHost)
	}
	if host == "" {
		host = r.Host
	}

	// Parse query parameters directly from the URL
	queryParams := r.URL.Query()

	// Determine scheme (http or https)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if forwardedProto := r.Header.Get(HeaderXForwardedProto); forwardedProto != "" {
		// If there are multiple forwarded protocols, use the last one
		if commaIdx := strings.LastIndex(forwardedProto, ","); commaIdx != -1 {
			scheme = strings.TrimSpace(forwardedProto[commaIdx+1:])
		} else {
			scheme = forwardedProto
		}
	}

	// Extract port from host or use default
	port := ""
	if hostPort := r.Header.Get(HeaderXForwardedPort); hostPort != "" {
		// If there are multiple forwarded ports, use the last one
		if commaIdx := strings.LastIndex(hostPort, ","); commaIdx != -1 {
			port = strings.TrimSpace(hostPort[commaIdx+1:])
		} else {
			port = hostPort
		}
	} else {
		// Use default ports based on scheme
		hostPort := r.Host
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
	if r.ProtoMajor == 1 {
		if r.ProtoMinor == 0 {
			protocol = "HTTP/1.0"
		} else {
			protocol = "HTTP/1.1"
		}
	} else if r.ProtoMajor == 2 {
		protocol = "HTTP/2.0"
	} else {
		protocol = r.Proto
	}

	// Get remote address (client IP)
	remoteAddr := r.RemoteAddr
	if xForwardedFor := r.Header.Get(HeaderXForwardedFor); xForwardedFor != "" {
		// Use the leftmost IP which is the original client
		remoteAddr = strings.Split(xForwardedFor, ",")[0]
		remoteAddr = strings.TrimSpace(remoteAddr)
	}
	// Remove port from remoteAddr if present
	remoteAddr = strings.Split(remoteAddr, ":")[0]

	headers := make(http.Header)
	for key, values := range r.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	headers.Set(HeaderHost, host)
	headers.Set(HeaderXOwnProxy, "true")
	headers.Set(HeaderXOwnProxyVersion, constants.Version)

	return &ServerRequest{
		Method:          r.Method,
		URL:             r.URL.String(),
		Path:            r.URL.Path,
		Host:            host,
		Headers:         headers,
		Body:            body,
		Query:           queryParams,
		Protocol:        protocol,
		Scheme:          scheme,
		Port:            port,
		RemoteAddr:      remoteAddr,
		OriginalRequest: r,
	}, nil
}

// Context returns a background context for the request
func (r *ServerRequest) Context() context.Context {
	return r.OriginalRequest.Context()
}
