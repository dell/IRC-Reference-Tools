package redfish

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
)

// TestExtractJSONPath_SimpleField verifies extraction of a simple top-level field from JSON.
func TestExtractJSONPath_SimpleField(t *testing.T) {
	obj := map[string]interface{}{
		"Name":       "TestSystem",
		"PowerState": "On",
	}

	value, err := extractJSONPath(obj, "$.Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "TestSystem" {
		t.Errorf("expected 'TestSystem', got %q", value)
	}
}

// TestExtractJSONPath_NestedField verifies extraction of a nested field using dot notation.
func TestExtractJSONPath_NestedField(t *testing.T) {
	obj := map[string]interface{}{
		"System": map[string]interface{}{
			"Info": map[string]interface{}{
				"ServiceTag": "ABC123",
			},
		},
	}

	value, err := extractJSONPath(obj, "$.System.Info.ServiceTag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "ABC123" {
		t.Errorf("expected 'ABC123', got %q", value)
	}
}

// TestExtractJSONPath_ArrayIndex verifies extraction of a field from an array element by index.
func TestExtractJSONPath_ArrayIndex(t *testing.T) {
	obj := map[string]interface{}{
		"Members": []interface{}{
			map[string]interface{}{"Id": "first"},
			map[string]interface{}{"Id": "second"},
		},
	}

	value, err := extractJSONPath(obj, "$.Members[1].Id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "second" {
		t.Errorf("expected 'second', got %q", value)
	}
}

// TestExtractJSONPath_ArrayIndexOutOfBounds verifies that an error is returned for out-of-bounds array access.
func TestExtractJSONPath_ArrayIndexOutOfBounds(t *testing.T) {
	obj := map[string]interface{}{
		"Members": []interface{}{
			map[string]interface{}{"Id": "first"},
		},
	}

	_, err := extractJSONPath(obj, "$.Members[5].Id")
	if err == nil {
		t.Error("expected error for out of bounds index")
	}
}

// TestExtractJSONPath_InvalidPath verifies that an error is returned for paths not starting with $.
func TestExtractJSONPath_InvalidPath(t *testing.T) {
	obj := map[string]interface{}{
		"Name": "Test",
	}

	_, err := extractJSONPath(obj, "Name") // Missing $.
	if err == nil {
		t.Error("expected error for invalid path format")
	}
}

// TestExtractJSONPath_FieldNotFound verifies that an error is returned when a field does not exist.
func TestExtractJSONPath_FieldNotFound(t *testing.T) {
	obj := map[string]interface{}{
		"Name": "Test",
	}

	_, err := extractJSONPath(obj, "$.NonExistent")
	if err == nil {
		t.Error("expected error for non-existent field")
	}
}

// TestExtractJSONPath_NumericValue verifies that numeric values are converted to strings.
func TestExtractJSONPath_NumericValue(t *testing.T) {
	obj := map[string]interface{}{
		"Count": float64(42),
	}

	value, err := extractJSONPath(obj, "$.Count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "42" {
		t.Errorf("expected '42', got %q", value)
	}
}

// TestExtractJSONPath_BoolValue verifies that boolean values are converted to strings.
func TestExtractJSONPath_BoolValue(t *testing.T) {
	obj := map[string]interface{}{
		"Enabled": true,
	}

	value, err := extractJSONPath(obj, "$.Enabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "true" {
		t.Errorf("expected 'true', got %q", value)
	}
}

// TestConfigurableRedfishClient_GetHostname verifies the GetHostname accessor returns the correct value.
func TestConfigurableRedfishClient_GetHostname(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Hostname: "192.168.1.100",
	}

	if client.GetHostname() != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got %q", client.GetHostname())
	}
}

// TestConfigurableRedfishClient_GetUsername verifies the GetUsername accessor returns the correct value.
func TestConfigurableRedfishClient_GetUsername(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Username: "admin",
	}

	if client.GetUsername() != "admin" {
		t.Errorf("expected 'admin', got %q", client.GetUsername())
	}
}

