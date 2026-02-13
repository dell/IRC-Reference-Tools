package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	ircredfish "github.com/dell/IRC-Reference-Tools/internal/redfish"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/IRC-Reference-Tools/internal/tcpconnect"
	"github.com/dell/IRC-Reference-Tools/internal/valve"
	"github.com/dell/IRC-Reference-Tools/internal/watchdog"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/wire"
)

var DataGroupsMap *databus.DataGroups

// systemTypeRegistry is the local cache of system type configs
var systemTypeRegistry *systemtype.Registry

var (
	setWatchdogTimeout = watchdog.SetWatchdogTimeout
	getValveStatus     = valve.GetValveStatus
)

// getSystemTypeForService determines the system type config name for a service
func getSystemTypeForService(service *auth.Service, authClient auth.AuthClientInterface) string {
	if service.ServiceType == auth.SYSTEM_TYPE_IRC {
		return "irc"
	}
	// For ServiceItems, get the Systemtypedesc from the DB
	si := authClient.GetServiceItemWithIP(service.Ip)
	if si.Systemtypedesc != "" {
		return si.Systemtypedesc
	}
	// Default to empty string if not specified
	return ""
}

// handleAuthServiceChannel Authenticates to the device and then launches the monitoring process
func handleAuthServiceChannel(ctx context.Context, serviceIn <-chan *auth.Service, dataBusService *databus.DataBusService, devices ircredfish.ConfigurableDevices, authClient auth.AuthClientInterface) {
	for {
		select {
		case <-ctx.Done():
			return
		case service := <-serviceIn:
			if service == nil {
				continue
			}

			// Determine system type and get config
			sysTypeName := getSystemTypeForService(service, authClient)
			if sysTypeName == "" {
				logging.Warn("No system type found for device, skipping", "ip", service.Ip)
				authClient.UpdateServiceXState(auth.CONNFAILED, service.Ip)
				continue
			}
			config := systemTypeRegistry.Get(sysTypeName)
			if config == nil {
				logging.Warn("No config found for system type, using default", "systemType", sysTypeName)
				authClient.UpdateServiceXState(auth.CONNFAILED, service.Ip)
				continue
			}

			device, err := ircredfish.ValidateAndAddConfigurableDevice(service, devices, config)

			if err == nil {
				// Case when service has redfish
				if config.IsParent {
					// IRC-specific initialization
					authClient.UpdateServiceState(auth.STARTING, service.Ip)
					initIRC(service, authClient)
				} else if config.PowerState != nil {
					// Get PowerState using configured endpoint
					powerState, err := device.Client.GetPowerState()
					if err != nil {
						logging.Error("Error getting power state", "err", err)
						authClient.UpdateServiceItemState(auth.CONNFAILED, service.Ip)
						continue
					}
					logging.Info("PowerState", "powerState", powerState)
					err = authClient.UpdateServiceItemState("Power Status "+powerState, service.Ip)
					if err != nil {
						logging.Error("Error updating service item state", "err", err)
						authClient.UpdateServiceItemState(auth.CONNFAILED, service.Ip)
					}
				}
				// Start the alert monitoring process
				go device.MonitorStart(dataBusService, authClient)
			} else if strings.Contains(err.Error(), "401") {
				logging.Error("Failed to add device, authentication failed", "ip", service.Ip)
				authClient.UpdateServiceState(auth.CONNFAILED, service.Ip)
				continue
			} else {
				// Case when service doesn't have redfish
				if config.IsParent {
					// IRC without redfish - should not happen
					logging.Error("Failed to add IRC", "ip", service.Ip)
					authClient.UpdateServiceState(auth.CONNFAILED, service.Ip)
				} else {
					// ServiceItem without redfish - check if reachable
					ok := tcpconnect.IsHostAccessible(service.Ip, nil, 2*time.Second)
					if !ok {
						logging.Error("Failed to add rack device", "ip", service.Ip)
						authClient.UpdateServiceItemState(auth.CONNFAILED, service.Ip)
					} else {
						logging.Info("Redfish not available but device reachable", "ip", service.Ip)
						authClient.UpdateServiceItemState(auth.RUNNING, service.Ip)
					}
				}
				continue
			}
		}
	}
}

