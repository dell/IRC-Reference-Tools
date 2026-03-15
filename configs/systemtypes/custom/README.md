# Custom System Types

This folder is for **user-defined system type configurations**. Add a YAML file here to define how the application interacts with your custom hardware or devices.

---

## Getting Started

1. Create a new `.yaml` file in this folder (e.g., `my-device.yaml`)
2. Define the required fields (see below)
3. Restart the containers
4. Your new system type will appear in the UI dropdown

---

## Configuration File Format

Each YAML file defines a single system type. Below is a complete example:

```yaml
name: "my-custom-device"

systemId:
  - uri: "/redfish/v1/Systems/1"
    path: "$.SerialNumber"

powerState:
  monitorType: "redfish-sse"
  uri: "/redfish/v1/Systems/1"
  path: "$.PowerState"
  powerOnMsgId: "Power.On"
  powerOffMsgId: "Power.Off"
  powerOnMessageArgs: "3-PoweredOn"
  powerOffMessageArgs: "3-PoweredOff"

sysInfo:
  hostname:
    uri: "/redfish/v1/Systems/1"
    path: "$.HostName"
  fwver:
    uri: "/redfish/v1/Managers/1"
    path: "$.FirmwareVersion"

sseFilter: "?$filter=EventType%20eq%20%27Alert%27"

shutdown:
  graceful:
    uri: "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "GracefulShutdown"}'
  force:
    uri: "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "ForceOff"}'
```

---

## Field Reference

| Field | Required | Status | Description |
|-------|----------|--------|-------------|
| `name` | **Yes** | ✅ Active | Unique identifier for this system type. This name appears in the UI dropdown. |
| `systemId` | **Yes** | ✅ Active | How to retrieve the unique system identifier (e.g., serial number, service tag). |
| `powerState` | **Yes** | ✅ Active | How to retrieve the current power state of the device. |
| `shutdown` | **Yes** | ✅ Active | API calls for graceful and/or forceful shutdown. |
| `sysInfo` | No | ✅ Active | Additional system information to collect (hostname, firmware version, etc.). |
| `sseFilter` | No | ✅ Active | Filter string for Server-Sent Events (SSE) subscription. |

---

## Field Details

### `name` (Required)
A unique name for your system type. This will appear in the UI dropdown for users to select.

```yaml
name: "my-custom-device"
```

### `systemId` (Required)
Defines how to retrieve the unique identifier for this device type. Tool performs a GET request to the specified URI and uses the JSONPath expression to extract the value from the response.

- **uri**: The Redfish API endpoint path
- **path**: JSONPath expression to extract the value from the response

```yaml
systemId:
  - uri: "/redfish/v1/Systems/1"
    path: "$.SerialNumber"
```

### `powerState` (Required)
Defines how to monitor and detect power state changes for this device. Supports different monitoring strategies.

#### Power State Configuration Fields

| Field | Required for | Description |
|-------|--------------|-------------|
| `monitorType` | **Yes** | Type of power state monitoring: `"redfish-sse"`, `"redfish-get"`, or `"tcp-ping"` |
| `uri` | redfish-sse, redfish-get | Redfish API endpoint for power state |
| `path` | redfish-sse, redfish-get | JSONPath expression to extract power state from response |
| `pollingInterval` | redfish-get (optional) | Polling interval in seconds for Redfish GET power-off detection. Defaults to 2 seconds if not set. |
| `powerOnMsgId` | redfish-sse | Message ID that indicates power on event |
| `powerOffMsgId` | redfish-sse | Message ID that indicates power off event |
| `powerOnMessageArgs` | redfish-sse (optional) | Message arguments that indicate power on state |
| `powerOffMessageArgs` | redfish-sse (optional) | Message arguments that indicate power off state |

#### Monitor Types

**1. `redfish-sse` (Redfish Server-Sent Events)**
- Monitors power state via Redfish SSE events
- Requires message ID configuration for event detection
- Best for devices that support SSE-based power monitoring

#### Message Arguments for Event Differentiation
Some systems use the same message ID for both power on and off events. In such cases, use `powerOnMessageArgs` and `powerOffMessageArgs` to differentiate between events. The message arguments use format "index-value" where:
- **index**: Position in the MessageArgs array for power state
- **value**: The specific value that indicates the power state

```yaml
powerState:
  monitorType: "redfish-sse"
  uri: "/redfish/v1/Systems/1"
  path: "$.PowerState"
  powerOnMessageArgs: "3-PoweredOn"
  powerOffMessageArgs: "3-PoweredOff"
```

