package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/IRC-Reference-Tools/internal/watchdog"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
)

var (
	watchdogPetInterval  = 5 * time.Second
	forceShutdownTimeout = 10 * time.Second
)

const (
	// IRC Events
	LEAKMSGID = "Dell.0.1.LCPO009"

	VALVEOPENINGMSGID = "Dell.0.1.LCR0006"
	VALVEOPENEDMSGID  = "Dell.0.1.LCR0001"
	VALVECLOSINGMSGID = "Dell.0.1.LCR0007"
	VALVECLOSEDMSGID  = "Dell.0.1.LCR0002"
)

var ircMessageIdState = map[string]string{
	VALVEOPENINGMSGID: "Opening",
	VALVEOPENEDMSGID:  "Open",
	VALVECLOSINGMSGID: "Closing",
	VALVECLOSEDMSGID:  "Closed",
}

func waitForPowerOffLoop(timeoutSeconds int, service auth.Service, detector PowerOffDetector) bool {
	logging.Info("[DEBUG] waitForPowerOffLoop started", "timeoutSeconds", timeoutSeconds, "detectorType", fmt.Sprintf("%T", detector), "pollInterval", detector.PollInterval())
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	poll := time.NewTicker(detector.PollInterval())
	pet := time.NewTicker(watchdogPetInterval)
	defer poll.Stop()
	defer pet.Stop()

	for {
		select {
		case <-poll.C:
			if detector.IsPoweredOff() {
				logging.Info("[DEBUG] waitForPowerOffLoop: power off detected, returning true")
				return true
			}
			if time.Now().After(deadline) {
				logging.Info("[DEBUG] waitForPowerOffLoop: deadline reached, returning false")
				return false
			}
		case <-pet.C:
			if service.Ip != "" {
				if err := watchdog.PetWatchdog(service); err != nil {
					logging.Error("error petting watchdog", "err", err)
				}
			}
		}
	}
}

func handleEvent(event databus.EventValue, ip string, authClient auth.AuthClientInterface) error {
	service := authClient.GetServiceWithIP(ip)
	if service.Ip != "" {
		return handleIRCEvent(event, service, authClient)
	}

	serviceItem := authClient.GetServiceItemWithIP(ip)
	if serviceItem.Ip != "" {
		return handleRackDeviceEvent(event, serviceItem, authClient)
	}
	return nil
}

func handleIRCEvent(event databus.EventValue, s auth.Service, authClient auth.AuthClientInterface) error {
	switch event.MessageId {
	case LEAKMSGID:
		logging.Debug("Received leak event", "ip", s.Ip)
		return handleLeakEvent(event, s, authClient)
	case VALVEOPENINGMSGID, VALVEOPENEDMSGID, VALVECLOSINGMSGID, VALVECLOSEDMSGID:
		logging.Debug("Received valve event", "ip", s.Ip)
		return handleValveUpdateEvent(event, s.Ip, authClient)
	}
	return nil
}

func handleRackDeviceEvent(event databus.EventValue, si auth.ServiceItem, authClient auth.AuthClientInterface) error {
	// Get system type config
	config := systemTypeRegistry.Get(si.Systemtypedesc)

	// Check if this event should be handled based on monitoring config
	if config.PowerState.MonitorType == systemtype.MonitorTypeRedfishSSE {
		if event.MessageId == config.PowerState.PowerOnMsgId {
			// If PowerOnMessageArgs exist, validate with checkPowerEvent, otherwise handle directly
			if config.PowerState.PowerOnMessageArgs == "" || checkPowerEvent(event.MessageArgs, config.PowerState.PowerOnMessageArgs) {
				return handlePowerEvent(event, si.Ip, authClient, auth.POWERSTATEON)
			}
		}
		if event.MessageId == config.PowerState.PowerOffMsgId {
			// If PowerOffMessageArgs exist, validate with checkPowerEvent, otherwise handle directly
			if config.PowerState.PowerOffMessageArgs == "" || checkPowerEvent(event.MessageArgs, config.PowerState.PowerOffMessageArgs) {
				return handlePowerEvent(event, si.Ip, authClient, auth.POWERSTATEOFF)
			}
		}
	}
	return nil
}

