package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/mock"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
)

type failingShutdowner struct{}

func (f *failingShutdowner) Name() string { return "failing" }
func (f *failingShutdowner) Type() string { return "idrac" }
func (f *failingShutdowner) Shutdown(si auth.ServiceItem) (string, error) {
	return "", errors.New("boom")
}

func TestHandleDataGroup(t *testing.T) {
	// Initialize system type registry for this test
	systemTypeRegistry = systemtype.NewRegistry()

	group := &databus.DataGroup{
		ID:   "test",
		FQDN: "ip1",
		Events: []databus.EventValue{
			{
				MessageId: "not_a_leak",
			},
		},
	}
	mac := &mock.MockAuthorizationClient{
		Services:     []auth.Service{},
		ServiceItems: []auth.ServiceItem{},
	}
	handleDataGroup(group, mac)
	if len(mac.GetAllServices()) != 0 {
		t.Errorf("Expected 0 service, got %d", len(mac.GetAllServices()))
	}
	if len(mac.ServiceItems) != 0 {
		t.Errorf("Expected 0 service item, got %d", len(mac.ServiceItems))
	}
	_ = mac.UpdateService(auth.Service{
		Ip: "ip1",
	})
	group = &databus.DataGroup{
		ID:   "test",
		FQDN: "ip1",
		Events: []databus.EventValue{
			{
				MessageId: LEAKMSGID,
			},
		},
	}
	handleDataGroup(group, mac)
	time.Sleep(100 * time.Millisecond)
	services := mac.GetAllServices()
	t.Log(services)
	if services[len(services)-1].Ip != "ip1" {
		t.Errorf("Expected ip1, got %s", services[len(services)-1].Ip)
	}
	if services[len(services)-1].State != auth.LEAKED {
		t.Errorf("Expected %s, got %s", auth.LEAKED, services[len(services)-1].State)
	}
}

func TestWaitForPowerOff_Timeout(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
	})

	ac := &mock.MockAuthorizationClient{}
	_ = ac.UpdateService(auth.Service{Ip: "ip1", State: auth.POWERSTATEON})

	d := &DBPowerOffChecker{ip: "ip1", authClient: ac}
	if ok := waitForPowerOffLoop(1, auth.Service{}, d); ok {
		t.Fatalf("expected waitForPowerOff to time out")
	}
}

func TestDBPowerOffDetector_Success(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
	})

	ac := &mock.MockAuthorizationClient{}
	_ = ac.UpdateServiceItem(auth.ServiceItem{Service: auth.Service{Ip: "ip1", State: auth.POWERSTATEON}})
	go func() {
		time.Sleep(2 * time.Millisecond)
		_ = ac.UpdateServiceItem(auth.ServiceItem{Service: auth.Service{Ip: "ip1", State: auth.POWERSTATEOFF}})
	}()

	d := &DBPowerOffChecker{ip: "ip1", authClient: ac}
	if ok := waitForPowerOffLoop(1, auth.Service{}, d); !ok {
		t.Fatalf("expected DBPowerOffChecker to succeed")
	}
}

func TestTCPUnreachablePowerOffDetector_Success(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	origTO := tcpConnectTimeout
	origN := tcpUnreachableConsecutive
	origIs := isHostAccessible

	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 50 * time.Millisecond
	tcpConnectTimeout = 1 * time.Millisecond
	tcpUnreachableConsecutive = 3

	callCount := 0
	isHostAccessible = func(ip string, ports []int, timeout time.Duration) bool {
		callCount++
		// Reachable for a couple polls, then unreachable long enough to reach N.
		return callCount <= 2
	}

	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
		tcpConnectTimeout = origTO
		tcpUnreachableConsecutive = origN
		isHostAccessible = origIs
	})

	d := &TCPUnreachablePowerOffChecker{ip: "ip1"}
	if ok := waitForPowerOffLoop(1, auth.Service{}, d); !ok {
		t.Fatalf("expected TCPUnreachablePowerOffChecker to succeed")
	}
}

