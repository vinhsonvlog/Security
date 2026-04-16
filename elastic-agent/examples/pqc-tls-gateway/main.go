// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func main() {
	var (
		listenAddr   string
		upstream     string
		certPath     string
		keyPath      string
		requirePQC   bool
		normalizeECS bool
	)

	flag.StringVar(&listenAddr, "listen", ":8443", "HTTPS listen address")
	flag.StringVar(&upstream, "upstream", "http://localhost:9200", "Elasticsearch/Logstash upstream URL")
	flag.StringVar(&certPath, "cert", "ssl/server.crt", "Server certificate PEM path")
	flag.StringVar(&keyPath, "key", "ssl/server.key", "Server private key PEM path")
	flag.BoolVar(&requirePQC, "require-pqc", true, "Reject TLS 1.3 connections that do not negotiate X25519MLKEM768")
	flag.BoolVar(&normalizeECS, "normalize-ecs", true, "Normalize incoming JSON logs to ECS-like fields before forwarding")
	flag.Parse()

	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("invalid upstream URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	proxy.Transport = http.DefaultTransport
	handler := http.Handler(proxy)
	if normalizeECS {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := normalizeJSONLogRequest(r); err != nil {
				http.Error(w, fmt.Sprintf("invalid JSON log payload: %v", err), http.StatusBadRequest)
				return
			}
			proxy.ServeHTTP(w, r)
		})
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519MLKEM768,
			tls.X25519,
		},
	}

	if requirePQC {
		// Enforce post-quantum hybrid group when the handshake completes.
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if state.CurveID != tls.X25519MLKEM768 {
				return fmt.Errorf("negotiated curve %s is not allowed; expected %s", state.CurveID.String(), tls.X25519MLKEM768.String())
			}
			return nil
		}
	}

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         tlsConfig,
	}

	log.Printf("PQC TLS gateway listening on %s and forwarding to %s", listenAddr, upstreamURL.String())
	if requirePQC {
		log.Printf("PQC enforcement: enabled (%s)", tls.X25519MLKEM768.String())
	}
	if normalizeECS {
		log.Printf("ECS normalization: enabled")
	}

	err = server.ListenAndServeTLS(certPath, keyPath)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("gateway terminated: %v", err)
	}
}

func normalizeJSONLogRequest(r *http.Request) error {
	if r == nil || r.Body == nil {
		return nil
	}

	method := strings.ToUpper(r.Method)
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return nil
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	_ = r.Body.Close()
	if len(body) == 0 {
		return nil
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	normalizeECSFields(doc)

	normalized, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	r.Body = io.NopCloser(bytes.NewReader(normalized))
	r.ContentLength = int64(len(normalized))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(normalized)), nil
	}
	r.Header.Set("Content-Length", fmt.Sprintf("%d", len(normalized)))

	return nil
}

func normalizeECSFields(doc map[string]any) {
	if _, ok := doc["@timestamp"]; !ok {
		doc["@timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	if _, ok := doc["message"]; !ok {
		if msg, ok := doc["msg"]; ok {
			doc["message"] = msg
		}
	}

	if level, ok := doc["level"]; ok {
		delete(doc, "level")
		logNode, _ := doc["log"].(map[string]any)
		if logNode == nil {
			logNode = map[string]any{}
		}
		if _, exists := logNode["level"]; !exists {
			logNode["level"] = fmt.Sprintf("%v", level)
		}
		doc["log"] = logNode
	}

	if _, ok := doc["event"]; !ok {
		doc["event"] = map[string]any{"dataset": "custom.pqc"}
	}
}