func checkPowerEvent(eventMessageArgs []string, powerMessageArgs string) bool {
	// Check messageArgs index-value match directly
	index, value := parseIndexValue(powerMessageArgs)
	if index >= 0 && index < len(eventMessageArgs) && eventMessageArgs[index] == value {
		return true
	}
	return false
}

// parseIndexValue parses a string in the format "index-value" (e.g., "2-Enabled")
// and returns the index as an integer and the value as a string.
// Used to extract the position and expected value from PowerOnMessageArgs/PowerOffMessageArgs
// configuration fields for matching against event MessageArgs.

func parseIndexValue(indexValue string) (int, string) {
	parts := strings.Split(indexValue, "-")
	idx, _ := strconv.Atoi(parts[0])
	return idx, parts[1]
}

func handleLeakEvent(event databus.EventValue, service auth.Service, authClient auth.AuthClientInterface) error {
	service.State = auth.LEAKED
	authClient.UpdateService(service)

	// Get all ServiceItems
	items := authClient.GetServiceItems(service.Ip)

	// Shutdown all ServiceItems
	ok := Shutdown(items, authClient, service)
	if !ok {
		logging.Error("Shutdown failed", "ip", service.Ip, "reason", "VALVE STILL OPEN")
		return fmt.Errorf("shutdown failed for %s : VALVE STILL OPEN", service.Ip)
	}

	// end watchdog
	err := watchdog.EndWatchdog(service)
	if err != nil {
		logging.Error("Error ending watchdog", "err", err, "reason", "VALVE STILL OPEN")
		return err
	}

	// All shutdown steps after leak detection are complete.
	// Reset IRC status.
	logging.Info("Shutdowns complete after detecting leak, resetting state", "ip", service.Ip)
	authClient.UpdateServiceState(auth.MONITORING, service.Ip)
	return nil
}

func Shutdown(items []auth.ServiceItem, authClient auth.AuthClientInterface, service auth.Service) bool {
	var wg sync.WaitGroup
	var totalCount atomic.Int64
	var successCount atomic.Int64
	for _, si := range items {
		var shutdowner Shutdowner
		if defaultShutdowner, ok := shutdowners[si.Systemtypedesc]; ok {
			shutdowner = defaultShutdowner
		} else {
			shutdowner = nil
			logging.Error("No shutdowner found, shutdown failed", "ip", si.Ip)
			si.State = auth.SHUTDOWNFAILED
			authClient.UpdateServiceItem(si)
			continue
		}
		wg.Add(1)
		go func(si auth.ServiceItem, shutdowner Shutdowner) {
			defer wg.Done()
			totalCount.Add(1)
			ok := shutdownWithFallback(si, shutdowner, authClient, service)
			if ok {
				successCount.Add(1)
			}
		}(si, shutdowner)
	}
	wg.Wait()
	total := totalCount.Load()
	successes := successCount.Load()
	overallSuccess := successes == total
	logging.Info("Shutdown Success", "overallSuccess", overallSuccess, "success", successes, "total", total)
	return overallSuccess
}

