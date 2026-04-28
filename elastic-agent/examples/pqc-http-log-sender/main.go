package main

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

var logHTTPClient = &http.Client{Timeout: defaultTimeout}

// TLSOptions holds optional TLS and mTLS file inputs.
type TLSOptions struct {
	RootCAPath     string
	ClientCertPath string
	ClientKeyPath  string
	ProxyURL       string
	ProxyHeaders   []string
}

// BuildHTTPClient creates an HTTP client that enforces TLS 1.3 and prefers
// post-quantum hybrid key exchange (X25519MLKEM768) for HTTPS connections.
func BuildHTTPClient(opts TLSOptions, timeout time.Duration) (*http.Client, error) {
	tlsConfig := &tls.Config{
		// PQC hybrid key exchange in Go TLS is available only on TLS 1.3.
		MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
			tls.X25519,
		},
	}

	if opts.RootCAPath != "" {
		rootCAs, err := loadRootCAs(opts.RootCAPath)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = rootCAs
	}

	if opts.ClientCertPath != "" || opts.ClientKeyPath != "" {
		if opts.ClientCertPath == "" || opts.ClientKeyPath == "" {
			return nil, errors.New("both client certificate and client key must be provided for mTLS")
		}

		cert, err := tls.LoadX509KeyPair(opts.ClientCertPath, opts.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		// Keep HTTP/2 enabled for performance; TLS policy is still enforced above.
		ForceAttemptHTTP2: true,
	}
	if opts.ProxyURL != "" {
		parsedProxyURL, err := url.Parse(opts.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
		if len(opts.ProxyHeaders) > 0 {
			transport.ProxyConnectHeader = make(http.Header, len(opts.ProxyHeaders))
			for _, h := range opts.ProxyHeaders {
				key, value, ok := strings.Cut(h, "=")
				if !ok {
					return nil, fmt.Errorf("invalid proxy header format %q, expected Key=Value", h)
				}
				key = strings.TrimSpace(key)
				value = strings.TrimSpace(value)
				if key == "" {
					return nil, fmt.Errorf("invalid proxy header format %q: empty header key", h)
				}
				transport.ProxyConnectHeader.Add(key, value)
			}
		}
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

// SetHTTPClient swaps the HTTP client used by SendLog.
func SetHTTPClient(client *http.Client) {
	if client != nil {
		logHTTPClient = client
	}
}

// SendLog sends JSON log data to Elasticsearch/Logstash endpoint via POST.
func SendLog(endpoint string, logData []byte) error {
	if endpoint == "" {
		return errors.New("endpoint is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), logHTTPClient.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(logData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := logHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send log request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return nil
}

func loadRootCAs(rootCAPath string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(rootCAPath)
	if err != nil {
		return nil, fmt.Errorf("read root CA file: %w", err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("no valid CA certificate found in %s", rootCAPath)
	}

	return pool, nil
}

func main() {
	var (
		endpoint       string
		logJSON        string
		rootCAPath     string
		clientCertPath string
		clientKeyPath  string
		proxyURL       string
		proxyHeaders   string
		timeout        time.Duration
	)

	flag.StringVar(&endpoint, "endpoint", "https://localhost:8443/logs-pqc/_doc", "Elasticsearch/Logstash HTTP endpoint")
	flag.StringVar(&logJSON, "log", `{"@timestamp":"2026-04-15T00:00:00Z","level":"info","message":"PQC TLS log from Go client"}`, "JSON log payload")
	flag.StringVar(&rootCAPath, "root-ca", "", "Path to Root CA PEM file (optional)")
	flag.StringVar(&clientCertPath, "client-cert", "", "Path to client certificate PEM file for mTLS (optional)")
	flag.StringVar(&clientKeyPath, "client-key", "", "Path to client private key PEM file for mTLS (optional)")
	flag.StringVar(&proxyURL, "proxy-url", "", "Proxy URL (for example, http://proxy:3128)")
	flag.StringVar(&proxyHeaders, "proxy-headers", "", "Comma-separated CONNECT proxy headers in Key=Value format")
	flag.DurationVar(&timeout, "timeout", defaultTimeout, "HTTP timeout")
	flag.Parse()

	headerValues := splitAndTrimCSV(proxyHeaders)

	client, err := BuildHTTPClient(TLSOptions{
		RootCAPath:     rootCAPath,
		ClientCertPath: clientCertPath,
		ClientKeyPath:  clientKeyPath,
		ProxyURL:       proxyURL,
		ProxyHeaders:   headerValues,
	}, timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Build client error: %v\n", err)
		os.Exit(1)
	}

	SetHTTPClient(client)

	if err := SendLog(endpoint, []byte(logJSON)); err != nil {
		fmt.Fprintf(os.Stderr, "Send log failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Log sent successfully")
}

func splitAndTrimCSV(in string) []string {
	if strings.TrimSpace(in) == "" {
		return nil
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
