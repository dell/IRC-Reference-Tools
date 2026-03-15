package main

import (
	"strings"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	ircredfish "github.com/dell/IRC-Reference-Tools/internal/redfish"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/IRC-Reference-Tools/internal/tcpconnect"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

var (
	powOffPollInterval = 2 * time.Second
	tcpConnectTimeout  = 250 * time.Millisecond

	// For TCP-based power-off detection, require N consecutive unreachable samples.
	// This avoids false positives from transient network blips.
	tcpUnreachableConsecutive = 3

	isHostAccessible = tcpconnect.IsHostAccessible

	// newRedfishGetClient creates a lightweight Redfish client for power state polling.
	// Skips the connection validation that NewConfigurableClient performs, since the
	// device may be in the process of shutting down.
	newRedfishGetClient = ircredfish.NewClientWithoutValidation
)

type PowerOffDetector interface {
	IsPoweredOff() bool
	PollInterval() time.Duration
}

type PowerOffDetectorFactory interface {
	ForServiceItem(si auth.ServiceItem, authClient auth.AuthClientInterface) PowerOffDetector
}

type defaultPowerOffDetectorFactory struct{}

func (defaultPowerOffDetectorFactory) ForServiceItem(si auth.ServiceItem, authClient auth.AuthClientInterface) PowerOffDetector {
	// Get system type config
	config := systemTypeRegistry.Get(si.Systemtypedesc)
	if config == nil || config.PowerState == nil {
		logging.Info("No config or PowerState for device, falling back to TCP", "ip", si.Ip, "systemtype", si.Systemtypedesc)
		return &TCPUnreachablePowerOffChecker{ip: si.Ip}
	}

	logging.Info("PowerOffDetectorFactory selecting detector", "ip", si.Ip, "systemtype", si.Systemtypedesc, "monitorType", config.PowerState.MonitorType)

	switch config.PowerState.MonitorType {
	case systemtype.MonitorTypeRedfishSSE:
		logging.Info("Using DBPowerOffChecker (redfish-sse)", "ip", si.Ip)
		return &DBPowerOffChecker{ip: si.Ip, authClient: authClient}
	case systemtype.MonitorTypeRedfishGet:
		client := newRedfishGetClient(si.Ip, si.Auth["username"], si.Auth["password"], config)
		interval := powOffPollInterval
		if config.PowerState.PollingInterval > 0 {
			interval = time.Duration(config.PowerState.PollingInterval) * time.Second
		}
		logging.Info("Using RedfishGetPowerOffChecker (redfish-get)", "ip", si.Ip, "pollInterval", interval)
		return &RedfishGetPowerOffChecker{
			ip:            si.Ip,
			authClient:    authClient,
			getPowerState: client.GetPowerState,
			pollInterval:  interval,
		}
	case systemtype.MonitorTypeTCPPing, "":
		return &TCPUnreachablePowerOffChecker{ip: si.Ip}
	default:
		logging.Warn("Unknown monitorType, defaulting to TCP", "monitorType", config.PowerState.MonitorType)
		return &TCPUnreachablePowerOffChecker{ip: si.Ip}
	}
}

type DBPowerOffChecker struct {
	ip         string
	authClient auth.AuthClientInterface
}

func (c *DBPowerOffChecker) IsPoweredOff() bool {
	svc := c.authClient.GetServiceItemWithIP(c.ip)
	if svc.State == auth.POWERSTATEOFF {
		logging.Info("Power off confirmed via DB", "ip", c.ip)
		return true
	}
	return false
}

func (c *DBPowerOffChecker) PollInterval() time.Duration {
	return powOffPollInterval
}

// RedfishGetPowerOffChecker polls the device directly via Redfish GET to detect power off.
// Used for devices with monitorType "redfish-get" that don't support SSE.
type RedfishGetPowerOffChecker struct {
	ip            string
	authClient    auth.AuthClientInterface
	getPowerState func() (string, error)
	pollInterval  time.Duration
}

func (c *RedfishGetPowerOffChecker) IsPoweredOff() bool {
	powerState, err := c.getPowerState()
	if err != nil {
		logging.Error("Failed to poll power state via Redfish GET", "ip", c.ip, "err", err)
		return false
	}
	logging.Info("Polled power state via Redfish GET", "ip", c.ip, "powerState", powerState)
	if strings.EqualFold(strings.TrimSpace(powerState), "off") {
		logging.Info("Power off confirmed via Redfish GET, updating DB", "ip", c.ip)
		c.authClient.UpdateServiceItemState(auth.POWERSTATEOFF, c.ip)
		return true
	}
	logging.Debug("Power still on via Redfish GET, will keep polling", "ip", c.ip, "powerState", powerState)
	return false
}

func (c *RedfishGetPowerOffChecker) PollInterval() time.Duration {
	if c.pollInterval > 0 {
		return c.pollInterval
	}
	return powOffPollInterval
}

type TCPUnreachablePowerOffChecker struct {
	ip                string
	unreachableStreak int
}

func (c *TCPUnreachablePowerOffChecker) IsPoweredOff() bool {
	accessible := isHostAccessible(c.ip, nil, tcpConnectTimeout)
	if !accessible {
		c.unreachableStreak++
		if c.unreachableStreak >= tcpUnreachableConsecutive {
			logging.Info("Power off inferred via TCP unreachable", "ip", c.ip)
			return true
		}
		return false
	}
	c.unreachableStreak = 0
	return false
}

func (c *TCPUnreachablePowerOffChecker) PollInterval() time.Duration {
	return powOffPollInterval
}