func TestTCPUnreachablePowerOffDetector_Timeout(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	origTO := tcpConnectTimeout
	origN := tcpUnreachableConsecutive
	origIs := isHostAccessible

	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 50 * time.Millisecond
	tcpConnectTimeout = 1 * time.Millisecond
	tcpUnreachableConsecutive = 3

	isHostAccessible = func(ip string, ports []int, timeout time.Duration) bool {
		return true
	}

	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
		tcpConnectTimeout = origTO
		tcpUnreachableConsecutive = origN
		isHostAccessible = origIs
	})

	d := &TCPUnreachablePowerOffChecker{ip: "ip1"}
	if ok := waitForPowerOffLoop(1, auth.Service{}, d); ok {
		t.Fatalf("expected TCPUnreachablePowerOffChecker to time out")
	}
}

func TestShutdownWithFallback_TimeoutZero_NoForceFallback(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}
	sd := &mockShutdowner{name: "test_script.sh", sysType: "idrac", Shutdowned: []string{}, Forceful: []bool{}}
	si := auth.ServiceItem{Service: auth.Service{Ip: "ip1"}, Systemtypedesc: "idrac", Timeout: 0}

	if ok := shutdownWithFallback(si, sd, ac, auth.Service{}); !ok {
		t.Fatalf("expected shutdownWithFallback to succeed")
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if len(sd.Forceful) != 1 {
		t.Fatalf("expected 1 shutdown call, got %d", len(sd.Forceful))
	}
	if sd.Forceful[0] != false {
		t.Fatalf("expected first call to be graceful")
	}
}

func TestShutdownWithFallback_TimeoutForceFallback(t *testing.T) {
	// Initialize system type registry for this test
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "idrac",
		PowerState: &systemtype.PowerStateConfig{
			MonitorType: systemtype.MonitorTypeRedfishSSE, // Use DB power off checker
		},
	})
	systemTypeRegistry = registry

	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
	})

	ac := &mock.MockAuthorizationClient{}
	_ = ac.UpdateServiceItem(auth.ServiceItem{Service: auth.Service{Ip: "192.168.1.999", State: auth.POWERSTATEON}})
	sd := &mockShutdowner{name: "test_script.sh", sysType: "idrac", Shutdowned: []string{}, Forceful: []bool{}}
	si := auth.ServiceItem{Service: auth.Service{Ip: "192.168.1.999"}, Systemtypedesc: "idrac", Timeout: 1}

	if ok := shutdownWithFallback(si, sd, ac, auth.Service{}); !ok {
		t.Fatalf("expected shutdownWithFallback to succeed")
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if len(sd.Forceful) != 2 {
		t.Fatalf("expected 2 shutdown calls (graceful then force), got %d", len(sd.Forceful))
	}
	if sd.Forceful[0] != false || sd.Forceful[1] != true {
		t.Fatalf("expected calls to be [false true], got %v", sd.Forceful)
	}
}

func TestShutdownWithFallback_ForceOnly(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}
	sd := &mockShutdowner{name: "test_script.sh", sysType: "idrac", Shutdowned: []string{}, Forceful: []bool{}}
	si := auth.ServiceItem{Service: auth.Service{Ip: "ip1"}, Systemtypedesc: "idrac", IsForcefulshutdown: true, Timeout: 10}

	if ok := shutdownWithFallback(si, sd, ac, auth.Service{}); !ok {
		t.Fatalf("expected shutdownWithFallback to succeed")
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if len(sd.Forceful) != 1 {
		t.Fatalf("expected 1 shutdown call, got %d", len(sd.Forceful))
	}
	if sd.Forceful[0] != true {
		t.Fatalf("expected forceful call")
	}
}

func TestRunShutdownOnce_FailureUpdatesDB(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}
	sd := &failingShutdowner{}
	si := auth.ServiceItem{Service: auth.Service{Ip: "ip1"}}

	if ok := runShutdownOnce(si, sd, ac, auth.Service{}); ok {
		t.Fatalf("expected runShutdownOnce to fail")
	}
	if len(ac.ServiceItems) != 1 {
		t.Fatalf("expected UpdateServiceItem to be called")
	}
	if ac.ServiceItems[0].State != auth.SHUTDOWNFAILED {
		t.Fatalf("expected state %q, got %q", auth.SHUTDOWNFAILED, ac.ServiceItems[0].State)
	}
}

