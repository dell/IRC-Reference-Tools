package httpclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const TIMEOUT = 10 // seconds

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPClient

// Init initializes the global HTTP client
func init() {
	client = &http.Client{
		Timeout: TIMEOUT * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// GET request with optional Basic Auth
func Get(url string, headers map[string]string, username, password string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create GET request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Add Basic Auth if username and password provided
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	return doRequest(req)
}

// POST request with JSON payload and optional Basic Auth
func Post(url string, payload interface{}, headers map[string]string, username, password string) (string, error) {
	return patchOrPost(url, payload, headers, username, password, http.MethodPost)
}

// PATCH request with JSON payload and optional Basic Auth
func Patch(url string, payload interface{}, headers map[string]string, username, password string) (string, error) {
	return patchOrPost(url, payload, headers, username, password, http.MethodPatch)
}

// Internal helper to PATCH or POST
func patchOrPost(url string, payload interface{}, headers map[string]string, username, password, method string) (string, error) {
	var jsonData []byte
	var err error

	switch v := payload.(type) {
	case string:
		jsonData = []byte(v) // use raw JSON
	default:
		jsonData, err = json.Marshal(v)
	}

	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create %s request: %w", method, err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Add Basic Auth if username and password provided
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	return doRequest(req)
}

// Internal helper to execute request
func doRequest(req *http.Request) (string, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}