**2. `redfish-get` (Redfish GET Polling)**
- Monitors power state by periodically polling the device via Redfish GET requests
- Requires `uri` and `path` to specify the Redfish endpoint and JSONPath for power state extraction
- Use `pollingInterval` to control how frequently the device is polled (in seconds). Defaults to 2 seconds if not specified.
- Best for devices that support Redfish but do not support SSE

```yaml
powerState:
  monitorType: "redfish-get"
  uri: "/redfish/v1/Systems/System_0"
  path: "$.PowerState"
  pollingInterval: 5
```

**3. `tcp-ping` (TCP Connectivity Check)**
- Monitors power state by checking TCP connectivity
- Checks default ports: 22, 80, 443
- Best for devices that don't support SSE or when network-based detection is preferred

```yaml
powerState:
  monitorType: "tcp-ping"
```

**4. Default Behavior**
- If `monitorType` is not specified, defaults to `"tcp-ping"`

### `shutdown` (Required)
Defines the API calls for shutting down the device. You must define at least `graceful` or `force` (or both).

Each shutdown action has:
- **uri**: The Redfish API endpoint path
- **method**: HTTP method (only "POST" or "PATCH")
- **body**: JSON payload to send
- **protocol**: (Optional) "https" or "http". Defaults to "https".

```yaml
shutdown:
  graceful:
    uri: "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "GracefulShutdown"}'
  force:
    uri: "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
    method: "POST"
    body: '{"ResetType": "ForceOff"}'
```

### `sysInfo` (Optional)
A collection of additional system information fields to retrieve. Each entry has a name (used internally) and an endpoint configuration.

Common fields include:
- `hostname` - Device hostname
- `fwver` - Firmware version

```yaml
sysInfo:
  hostname:
    uri: "/redfish/v1/Systems/1"
    path: "$.HostName"
  fwver:
    uri: "/redfish/v1/Managers/1"
    path: "$.FirmwareVersion"
```

### `sseFilter` (Optional)
Filter string appended to the SSE (Server-Sent Events) subscription URL. Used to filter which events are received.

```yaml
sseFilter: "?$filter=EventType%20eq%20%27Alert%27"
```

---

## JSONPath Syntax

The `path` field uses JSONPath syntax to extract values from JSON responses:

| Syntax | Description | Example |
|--------|-------------|---------|
| `$.Field` | Top-level field | `$.PowerState` |
| `$.Parent.Child` | Nested field | `$.Oem.Dell.ServiceTag` |
| `$.Array[0]` | Array element by index | `$.IPv4Addresses[0].Address` |

---

## Choosing the Right Monitor Type

| Monitor Type | Use Case | Pros | Cons |
|--------------|----------|------|------|
| **`redfish-sse`** | Devices with SSE support | Event-driven, instant detection, less network traffic | Requires SSE support, more complex setup |
| **`redfish-get`** | Devices with Redfish but no SSE | Works with any Redfish device, configurable polling interval | Polling-based, slightly higher network traffic than SSE |
| **`tcp-ping`** | Simple network monitoring | Simple, works with any network device, no Redfish required | Polling-based, less precise, more network traffic |

**Recommendation**: Use `redfish-sse` for modern Redfish-compliant devices that support Server-Sent Events. Use `redfish-get` for Redfish-capable devices that do not support SSE (e.g., NvBMC). Use `tcp-ping` for older devices or when Redfish is not available.

---

## Tips

- **Test your endpoints first**: Use a tool like `curl` to verify the Redfish endpoints work on your device before creating the config.
- **Check the JSON structure**: Make sure your JSONPath expressions match the actual response structure.
- **Start simple**: Begin with just `name` and `systemId`, then add more fields as needed.
- **Use existing configs as reference**: Look at the files in the `default/` folder for working examples.

---

## Troubleshooting

- **System type not appearing in UI**: Make sure the file has a `.yaml` or `.yml` extension and contains a valid `name` field.
- **Device not connecting**: Verify the Redfish endpoints are correct for your device type.
- **Values not being retrieved**: Check that your JSONPath expressions match the actual JSON response structure.
- **Power state events not working**: 
  - For `redfish-sse`: Verify `powerOnMsgId` and `powerOffMsgId` match the actual message IDs from your device
  - For `redfish-get`: Verify the `uri` and `path` return a valid power state (e.g., `"On"` or `"Off"`). Adjust `pollingInterval` if needed.
  - For `tcp-ping`: Ensure the device is reachable on ports 22, 80, or 443 when powered on
- **Configuration validation errors**: Check that all required fields for your `monitorType` are present.
