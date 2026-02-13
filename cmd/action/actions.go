package main

import (
	"fmt"

	"github.com/dell/IRC-Reference-Tools/internal/httpclient"
	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

// Shutdowner interface for executing shutdown actions
type Shutdowner interface {
	Shutdown(auth.ServiceItem) (string, error)
	Name() string
	Type() string
}

// httpShutdowner executes shutdown via HTTP API calls using ShutdownConfig
type httpShutdowner struct {
	name   string
	config *systemtype.ShutdownConfig
}

func newHTTPShutdowner(cfg *systemtype.SystemTypeConfig) *httpShutdowner {
	return &httpShutdowner{
		name:   cfg.Name,
		config: cfg.Shutdown,
	}
}

func (h *httpShutdowner) Name() string {
	return h.name
}

func (h *httpShutdowner) Type() string {
	return h.name
}

func (h *httpShutdowner) Shutdown(si auth.ServiceItem) (string, error) {
	if h.config == nil {
		return "", fmt.Errorf("no shutdown config for %s", h.name)
	}

	// Select action based on forceful flag
	var action *systemtype.APIAction
	if si.IsForcefulshutdown {
		action = h.config.Force
		if action == nil {
			// Fall back to graceful if force not configured
			action = h.config.Graceful
			logging.Warn("Force shutdown not configured, using graceful", "name", h.name)
		}
	} else {
		action = h.config.Graceful
		if action == nil {
			return "", fmt.Errorf("graceful shutdown not configured for %s", h.name)
		}
	}

	return h.executeAction(action, si)
}

func (h *httpShutdowner) executeAction(action *systemtype.APIAction, si auth.ServiceItem) (string, error) {
	url := action.Protocol + "://" + si.Service.Ip + action.URI

	// Execute the HTTP request using httpclient package
	var result string
	var err error

	username := si.Service.Auth["username"]
	password := si.Service.Auth["password"]

	switch action.Method {
	case "POST":
		result, err = httpclient.Post(url, action.Body, nil, username, password)
	case "PATCH":
		result, err = httpclient.Patch(url, action.Body, nil, username, password)
	default:
		return "", fmt.Errorf("unsupported HTTP method: %s", action.Method)
	}

	if err != nil {
		return "", fmt.Errorf("failed to execute shutdown request: %w", err)
	}

	logging.Info("Shutdown request completed successfully", "url", url)
	return result, nil
}

// shutdownerFromConfig creates a Shutdowner from a SystemTypeConfig
func shutdownerFromConfig(cfg *systemtype.SystemTypeConfig) Shutdowner {
	if cfg.Shutdown == nil {
		return nil
	}
	if cfg.Shutdown.Graceful == nil && cfg.Shutdown.Force == nil {
		return nil
	}
	return newHTTPShutdowner(cfg)
}