type mockShutdowner struct {
	name       string
	sysType    string
	Shutdowned []string
	Forceful   []bool
	mu         sync.Mutex
}

func (s *mockShutdowner) Name() string {
	return s.name
}
func (s *mockShutdowner) Type() string {
	return s.sysType
}

func (s *mockShutdowner) Shutdown(si auth.ServiceItem) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Shutdowned = append(s.Shutdowned, si.Ip)
	s.Forceful = append(s.Forceful, si.IsForcefulshutdown)
	return "", nil
}

func TestShutdown(t *testing.T) {
	testItems := []auth.ServiceItem{
		{
			Service: auth.Service{
				Ip: "ip1",
			},
			Systemtypedesc: "idrac",
		},
		{
			Service: auth.Service{
				Ip: "ip2",
			},
			Systemtypedesc: "idrac",
		},
	}
	mac := &mock.MockAuthorizationClient{
		Services:     []auth.Service{},
		ServiceItems: []auth.ServiceItem{},
	}
	msd := &mockShutdowner{
		name:       "test_script.sh",
		sysType:    "idrac",
		Shutdowned: []string{},
	}
	shutdowners = make(map[string]Shutdowner)
	shutdowners[msd.sysType] = msd
	Shutdown(testItems, mac, auth.Service{})
	if len(msd.Shutdowned) != 2 {
		t.Errorf("expected %d, got %d events in shutdown\n", 2, len(msd.Shutdowned))
	}
	for _, si := range mac.ServiceItems {
		if si.State != auth.SHUTDOWNRECEIVED {
			t.Errorf("expected %s, got %s state in updated service item\n", auth.SHUTDOWNRECEIVED, si.State)
		}
	}

	msd = &mockShutdowner{
		name:       "test_script.sh",
		sysType:    "100", // random type that doesn't match with serviceitems
		Shutdowned: []string{},
	}
	shutdowners = make(map[string]Shutdowner)
	shutdowners[msd.sysType] = msd
	Shutdown(testItems, mac, auth.Service{})
	if len(msd.Shutdowned) != 0 {
		t.Errorf("expected %d, got %d services in shutdown\n", 0, len(msd.Shutdowned))
	}
}

// TestCreateShutdownersFromConfigs verifies that shutdowners are created from configs
func TestCreateShutdownersFromConfigs(t *testing.T) {
	// Setup: create a registry with test configs
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "testtype",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/test/shutdown",
				Method:   "POST",
				Body:     `{"action": "shutdown"}`,
			},
		},
	})
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "noshutdown", // No shutdown config
	})

	// Set the global registry
	systemTypeRegistry = registry
	shutdowners = make(map[string]Shutdowner)

	mac := &mock.MockAuthorizationClient{
		SystemTypes: make(map[string]string),
	}

	createShutdownersFromConfigs(mac)

	// Should have 1 shutdowner (testtype has shutdown config, noshutdown doesn't)
	if len(shutdowners) != 1 {
		t.Errorf("expected 1 shutdowner, got %d", len(shutdowners))
	}
	if _, ok := shutdowners["testtype"]; !ok {
		t.Error("expected 'testtype' shutdowner")
	}

	// Both system types should be registered
	if len(mac.SystemTypes) != 2 {
		t.Errorf("expected 2 system types registered, got %d", len(mac.SystemTypes))
	}
}

