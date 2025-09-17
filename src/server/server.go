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
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"ownstak-proxy/src/constants"
	"ownstak-proxy/src/logger"
	"ownstak-proxy/src/utils"

	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Server struct {
	Host              string
	HttpPort          string
	HttpsPort         string
	CertFile          string
	KeyFile           string
	CaFile            string
	ReqReadTimeout    time.Duration
	ReqIdleTimeout    time.Duration
	ReqMaxHeadersSize int
	ReqMaxBodySize    int
	ResWriteTimeout   time.Duration
	MaxMemory         uint64
	UsedMemory        uint64
	MiddlewaresChain  *MiddlewaresChain
	StartTime         time.Time
	ServerId          string
	HttpServer        *http.Server
	HttpsServer       *http.Server
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
			logger.Warn("Invalid REQ_READ_TIMEOUT format, using default: %v", reqReadTimeout)
		}
	}

	// Set the maximum time the server will wait for the client to receive the response
	resWriteTimeoutStr := utils.GetEnv(constants.EnvResWriteTimeout)
	resWriteTimeout := 2 * time.Hour // Defaults to 2 hours
	if resWriteTimeoutStr != "" {
		if wt, err := time.ParseDuration(resWriteTimeoutStr); err == nil {
			resWriteTimeout = wt
		} else {
			logger.Warn("Invalid RES_WRITE_TIMEOUT format, using default: %v", resWriteTimeout)
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
			logger.Warn("Invalid REQ_IDLE_TIMEOUT format, using default: %v", reqIdleTimeout)
		}
	}

	// Set the maximum total size of accepted request headers in bytes
	reqMaxHeadersSizeStr := utils.GetEnv(constants.EnvReqMaxHeadersSize)
	reqMaxHeadersSize := 64 * 1024 // Defaults to 64KiB
	if reqMaxHeadersSizeStr != "" {
		if size, err := strconv.Atoi(reqMaxHeadersSizeStr); err == nil {
			reqMaxHeadersSize = size
		} else {
			logger.Warn("Invalid REQ_MAX_HEADERS_SIZE format, using default: %d", reqMaxHeadersSize)
		}
	}

	// Set the maximum size of the request body in bytes
	// NOTE: The req body is always buffered whole in memory before invoking the lambda,
	// so we need to have a reasonable limit.
	reqMaxBodySizeStr := utils.GetEnv(constants.EnvReqMaxBodySize)
	reqMaxBodySize := 6 * 1024 * 1024 // Defaults to 6MiB
	if reqMaxBodySizeStr != "" {
		if size, err := strconv.Atoi(reqMaxBodySizeStr); err == nil {
			reqMaxBodySize = size
		} else {
			logger.Warn("Invalid REQ_MAX_BODY_SIZE format, using default: %d", reqMaxBodySize)
		}
	}

	maxMemoryStr := utils.GetEnv(constants.EnvMaxMemory)
	maxMemory, err := utils.GetAvailableMemory()
	if err != nil {
		logger.Error("Failed to get default max memory: %v", err)
		maxMemory = 0
	}
	if maxMemoryStr != "" {
		if size, err := utils.ParseMemorySize(maxMemoryStr); err == nil {
			maxMemory = size
		} else {
			logger.Warn("Invalid MAX_MEMORY format '%s', using default: %d", maxMemoryStr, maxMemory)
		}
	}
	gcMaxMemory := maxMemory * 80 / 100

	// Adjust Golang Garbage Collector to correctly
	// detect configured cgroup max memory limit inside container
	debug.SetGCPercent(10)
	debug.SetMemoryLimit(int64(gcMaxMemory))
	logger.Info("Max memory: %s, GC max memory: %s", utils.FormatBytes(maxMemory), utils.FormatBytes(gcMaxMemory))

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
		Host:              host,
		HttpPort:          httpPort,
		HttpsPort:         httpsPort,
		CertFile:          certFile,
		KeyFile:           keyFile,
		CaFile:            caFile,
		ReqReadTimeout:    reqReadTimeout,
		ResWriteTimeout:   resWriteTimeout,
		ReqIdleTimeout:    reqIdleTimeout,
		ReqMaxHeadersSize: reqMaxHeadersSize,
		ReqMaxBodySize:    reqMaxBodySize,
		MaxMemory:         maxMemory,
		StartTime:         time.Now(),
		ServerId:          serverId,
		MiddlewaresChain:  NewMiddlewaresChain(),

		HttpServer:  &http.Server{},
		HttpsServer: &http.Server{},
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
	server.MiddlewaresChain.ExecuteOnStart(server)

	// Ensure the directory for the certificate file exists
	certDir := filepath.Dir(server.CertFile)
	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		logger.Fatal("Failed to create directory for certificates: %v", err)
	}

	// Check if certificate file exists
	if _, err := os.Stat(server.CertFile); os.IsNotExist(err) {
		logger.Warn("Certificate file does not exist, generating self-signed certificate")
		server.generateSelfSignedCert()
	} else {
		// Check if the certificate is valid for localhost
		cert := server.loadCertificate()
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err == nil {
			hasLocalhost := false
			for _, dnsName := range x509Cert.DNSNames {
				if dnsName == "localhost" || dnsName == "127.0.0.1" {
					hasLocalhost = true
					break
				}
			}
			if !hasLocalhost {
				logger.Warn("Existing certificate doesn't include localhost, regenerating...")
				server.generateSelfSignedCert()
			}
		}
	}

	// Create a channel to listen for signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Update current total memory usage every 25ms on background
	ticker := time.NewTicker(25 * time.Millisecond)
	go func() {
		for range ticker.C {
			server.UsedMemory, _ = utils.GetUsedMemory()
		}
	}()

	// Create HTTP/2.0 server configuration
	http2Server := &http2.Server{
		MaxConcurrentStreams: 250, // max streams per single client connection
		ReadIdleTimeout:      server.ReqReadTimeout,
		IdleTimeout:          server.ReqIdleTimeout,
		WriteByteTimeout:     server.ResWriteTimeout,
	}

	// Create HTTP server listener with support for HTTP/1.0, HTTP/1.1 and HTTP/2.0.
	server.HttpServer = &http.Server{
		Addr: fmt.Sprintf("%s:%s", server.Host, server.HttpPort),
		// Configure HTTP/2.0 over unencrypted connection through H2C upgrade mechanism.
		// NOTE: This is not supported by the web browsers, but by proxies and load balancers such as AWS ALB.
		Handler:        h2c.NewHandler(http.HandlerFunc(server.handleRequest), http2Server),
		ReadTimeout:    server.ReqReadTimeout,
		IdleTimeout:    server.ReqIdleTimeout,
		MaxHeaderBytes: server.ReqMaxHeadersSize,
		WriteTimeout:   server.ResWriteTimeout,
	}

	// Create HTTPS server listener with support for HTTP/1.1 and HTTP/2.0.
	server.HttpsServer = &http.Server{
		Addr: fmt.Sprintf("%s:%s", server.Host, server.HttpsPort),
		TLSConfig: &tls.Config{
			// Allow only TLS 1.2 and above (ssl3.0, TLS 1.0 and 1.1 are insecure)
			// See: https://www.cloudflare.com/en-gb/learning/ssl/what-is-ssl/
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				server.loadCertificate(),
			},
			NextProtos: []string{"h2", "http/1.1"}, // Enable HTTP/2 protocol negotiation
			// Allow insecure connections for development
			InsecureSkipVerify: constants.Mode == "development",
			// Allow only common still supported cipher suites
			// See: https://ssllabs.com or https://developers.cloudflare.com/ssl/edge-certificates/additional-options/cipher-suites/
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			},
		},
		Handler:        http.HandlerFunc(server.handleRequest),
		ReadTimeout:    server.ReqReadTimeout,
		WriteTimeout:   server.ResWriteTimeout,
		IdleTimeout:    server.ReqIdleTimeout,
		MaxHeaderBytes: server.ReqMaxHeadersSize,
	}
	// Configure HTTP/2 over TLS for HTTPS server
	http2.ConfigureServer(server.HttpsServer, http2Server)

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("Starting HTTP server on %s:%s", server.Host, server.HttpPort)
		if err := server.HttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed: %v", err)
		}
	}()

	// Start HTTPS server in a goroutine
	go func() {
		logger.Info("Starting HTTPS server on %s:%s", server.Host, server.HttpsPort)
		if err := server.HttpsServer.ListenAndServeTLS(server.CertFile, server.KeyFile); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTPS server failed: %v", err)
		}
	}()

	// Block until we receive a SIGINT or SIGTERM signal
	<-stop
	logger.Info("Shutting down server...")
	server.MiddlewaresChain.ExecuteOnStop(server)

	// Create a deadline to wait for existing connections to finish
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Gracefully shut down the HTTP server
	logger.Info("Shutting down HTTP server...")
	if err := server.HttpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server forced to shutdown: %v", err)
	}

	// Gracefully shut down the HTTPS server
	logger.Info("Shutting down HTTPS server...")
	if err := server.HttpsServer.Shutdown(ctx); err != nil {
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
	httpReq.Body = http.MaxBytesReader(httpRes, httpReq.Body, int64(server.ReqMaxBodySize))

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

	// Handle all other req creation errors and log them
	if reqErr != nil {
		logger.Error("Failed to create server request: %v", reqErr)
		ctx.Error(fmt.Sprintf("Failed to create server request: %v", reqErr), StatusInternalError)
		return
	}

	// If req contains content-length header, check if it's too large
	// so it fails immediately and we don't waste resources to even read the body
	contentLength, _ := strconv.Atoi(req.Headers.Get(HeaderContentLength))
	if contentLength > server.ReqMaxBodySize {
		ctx.Error(fmt.Sprintf("Request body too large: The maximum accepted size is %d bytes.", server.ReqMaxBodySize), StatusContentTooLarge)
		return
	}

	// Stop accepting new requests if memory usage is over 90% of the configured max memory to prevent crashes and to keep the server stable.
	if server.UsedMemory >= server.MaxMemory*90/100 {
		usedMemoryUsageStr := utils.FormatBytes(server.UsedMemory)
		maxMemoryUsageStr := utils.FormatBytes(server.MaxMemory)
		ctx.Error(fmt.Sprintf("Server is overloaded: OwnStak proxy server couldn't handle the request because of high memory usage. Please try again later. (memory usage: %s / %s)", usedMemoryUsageStr, maxMemoryUsageStr), StatusServiceOverloaded)
		return
	}

	// If there's no provider set, return an error
	provider := utils.GetEnv(constants.EnvProvider)
	if provider == "" {
		// If no provider is set, return an error
		ctx.Error(fmt.Sprintf("Unknown provider: The %s environment variable is not set. ", constants.EnvProvider), StatusServiceUnavailable)
		return
	}

	// Execute middlewares chain
	server.MiddlewaresChain.ExecuteOnRequest(ctx)
	server.MiddlewaresChain.ExecuteOnResponse(ctx)
}

