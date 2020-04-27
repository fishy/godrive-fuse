package main

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// Default values for HTTPClientConfig
const (
	DefaultHTTPTimeout = time.Second * 5
)

// HTTPClientConfig are the configs used to create the http client used in both
// OAuth and Drive API.
type HTTPClientConfig struct {
	// Timeout in all HTTP requests. If <= 0, DefaultHTTPTimeout will be used.
	Timeout time.Duration `yaml:"timeout"`
}

func getClientContext(ctx context.Context, cfg HTTPClientConfig) context.Context {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultHTTPTimeout
	}
	client := &http.Client{
		Timeout: cfg.Timeout,
	}
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}
