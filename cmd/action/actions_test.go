package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

// TestHTTPShutdowner_Name verifies that the Name method returns the config name.
func TestHTTPShutdowner_Name(t *testing.T) {
	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				URI:    "/shutdown",
				Method: "POST",
			},
		},
	}

	s := newHTTPShutdowner(cfg)
	if s.Name() != "testtype" {
		t.Errorf("expected name 'testtype', got %q", s.Name())
	}
}

// TestHTTPShutdowner_Type verifies that the Type method returns the config name.
func TestHTTPShutdowner_Type(t *testing.T) {
	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				URI:    "/shutdown",
				Method: "POST",
			},
		},
	}

	s := newHTTPShutdowner(cfg)
	if s.Type() != "testtype" {
		t.Errorf("expected type 'testtype', got %q", s.Type())
	}
}

// TestHTTPShutdowner_Shutdown_Graceful verifies that graceful shutdown executes the correct HTTP call.
func TestHTTPShutdowner_Shutdown_Graceful(t *testing.T) {
	// Create a test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/shutdown/graceful" {
			t.Errorf("expected /shutdown/graceful, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Extract host from server URL (remove https://)
	host := server.URL[8:] // Remove "https://"

	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown/graceful",
				Method:   "POST",
				Body:     `{"action": "graceful"}`,
			},
		},
	}

	s := newHTTPShutdowner(cfg)
	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: host,
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
		IsForcefulshutdown: false,
	}

	result, err := s.Shutdown(si)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"status": "ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

// TestHTTPShutdowner_Shutdown_Force verifies that force shutdown executes the correct HTTP call.
func TestHTTPShutdowner_Shutdown_Force(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shutdown/force" {
			t.Errorf("expected /shutdown/force, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "forced"}`))
	}))
	defer server.Close()

	host := server.URL[8:]

	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown/graceful",
				Method:   "POST",
			},
			Force: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown/force",
				Method:   "POST",
			},
		},
	}

	s := newHTTPShutdowner(cfg)
	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: host,
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
		IsForcefulshutdown: true,
	}

	result, err := s.Shutdown(si)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"status": "forced"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

// TestHTTPShutdowner_Shutdown_ForceFallbackToGraceful verifies that force falls back to graceful when force is not configured.
func TestHTTPShutdowner_Shutdown_ForceFallbackToGraceful(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should fall back to graceful since force is not configured
		if r.URL.Path != "/shutdown/graceful" {
			t.Errorf("expected /shutdown/graceful (fallback), got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host := server.URL[8:]

	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown/graceful",
				Method:   "POST",
			},
			// Force is nil - should fall back to graceful
		},
	}

	s := newHTTPShutdowner(cfg)
	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: host,
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
		IsForcefulshutdown: true, // Requesting force, but only graceful is configured
	}

	_, err := s.Shutdown(si)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHTTPShutdowner_Shutdown_NoGracefulConfigured verifies that an error is returned when graceful shutdown is requested but not configured.
func TestHTTPShutdowner_Shutdown_NoGracefulConfigured(t *testing.T) {
	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Force: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown/force",
				Method:   "POST",
			},
			// Graceful is nil
		},
	}

	s := newHTTPShutdowner(cfg)
	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: "127.0.0.1",
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
		IsForcefulshutdown: false, // Requesting graceful, but only force is configured
	}

	_, err := s.Shutdown(si)
	if err == nil {
		t.Error("expected error when graceful not configured")
	}
}

// TestHTTPShutdowner_Shutdown_NilConfig verifies that an error is returned when the shutdown config is nil.
func TestHTTPShutdowner_Shutdown_NilConfig(t *testing.T) {
	s := &httpShutdowner{
		name:   "test",
		config: nil,
	}

	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: "127.0.0.1",
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
	}

	_, err := s.Shutdown(si)
	if err == nil {
		t.Error("expected error when config is nil")
	}
}

// TestHTTPShutdowner_Shutdown_HTTPError verifies that an error is returned for non-2xx HTTP responses.
func TestHTTPShutdowner_Shutdown_HTTPError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	host := server.URL[8:]

	cfg := &systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown",
				Method:   "POST",
			},
		},
	}

	s := newHTTPShutdowner(cfg)
	si := auth.ServiceItem{
		Service: auth.Service{
			Ip: host,
			Auth: map[string]string{
				"username": "admin",
				"password": "password",
			},
		},
	}

	_, err := s.Shutdown(si)
	if err == nil {
		t.Error("expected error for HTTP 500 response")
	}
}