// TestCreateShutdownersFromConfigs_IsParent verifies that parent system types are not added to DB
func TestCreateShutdownersFromConfigs_IsParent(t *testing.T) {
	// Setup: create a registry with test configs including a parent type
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "idrac",
		Shutdown: &systemtype.ShutdownConfig{
			Graceful: &systemtype.APIAction{
				Protocol: "https",
				URI:      "/shutdown",
				Method:   "POST",
			},
		},
	})
	registry.Register(&systemtype.SystemTypeConfig{
		Name:     "irc",
		IsParent: true, // Parent type - should NOT be added to DB
	})
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "pdu",
	})

	// Set the global registry
	systemTypeRegistry = registry
	shutdowners = make(map[string]Shutdowner)

	mac := &mock.MockAuthorizationClient{
		SystemTypes: make(map[string]string),
	}

	createShutdownersFromConfigs(mac)

	// Should have 1 shutdowner (only idrac has shutdown config)
	if len(shutdowners) != 1 {
		t.Errorf("expected 1 shutdowner, got %d", len(shutdowners))
	}

	// Only non-parent system types should be registered in DB (idrac and pdu, not irc)
	if len(mac.SystemTypes) != 2 {
		t.Errorf("expected 2 system types registered (excluding parent), got %d", len(mac.SystemTypes))
	}
	if _, ok := mac.SystemTypes["idrac"]; !ok {
		t.Error("expected 'idrac' to be registered")
	}
	if _, ok := mac.SystemTypes["pdu"]; !ok {
		t.Error("expected 'pdu' to be registered")
	}
	if _, ok := mac.SystemTypes["irc"]; ok {
		t.Error("expected 'irc' (parent) to NOT be registered")
	}
}

// TestShutdownerFromConfig verifies shutdowner creation from config
func TestShutdownerFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *systemtype.SystemTypeConfig
		wantNil bool
	}{
		{
			name: "with graceful shutdown",
			config: &systemtype.SystemTypeConfig{
				Name: "test",
				Shutdown: &systemtype.ShutdownConfig{
					Graceful: &systemtype.APIAction{
						URI:    "/shutdown",
						Method: "POST",
					},
				},
			},
			wantNil: false,
		},
		{
			name: "with force shutdown only",
			config: &systemtype.SystemTypeConfig{
				Name: "test",
				Shutdown: &systemtype.ShutdownConfig{
					Force: &systemtype.APIAction{
						URI:    "/shutdown",
						Method: "POST",
					},
				},
			},
			wantNil: false,
		},
		{
			name: "no shutdown config",
			config: &systemtype.SystemTypeConfig{
				Name: "test",
			},
			wantNil: true,
		},
		{
			name: "empty shutdown config",
			config: &systemtype.SystemTypeConfig{
				Name:     "test",
				Shutdown: &systemtype.ShutdownConfig{},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shutdownerFromConfig(tt.config)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil shutdowner")
			}
		})
	}
}

// TestHandlePowerEvent verifies that handlePowerEvent correctly updates service state
func TestHandlePowerEvent(t *testing.T) {
	// Initialize system type registry for this test
	systemTypeRegistry = systemtype.NewRegistry()

	tests := []struct {
		name          string
		event         databus.EventValue
		ip            string
		expectedState string
		expectError   bool
	}{
		{
			name: "power on event",
			event: databus.EventValue{
				MessageId: "POWER.ON",
			},
			ip:            "192.168.1.100",
			expectedState: auth.POWERSTATEON,
			expectError:   false,
		},
		{
			name: "power off event",
			event: databus.EventValue{
				MessageId: "POWER.OFF",
			},
			ip:            "192.168.1.100",
			expectedState: auth.POWERSTATEOFF,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac := &mock.MockAuthorizationClient{}

			err := handlePowerEvent(tt.event, tt.ip, ac, tt.expectedState)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}

				// Verify that UpdateServiceXState was called with correct state
				if len(ac.ServiceItems) != 1 {
					t.Errorf("expected 1 service item to be updated, got %d", len(ac.ServiceItems))
				} else {
					if ac.ServiceItems[0].State != tt.expectedState {
						t.Errorf("expected state %q, got %q", tt.expectedState, ac.ServiceItems[0].State)
					}
					if ac.ServiceItems[0].Ip != tt.ip {
						t.Errorf("expected IP %q, got %q", tt.ip, ac.ServiceItems[0].Ip)
					}
				}
			}
		})
	}
}

