package redfish

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
)

// RedfishPayload represents a Redfish response payload
type RedfishPayload struct {
	Object map[string]interface{}
	Array  []interface{}
	Float  float64
}

// RedfishEvent represents an SSE event from Redfish
type RedfishEvent struct {
	Err     error
	ID      string
	Payload *RedfishPayload
}

// ConfigurableRedfishClient is a Redfish client configured via SystemTypeConfig
type ConfigurableRedfishClient struct {
	Hostname   string
	Username   string
	Password   string
	HttpClient *http.Client
	Config     *systemtype.SystemTypeConfig
	FwVer      string
	IsIPv6     int
}

// NewConfigurableClient creates a new ConfigurableRedfishClient with the given config
func NewConfigurableClient(hostname, username, password string, config *systemtype.SystemTypeConfig) (*ConfigurableRedfishClient, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &ConfigurableRedfishClient{
		Hostname:   hostname,
		Username:   username,
		Password:   password,
		HttpClient: &http.Client{Transport: tr},
		Config:     config,
	}

	// Validate connection
	_, err := client.GetUri("/redfish/v1")
	if err != nil {
		return client, err
	}
	return client, nil
}

func (c *ConfigurableRedfishClient) addAuthToRequest(req *http.Request) {
	req.SetBasicAuth(c.Username, c.Password)
}

func (c *ConfigurableRedfishClient) GetHostname() string {
	return c.Hostname
}

func (c *ConfigurableRedfishClient) GetUsername() string {
	return c.Username
}

func (c *ConfigurableRedfishClient) GetPassword() string {
	return c.Password
}

func (c *ConfigurableRedfishClient) SetFwVer(ver string) {
	c.FwVer = ver
}

// GetUri fetches a Redfish URI and returns the payload
func (c *ConfigurableRedfishClient) GetUri(uri string) (*RedfishPayload, error) {
	if c.IsIPv6 == 0 {
		split := strings.Split(c.Hostname, ":")
		if len(split) > 2 {
			c.IsIPv6 = 1
			c.Hostname = "[" + c.Hostname + "]"
		} else {
			c.IsIPv6 = 2
		}
	}
	req, err := http.NewRequest("GET", "https://"+c.Hostname+uri, nil)
	if err != nil {
		return nil, err
	}
	c.addAuthToRequest(req)
	req.Header.Add("Accept", "application/json")
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetUri failed for %s with Error code %d",
			c.Hostname+uri, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ret := new(RedfishPayload)
	err = json.Unmarshal(body, &ret.Object)
	if err != nil {
		return nil, fmt.Errorf("Body %s could not parse to JSON %w", body, err)
	}
	return ret, nil
}

// GetSystemId extracts system ID using configured endpoints
func (c *ConfigurableRedfishClient) GetSystemId() (string, error) {
	if c.Config == nil || c.Config.SystemID == nil {
		return "", errors.New("no SystemID endpoint configured")
	}

	payload, err := c.GetUri(c.Config.SystemID.URI)
	if err != nil {
		logging.Error("Failed to get URI", "uri", c.Config.SystemID.URI, "err", err)
		return "", err
	}

	value, err := extractJSONPath(payload.Object, c.Config.SystemID.Path)
	if err != nil {
		logging.Error("Failed to extract path", "path", c.Config.SystemID.Path, "uri", c.Config.SystemID.URI, "err", err)
		return "", err
	}

	if value != "" {
		return value, nil
	}

	return "", errors.New("unable to determine System ID from configured endpoint")
}

// GetSysInfo extracts system info using configured endpoints
func (c *ConfigurableRedfishClient) GetSysInfo() (hostname, fwver string, err error) {
	if c.Config == nil || c.Config.SysInfo == nil {
		return
	}

	fields := map[string]*string{
		"hostname": &hostname,
		"fwver":    &fwver,
	}

	for fieldName, fieldPtr := range fields {
		endpoint, ok := c.Config.SysInfo[fieldName]
		if !ok {
			continue
		}

		payload, getErr := c.GetUri(endpoint.URI)
		if getErr != nil {
			logging.Error("Failed to get URI", "uri", endpoint.URI, "field", fieldName, "err", getErr)
			continue
		}

		value, extractErr := extractJSONPath(payload.Object, endpoint.Path)
		if extractErr != nil {
			logging.Error("Failed to extract path", "path", endpoint.Path, "field", fieldName, "err", extractErr)
			continue
		}

		*fieldPtr = value
	}

	return
}

// GetPowerState extracts power state using configured endpoint
func (c *ConfigurableRedfishClient) GetPowerState() (string, error) {
	if c.Config == nil || c.Config.PowerState == nil {
		return "", errors.New("no PowerState endpoint configured")
	}

	payload, err := c.GetUri(c.Config.PowerState.URI)
	if err != nil {
		return "", fmt.Errorf("failed to get power state URI %s: %w", c.Config.PowerState.URI, err)
	}

	value, err := extractJSONPath(payload.Object, c.Config.PowerState.Path)
	if err != nil {
		return "", fmt.Errorf("failed to extract power state: %w", err)
	}

	return value, nil
}

