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
	Method     string
	URL        string
	Path       string
	Headers    http.Header
	Host       string
	Body       []byte
	Query      url.Values
	Protocol   string // HTTP protocol version (e.g., HTTP/1.1, HTTP/2)
	Scheme     string // URL scheme (http, https)
	Port       string // Port number
	RemoteAddr string // Client's IP address and port
}

// NewServerRequest creates a new serverRequest from an http.Request
func NewServerRequest(r *http.Request) (*ServerRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// If the forwarded host is set, use it instead of the original host.
	// e.g: https://site-125.aws-2-account.ownstak.link
	// e.g: https://site-125.aws-2-account.ownstak.link,https://site-125.aws-3-account.ownstak.link
	host := r.Host
	if forwardedHost := r.Header.Get(HeaderXForwardedHost); forwardedHost != "" {
		// If there are multiple forwarded hosts, use the last one
		if commaIdx := strings.LastIndex(forwardedHost, ","); commaIdx != -1 {
			host = strings.TrimSpace(forwardedHost[commaIdx+1:])
		} else {
			host = forwardedHost
		}
	}
	// Remove port from host if present
	host = strings.Split(host, ":")[0]

	// Parse query parameters directly from the URL
	queryParams := r.URL.Query()

	// Determine scheme (http or https)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if forwardedProto := r.Header.Get(HeaderXForwardedProto); forwardedProto != "" {
		scheme = forwardedProto
	}

	// Extract port from host or use default
	port := ""
	if hostPort := strings.Split(host, ":"); len(hostPort) > 1 {
		port = hostPort[1]
	} else {
		// Use default ports based on scheme
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
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
		Method:     r.Method,
		URL:        r.URL.String(),
		Path:       r.URL.Path,
		Host:       host,
		Headers:    headers,
		Body:       body,
		Query:      queryParams,
		Protocol:   protocol,
		Scheme:     scheme,
		Port:       port,
		RemoteAddr: remoteAddr,
	}, nil
}

// Context returns a background context for the request
func (r *ServerRequest) Context() context.Context {
	return context.Background()
}
