package main

import (
	"errors"
	"sync"
	"testing"

	"github.com/dell/IRC-Reference-Tools/internal/mock"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

type recordingAuthClient struct {
	mock.MockAuthorizationClient

	mu                 sync.Mutex
	updateValveCalls   []updateValveCall
	updateServiceCalls []updateServiceCall

	updateValveErr   error
	updateServiceErr error
}

type updateValveCall struct {
	ip string
	v1 string
	v2 string
}

type updateServiceCall struct {
	state string
	ip    string
}

func (r *recordingAuthClient) UpdateValveState(sip, v1, v2 string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateValveCalls = append(r.updateValveCalls, updateValveCall{ip: sip, v1: v1, v2: v2})
	return r.updateValveErr
}

func (r *recordingAuthClient) UpdateServiceState(state string, sip string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateServiceCalls = append(r.updateServiceCalls, updateServiceCall{state: state, ip: sip})
	return r.updateServiceErr
}

func TestUpdateInitialValveStates_Success(t *testing.T) {
	origGet := getValveStatus
	getValveStatus = func(service auth.Service) (string, string, error) {
		return "Open", "Closed", nil
	}
	t.Cleanup(func() {
		getValveStatus = origGet
	})

	ac := &recordingAuthClient{}
	service := auth.Service{Ip: "ip1"}
	if err := UpdateInitialValveStates(service, ac); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if len(ac.updateValveCalls) != 1 {
		t.Fatalf("expected 1 UpdateValveState call, got %d", len(ac.updateValveCalls))
	}
	if ac.updateValveCalls[0].ip != "ip1" || ac.updateValveCalls[0].v1 != "Open" || ac.updateValveCalls[0].v2 != "Closed" {
		t.Fatalf("unexpected UpdateValveState call: %+v", ac.updateValveCalls[0])
	}
}

func TestUpdateInitialValveStates_GetValveStatusError(t *testing.T) {
	origGet := getValveStatus
	getValveStatus = func(service auth.Service) (string, string, error) {
		return "", "", errors.New("boom")
	}
	t.Cleanup(func() {
		getValveStatus = origGet
	})

	ac := &recordingAuthClient{}
	service := auth.Service{Ip: "ip1"}
	if err := UpdateInitialValveStates(service, ac); err == nil {
		t.Fatalf("expected error")
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if len(ac.updateValveCalls) != 0 {
		t.Fatalf("expected 0 UpdateValveState calls, got %d", len(ac.updateValveCalls))
	}
}

func TestUpdateInitialValveStates_UpdateValveStateError(t *testing.T) {
	origGet := getValveStatus
	getValveStatus = func(service auth.Service) (string, string, error) {
		return "Open", "Closed", nil
	}
	t.Cleanup(func() {
		getValveStatus = origGet
	})

	ac := &recordingAuthClient{updateValveErr: errors.New("db down")}
	service := auth.Service{Ip: "ip1"}
	if err := UpdateInitialValveStates(service, ac); err == nil {
		t.Fatalf("expected error")
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if len(ac.updateValveCalls) != 1 {
		t.Fatalf("expected 1 UpdateValveState call, got %d", len(ac.updateValveCalls))
	}
}

func TestInitIRC_WatchdogFailureUpdatesServiceState(t *testing.T) {
	origSet := setWatchdogTimeout
	origGet := getValveStatus
	setWatchdogTimeout = func(service auth.Service, timeout int) error {
		return errors.New("boom")
	}
	getValveStatus = func(service auth.Service) (string, string, error) {
		return "Open", "Closed", nil
	}
	t.Cleanup(func() {
		setWatchdogTimeout = origSet
		getValveStatus = origGet
	})

	ac := &recordingAuthClient{}
	svc := &auth.Service{Ip: "ip1"}
	initIRC(svc, ac)

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if len(ac.updateServiceCalls) != 1 {
		t.Fatalf("expected 1 UpdateServiceState call, got %d", len(ac.updateServiceCalls))
	}
	if ac.updateServiceCalls[0].state != auth.CONNFAILED || ac.updateServiceCalls[0].ip != "ip1" {
		t.Fatalf("unexpected UpdateServiceState call: %+v", ac.updateServiceCalls[0])
	}
	if len(ac.updateValveCalls) != 0 {
		t.Fatalf("expected no UpdateValveState calls on watchdog failure")
	}
}

func TestInitIRC_ValveInitFailureUpdatesServiceState(t *testing.T) {
	origSet := setWatchdogTimeout
	origGet := getValveStatus
	setWatchdogTimeout = func(service auth.Service, timeout int) error {
		return nil
	}
	getValveStatus = func(service auth.Service) (string, string, error) {
		return "", "", errors.New("boom")
	}
	t.Cleanup(func() {
		setWatchdogTimeout = origSet
		getValveStatus = origGet
	})

	ac := &recordingAuthClient{}
	svc := &auth.Service{Ip: "ip1"}
	initIRC(svc, ac)

	ac.mu.Lock()
	defer ac.mu.Unlock()
	if len(ac.updateServiceCalls) != 1 {
		t.Fatalf("expected 1 UpdateServiceState call, got %d", len(ac.updateServiceCalls))
	}
	if ac.updateServiceCalls[0].state != auth.CONNFAILED || ac.updateServiceCalls[0].ip != "ip1" {
		t.Fatalf("unexpected UpdateServiceState call: %+v", ac.updateServiceCalls[0])
	}
}