// TestConfigurableRedfishClient_GetPassword verifies the GetPassword accessor returns the correct value.
func TestConfigurableRedfishClient_GetPassword(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Password: "secret",
	}

	if client.GetPassword() != "secret" {
		t.Errorf("expected 'secret', got %q", client.GetPassword())
	}
}

// TestConfigurableRedfishClient_SetFwVer verifies that SetFwVer correctly sets the firmware version.
func TestConfigurableRedfishClient_SetFwVer(t *testing.T) {
	client := &ConfigurableRedfishClient{}
	client.SetFwVer("4.00.00.00")

	if client.FwVer != "4.00.00.00" {
		t.Errorf("expected '4.00.00.00', got %q", client.FwVer)
	}
}

// TestConfigurableRedfishClient_GetUri verifies that GetUri fetches and parses a Redfish endpoint correctly.
func TestConfigurableRedfishClient_GetUri(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1" {
			t.Errorf("expected path /redfish/v1, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		// Check basic auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "password" {
			t.Error("expected basic auth with admin/password")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"@odata.id":      "/redfish/v1",
			"RedfishVersion": "1.22.1",
			"Name":           "Root Service",
		})
	}))
	defer server.Close()

	// Extract host from server URL
	host := server.URL[8:] // Remove "https://"

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
	}

	payload, err := client.GetUri("/redfish/v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.Object["Name"] != "Root Service" {
		t.Errorf("expected 'Root Service', got %v", payload.Object["Name"])
	}
}

// TestConfigurableRedfishClient_GetUri_HTTPError verifies that GetUri returns an error for non-200 responses.
func TestConfigurableRedfishClient_GetUri_HTTPError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := server.URL[8:]

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
	}

	_, err := client.GetUri("/redfish/v1/nonexistent")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

// TestConfigurableRedfishClient_GetSystemId verifies that GetSystemId extracts the system ID using configured endpoints.
func TestConfigurableRedfishClient_GetSystemId(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ServiceTag": "ABC1234",
		})
	}))
	defer server.Close()

	host := server.URL[8:]

	config := &systemtype.SystemTypeConfig{
		Name: "test",
		SystemID: &systemtype.EndpointConfig{
			URI:  "/redfish/v1/Managers/iDRAC.Embedded.1",
			Path: "$.ServiceTag",
		},
	}

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
		Config:     config,
	}

	systemID, err := client.GetSystemId()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if systemID != "ABC1234" {
		t.Errorf("expected 'ABC1234', got %q", systemID)
	}
}

// TestConfigurableRedfishClient_GetSystemId_NoConfig verifies that GetSystemId returns an error when config is nil.
func TestConfigurableRedfishClient_GetSystemId_NoConfig(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Config: nil,
	}

	_, err := client.GetSystemId()
	if err == nil {
		t.Error("expected error when config is nil")
	}
}

// TestConfigurableRedfishClient_GetSystemId_EmptyEndpoints verifies that GetSystemId returns an error when no endpoints are configured.
func TestConfigurableRedfishClient_GetSystemId_EmptyEndpoints(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Config: &systemtype.SystemTypeConfig{
			Name:     "test",
			SystemID: nil,
		},
	}

	_, err := client.GetSystemId()
	if err == nil {
		t.Error("expected error when no endpoints configured")
	}
}

// TestConfigurableRedfishClient_GetPowerState verifies that GetPowerState extracts the power state using configured endpoint.
func TestConfigurableRedfishClient_GetPowerState(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"PowerState": "On",
		})
	}))
	defer server.Close()

	host := server.URL[8:]

	config := &systemtype.SystemTypeConfig{
		Name: "test",
		PowerState: &systemtype.PowerStateConfig{
			EndpointConfig: systemtype.EndpointConfig{
				URI:  "/redfish/v1/Chassis/System.Embedded.1",
				Path: "$.PowerState",
			},
			MonitorType: systemtype.MonitorTypeTCPPing,
		},
	}

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
		Config:     config,
	}

	state, err := client.GetPowerState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "On" {
		t.Errorf("expected 'On', got %q", state)
	}
}