func shutdownWithFallback(si auth.ServiceItem, shutdowner Shutdowner, authClient auth.AuthClientInterface, service auth.Service) bool {
	logging.Debug("Shutdown with fallback", "serviceItem", si)
	// If shutdown is explicitly forceful, skip graceful.
	stConfig := systemTypeRegistry.Get(si.Systemtypedesc)
	if si.IsForcefulshutdown && stConfig.PowerState.MonitorType == systemtype.MonitorTypeRedfishSSE {
		ok := runShutdownOnce(si, shutdowner, authClient, service)
		if !ok {
			return false
		}
		// Wait for forceful shutdown timeout
		time.Sleep(forceShutdownTimeout)
		return true
	} else if si.IsForcefulshutdown && stConfig.PowerState.MonitorType == systemtype.MonitorTypeRedfishGet {
		ok := runShutdownOnce(si, shutdowner, authClient, service)
		if !ok {
			return false
		}
		// Poll via Redfish GET to confirm power off
		detector := defaultPowerOffDetectorFactory{}.ForServiceItem(si, authClient)
		timeout := si.Timeout
		if timeout <= 0 {
			timeout = int(forceShutdownTimeout.Seconds())
		}
		waitForPowerOffLoop(timeout, service, detector)
		return true
	}

	// Phase 1: graceful
	si.IsForcefulshutdown = false
	if ok := runShutdownOnce(si, shutdowner, authClient, service); !ok {
		return false
	}

	// No timeout configured: don't attempt a forceful fallback.
	if si.Timeout <= 0 {
		return true
	}
	logging.Info("Waiting for power off", "ip", si.Ip)

	factory := defaultPowerOffDetectorFactory{}
	detector := factory.ForServiceItem(si, authClient)
	if waitForPowerOffLoop(si.Timeout, service, detector) {
		return true
	}

	// Phase 2: forceful
	logging.Warn("Graceful shutdown timed out, falling back to forceful shutdown", "ip", si.Ip)
	si.IsForcefulshutdown = true
	return runShutdownOnce(si, shutdowner, authClient, service)
}

func runShutdownOnce(si auth.ServiceItem, shutdowner Shutdowner, authClient auth.AuthClientInterface, service auth.Service) bool {
	logging.Info("Calling shutdowner", "name", shutdowner.Name(), "ip", si.Ip, "force", si.IsForcefulshutdown)
	out, err := shutdowner.Shutdown(si)
	logging.Debug("Output", "output", out)
	if err != nil {
		logging.Error("shutdown failed", "err", err)
		si.State = auth.SHUTDOWNFAILED
		authClient.UpdateServiceItem(si)
		return false
	}
	si.State = auth.SHUTDOWNRECEIVED
	logging.Info("Shutdown sent, updating state to DB", "ip", si.Ip)
	authClient.UpdateServiceItem(si)

	if service.Ip != "" {
		err = watchdog.PetWatchdog(service)
		if err != nil {
			logging.Error("error petting watchdog", "err", err)
			return false
		}
	}
	return true
}

func handlePowerEvent(event databus.EventValue, ip string, authClient auth.AuthClientInterface, state string) error {
	logging.Warn("Received power update event", "ip", ip, "messageId", event.MessageId, "state", state)
	err := authClient.UpdateServiceXState(state, ip)
	if err != nil {
		logging.Error("Error updating service state", "err", err)
		return err
	}
	return nil
}

func handleValveUpdateEvent(event databus.EventValue, ip string, authClient auth.AuthClientInterface) error {
	logging.Debug("Received valve update event", "ip", ip, "event", event)
	state, ok := ircMessageIdState[event.MessageId]
	if !ok {
		logging.Warn("Valve MessageID has no state", "messageId", event.MessageId)
		return fmt.Errorf("Valve MessageID has no state: %s", event.MessageId)
	}
	var valve string
	if len(event.MessageArgs) > 0 {
		valve = event.MessageArgs[0]
	} else {
		logging.Warn("No valve identifier in MessageArgs for event", "messageId", event.MessageId)
		return fmt.Errorf("no valve identifier in MessageArgs")
	}
	if valve == "1" {
		err := authClient.UpdateValveState(ip, state, "")
		if err != nil {
			logging.Error("Error updating valve state", "err", err)
			return err
		}
		logging.Info("Updated valve1 state", "ip", ip, "state", state)
	} else if valve == "2" {
		err := authClient.UpdateValveState(ip, "", state)
		if err != nil {
			logging.Error("Error updating valve state", "err", err)
			return err
		}
		logging.Info("Updated valve2 state", "ip", ip, "state", state)
	}
	return nil
}
