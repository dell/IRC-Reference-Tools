package systemtype

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewRegistry verifies that NewRegistry returns an empty, non-nil registry.
func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.GetAll()) != 0 {
		t.Errorf("expected empty registry, got %d configs", len(r.GetAll()))
	}
}

// TestRegistry_RegisterAndGet verifies that configs can be registered and retrieved by name.
func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()

	cfg := &SystemTypeConfig{
		Name: "testtype",
	}
	r.Register(cfg)

	got := r.Get("testtype")
	if got == nil {
		t.Fatal("expected to get registered config")
	}
	if got.Name != "testtype" {
		t.Errorf("expected name 'testtype', got %q", got.Name)
	}

	// Test non-existent
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for non-existent config")
	}
}

// TestRegistry_GetAll verifies that GetAll returns all registered configs.
func TestRegistry_GetAll(t *testing.T) {
	r := NewRegistry()

	r.Register(&SystemTypeConfig{Name: "type1"})
	r.Register(&SystemTypeConfig{Name: "type2"})

	all := r.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 configs, got %d", len(all))
	}
}

// TestRegistry_GetNames verifies that GetNames returns all registered config names.
func TestRegistry_GetNames(t *testing.T) {
	r := NewRegistry()

	r.Register(&SystemTypeConfig{Name: "alpha"})
	r.Register(&SystemTypeConfig{Name: "beta"})

	names := r.GetNames()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}

	// Check both names are present
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["alpha"] || !found["beta"] {
		t.Errorf("expected alpha and beta, got %v", names)
	}
}

// TestRegistry_LoadFromFile verifies that a YAML config file is correctly loaded and parsed.
func TestRegistry_LoadFromFile(t *testing.T) {
	// Create temp directory and YAML file
	tmpDir, err := os.MkdirTemp("", "registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlContent := `name: "testconfig"
isBuiltIn: true
systemId:
  uri: "/redfish/v1"
  path: "$.ServiceTag"
powerState:
  uri: "/redfish/v1/Chassis/1"
  path: "$.PowerState"
sseFilter: "?$filter=EventType"
shutdown:
  graceful:
    uri: "/test/shutdown"
    method: "POST"
`
	yamlPath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml file: %v", err)
	}

	r := NewRegistry()
	cfg, err := r.LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if cfg.Name != "testconfig" {
		t.Errorf("expected name 'testconfig', got %q", cfg.Name)
	}
	if !cfg.IsBuiltIn {
		t.Error("expected IsBuiltIn to be true")
	}
	if cfg.SystemID == nil {
		t.Errorf("expected 1 systemId entry, got %v", cfg.SystemID)
	}
	if cfg.SSEFilter != "?$filter=EventType" {
		t.Errorf("unexpected sseFilter: %q", cfg.SSEFilter)
	}

	// Verify it's registered
	if r.Get("testconfig") == nil {
		t.Error("config should be registered after LoadFromFile")
	}
}

// TestRegistry_LoadFromFile_MissingName verifies that an error is returned when the name field is missing.
func TestRegistry_LoadFromFile_MissingName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlContent := `isBuiltIn: true
systemId:
  - uri: "/redfish/v1"
    path: "$.ServiceTag"
`
	yamlPath := filepath.Join(tmpDir, "noname.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml file: %v", err)
	}

	r := NewRegistry()
	_, err = r.LoadFromFile(yamlPath)
	if err == nil {
		t.Error("expected error for missing name field")
	}
}

// TestRegistry_LoadFromDirectory verifies that all YAML files in a directory are loaded.
func TestRegistry_LoadFromDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two YAML files
	yaml1 := `name: "config1"
systemId:
  uri: "/redfish/v1"
  path: "$.Id"
powerState:
  uri: "/redfish/v1/Chassis/1"
  path: "$.PowerState"
shutdown:
  graceful:
    uri: "/shutdown"
    method: "POST"
`
	yaml2 := `name: "config2"
systemId:
  uri: "/redfish/v1"
  path: "$.Id"
powerState:
  uri: "/redfish/v1/Chassis/1"
  path: "$.PowerState"
shutdown:
  graceful:
    uri: "/shutdown"
    method: "POST"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "config1.yaml"), []byte(yaml1), 0644); err != nil {
		t.Fatalf("failed to write yaml1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config2.yml"), []byte(yaml2), 0644); err != nil {
		t.Fatalf("failed to write yaml2: %v", err)
	}
	// Create a non-yaml file that should be ignored
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	r := NewRegistry()
	if err := r.LoadFromDirectory(tmpDir); err != nil {
		t.Fatalf("LoadFromDirectory failed: %v", err)
	}

	if len(r.GetAll()) != 2 {
		t.Errorf("expected 2 configs, got %d", len(r.GetAll()))
	}
	if r.Get("config1") == nil {
		t.Error("expected config1 to be loaded")
	}
	if r.Get("config2") == nil {
		t.Error("expected config2 to be loaded")
	}
}

