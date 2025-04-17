package server

import (
	"net/http"
	"ownstak-proxy/src/constants"
)

type ServerResponse struct {
	Status  int
	Headers http.Header
	Body    []byte
}

// NewServerResponse creates a new ServerResponse with default values
func NewServerResponse() *ServerResponse {
	headers := make(http.Header)
	headers.Set(HeaderContentType, ContentTypePlain)
	headers.Set(HeaderXOwnProxyVersion, constants.Version)

	return &ServerResponse{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    []byte{},
	}
}

// WriteTo writes the ServerResponse to an http.ResponseWriter
func (res *ServerResponse) WriteTo(w http.ResponseWriter) {
	for key, values := range res.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(res.Status)
	w.Write(res.Body)
}

func (res *ServerResponse) Clear() {
	res.Status = http.StatusOK
	res.Headers = make(http.Header)
	res.Body = []byte{}
}
