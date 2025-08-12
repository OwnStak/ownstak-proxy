package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/utils"
)

type Server struct {
	host             string
	httpPort         string
	httpsPort        string
	certFile         string
	keyFile          string
	caFile           string
	reqReadTimeout   time.Duration
	reqWriteTimeout  time.Duration
	reqIdleTimeout   time.Duration
	reqMaxHeadersSize int
	reqMaxBodySize    int
	resWriteTimeout   time.Duration
	MiddlewaresChain *MiddlewaresChain
	startTime        time.Time
	serverId         string
}

func NewServer() *Server {
	// Generate a unique server ID
	serverId := uuid.New().String()

	// Set default values if environment variables are not set
	host := utils.GetEnv(constants.EnvHost)
	httpPort := utils.GetEnv(constants.EnvHttpPort)
	httpsPort := utils.GetEnv(constants.EnvHttpsPort)
	certFile := utils.GetEnv(constants.EnvHttpsCert)
	keyFile := utils.GetEnv(constants.EnvHttpsCertKey)
	caFile := utils.GetEnv(constants.EnvHttpsCertCa)

	// Set the maximum time the server will wait for a request from the client
	reqReadTimeoutStr := utils.GetEnv(constants.EnvReqReadTimeout)
	reqReadTimeout := 2 * time.Minute // Defaults to 2 minutes
	if reqReadTimeoutStr != "" {
		if rt, err := time.ParseDuration(reqReadTimeoutStr); err == nil {
			reqReadTimeout = rt
		} else {
			logger.Warn(fmt.Sprintf("Invalid REQ_READ_TIMEOUT format, using default: %d", reqReadTimeout))
		}
	}

	// Set the maximum time the server will wait for the client to receive the response
	resWriteTimeoutStr := utils.GetEnv(constants.EnvResWriteTimeout)
	resWriteTimeout := 2 * time.Hour // Defaults to 2 hours
	if resWriteTimeoutStr != "" {
		if wt, err := time.ParseDuration(resWriteTimeoutStr); err == nil {
			resWriteTimeout = wt
		} else {
			logger.Warn(fmt.Sprintf("Invalid RES_WRITE_TIMEOUT format, using default: %d", resWriteTimeout))
		}
	}

	// Set the maximum time the server will wait for a client to send next requests
	// when using keep-alive or initial connection
	reqIdleTimeoutStr := utils.GetEnv(constants.EnvReqIdleTimeout)
	reqIdleTimeout := 60 * time.Second // Defaults to 60 seconds
	if reqIdleTimeoutStr != "" {
		if it, err := time.ParseDuration(reqIdleTimeoutStr); err == nil {
			reqIdleTimeout = it
		} else {
			logger.Warn(fmt.Sprintf("Invalid REQ_IDLE_TIMEOUT format, using default: %d", reqIdleTimeout))
		}
	}

	// Set the maximum total size of accepted request headers in bytes
	reqMaxHeadersSizeStr := utils.GetEnv(constants.EnvReqMaxHeadersSize)
	reqMaxHeadersSize := 64 * 1024 // Defaults to 64KiB
	if reqMaxHeadersSizeStr != "" {
		if size, err := strconv.Atoi(reqMaxHeadersSizeStr); err == nil {
			reqMaxHeadersSize = size
		} else {
			logger.Warn(fmt.Sprintf("Invalid REQ_MAX_HEADERS_SIZE format, using default: %d", reqMaxHeadersSize))
		}
	}

	// Set the maximum size of the request body in bytes
	// NOTE: The req body is always buffered whole in memory before invoking the lambda, 
	// so we need to have a reasonable limit.
	reqMaxBodySizeStr := utils.GetEnv(constants.EnvReqMaxBodySize)
	reqMaxBodySize := 10 * 1024 * 1024 // Defaults to 10MiB
	if reqMaxBodySizeStr != "" {
		if size, err := strconv.Atoi(reqMaxBodySizeStr); err == nil {
			reqMaxBodySize = size
		} else {
			logger.Warn(fmt.Sprintf("Invalid REQ_MAX_BODY_SIZE format, using default: %d", reqMaxBodySize))
		}
	}

	if host == "" {
		host = "0.0.0.0"
	}

	if httpPort == "" {
		httpPort = "80"
	}

	if httpsPort == "" {
		httpsPort = "443"
	}

	if certFile == "" {
		certFile = "/tmp/certs/cert.pem"
	}

	if keyFile == "" {
		keyFile = "/tmp/certs/key.pem"
	}

	if caFile == "" {
		caFile = "/tmp/certs/ca.pem"
	}

	return &Server{
		host:             host,
		httpPort:         httpPort,
		httpsPort:        httpsPort,
		certFile:         certFile,
		keyFile:          keyFile,
		caFile:           caFile,
		reqReadTimeout:   reqReadTimeout,
		resWriteTimeout:  resWriteTimeout,
		reqIdleTimeout:   reqIdleTimeout,
		reqMaxHeadersSize: reqMaxHeadersSize,
		reqMaxBodySize:    reqMaxBodySize,
		startTime:        time.Now(),
		serverId:         serverId,
		MiddlewaresChain: NewMiddlewaresChain(),
	}
}