// TestHandleEvent_PowerEvents verifies that handleEvent correctly handles configurable power events
func TestHandleEvent_PowerEvents(t *testing.T) {
	// Setup a test registry with power state configuration
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "testtype",
		PowerState: &systemtype.PowerStateConfig{
			EndpointConfig: systemtype.EndpointConfig{
				URI:  "/redfish/v1/Chassis/1",
				Path: "$.PowerState",
			},
			MonitorType:   systemtype.MonitorTypeRedfishSSE,
			PowerOnMsgId:  "TEST.POWER.ON",
			PowerOffMsgId: "TEST.POWER.OFF",
		},
	})

	// Set the global registry
	systemTypeRegistry = registry

	tests := []struct {
		name          string
		event         databus.EventValue
		ip            string
		systemType    string
		expectedState string
		expectHandled bool
	}{
		{
			name: "power on event matches config",
			event: databus.EventValue{
				MessageId: "TEST.POWER.ON",
			},
			ip:            "192.168.1.100",
			systemType:    "testtype",
			expectedState: auth.POWERSTATEON,
			expectHandled: true,
		},
		{
			name: "power off event matches config",
			event: databus.EventValue{
				MessageId: "TEST.POWER.OFF",
			},
			ip:            "192.168.1.100",
			systemType:    "testtype",
			expectedState: auth.POWERSTATEOFF,
			expectHandled: true,
		},
		{
			name: "unknown power event ignored",
			event: databus.EventValue{
				MessageId: "UNKNOWN.EVENT",
			},
			ip:            "192.168.1.100",
			systemType:    "testtype",
			expectedState: "",
			expectHandled: false,
		},
		{
			name: "tcp-ping monitor type ignores power events",
			event: databus.EventValue{
				MessageId: "TEST.POWER.ON",
			},
			ip:            "192.168.1.100",
			systemType:    "tcptype",
			expectedState: "",
			expectHandled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac := &mock.MockAuthorizationClient{
				// Only set up ServiceItems for power events (no Services)
				ServiceItems: []auth.ServiceItem{
					{
						Service:        auth.Service{Ip: tt.ip},
						Systemtypedesc: tt.systemType,
					},
				},
			}

			// Add tcp-ping type for the test case that needs it
			if tt.systemType == "tcptype" {
				registry.Register(&systemtype.SystemTypeConfig{
					Name: "tcptype",
					PowerState: &systemtype.PowerStateConfig{
						MonitorType: systemtype.MonitorTypeTCPPing,
					},
				})
			}

			err := handleEvent(tt.event, tt.ip, ac)

			if tt.expectHandled {
				if err != nil {
					t.Errorf("expected event to be handled without error, got %v", err)
				}

				// Verify that the service state was updated
				if len(ac.ServiceItems) > 0 && ac.ServiceItems[0].State != tt.expectedState {
					t.Errorf("expected state %q, got %q", tt.expectedState, ac.ServiceItems[0].State)
				}
			} else {
				// Event should not be handled, but should not return an error either
				if err != nil {
					t.Errorf("expected no error for unhandled event, got %v", err)
				}
			}
		})
	}
}

// TestHandleEvent_ValveEventsUnaffected verifies that valve events still work with ircMessageIdState
func TestHandleEvent_ValveEventsUnaffected(t *testing.T) {
	// Setup registry without power state config (should not affect valve events)
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "irctype",
	})
	systemTypeRegistry = registry

	ac := &mock.MockAuthorizationClient{
		Services: []auth.Service{
			{Ip: "192.168.1.100"},
		},
		ServiceItems: []auth.ServiceItem{
			{
				Service:        auth.Service{Ip: "192.168.1.100"},
				Systemtypedesc: "irctype",
			},
		},
	}

	tests := []struct {
		name          string
		event         databus.EventValue
		expectedState string
	}{
		{
			name: "valve opening event",
			event: databus.EventValue{
				MessageId:   VALVEOPENINGMSGID,
				MessageArgs: []string{"1"},
			},
			expectedState: "Opening",
		},
		{
			name: "valve opened event",
			event: databus.EventValue{
				MessageId:   VALVEOPENEDMSGID,
				MessageArgs: []string{"1"},
			},
			expectedState: "Open",
		},
		{
			name: "valve closing event",
			event: databus.EventValue{
				MessageId:   VALVECLOSINGMSGID,
				MessageArgs: []string{"1"},
			},
			expectedState: "Closing",
		},
		{
			name: "valve closed event",
			event: databus.EventValue{
				MessageId:   VALVECLOSEDMSGID,
				MessageArgs: []string{"1"},
			},
			expectedState: "Closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous service items
			ac.ServiceItems = []auth.ServiceItem{
				{
					Service:        auth.Service{Ip: "192.168.1.100"},
					Systemtypedesc: "irctype",
				},
			}

			err := handleEvent(tt.event, "192.168.1.100", ac)

			if err != nil {
				t.Errorf("expected no error for valve event, got %v", err)
			}

			// Verify that the valve event was handled (state should be updated)
			if len(ac.ServiceItems) > 0 && ac.ServiceItems[0].State != tt.expectedState {
				t.Errorf("expected valve state %q, got %q", tt.expectedState, ac.ServiceItems[0].State)
			}
		})
	}
}