func (server *Server) generateSelfSignedCert() {
	// Generate a private key for the CA
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal("Failed to generate CA private key: %v", err)
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
		logger.Fatal("Failed to create CA certificate: %v", err)
	}

	// Write the CA certificate to a file
	caOut, err := os.Create(server.CaFile)
	if err != nil {
		logger.Fatal("Failed to open ca.pem for writing: %v", err)
	}
	if err := pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDerBytes}); err != nil {
		logger.Fatal("Failed to write data to ca.pem: %v", err)
	}
	if err := caOut.Close(); err != nil {
		logger.Fatal("Error closing ca.pem: %v", err)
	}
	logger.Info("Written CA Certificate to ca.pem")

	// Generate a private key for the server
	serverPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal("Failed to generate server private key: %v", err)
	}

	// Create a template for the server certificate
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{constants.AppName},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// Add Subject Alternative Names for localhost and common development hosts
		DNSNames: []string{
			"localhost",
			"127.0.0.1",
			"0.0.0.0",
			"*.localhost",
			"*.local",
		},
		IPAddresses: []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv4(0, 0, 0, 0),
			net.IPv6loopback,
		},
	}

	// Create a server certificate signed by the CA
	serverDerBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &serverPriv.PublicKey, caPriv)
	if err != nil {
		logger.Fatal("Failed to create server certificate: %v", err)
	}

	// Write the server certificate to a file
	certOut, err := os.Create(server.CertFile)
	if err != nil {
		logger.Fatal("Failed to open cert.pem for writing: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverDerBytes}); err != nil {
		logger.Fatal("Failed to write data to cert.pem: %v", err)
	}
	if err := certOut.Close(); err != nil {
		logger.Fatal("Error closing cert.pem: %v", err)
	}
	logger.Info("Written Server Certificate to cert.pem")
	logger.Info("Certificate valid for: %v", serverTemplate.DNSNames)
	logger.Info("Certificate valid for IPs: %v", serverTemplate.IPAddresses)

	// Write the server private key to a file
	keyOut, err := os.Create(server.KeyFile)
	if err != nil {
		logger.Fatal("Failed to open key.pem for writing: %v", err)
	}
	serverPrivBytes, err := x509.MarshalECPrivateKey(serverPriv)
	if err != nil {
		logger.Fatal("Unable to marshal server private key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverPrivBytes}); err != nil {
		logger.Fatal("Failed to write data to key.pem: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		logger.Fatal("Error closing key.pem: %v", err)
	}
	logger.Info("Written Server Private Key to key.pem")
}

func (server *Server) loadCertificate() tls.Certificate {
	cert, err := tls.LoadX509KeyPair(server.CertFile, server.KeyFile)
	if err != nil {
		logger.Fatal("Failed to load certificate: %v", err)
	}
	return cert
}

// HandleRequest is a public wrapper around handleRequest for testing
func (server *Server) HandleRequest(httpRes http.ResponseWriter, httpReq *http.Request) {
	server.handleRequest(httpRes, httpReq)
}
