package redfish

import (
	"testing"

	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

// TestNewConfigurableDevices verifies that NewConfigurableDevices returns an empty, non-nil map.
func TestNewConfigurableDevices(t *testing.T) {
	devices := NewConfigurableDevices()
	if devices == nil {
		t.Fatal("expected non-nil devices map")
	}
	if len(devices) != 0 {
		t.Errorf("expected empty map, got %d items", len(devices))
	}
}

// TestConfigurableDevices_AddAndRetrieve verifies that devices can be added to and retrieved from the map.
func TestConfigurableDevices_AddAndRetrieve(t *testing.T) {
	devices := NewConfigurableDevices()

	device := &ConfigurableDevice{
		SystemID: "TEST123",
		HostName: "testhost",
	}

	devices["192.168.1.100"] = device

	retrieved := devices["192.168.1.100"]
	if retrieved == nil {
		t.Fatal("expected to retrieve device")
	}
	if retrieved.SystemID != "TEST123" {
		t.Errorf("expected SystemID 'TEST123', got %q", retrieved.SystemID)
	}
}

// TestConfigurableDevice_Structure verifies that all ConfigurableDevice fields are correctly assigned.
func TestConfigurableDevice_Structure(t *testing.T) {
	config := &systemtype.SystemTypeConfig{
		Name:        "idrac",
		HasWatchdog: true,
		HasValves:   false,
	}

	device := &ConfigurableDevice{
		Config:   config,
		SystemID: "ABC123",
		HostName: "testhost",
		FwVer:    "4.00.00.00",
		State:    auth.STARTING,
	}

	if device.Config.Name != "idrac" {
		t.Errorf("expected config name 'idrac', got %q", device.Config.Name)
	}
	if device.SystemID != "ABC123" {
		t.Errorf("expected SystemID 'ABC123', got %q", device.SystemID)
	}
	if device.HostName != "testhost" {
		t.Errorf("expected HostName 'testhost', got %q", device.HostName)
	}
	if device.State != auth.STARTING {
		t.Errorf("expected State STARTING, got %q", device.State)
	}
}

// TestValidateAndAddConfigurableDevice_EmptyIP verifies that an error is returned when the service IP is empty.
func TestValidateAndAddConfigurableDevice_EmptyIP(t *testing.T) {
	devices := NewConfigurableDevices()
	config := &systemtype.SystemTypeConfig{Name: "test"}

	service := &auth.Service{
		Ip: "",
	}

	_, err := ValidateAndAddConfigurableDevice(service, devices, config)
	if err == nil {
		t.Error("expected error for empty IP")
	}
}

// TestValidateAndAddConfigurableDevice_DuplicateIP verifies that an error is returned when adding a device with a duplicate IP.
func TestValidateAndAddConfigurableDevice_DuplicateIP(t *testing.T) {
	devices := NewConfigurableDevices()
	config := &systemtype.SystemTypeConfig{Name: "test"}

	// Add first device
	devices["192.168.1.100"] = &ConfigurableDevice{
		SystemID: "EXISTING",
	}

	service := &auth.Service{
		Ip: "192.168.1.100",
		Auth: map[string]string{
			"username": "admin",
			"password": "password",
		},
	}

	_, err := ValidateAndAddConfigurableDevice(service, devices, config)
	if err == nil {
		t.Error("expected error for duplicate IP")
	}
}

// TestConfigurableDevice_EventsChannel verifies that the Events channel can be initialized and used for sending/receiving events.
func TestConfigurableDevice_EventsChannel(t *testing.T) {
	device := &ConfigurableDevice{}

	// Events channel should be nil initially
	if device.Events != nil {
		t.Error("expected Events channel to be nil initially")
	}

	// Simulate StartAlertListener initialization
	device.Events = make(chan *RedfishEvent, 10)

	if device.Events == nil {
		t.Error("expected Events channel to be initialized")
	}

	// Test sending an event
	event := &RedfishEvent{
		ID: "test123",
		Payload: &RedfishPayload{
			Object: map[string]interface{}{"test": "value"},
		},
	}

	go func() {
		device.Events <- event
	}()

	received := <-device.Events
	if received.ID != "test123" {
		t.Errorf("expected event ID 'test123', got %q", received.ID)
	}
}

// TestConfigurableDevice_ContextCancel verifies that Ctx and CtxCancel are nil on a new device.
func TestConfigurableDevice_ContextCancel(t *testing.T) {
	device := &ConfigurableDevice{}

	// Ctx and CtxCancel should be nil initially
	if device.Ctx != nil || device.CtxCancel != nil {
		t.Error("expected Ctx and CtxCancel to be nil initially")
	}
}

// TestParseAndSendEvents_NoEventsField verifies that parseAndSendEvents handles payloads without an Events field gracefully.
func TestParseAndSendEvents_NoEventsField(t *testing.T) {
	device := &ConfigurableDevice{
		SystemID: "TEST123",
	}

	// Payload without Events field - should log and return without panic
	payload := &RedfishPayload{
		Object: map[string]interface{}{
			"Name": "Test",
		},
	}

	// This should not panic
	device.parseAndSendEvents(payload, nil)
}

// TestParseAndSendEvents_EventsNotArray verifies that parseAndSendEvents handles payloads where Events is not an array.
func TestParseAndSendEvents_EventsNotArray(t *testing.T) {
	device := &ConfigurableDevice{
		SystemID: "TEST123",
	}

	// Payload with Events that is not an array
	payload := &RedfishPayload{
		Object: map[string]interface{}{
			"Events": "not an array",
		},
	}

	// This should not panic
	device.parseAndSendEvents(payload, nil)
}

// TestConfigurableDevice_ClientAccessors verifies that the device's client accessor methods return correct values.
func TestConfigurableDevice_ClientAccessors(t *testing.T) {
	client := &ConfigurableRedfishClient{
		Hostname: "192.168.1.100",
		Username: "admin",
		Password: "secret",
	}

	device := &ConfigurableDevice{
		Client: client,
	}

	if device.Client.GetHostname() != "192.168.1.100" {
		t.Errorf("expected hostname '192.168.1.100', got %q", device.Client.GetHostname())
	}
	if device.Client.GetUsername() != "admin" {
		t.Errorf("expected username 'admin', got %q", device.Client.GetUsername())
	}
	if device.Client.GetPassword() != "secret" {
		t.Errorf("expected password 'secret', got %q", device.Client.GetPassword())
	}
}

// TestConfigurableDevice_StateTransitions verifies that device state can be set to various auth states.
func TestConfigurableDevice_StateTransitions(t *testing.T) {
	device := &ConfigurableDevice{}

	// Test state transitions
	states := []string{
		auth.STARTING,
		auth.RUNNING,
		auth.MONITORING,
		auth.CONNFAILED,
	}

	for _, state := range states {
		device.State = state
		if device.State != state {
			t.Errorf("expected state %q, got %q", state, device.State)
		}
	}
}