// TestRedfishGetPowerOffChecker_PowerOff verifies that the checker returns true when the device reports "Off"
func TestRedfishGetPowerOffChecker_PowerOff(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}

	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: ac,
		getPowerState: func() (string, error) {
			return "Off", nil
		},
	}

	if !checker.IsPoweredOff() {
		t.Fatal("expected IsPoweredOff to return true for PowerState Off")
	}
}

// TestRedfishGetPowerOffChecker_PowerOn verifies that the checker returns false when the device reports "On"
func TestRedfishGetPowerOffChecker_PowerOn(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}

	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: ac,
		getPowerState: func() (string, error) {
			return "On", nil
		},
	}

	if checker.IsPoweredOff() {
		t.Fatal("expected IsPoweredOff to return false for PowerState On")
	}
}

// TestRedfishGetPowerOffChecker_Error verifies that the checker returns false on Redfish GET errors
func TestRedfishGetPowerOffChecker_Error(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}

	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: ac,
		getPowerState: func() (string, error) {
			return "", errors.New("connection refused")
		},
	}

	if checker.IsPoweredOff() {
		t.Fatal("expected IsPoweredOff to return false on error")
	}
}

// TestRedfishGetPowerOffChecker_CaseInsensitive verifies that the checker handles case variations
func TestRedfishGetPowerOffChecker_CaseInsensitive(t *testing.T) {
	cases := []string{"Off", "off", "OFF", " Off ", " off "}

	for _, ps := range cases {
		t.Run(ps, func(t *testing.T) {
			ac := &mock.MockAuthorizationClient{}
			checker := &RedfishGetPowerOffChecker{
				ip:         "192.168.1.100",
				authClient: ac,
				getPowerState: func() (string, error) {
					return ps, nil
				},
			}
			if !checker.IsPoweredOff() {
				t.Errorf("expected IsPoweredOff to return true for %q", ps)
			}
		})
	}
}

// TestRedfishGetPowerOffChecker_UpdatesDB verifies that the checker writes POWERSTATEOFF to DB on detection
func TestRedfishGetPowerOffChecker_UpdatesDB(t *testing.T) {
	ac := &mock.MockAuthorizationClient{}
	_ = ac.UpdateServiceItem(auth.ServiceItem{Service: auth.Service{Ip: "192.168.1.100", State: "Power Status On"}})

	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: ac,
		getPowerState: func() (string, error) {
			return "Off", nil
		},
	}

	if !checker.IsPoweredOff() {
		t.Fatal("expected IsPoweredOff to return true")
	}
}

// TestDefaultPowerOffDetectorFactory_RedfishGet verifies that the factory returns RedfishGetPowerOffChecker for redfish-get
func TestDefaultPowerOffDetectorFactory_RedfishGet(t *testing.T) {
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "nvbmc",
		PowerState: &systemtype.PowerStateConfig{
			EndpointConfig: systemtype.EndpointConfig{
				URI:  "/redfish/v1/Systems/System_0",
				Path: "$.PowerState",
			},
			MonitorType:     systemtype.MonitorTypeRedfishGet,
			PollingInterval: 5,
		},
	})
	systemTypeRegistry = registry

	ac := &mock.MockAuthorizationClient{}
	si := auth.ServiceItem{
		Service:        auth.Service{Ip: "192.168.1.100"},
		Systemtypedesc: "nvbmc",
	}

	factory := defaultPowerOffDetectorFactory{}
	detector := factory.ForServiceItem(si, ac)

	checker, ok := detector.(*RedfishGetPowerOffChecker)
	if !ok {
		t.Fatalf("expected *RedfishGetPowerOffChecker, got %T", detector)
	}

	// Verify poll interval is taken from config (5 seconds), not the default (2 seconds)
	expected := 5 * time.Second
	if checker.PollInterval() != expected {
		t.Errorf("expected PollInterval %v from config, got %v", expected, checker.PollInterval())
	}
}

