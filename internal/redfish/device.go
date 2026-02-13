package redfish

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
)

// ConfigurableDevice represents a Redfish device with configurable endpoints
type ConfigurableDevice struct {
	Client    *ConfigurableRedfishClient
	Config    *systemtype.SystemTypeConfig
	SystemID  string
	HostName  string
	FwVer     string
	State     string
	LastEvent time.Time
	Events    chan *RedfishEvent
	Ctx       context.Context
	CtxCancel context.CancelFunc
}

// ConfigurableDevices is a map of IP to ConfigurableDevice
type ConfigurableDevices map[string]*ConfigurableDevice

// NewConfigurableDevices creates a new empty devices map
func NewConfigurableDevices() ConfigurableDevices {
	return make(ConfigurableDevices)
}

// ValidateAndAddConfigurableDevice creates and validates a new configurable device
func ValidateAndAddConfigurableDevice(service *auth.Service, devices ConfigurableDevices, config *systemtype.SystemTypeConfig) (*ConfigurableDevice, error) {
	if service.Ip == "" {
		return nil, fmt.Errorf("ServiceIP is empty")
	}

	if devices[service.Ip] != nil {
		return nil, fmt.Errorf("Device with IP %s already exists", service.Ip)
	}

	log.Printf("Creating configurable device for %s with config %s", service.Ip, config.Name)

	client, err := NewConfigurableClient(
		service.Ip,
		service.Auth["username"],
		service.Auth["password"],
		config,
	)

	device := &ConfigurableDevice{
		Client: client,
		Config: config,
	}

	if err != nil {
		log.Printf("%s: Failed to instantiate redfish client %v", service.Ip, err)
		device.State = auth.CONNFAILED
	} else {
		device.State = auth.STARTING
	}

	ctx, cancel := context.WithCancel(context.Background())
	device.Ctx = ctx
	device.CtxCancel = cancel

	devices[service.Ip] = device
	return device, err
}

// StartAlertListener starts listening for SSE alerts
func (d *ConfigurableDevice) StartAlertListener(dataBusService *databus.DataBusService) {
	if d.Events == nil {
		d.Events = make(chan *RedfishEvent, 10)
	}

	log.Printf("%s: Starting alert listener...", d.SystemID)
	go d.Client.ListenForAlerts(d.Ctx, d.Events)

	for {
		event := <-d.Events
		if event == nil {
			log.Printf("%s: Got SSE nil event", d.SystemID)
			continue
		}

		if event.Err != nil {
			log.Printf("%s: Got SSE error %s", d.SystemID, event.Err)
			if strings.Contains(event.Err.Error(), "connection error") {
				log.Printf("Sleep 5 minutes before restarting SSE connection for %s", d.SystemID)
				time.Sleep(time.Minute * 5)
			}
			go d.Client.ListenForAlerts(d.Ctx, d.Events)
			continue
		}

		d.LastEvent = time.Now()
		if event.Payload != nil {
			if ot, ok := event.Payload.Object["@odata.type"].(string); ok {
				if strings.Contains(ot, ".Event") {
					log.Printf("%s: Got new event", d.SystemID)
					d.parseAndSendEvents(event.Payload, dataBusService)
					continue
				}
				log.Printf("%s: Got unknown event type %s", d.SystemID, ot)
			}
		}
		log.Printf("%s: Got bad SSE event", d.SystemID)
	}
}

// parseAndSendEvents parses Redfish events and sends them to the databus
func (d *ConfigurableDevice) parseAndSendEvents(events *RedfishPayload, dataBusService *databus.DataBusService) {
	eventData, ok := events.Object["Events"]
	if !ok {
		log.Printf("%s: Unable to get Events from payload", d.SystemID)
		return
	}

	eventsArray, ok := eventData.([]interface{})
	if !ok {
		log.Printf("%s: Events is not an array", d.SystemID)
		return
	}

	group := &databus.DataGroup{
		HostID:   d.Client.GetHostname(),
		HostName: d.HostName,
		System:   d.SystemID,
		FwVer:    d.FwVer,
	}

	if id, ok := events.Object["Id"].(string); ok {
		group.ID = id
	}

	for _, evt := range eventsArray {
		evtMap, ok := evt.(map[string]interface{})
		if !ok {
			continue
		}

		if evtMap["EventId"] == nil {
			continue
		}

		data := databus.EventValue{}

		if oc, ok := evtMap["OriginOfCondition"].(map[string]interface{}); ok {
			if odataID, ok := oc["@odata.id"].(string); ok {
				data.OriginOfCondition = odataID
			}
		}

		if v, ok := evtMap["EventId"].(string); ok {
			data.EventId = v
		}
		if v, ok := evtMap["EventType"].(string); ok {
			data.EventType = v
		}
		if v, ok := evtMap["EventTimestamp"].(string); ok {
			data.EventTimestamp = v
		}
		if v, ok := evtMap["MemberId"].(string); ok {
			data.MemberId = v
		}
		if v, ok := evtMap["MessageSeverity"].(string); ok {
			data.MessageSeverity = v
		}
		if v, ok := evtMap["Message"].(string); ok {
			data.Message = v
		}
		if v, ok := evtMap["MessageId"].(string); ok {
			data.MessageId = v
		}
		if args, ok := evtMap["MessageArgs"].([]interface{}); ok {
			for _, a := range args {
				if s, ok := a.(string); ok {
					data.MessageArgs = append(data.MessageArgs, s)
				}
			}
		}

		group.Events = append(group.Events, data)
	}

	dataBusService.SendGroup(*group)
}

// MonitorStart initializes the device and starts monitoring
func (d *ConfigurableDevice) MonitorStart(dataBusService *databus.DataBusService, authClient auth.AuthClientInterface) {
	// Get system ID using configured endpoints
	systemID, err := d.Client.GetSystemId()
	if err != nil || systemID == "" {
		log.Printf("%s: Failed to get system id! %v", d.Client.GetHostname(), err)
		authClient.UpdateServiceState(auth.CONNFAILED, d.Client.GetHostname())
		return
	}

	// Get system info using configured endpoints
	hostName, fwver, err := d.Client.GetSysInfo()
	if err != nil || hostName == "" {
		log.Printf("%s: Failed to get hostName! %v", d.Client.GetHostname(), err)
		hostName = systemID
	}

	log.Printf("%s: Got System ID %s, Hostname %s", d.Client.GetHostname(), systemID, hostName)

	d.SystemID = systemID
	d.HostName = hostName
	d.FwVer = fwver

	d.Client.SetFwVer(fwver)

	// Update state and start listening
	authClient.UpdateServiceState(auth.MONITORING, d.Client.GetHostname())
	d.State = auth.RUNNING

	// Start alert listener
	d.StartAlertListener(dataBusService)
}