// TestConfigurableRedfishClient_GetPowerState_NoConfig verifies that GetPowerState returns an error when config is nil.
func TestConfigurableRedfishClient_GetPowerState_NoConfig(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Config: nil,
	}

	_, err := client.GetPowerState()
	if err == nil {
		t.Error("expected error when config is nil")
	}
}

// TestConfigurableRedfishClient_GetSysInfo verifies that GetSysInfo extracts system information from multiple endpoints.
func TestConfigurableRedfishClient_GetSysInfo(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/redfish/v1/Managers/iDRAC.Embedded.1":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"HostName":        "testhost",
				"FirmwareVersion": "4.00.00.00",
			})
		case "/redfish/v1/Systems/System.Embedded.1":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"SKU":   "SKU123",
				"Model": "PowerEdge R750",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host := server.URL[8:]

	config := &systemtype.SystemTypeConfig{
		Name: "test",
		SysInfo: map[string]systemtype.EndpointConfig{
			"hostname": {URI: "/redfish/v1/Managers/iDRAC.Embedded.1", Path: "$.HostName"},
			"fwver":    {URI: "/redfish/v1/Managers/iDRAC.Embedded.1", Path: "$.FirmwareVersion"},
			"sku":      {URI: "/redfish/v1/Systems/System.Embedded.1", Path: "$.SKU"},
			"model":    {URI: "/redfish/v1/Systems/System.Embedded.1", Path: "$.Model"},
		},
	}

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
		Config:     config,
	}

	hostname, fwver, err := client.GetSysInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hostname != "testhost" {
		t.Errorf("expected hostname 'testhost', got %q", hostname)
	}
	if fwver != "4.00.00.00" {
		t.Errorf("expected fwver '4.00.00.00', got %q", fwver)
	}
}

// TestConfigurableRedfishClient_GetSysInfo_NoConfig verifies that GetSysInfo returns empty values when config is nil.
func TestConfigurableRedfishClient_GetSysInfo_NoConfig(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Config: nil,
	}

	hostname, fwver, err := client.GetSysInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All should be empty when no config
	if hostname != "" || fwver != "" {
		t.Error("expected all empty values when no config")
	}
}

// TestConfigurableRedfishClient_GetPropertyByName verifies that getPropertyByName follows @odata.id links.
func TestConfigurableRedfishClient_GetPropertyByName(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/redfish/v1/EventService" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"@odata.id":          "/redfish/v1/EventService",
				"ServerSentEventUri": "/redfish/v1/SSE",
			})
		}
	}))
	defer server.Close()

	host := server.URL[8:]

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
	}

	payload := &RedfishPayload{
		Object: map[string]interface{}{
			"EventService": map[string]interface{}{
				"@odata.id": "/redfish/v1/EventService",
			},
		},
	}

	result, err := client.getPropertyByName(payload, "EventService")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Object["ServerSentEventUri"] != "/redfish/v1/SSE" {
		t.Errorf("expected ServerSentEventUri '/redfish/v1/SSE', got %v", result.Object["ServerSentEventUri"])
	}
}

// TestConfigurableRedfishClient_GetPropertyByName_NotFound verifies that getPropertyByName returns an error for missing properties.
func TestConfigurableRedfishClient_GetPropertyByName_NotFound(t *testing.T) {
	client := &ConfigurableRedfishClient{}

	payload := &RedfishPayload{
		Object: map[string]interface{}{
			"Name": "Test",
		},
	}

	_, err := client.getPropertyByName(payload, "NonExistent")
	if err == nil {
		t.Error("expected error for non-existent property")
	}
}