// TestRegistry_LoadFromDirectory_NonExistent verifies that an error is returned for non-existent directories.
func TestRegistry_LoadFromDirectory_NonExistent(t *testing.T) {
	r := NewRegistry()
	err := r.LoadFromDirectory("/nonexistent/path/12345")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

// TestApplyDefaults verifies that applyDefaults sets the default protocol to https when not specified.
func TestApplyDefaults(t *testing.T) {
	cfg := &SystemTypeConfig{
		Name: "test",
		Shutdown: &ShutdownConfig{
			Graceful: &APIAction{
				URI:    "/shutdown",
				Method: "POST",
				// Protocol is empty - should be set to https
			},
			Force: &APIAction{
				URI:      "/shutdown",
				Method:   "POST",
				Protocol: "http", // Already set - should not change
			},
		},
	}

	applyDefaults(cfg)

	if cfg.Shutdown.Graceful.Protocol != "https" {
		t.Errorf("expected graceful protocol 'https', got %q", cfg.Shutdown.Graceful.Protocol)
	}
	if cfg.Shutdown.Force.Protocol != "http" {
		t.Errorf("expected force protocol 'http' (unchanged), got %q", cfg.Shutdown.Force.Protocol)
	}
}

// TestApplyDefaults_NilShutdown verifies that applyDefaults handles nil shutdown config without panicking.
func TestApplyDefaults_NilShutdown(t *testing.T) {
	cfg := &SystemTypeConfig{
		Name: "test",
	}

	// Should not panic
	applyDefaults(cfg)
}

// TestLoadFromFile_WithShutdownConfig verifies that shutdown config is correctly parsed from YAML with defaults applied.
func TestLoadFromFile_WithShutdownConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlContent := `name: "idrac"
systemId:
  uri: "/redfish/v1"
  path: "$.ServiceTag"
powerState:
  uri: "/redfish/v1/Chassis/System.Embedded.1"
  path: "$.PowerState"
shutdown:
  graceful:
    uri: "/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "GracefulShutdown"}'
  force:
    uri: "/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "ForceOff"}'
`
	yamlPath := filepath.Join(tmpDir, "idrac.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml file: %v", err)
	}

	r := NewRegistry()
	cfg, err := r.LoadFromFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if cfg.Shutdown == nil {
		t.Fatal("expected shutdown config")
	}
	if cfg.Shutdown.Graceful == nil {
		t.Fatal("expected graceful shutdown action")
	}
	if cfg.Shutdown.Force == nil {
		t.Fatal("expected force shutdown action")
	}

	// Verify defaults were applied
	if cfg.Shutdown.Graceful.Protocol != "https" {
		t.Errorf("expected graceful protocol 'https', got %q", cfg.Shutdown.Graceful.Protocol)
	}
	if cfg.Shutdown.Force.Protocol != "https" {
		t.Errorf("expected force protocol 'https', got %q", cfg.Shutdown.Force.Protocol)
	}

	if cfg.Shutdown.Graceful.Method != "POST" {
		t.Errorf("expected method 'POST', got %q", cfg.Shutdown.Graceful.Method)
	}
}