// Use adds a middleware to the chain that can intercept or process the request.
func (server *Server) Use(mw Middleware) *Server {
	if mw == nil || reflect.ValueOf(mw).IsNil() {
		return server
	}
	middlewareName := reflect.TypeOf(mw).Elem().Name()
	logger.Info("Adding middleware: %s", middlewareName)
	server.MiddlewaresChain.Add(mw)
	return server
}

// Start begins the server
func (server *Server) Start() {
	// Ensure the directory for the certificate file exists
	certDir := filepath.Dir(server.certFile)
	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create directory for certificates: %s", err))
	}

	// Check if certificate file exists
	if _, err := os.Stat(server.certFile); os.IsNotExist(err) {
		logger.Warn("Certificate file does not exist, generating self-signed certificate")
		server.generateSelfSignedCert()
	}

	// Create a channel to listen for signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:           fmt.Sprintf("%s:%s", server.host, server.httpPort),
		Handler:        http.HandlerFunc(server.handleRequest),
		ReadTimeout:    server.reqReadTimeout,
		IdleTimeout:    server.reqIdleTimeout,
		MaxHeaderBytes: server.reqMaxHeadersSize,
		WriteTimeout:   server.resWriteTimeout,
	}

	// Create HTTPS server with HTTP/2 support
	httpsServer := &http.Server{
		Addr: fmt.Sprintf("%s:%s", server.host, server.httpsPort),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				server.loadCertificate(),
			},
			NextProtos: []string{"h2", "http/1.1"}, // Enable HTTP/2 protocol negotiation
		},
		Handler:        http.HandlerFunc(server.handleRequest),
		ReadTimeout:    server.reqReadTimeout,
		WriteTimeout:   server.resWriteTimeout,
		IdleTimeout:    server.reqIdleTimeout,
		MaxHeaderBytes: server.reqMaxHeadersSize,
	}

	// Configure HTTP/2
	http2.ConfigureServer(httpsServer, &http2.Server{
		// HTTP/2 specific settings can go here
		MaxConcurrentStreams: 250, // max streams per client connection
		IdleTimeout:          server.reqIdleTimeout,
	})

	// Start HTTP server in a goroutine
	go func() {
		logger.Info(fmt.Sprintf("Starting HTTP server on %s:%s", server.host, server.httpPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("HTTP server failed: %s", err))
		}
	}()

	// Start HTTPS server in a goroutine
	go func() {
		logger.Info(fmt.Sprintf("Starting HTTPS server on %s:%s", server.host, server.httpsPort))
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("HTTPS server failed: %s", err))
		}
	}()

	// Block until we receive a SIGINT or SIGTERM signal
	<-stop
	logger.Info("Shutting down server...")

	// Create a deadline to wait for existing connections to finish
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Gracefully shut down the HTTP server
	logger.Info("Shutting down HTTP server...")
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server forced to shutdown: %v", err)
	}

	// Gracefully shut down the HTTPS server
	logger.Info("Shutting down HTTPS server...")
	if err := httpsServer.Shutdown(ctx); err != nil {
		logger.Error("HTTPS server forced to shutdown: %v", err)
	}

	logger.Info("Server exited gracefully")
}