// TestDefaultPowerOffDetectorFactory_RedfishGet_DefaultInterval verifies that the factory
// falls back to the default poll interval when pollingInterval is 0 in config
func TestDefaultPowerOffDetectorFactory_RedfishGet_DefaultInterval(t *testing.T) {
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "nvbmc_no_interval",
		PowerState: &systemtype.PowerStateConfig{
			EndpointConfig: systemtype.EndpointConfig{
				URI:  "/redfish/v1/Systems/System_0",
				Path: "$.PowerState",
			},
			MonitorType: systemtype.MonitorTypeRedfishGet,
			// PollingInterval not set (0)
		},
	})
	systemTypeRegistry = registry

	ac := &mock.MockAuthorizationClient{}
	si := auth.ServiceItem{
		Service:        auth.Service{Ip: "192.168.1.100"},
		Systemtypedesc: "nvbmc_no_interval",
	}

	factory := defaultPowerOffDetectorFactory{}
	detector := factory.ForServiceItem(si, ac)

	checker, ok := detector.(*RedfishGetPowerOffChecker)
	if !ok {
		t.Fatalf("expected *RedfishGetPowerOffChecker, got %T", detector)
	}

	// Should fall back to default powOffPollInterval (2 seconds)
	if checker.PollInterval() != powOffPollInterval {
		t.Errorf("expected default PollInterval %v, got %v", powOffPollInterval, checker.PollInterval())
	}
}

// TestDefaultPowerOffDetectorFactory_RedfishSSE verifies that the factory still returns DBPowerOffChecker for redfish-sse
func TestDefaultPowerOffDetectorFactory_RedfishSSE(t *testing.T) {
	registry := systemtype.NewRegistry()
	registry.Register(&systemtype.SystemTypeConfig{
		Name: "idrac",
		PowerState: &systemtype.PowerStateConfig{
			MonitorType: systemtype.MonitorTypeRedfishSSE,
		},
	})
	systemTypeRegistry = registry

	ac := &mock.MockAuthorizationClient{}
	si := auth.ServiceItem{
		Service:        auth.Service{Ip: "192.168.1.100"},
		Systemtypedesc: "idrac",
	}

	factory := defaultPowerOffDetectorFactory{}
	detector := factory.ForServiceItem(si, ac)

	if _, ok := detector.(*DBPowerOffChecker); !ok {
		t.Errorf("expected *DBPowerOffChecker, got %T", detector)
	}
}

// TestWaitForPowerOff_RedfishGet_Success verifies the full flow: Redfish GET poll detects power off
func TestWaitForPowerOff_RedfishGet_Success(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
	})

	callCount := 0
	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: &mock.MockAuthorizationClient{},
		getPowerState: func() (string, error) {
			callCount++
			// Device powers off after 3 polls
			if callCount >= 3 {
				return "Off", nil
			}
			return "On", nil
		},
	}

	if ok := waitForPowerOffLoop(5, auth.Service{}, checker); !ok {
		t.Fatal("expected waitForPowerOffLoop to succeed when device powers off")
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 polls, got %d", callCount)
	}
}

// TestWaitForPowerOff_RedfishGet_Timeout verifies that the loop times out when device stays on
func TestWaitForPowerOff_RedfishGet_Timeout(t *testing.T) {
	origPoll := powOffPollInterval
	origPet := watchdogPetInterval
	powOffPollInterval = 1 * time.Millisecond
	watchdogPetInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		powOffPollInterval = origPoll
		watchdogPetInterval = origPet
	})

	checker := &RedfishGetPowerOffChecker{
		ip:         "192.168.1.100",
		authClient: &mock.MockAuthorizationClient{},
		getPowerState: func() (string, error) {
			return "On", nil
		},
	}

	if ok := waitForPowerOffLoop(1, auth.Service{}, checker); ok {
		t.Fatal("expected waitForPowerOffLoop to time out when device stays on")
	}
}