// TestValidatePowerState verifies that validatePowerState correctly validates PowerStateConfig
func TestValidatePowerState(t *testing.T) {
	tests := []struct {
		name        string
		config      *SystemTypeConfig
		path        string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid redfish-sse config",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Chassis/1",
						Path: "$.PowerState",
					},
					MonitorType:   MonitorTypeRedfishSSE,
					PowerOnMsgId:  "POWER.ON",
					PowerOffMsgId: "POWER.OFF",
				},
			},
			path:        "test.yaml",
			expectError: false,
		},
		{
			name: "valid tcp-ping config",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					MonitorType: MonitorTypeTCPPing,
				},
			},
			path:        "test.yaml",
			expectError: false,
		},
		{
			name: "valid redfish-get config",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Systems/1",
						Path: "$.PowerState",
					},
					MonitorType: MonitorTypeRedfishGet,
				},
			},
			path:        "test.yaml",
			expectError: false,
		},
		{
			name: "missing power state",
			config: &SystemTypeConfig{
				PowerState: nil,
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "missing required 'powerState' field",
		},
		{
			name: "redfish-sse missing message IDs",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Chassis/1",
						Path: "$.PowerState",
					},
					MonitorType: MonitorTypeRedfishSSE,
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "powerOnMsgId and powerOffMsgId required when monitorType is 'redfish-sse'",
		},
		{
			name: "redfish-sse with valid message args",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Systems/1",
						Path: "$.PowerState",
					},
					MonitorType:         MonitorTypeRedfishSSE,
					PowerOnMsgId:        "EventLog.1.0.StatusChange",
					PowerOffMsgId:       "EventLog.1.0.StatusChange",
					PowerOnMessageArgs:  "3-Enabled",
					PowerOffMessageArgs: "3-StandbyOffline",
				},
			},
			path:        "test.yaml",
			expectError: false,
		},
		{
			name: "redfish-sse with invalid message args format - missing hyphen",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Systems/1",
						Path: "$.PowerState",
					},
					MonitorType:         MonitorTypeRedfishSSE,
					PowerOnMsgId:        "EventLog.1.0.StatusChange",
					PowerOffMsgId:       "EventLog.1.0.StatusChange",
					PowerOnMessageArgs:  "3Enabled",
					PowerOffMessageArgs: "3StandbyOffline",
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "messageArgs must be in format 'index-value'",
		},
		{
			name: "redfish-sse with invalid message args format - non-numeric index",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI:  "/redfish/v1/Systems/1",
						Path: "$.PowerState",
					},
					MonitorType:         MonitorTypeRedfishSSE,
					PowerOnMsgId:        "EventLog.1.0.StatusChange",
					PowerOffMsgId:       "EventLog.1.0.StatusChange",
					PowerOnMessageArgs:  "abc-Enabled",
					PowerOffMessageArgs: "3-StandbyOffline",
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "messageArgs index must be a number",
		},
		{
			name: "redfish-sse missing URI",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						Path: "$.PowerState",
					},
					MonitorType:   MonitorTypeRedfishSSE,
					PowerOnMsgId:  "POWER.ON",
					PowerOffMsgId: "POWER.OFF",
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "uri and path required when monitorType is 'redfish-sse'",
		},
		{
			name: "redfish-sse missing path",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					EndpointConfig: EndpointConfig{
						URI: "/redfish/v1/Chassis/1",
					},
					MonitorType:   MonitorTypeRedfishSSE,
					PowerOnMsgId:  "POWER.ON",
					PowerOffMsgId: "POWER.OFF",
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "uri and path required when monitorType is 'redfish-sse'",
		},
		{
			name: "invalid monitor type",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					MonitorType: "invalid-type",
				},
			},
			path:        "test.yaml",
			expectError: true,
			errorMsg:    "invalid monitorType 'invalid-type', must be 'redfish-sse', 'redfish-get', or 'tcp-ping'",
		},
		{
			name: "empty monitor type defaults to tcp-ping",
			config: &SystemTypeConfig{
				PowerState: &PowerStateConfig{
					MonitorType: "",
				},
			},
			path:        "test.yaml",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePowerState(tt.config, tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %q", err.Error())
				}
			}

			// Check that empty monitor type gets defaulted to tcp-ping
			if tt.config.PowerState != nil && tt.config.PowerState.MonitorType == "" {
				if tt.config.PowerState.MonitorType != MonitorTypeTCPPing {
					t.Errorf("expected monitor type to be defaulted to %q, got %q", MonitorTypeTCPPing, tt.config.PowerState.MonitorType)
				}
			}
		})
	}
}

// contains checks if the error message contains the expected substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