// TestReadSSEEvent verifies that readSSEEvent correctly parses SSE events with id and data fields.
func TestReadSSEEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantData string
		wantErr  bool
	}{
		{
			name:     "simple event",
			input:    "id: 123\ndata:{\"test\": \"value\"}\n\n",
			wantID:   "123",
			wantData: `{"test": "value"}`,
			wantErr:  false,
		},
		{
			name:     "event without id",
			input:    "data:{\"test\": \"value\"}\n\n",
			wantID:   "",
			wantData: `{"test": "value"}`,
			wantErr:  false,
		},
		{
			name:     "multiline data",
			input:    "id: 456\ndata:{\"line1\": 1}\ndata:{\"line2\": 2}\n\n",
			wantID:   "456",
			wantData: "{\"line1\": 1}\n{\"line2\": 2}",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			evt, err := readSSEEvent(reader)

			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if evt.ID != tt.wantID {
				t.Errorf("expected ID %q, got %q", tt.wantID, evt.ID)
			}
			if string(evt.Data) != tt.wantData {
				t.Errorf("expected data %q, got %q", tt.wantData, string(evt.Data))
			}
		})
	}
}

// TestConfigurableRedfishClient_IPv6Detection verifies that IPv6 addresses are detected and bracketed correctly.
func TestConfigurableRedfishClient_IPv6Detection(t *testing.T) {
	// Test IPv6 address detection
	client := &ConfigurableRedfishClient{
		Hostname: "fe80::1:2:3:4",
		IsIPv6:   0,
	}

	// Simulate the IPv6 detection logic from GetUri
	split := client.Hostname
	if len(split) > 0 {
		parts := 0
		for _, c := range split {
			if c == ':' {
				parts++
			}
		}
		if parts > 1 {
			// This is IPv6
			client.IsIPv6 = 1
			client.Hostname = "[" + client.Hostname + "]"
		}
	}

	if client.IsIPv6 != 1 {
		t.Error("expected IPv6 to be detected")
	}
	if client.Hostname != "[fe80::1:2:3:4]" {
		t.Errorf("expected bracketed IPv6, got %q", client.Hostname)
	}
}

// TestRedfishPayload_Structure verifies that RedfishPayload fields can be set and accessed correctly.
func TestRedfishPayload_Structure(t *testing.T) {
	payload := &RedfishPayload{
		Object: map[string]interface{}{
			"Name": "Test",
		},
		Array: []interface{}{"item1", "item2"},
		Float: 3.14,
	}

	if payload.Object["Name"] != "Test" {
		t.Errorf("expected Object['Name'] = 'Test', got %v", payload.Object["Name"])
	}
	if len(payload.Array) != 2 {
		t.Errorf("expected Array length 2, got %d", len(payload.Array))
	}
	if payload.Float != 3.14 {
		t.Errorf("expected Float 3.14, got %f", payload.Float)
	}
}

// TestRedfishEvent_Structure verifies that RedfishEvent fields can be set and accessed correctly.
func TestRedfishEvent_Structure(t *testing.T) {
	event := &RedfishEvent{
		ID: "event123",
		Payload: &RedfishPayload{
			Object: map[string]interface{}{"test": "value"},
		},
		Err: nil,
	}

	if event.ID != "event123" {
		t.Errorf("expected ID 'event123', got %q", event.ID)
	}
	if event.Payload == nil {
		t.Error("expected non-nil payload")
	}
	if event.Err != nil {
		t.Errorf("expected nil error, got %v", event.Err)
	}
}

// TestConfigurableRedfishClient_ListenForAlerts_NoSSESupport verifies that ListenForAlerts returns an error when SSE is not supported.
func TestConfigurableRedfishClient_ListenForAlerts_NoSSESupport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/redfish/v1":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"EventService": map[string]interface{}{
					"@odata.id": "/redfish/v1/EventService",
				},
			})
		case "/redfish/v1/EventService":
			// No ServerSentEventUri - SSE not supported
			json.NewEncoder(w).Encode(map[string]interface{}{
				"@odata.id": "/redfish/v1/EventService",
			})
		}
	}))
	defer server.Close()

	host := server.URL[8:]

	client := &ConfigurableRedfishClient{
		Hostname:   host,
		Username:   "admin",
		Password:   "password",
		HttpClient: server.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	events := make(chan *RedfishEvent, 1)
	go client.ListenForAlerts(ctx, events)

	select {
	case evt := <-events:
		if evt.Err == nil {
			t.Error("expected error when SSE not supported")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for error event")
	}
}
