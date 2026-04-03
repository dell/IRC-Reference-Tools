package systemtype

// Monitor type constants
const (
	MonitorTypeRedfishSSE = "redfish-sse"
	MonitorTypeRedfishGet = "redfish-get"
	MonitorTypeTCPPing    = "tcp-ping"
)

// EndpointConfig defines a URI and JSONPath to extract a value from a Redfish response
type EndpointConfig struct {
	URI  string `json:"uri" yaml:"uri"`   // e.g., "/redfish/v1/Chassis/System.Embedded.1"
	Path string `json:"path" yaml:"path"` // JSONPath, e.g., "$.PowerState"
}

// APIAction defines a single shutdown action (graceful or force)
type APIAction struct {
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"` // Protocol, e.g., "https" or "http". Defaults to "https" if not specified
	URI      string `json:"uri" yaml:"uri"`                               // e.g., "/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset"
	Method   string `json:"method" yaml:"method"`                         // HTTP method, e.g., "POST"
	Body     string `json:"body" yaml:"body"`                             // JSON body, e.g., "{\"ResetType\": \"GracefulShutdown\"}"
}

// ShutdownConfig defines shutdown actions for a system type
type ShutdownConfig struct {
	Graceful *APIAction `json:"graceful,omitempty" yaml:"graceful,omitempty"` // Graceful shutdown action
	Force    *APIAction `json:"force,omitempty" yaml:"force,omitempty"`       // Forceful shutdown action
}

// PowerStateConfig defines power state monitoring configuration
type PowerStateConfig struct {
	EndpointConfig      `json:",inline" yaml:",inline"` // Embed uri and path
	MonitorType         string                          `json:"monitorType" yaml:"monitorType"`                                     // "redfish-sse", "redfish-get", or "tcp-ping"
	PollingInterval     int                             `json:"pollingInterval,omitempty" yaml:"pollingInterval,omitempty"`         // For redfish-get only (seconds)
	PowerOnMsgId        string                          `json:"powerOnMsgId,omitempty" yaml:"powerOnMsgId,omitempty"`               // For redfish-sse only
	PowerOffMsgId       string                          `json:"powerOffMsgId,omitempty" yaml:"powerOffMsgId,omitempty"`             // For redfish-sse only
	PowerOnMessageArgs  string                          `json:"powerOnMessageArgs,omitempty" yaml:"powerOnMessageArgs,omitempty"`   // MessageArgs containing these strings indicate power on
	PowerOffMessageArgs string                          `json:"powerOffMessageArgs,omitempty" yaml:"powerOffMessageArgs,omitempty"` // MessageArgs containing these strings indicate power off
}

// SystemTypeConfig defines per-system-type behavior for Redfish interactions and shutdown actions
type SystemTypeConfig struct {
	Name      string `json:"name" yaml:"name"`           // e.g., "idrac", "irc", "pdu"
	IsBuiltIn bool   `json:"isBuiltIn" yaml:"isBuiltIn"` // true for default configs, false for custom
	IsParent  bool   `json:"isParent" yaml:"isParent"`   // true for parent system types (e.g., IRC) that should not be added to DB

	// Power State Configuration
	PowerState *PowerStateConfig `json:"powerState,omitempty" yaml:"powerState,omitempty"` // Power state monitoring configuration

	// System ID Configuration
	SystemID *EndpointConfig           `json:"systemId,omitempty" yaml:"systemId,omitempty"` // URI to try for system ID extraction
	SysInfo  map[string]EndpointConfig `json:"sysInfo,omitempty" yaml:"sysInfo,omitempty"`   // Map of field name -> endpoint (hostname, fqdn, sku, etc.)

	// SSE Configuration
	SSEFilter string `json:"sseFilter,omitempty" yaml:"sseFilter,omitempty"` // e.g., "?$filter=EventType%20eq%20%27Alert%27"

	// Shutdown Configuration
	Shutdown *ShutdownConfig `json:"shutdown,omitempty" yaml:"shutdown,omitempty"` // Shutdown actions (graceful and/or force)

	// IRC-specific features
	HasWatchdog bool `json:"hasWatchdog,omitempty" yaml:"hasWatchdog"` // Whether to set/pet watchdog
	HasValves   bool `json:"hasValves,omitempty" yaml:"hasValves"`     // Whether to track valve state
}
