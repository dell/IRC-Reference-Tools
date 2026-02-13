# Leak Detection and Shutdown Workflow

High-level workflow for detecting liquid cooling leaks via IRC devices and automatically shutting down associated rack devices.

## Workflow Steps

### Step 1: Event Monitoring

The `redfishread` service connects to each IRC device over Redfish SSE (Server-Sent Events) and listens for Alert-type events in real time. Events are filtered to only receive alerts using the system type's configured SSE filter.

### Step 2: Leak Event Detection

A leak is identified when an incoming event has **MessageId `Dell.0.1.LCPO009`**. This is the sole trigger for the shutdown sequence.

### Step 3: Device Association Lookup

On leak detection, the `action` service looks up all rack devices associated with the IRC that reported the leak. Each associated device record includes its IP, credentials, system type, shutdown mode (graceful or force), and timeout.

### Step 4: Shutdown Execution

All associated devices are shut down concurrently. For each device:

- **Force shutdown (default and recommended)**: A force shutdown request is sent immediately. This is the default behavior and is recommended to ensure devices are powered off as quickly as possible during a leak event.
- **Graceful shutdown (optional)**: If graceful shutdown is explicitly configured for a device, a graceful shutdown request is sent instead. If the device does not power off within its **configured timeout**, the system automatically escalates to a force shutdown request. Select this option only if a graceful shutdown is specifically needed for the device.

### Step 5: Power State Verification

After issuing a shutdown command, the system verifies the device has powered off using the `monitorType` defined in the device's system type YAML configuration (see [configs/systemtypes/custom/README.md](../configs/systemtypes/custom/README.md)):

- **`redfish-sse`**: Listens for power-off SSE events matching the configured `powerOffMsgId`.
- **`tcp-ping`**: Polls TCP connectivity (ports 22, 80, 443) until the device is unreachable.

If the device is confirmed off, the shutdown is marked successful. If the graceful timeout expires, the system automatically escalates to force shutdown and waits for a secondary timeout.

### Step 6: State Update

Once all associated devices have been shut down (or marked as failed), the IRC's state is reset from `LEAKED` back to `MONITORING`, ready to handle subsequent events.