func (server *Server) handleRequest(httpRes http.ResponseWriter, httpReq *http.Request) {
	if httpReq == nil {
		logger.Debug("httpReq is nil, nothing to do")
		return
	}

	// Set limit to the request body size
	httpReq.Body = http.MaxBytesReader(httpRes, httpReq.Body, int64(server.reqMaxBodySize))

	// Create a new Request object from the http.Request
	req, reqErr := NewRequest(httpReq)

	// No req and no error, meaning the client is gone/disconnected gracefully,
	// before we could start handling the request. Just return.
	if req == nil && reqErr == nil {
		logger.Debug("Request is nil because the client is gone")
		return
	}

	// If req creation failed, assign new empty request with defaults for the context,
	// so we can render the "nice-looking" error page.
	if req == nil {
		req, _ = NewRequest()
	}
	// Create a new Response with the http.ResponseWriter
	res := NewResponse(httpRes)
	// Create a context containing request, response
	ctx := NewRequestContext(req, res, server)
	// Log incoming requests in debug mode
	logger.Debug("%s %s", req.Method, req.URL)

	// Always end and send the response when we're done
	defer res.End()

	// Handle known req creation errors
	if reqErr != nil && strings.Contains(reqErr.Error(), "http: request body too large") {
		ctx.Error(fmt.Sprintf("Request body too large. Maximum accepted size is %d bytes.", server.reqMaxBodySize), StatusRequestEntityTooLarge)
		return
	}

	// Handle all other req creation errors and log them
	if reqErr != nil {
		logger.Error("Failed to create server request: %v", reqErr)
		ctx.Error(fmt.Sprintf("Failed to create server request: %v", reqErr), StatusInternalServerError)
		return
	}

	// If there's no provider set, return an error
	provider := utils.GetEnv(constants.EnvProvider)
	if provider == "" {
		// If no provider is set, return an error
		ctx.Error(fmt.Sprintf("Unknown provider: The %s environment variable is not set. ", constants.EnvProvider), StatusServiceUnavailable)
		return
	}

	// Execute middleware chain
	server.MiddlewaresChain.Execute(ctx)
}

func (server *Server) generateSelfSignedCert() {
	// Generate a private key for the CA
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to generate CA private key: %s", err))
	}

	// Create a template for the CA certificate
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{fmt.Sprintf("%s CA", constants.AppName)},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create a self-signed CA certificate
	caDerBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create CA certificate: %s", err))
	}

	// Write the CA certificate to a file
	caOut, err := os.Create(server.caFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to open ca.pem for writing: %s", err))
	}
	if err := pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDerBytes}); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to write data to ca.pem: %s", err))
	}
	if err := caOut.Close(); err != nil {
		logger.Fatal(fmt.Sprintf("Error closing ca.pem: %s", err))
	}
	logger.Info("Written CA Certificate to ca.pem")

	// Generate a private key for the server
	serverPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to generate server private key: %s", err))
	}

	// Create a template for the server certificate
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{constants.AppName},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create a server certificate signed by the CA
	serverDerBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &serverPriv.PublicKey, caPriv)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create server certificate: %s", err))
	}

	// Write the server certificate to a file
	certOut, err := os.Create(server.certFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to open cert.pem for writing: %s", err))
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverDerBytes}); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to write data to cert.pem: %s", err))
	}
	if err := certOut.Close(); err != nil {
		logger.Fatal(fmt.Sprintf("Error closing cert.pem: %s", err))
	}
	logger.Info("Written Server Certificate to cert.pem")

	// Write the server private key to a file
	keyOut, err := os.Create(server.keyFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to open key.pem for writing: %s", err))
	}
	serverPrivBytes, err := x509.MarshalECPrivateKey(serverPriv)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Unable to marshal server private key: %v", err))
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverPrivBytes}); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to write data to key.pem: %s", err))
	}
	if err := keyOut.Close(); err != nil {
		logger.Fatal(fmt.Sprintf("Error closing key.pem: %s", err))
	}
	logger.Info("Written Server Private Key to key.pem")
}

func (server *Server) loadCertificate() tls.Certificate {
	cert, err := tls.LoadX509KeyPair(server.certFile, server.keyFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load certificate: %s", err))
	}
	return cert
}

// StartTime returns the time when the server was started
func (server *Server) StartTime() time.Time {
	return server.startTime
}

// ServerId returns the unique identifier for this server instance
func (server *Server) ServerId() string {
	return server.serverId
}

// HandleRequest is a public wrapper around handleRequest for testing
func (server *Server) HandleRequest(httpRes http.ResponseWriter, httpReq *http.Request) {
	server.handleRequest(httpRes, httpReq)
}
