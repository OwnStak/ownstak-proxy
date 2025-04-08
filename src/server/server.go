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
	"syscall"
	"time"

	"golang.org/x/net/http2"

	"ownstack-proxy/src/logger"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Server struct {
	host        string
	httpPort    string
	httpsPort   string
	certFile    string
	keyFile     string
	caFile      string
	readTimeout time.Duration
	cache       *Cache
	middleware  *MiddlewareChain
	startTime   time.Time
	serverId    string
}

func NewServer() *Server {
	// Generate a unique server ID
	serverId := uuid.New().String()

	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		logger.Warn("Error loading .env file, using default values")
	}

	// Set default values if environment variables are not set
	host := os.Getenv("HOST")
	httpPort := os.Getenv("HTTP_PORT")
	httpsPort := os.Getenv("HTTPS_PORT")
	certFile := os.Getenv("HTTPS_CERT")
	keyFile := os.Getenv("HTTPS_CERT_KEY")
	caFile := os.Getenv("HTTPS_CERT_CA")

	// Read timeout configuration from environment
	readTimeoutStr := os.Getenv("READ_TIMEOUT")
	readTimeout := 10 * time.Second // Default to 10 seconds
	if readTimeoutStr != "" {
		if rt, err := time.ParseDuration(readTimeoutStr); err == nil {
			readTimeout = rt
		} else {
			logger.Warn("Invalid READ_TIMEOUT format, using default: 10s")
		}
	}

	// Get cache max size from environment or use default (100MB)
	cacheMaxSizeStr := os.Getenv("CACHE_MAX_SIZE")
	cacheMaxSize := 100 * 1024 * 1024 // Default to 100MB
	if cacheMaxSizeStr != "" {
		if size, err := strconv.Atoi(cacheMaxSizeStr); err == nil {
			cacheMaxSize = size
		} else {
			logger.Warn("Invalid CACHE_MAX_SIZE format, using default: 100MB")
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
		host:        host,
		httpPort:    httpPort,
		httpsPort:   httpsPort,
		certFile:    certFile,
		keyFile:     keyFile,
		caFile:      caFile,
		readTimeout: readTimeout,
		middleware:  NewMiddlewareChain(),
		cache:       NewCache(cacheMaxSize),
		startTime:   time.Now(),
		serverId:    serverId,
	}
}

// Use adds a middleware to the chain that can intercept or process the request.
func (s *Server) Use(mw Middleware) *Server {
	if mw == nil {
		logger.Error("Failed to add middleware: middleware is nil")
		return s
	}
	middlewareName := reflect.TypeOf(mw).Elem().Name()
	logger.Info("Adding middleware: %s", middlewareName)
	s.middleware.Use(mw)
	return s
}