// ListenForAlerts starts SSE listener for alerts using configured filter
func (c *ConfigurableRedfishClient) ListenForAlerts(ctx context.Context, event chan<- *RedfishEvent) {
	ret := new(RedfishEvent)
	serviceRoot, err := c.GetUri("/redfish/v1")
	if err != nil {
		logging.Error("Unable to get service root", "err", err)
		ret.Err = err
		event <- ret
		return
	}

	eventService, err := c.getPropertyByName(serviceRoot, "EventService")
	if err != nil {
		ret.Err = err
		event <- ret
		return
	}

	if eventService.Object["ServerSentEventUri"] == nil {
		logging.Error("Don't support POST back yet")
		ret.Err = errors.New("don't support POST back yet")
		event <- ret
		return
	}

	sseUri := "https://" + c.Hostname + eventService.Object["ServerSentEventUri"].(string)

	// Append configured filter
	if c.Config != nil && c.Config.SSEFilter != "" {
		sseUri += c.Config.SSEFilter
	}

	ret.Err = c.StartSSE(ctx, event, sseUri)
	if ret.Err != nil {
		event <- ret
	}
}

// SSE Event parsing
type sseEvent struct {
	ID   string
	Data []byte
}

// StartSSE starts an SSE connection
func (c *ConfigurableRedfishClient) StartSSE(ctx context.Context, event chan<- *RedfishEvent, sseURI string) error {
	req, err := http.NewRequest("GET", sseURI, nil)
	if err != nil {
		return err
	}
	c.addAuthToRequest(req)
	req.Header.Add("Accept", "*/*")

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		logging.Error("Error connecting", "err", err)
		return errors.New("connection error")
	}

	lastTS := time.Now()
	reader := bufio.NewReader(resp.Body)

	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			sseEvt, err := readSSEEvent(reader)
			redfishEvent := new(RedfishEvent)
			redfishEvent.ID = sseEvt.ID

			if err != nil {
				redfishEvent.Err = err
				if strings.Contains(err.Error(), "EOF") {
					if lastTS.Before(time.Now().Add(-time.Minute * 60)) {
						redfishEvent.Err = errors.New("sse idle timeout")
					} else {
						redfishEvent.Err = errors.New("connection error")
					}
				}
				redfishEvent.Payload = nil
				event <- redfishEvent
				resp.Body.Close()
				return nil
			}

			lastTS = time.Now()
			ret := new(RedfishPayload)
			err = json.Unmarshal(sseEvt.Data, &ret.Object)
			if err != nil {
				logging.Error("Failed to parse message", "err", err)
				continue
			}
			redfishEvent.Payload = ret
			redfishEvent.Err = nil
			event <- redfishEvent
		}
	}
}

// readSSEEvent reads a single SSE event from the reader
func readSSEEvent(reader *bufio.Reader) (sseEvent, error) {
	var evt sseEvent
	var dataBuffer bytes.Buffer

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return evt, err
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// Empty line marks end of event
			if dataBuffer.Len() > 0 {
				evt.Data = dataBuffer.Bytes()
				return evt, nil
			}
			continue
		}

		if strings.HasPrefix(line, "id:") {
			evt.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if dataBuffer.Len() > 0 {
				dataBuffer.WriteByte('\n')
			}
			dataBuffer.WriteString(data)
		}
	}
}

// extractJSONPath extracts a value from a JSON object using a simple path notation
// Supports: $.field, $.field.subfield, $.array[0].field
func extractJSONPath(obj map[string]interface{}, path string) (string, error) {
	if !strings.HasPrefix(path, "$.") {
		return "", fmt.Errorf("invalid JSONPath: must start with $.")
	}

	path = strings.TrimPrefix(path, "$.")
	parts := strings.Split(path, ".")

	var current interface{} = obj
	for _, part := range parts {
		// Handle array index notation: field[0]
		if idx := strings.Index(part, "["); idx != -1 {
			fieldName := part[:idx]
			indexStr := strings.TrimSuffix(part[idx+1:], "]")
			var index int
			fmt.Sscanf(indexStr, "%d", &index)

			if m, ok := current.(map[string]interface{}); ok {
				current = m[fieldName]
			} else {
				return "", fmt.Errorf("expected object at %s", fieldName)
			}

			if arr, ok := current.([]interface{}); ok {
				if index >= len(arr) {
					return "", fmt.Errorf("index %d out of bounds", index)
				}
				current = arr[index]
			} else {
				return "", fmt.Errorf("expected array at %s", part)
			}
		} else {
			if m, ok := current.(map[string]interface{}); ok {
				current = m[part]
			} else {
				return "", fmt.Errorf("expected object at %s", part)
			}
		}

		if current == nil {
			return "", fmt.Errorf("field %s not found", part)
		}
	}

	switch v := current.(type) {
	case string:
		return v, nil
	case float64:
		return fmt.Sprintf("%v", v), nil
	case bool:
		return fmt.Sprintf("%v", v), nil
	default:
		return "", fmt.Errorf("unexpected type %T at path %s", current, path)
	}
}

// getPropertyByName gets a property from a Redfish payload by following the @odata.id link
func (c *ConfigurableRedfishClient) getPropertyByName(payload *RedfishPayload, name string) (*RedfishPayload, error) {
	if payload.Object[name] == nil {
		return nil, fmt.Errorf("property %s not found", name)
	}

	prop, ok := payload.Object[name].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("property %s is not an object", name)
	}

	// If it has @odata.id, fetch it
	if odataID, ok := prop["@odata.id"].(string); ok {
		return c.GetUri(odataID)
	}

	ret := new(RedfishPayload)
	ret.Object = prop
	return ret, nil
}
