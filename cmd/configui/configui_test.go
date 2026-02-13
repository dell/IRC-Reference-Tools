package main

import (
	"encoding/json"
	"testing"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

func TestMarshalService(t *testing.T) {
	service := &auth.Service{
		ServiceType: auth.SYSTEM_TYPE_IDRAC,
		Ip:          "89.0.142.86",
		AuthType:    auth.AuthTypeUsernamePassword,
		Auth: map[string]string{
			"username": "admin",
			"password": "password",
		},
	}
	jsonStr, err := json.Marshal(service.Auth)
	if err != nil {
		t.Errorf("Failed to marshal service auth: %v", err)
	}

	t.Log(string(jsonStr))
}