// Start begins the server
func (s *Server) Start() {
	// Ensure the directory for the certificate file exists
	certDir := filepath.Dir(s.certFile)
	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create directory for certificates: %s", err))
	}

	// Check if certificate file exists
	if _, err := os.Stat(s.certFile); os.IsNotExist(err) {
		logger.Info("Certificate file does not exist, generating self-signed certificate")
		s.generateSelfSignedCert()
	}

	// Create a channel to listen for signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:        fmt.Sprintf("%s:%s", s.host, s.httpPort),
		Handler:     http.HandlerFunc(s.handleRequest),
		ReadTimeout: s.readTimeout,
	}

	// Create HTTPS server with HTTP/2 support
	httpsServer := &http.Server{
		Addr: fmt.Sprintf("%s:%s", s.host, s.httpsPort),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			Certificates: []tls.Certificate{
				s.loadCertificate(),
			},
			NextProtos: []string{"h2", "http/1.1"}, // Enable HTTP/2 protocol negotiation
		},
		Handler:     http.HandlerFunc(s.handleRequest),
		ReadTimeout: s.readTimeout,
	}

	// Configure HTTP/2
	http2.ConfigureServer(httpsServer, &http2.Server{
		// HTTP/2 specific settings can go here
		MaxConcurrentStreams: 250,
		IdleTimeout:          10 * time.Second,
	})

	// Start HTTP server in a goroutine
	go func() {
		logger.Info(fmt.Sprintf("Starting HTTP server on %s:%s", s.host, s.httpPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(fmt.Sprintf("HTTP server failed: %s", err))
		}
	}()

	// Start HTTPS server in a goroutine
	go func() {
		logger.Info(fmt.Sprintf("Starting HTTPS server on %s:%s", s.host, s.httpsPort))
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

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Create a new serverRequest
	req, err := NewServerRequest(r)
	if err != nil {
		logger.Error("Failed to create server request: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log incoming requests in debug mode
	logger.Debug("%s %s", req.Method, req.URL)

	// Create a new ServerResponse
	res := NewServerResponse()

	// Create a context containing request, response, and globally shared cache
	ctx := NewServerContext(req, res, s)

	// Execute middleware chain
	s.middleware.Execute(ctx)

	// Return the response
	res.WriteTo(w)
}

func (s *Server) generateSelfSignedCert() {
	// Generate a private key for the CA
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to generate CA private key: %s", err))
	}

	// Create a template for the CA certificate
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"My CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
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
	caOut, err := os.Create(s.caFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to open ca.pem for writing: %s", err))
	}
	if err := pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDerBytes}); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to write data to ca.pem: %s", err))
	}
	if err := caOut.Close(); err != nil {
		logger.Fatal(fmt.Sprintf("Error closing ca.pem: %s", err))
	}
	logger.Info("Written ca.pem")

	// Generate a private key for the server
	serverPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to generate server private key: %s", err))
	}

	// Create a template for the server certificate
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"My Company"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
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
	certOut, err := os.Create(s.certFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to open cert.pem for writing: %s", err))
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverDerBytes}); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to write data to cert.pem: %s", err))
	}
	if err := certOut.Close(); err != nil {
		logger.Fatal(fmt.Sprintf("Error closing cert.pem: %s", err))
	}
	logger.Info("Written cert.pem")

	// Write the server private key to a file
	keyOut, err := os.Create(s.keyFile)
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
	logger.Info("Written key.pem")
}

func (s *Server) loadCertificate() tls.Certificate {
	cert, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to load certificate: %s", err))
	}
	return cert
}

// CacheSet adds a key-value pair to the cache with an optional expiration time
// Returns true if the value was set, false if it couldn't be set due to size constraints
func (s *Server) CacheSet(key string, value string, expiration time.Duration) bool {
	return s.cache.Set(key, value, expiration)
}

// CacheGet retrieves a value from the cache by key
// Returns the value and a boolean indicating if the value was found and not expired
func (s *Server) CacheGet(key string) (string, bool) {
	return s.cache.Get(key)
}

// CacheDelete removes a key-value pair from the cache
func (s *Server) CacheDelete(key string) {
	s.cache.Delete(key)
}

// CacheSize returns the current size of the cache in bytes
func (s *Server) CacheSize() int {
	return s.cache.Size()
}

// CacheCount returns the number of entries in the cache
func (s *Server) CacheCount() int {
	return s.cache.Count()
}

// CacheClear removes all entries from the cache
func (s *Server) CacheClear() {
	s.cache.Clear()
}

// StartTime returns the time when the server was started
func (s *Server) StartTime() time.Time {
	return s.startTime
}

// ServerId returns the unique identifier for this server instance
func (s *Server) ServerId() string {
	return s.serverId
}

// CacheMaxSize returns the maximum size of the cache in bytes
func (s *Server) CacheMaxSize() int {
	return s.cache.maxSize
}

// CacheSectionCount returns the number of sections in the cache
func (s *Server) CacheSectionCount() int {
	return s.cache.sectionCount
}
