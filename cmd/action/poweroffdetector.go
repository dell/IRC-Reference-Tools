package main

import (
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/IRC-Reference-Tools/internal/tcpconnect"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

var (
	tcpConnectTimeout = 250 * time.Millisecond

	// For TCP-based power-off detection, require N consecutive unreachable samples.
	// This avoids false positives from transient network blips.
	tcpUnreachableConsecutive = 3

	isHostAccessible = tcpconnect.IsHostAccessible
)

type PowerOffDetector interface {
	IsPoweredOff() bool
}

type PowerOffDetectorFactory interface {
	ForServiceItem(si auth.ServiceItem, authClient auth.AuthClientInterface) PowerOffDetector
}

type defaultPowerOffDetectorFactory struct{}

func (defaultPowerOffDetectorFactory) ForServiceItem(si auth.ServiceItem, authClient auth.AuthClientInterface) PowerOffDetector {
	// Get system type config
	config := systemTypeRegistry.Get(si.Systemtypedesc)
	if config == nil || config.PowerState == nil {
		return &TCPUnreachablePowerOffChecker{ip: si.Ip}
	}

	switch config.PowerState.MonitorType {
	case systemtype.MonitorTypeRedfishSSE, systemtype.MonitorTypeRedfishGet:
		return &DBPowerOffChecker{ip: si.Ip, authClient: authClient}
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