func initIRC(service *auth.Service, authClient auth.AuthClientInterface) {
	err := setWatchdogTimeout(*service, 60)
	if err != nil {
		logging.Error("Error setting watchdog timeout", "err", err)
		authClient.UpdateServiceState(auth.CONNFAILED, service.Ip)
		return
	}
	err = UpdateInitialValveStates(*service, authClient)
	if err != nil {
		logging.Error("Error updating valve states", "err", err)
		authClient.UpdateServiceState(auth.CONNFAILED, service.Ip)
		return
	}
}

func UpdateInitialValveStates(service auth.Service, authClient auth.AuthClientInterface) error {
	vstatus1, vstatus2, err := getValveStatus(service)
	if err != nil {
		logging.Error("Error getting valve status", "err", err)
		return err
	}
	logging.Info("Valve status", "vstate1", vstatus1, "vstate2", vstatus2)
	logging.Info("Updating valve status for device", "ip", service.Ip)
	err = authClient.UpdateValveState(service.Ip, vstatus1, vstatus2)
	if err != nil {
		logging.Error("Error updating valve status", "err", err)
		return err
	}
	return nil
}

func main() {
	// Load system type configurations
	var err error
	systemTypeRegistry, err = systemtype.LoadConfigs()
	if err != nil {
		logging.Warn("Warning: failed to load system type configs", "err", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mb := messagebus.InitializeStompMB()
	defer mb.Close()

	dataBusService := databus.NewDataBusServiceWithBus(mb)
	authClient := auth.NewAuthClientWithBus(mb, "redfishread")

	serviceIn := auth.NewServiceChan(10)
	envelopes := databus.NewCommandChan()
	devices := ircredfish.NewConfigurableDevices()
	authClient.ResendAll()

	// GetService starts a goroutine that subscribes to service updates and
	// sends them to the serviceIn channel. The context allows us to cancel
	// the underlying subscription on shutdown.
	authClient.GetService(ctx, serviceIn)
	go handleAuthServiceChannel(ctx, serviceIn, dataBusService, devices, authClient)
	go dataBusService.ReceiveEnvelopes(envelopes)
	for {
		select {
		case env, ok := <-envelopes:
			if !ok {
				logging.Error("Envelopes channel closed, shutting down")
				cancel()
				return
			}
			logging.Info("Received command in redfishread", "command", env.Type)
			switch env.Type {
			case databus.GET:
				// TODO: Implement data groups for configurable devices
				logging.Warn("GET command not yet implemented for configurable devices")
			case databus.GETPRODUCERS:
				producers := databus.NewDataProducers(len(devices))
				i := 0
				for _, dev := range devices {
					producer := databus.NewDataProducer()
					producer.Hostname = dev.Client.GetHostname()
					producer.Username = dev.Client.GetUsername()
					producer.State = dev.State
					producer.LastEvent = dev.LastEvent
					producers[i] = producer
					i = i + 1
				}
				err := dataBusService.Reply(env, producers)
				if err != nil {
					logging.Error("SendProducersToQueue error, continuing")
				}
			case databus.DELETEPRODUCER:
				var service auth.Service
				if err := wire.DecodePayload(env.Payload, &service); err != nil {
					logging.Error("Failed decoding DELETEPRODUCER payload: %v", err)
					continue
				}
				logging.Info("Received DELETEPRODUCER command for service IP:", "ip", service.Ip)
				if _, exists := devices[service.Ip]; !exists {
					logging.Info("No device found for service IP:", "ip", service.Ip)
					continue
				}
				devices[service.Ip].CtxCancel()
				fmt.Println("context has been cancelled for service IP:", service.Ip)
				delete(devices, service.Ip)
			case auth.TERMINATE:
				logging.Info("Received terminate command, shutting down")
				cancel()
				return
			}
		case sig := <-sigChan:
			logging.Info("Received signal, shutting down gracefully", "signal", sig)
			cancel()
			return
		case <-ctx.Done():
			logging.Info("Context cancelled, exiting redfishread")
			return
		}
	}
}
