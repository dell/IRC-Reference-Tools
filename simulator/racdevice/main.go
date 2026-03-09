package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sseHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}

	startupEvent []byte
}

func newSSEHub(startupEvent []byte) *sseHub {
	return &sseHub{
		clients:      make(map[chan []byte]struct{}),
		startupEvent: startupEvent,
	}
}

func (h *sseHub) addClient(ch chan []byte) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *sseHub) removeClient(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *sseHub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Drop if the client is slow.
		}
	}
}

type server struct {
	port int
	hub  *sseHub

	username string
	password string

	powerMu    sync.Mutex
	powerState string
	deviceType string
}

func (s *server) requireAuth() bool {
	return strings.TrimSpace(s.username) != "" || strings.TrimSpace(s.password) != ""
}

func (s *server) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAuth() {
			next.ServeHTTP(w, r)
			return
		}

		u, p, ok := r.BasicAuth()
		if ok {
			uOK := subtle.ConstantTimeCompare([]byte(u), []byte(s.username)) == 1
			pOK := subtle.ConstantTimeCompare([]byte(p), []byte(s.password)) == 1
			if uOK && pOK {
				next.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="idrac-mock"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/redfish/v1", enforceMethod(http.MethodGet, s.handleRoot))
	mux.HandleFunc("/redfish/v1/EventService", enforceMethod(http.MethodGet, s.handleEventService))
	mux.HandleFunc("/redfish/v1/SSE", enforceMethod(http.MethodGet, s.handleSSE))
	mux.HandleFunc("/redfish/v1/EventService/SSE", enforceMethod(http.MethodGet, s.handleSSE))
	mux.HandleFunc("/redfish/v1/Chassis/System.Embedded.1", enforceMethod(http.MethodGet, s.handleChassis))
	mux.HandleFunc("/redfish/v1/Systems/System_0", enforceMethod(http.MethodGet, s.handleChassis)) // BMC: NvLInk
	mux.HandleFunc("/redfish/v1/Systems/Self", enforceMethod(http.MethodGet, s.handleChassis))     // BMC: CDU
	mux.HandleFunc("/redfish/v1/Chassis/1", enforceMethod(http.MethodGet, s.handleChassis))        // BMC: NvBMC
	mux.HandleFunc("/redfish/v1/Managers/iDRAC.Embedded.1", enforceMethod(http.MethodGet, s.handleManager))
	mux.HandleFunc("/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset", enforceMethod(http.MethodPost, s.handleReset))
	mux.HandleFunc("/redfish/v1/Systems/System_0/Actions/ComputerSystem.Reset", enforceMethod(http.MethodPost, s.handleReset))        // BMC: NvLink and NVBMC
	mux.HandleFunc("/redfish/v1/Managers/Self/Actions/Oem/LITEONManager.SystemSwitch", enforceMethod(http.MethodPost, s.handleReset)) // BMC: CDU
	mux.HandleFunc("/redfish/v1/Systems/Self/Actions/ComputerSystem.Reset", enforceMethod(http.MethodPost, s.handleReset))            // BMC: AMI

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	return s.basicAuthMiddleware(mux)
}

func enforceMethod(method string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"method not allowed"}`))
			return
		}
		handler(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"EventService": map[string]any{
			"@odata.id": "/redfish/v1/EventService",
		},
		"Oem": map[string]any{
			"Dell": map[string]any{
				"ServiceTag": fmt.Sprintf("SVC%d", s.port),
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleEventService(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"ServerSentEventUri": "/redfish/v1/SSE",
		"Status": map[string]any{
			"State": "Enabled",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleChassis(w http.ResponseWriter, r *http.Request) {
	s.powerMu.Lock()
	ps := s.powerState
	s.powerMu.Unlock()

	resp := map[string]any{
		"PowerState":   ps,
		"SerialNumber": fmt.Sprintf("SN%d", s.port),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleManager(w http.ResponseWriter, r *http.Request) {
	selectQ := r.URL.Query().Get("$select")
	if selectQ != "" && selectQ != "FirmwareVersion" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported $select"})
		return
	}

	resp := map[string]any{
		"FirmwareVersion": "1.30.10.51",
	}
	writeJSON(w, http.StatusOK, resp)
}

type resetRequest struct {
	ResetType string `json:"ResetType"`
}

type resetRequest_cdu struct {
	Switch string `json:"Switch"`
}

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	eventJSON := startupEventJSON
	eventJSON_turningOff := turningOffEventJSON
	switch s.deviceType {
	case "nvlink":
		eventJSON = startupEventJSON_nvlink
		eventJSON_turningOff = turningOffEventJSON_nvlink
	case "ami":
		eventJSON = startupEventJSON_ami
		eventJSON_turningOff = turningOffEventJSON_ami
	case "cdu_lantern":
		eventJSON = startupEventJSON_cdu
		eventJSON_turningOff = turningOffEventJSON_cdu
	case "nvbmc":
		// nvbmc does not support power on/off SSE events
	default: // idrac
	}

	var resetType string
	switch s.deviceType {
	case "cdu_lantern":
		var req resetRequest_cdu
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}

		resetType = strings.ToLower(strings.TrimSpace(req.Switch))
		if resetType != "0" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Switch must be 0 for Off"})
			return
		}
	default:
		// idrac, nvbmc, ami, nvlink
		var req resetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}

		resetType = strings.ToLower(strings.TrimSpace(req.ResetType))
		if resetType != "gracefulshutdown" && resetType != "forceoff" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ResetType must be GracefulShutdown or Forceoff"})
			return
		}
	}

	if resetType == "gracefulshutdown" || resetType == "forceoff" || resetType == "0" {
		s.powerMu.Lock()
		s.powerState = "Off"
		s.powerMu.Unlock()

		// Send event 10 seconds after Reset action.
		go func() {
			t := time.NewTimer(10 * time.Second)
			defer t.Stop()
			<-t.C
			s.hub.broadcast([]byte(eventJSON_turningOff))
		}()
		// Send event 30 seconds after Reset action.
		go func() {
			t := time.NewTimer(30 * time.Second)
			defer t.Stop()
			<-t.C
			s.hub.broadcast([]byte(eventJSON))
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "OK"})
}

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientCh := make(chan []byte, 16)
	s.hub.addClient(clientCh)
	defer s.hub.removeClient(clientCh)

	// Send the startup event immediately upon connection.
	_, _ = fmt.Fprintf(w, "data: %s\n\n", s.hub.startupEvent)
	flusher.Flush()

	ctx := r.Context()
	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAlive.C:
			// Comment keepalive.
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case msg := <-clientCh:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

const startupEventJSON = `{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"35275","EventTimestamp":"2026-01-13T20:46:57-06:00","MessageSeverity":"OK","Message":"System is turning on.","MessageId":"IDRAC.SYS1000","MemberId":"0","MessageArgs":null,"OriginOfCondition":{"@odata.id":""},"Oem":{"Dell":{"AgentID":"DE","BitMask":"4097","Category":"Audit","ServerHostname":"mockhost"}}}],"Id":"28084","Context":"SSE-Subscription-11"}`

const turningOffEventJSON = `{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"35272","EventTimestamp":"2026-01-13T20:44:25-06:00","MessageSeverity":"OK","Message":"System is turning off.","MessageId":"IDRAC.SYS1001","MemberId":"0","MessageArgs":null,"OriginOfCondition":{"@odata.id":""},"Oem":{"Dell":{"AgentID":"DE","BitMask":"4097","Category":"Audit","ServerHostname":"mockhost"}}}],"Id":"28060","Context":"SSE-Subscription-9"}`
const startupEventJSON_nvlink = `{ "@odata.context": "/redfish/v1/$metadata#Event.Event", "@odata.id": "/redfish/v1/EventService/Subscriptions/1", "@odata.type": "#Event.v1_7_0.Event", "Id": "1", "Name": "Power On Event", "Description": "NVIDIA NVLink switch powered on", "Events": [ { "EventId": "12345", "EventTimestamp": "2026-02-17T18:04:00Z", "EventType": "StatusChange", "MessageId": "EventLog.1.0.StatusChange", "Message": "NVIDIA NVLink Switch powered on successfully", "Severity": "Information", "OriginOfCondition": "/redfish/v1/Systems/NVLinkSwitch1", "MessageArgs": [ "NVLinkSwitch1", "Powered On", "Normal" ] } ] }`

const turningOffEventJSON_nvlink = `{ "@odata.context": "/redfish/v1/$metadata#Event.Event", "@odata.id": "/redfish/v1/EventService/Subscriptions/1", "@odata.type": "#Event.v1_7_0.Event", "Id": "1", "Name": "Power Off Event", "Description": "NVIDIA NVLink switch powered off", "Events": [ { "EventId": "12346", "EventTimestamp": "2026-02-17T18:05:00Z", "EventType": "StatusChange", "MessageId": "EventLog.1.0.StatusChange", "Message": "NVIDIA NVLink Switch powered off", "Severity": "Warning", "OriginOfCondition": "/redfish/v1/Systems/NVLinkSwitch1", "MessageArgs": [ "NVLinkSwitch1", "Powered Off", "Normal" ] } ] }`

const startupEventJSON_ami = `{"@odata.type":"#Event.v1_4_1.Event","Events":[{"EventTimestamp":"2026-02-13T15:20:42-05:00","MessageId":"EventLog.1.0.StatusChange","Context":"Subscription_SSE_1","EventType":"Other","EventId":"9037","MemberId":"13","OriginOfCondition":{"@odata.id":"/redfish/v1/Systems/Self"},"Severity":"OK","Message":"The status State of resource at /redfish/v1/Systems/Self has changed from  StandbyOffline to Enabled  ","MessageArgs":["State","/redfish/v1/Systems/Self","StandbyOffline", "Enabled"]}],"Id":"9037","Events@odata.count":1,"@odata.context":"/redfish/v1/$metadata#Event.Event","Name":"Event Array","Context":"Subscription_SSE_1"}`

const turningOffEventJSON_ami = `{"@odata.type":"#Event.v1_4_1.Event","Events":[{"EventTimestamp":"2026-02-13T15:20:42-05:00","MessageId":"EventLog.1.0.StatusChange","Context":"Subscription_SSE_1","EventType":"Other","EventId":"9037","MemberId":"13","OriginOfCondition":{"@odata.id":"/redfish/v1/Systems/Self"},"Severity":"OK","Message":"The status State of resource at /redfish/v1/Systems/Self has changed from  Enabled to StandbyOffline ","MessageArgs":["State","/redfish/v1/Systems/Self","Enabled", "StandbyOffline"]}],"Id":"9037","Events@odata.count":1,"@odata.context":"/redfish/v1/$metadata#Event.Event","Name":"Event Array","Context":"Subscription_SSE_1"}`

const startupEventJSON_cdu = `{"@odata.type":"#Event.v1_4_1.Event","Events":[{"EventTimestamp":"2026-02-18T12:54:14-05:00","Category":"System","Message":"CDU is on runing state","Context":"Subscription_SSE_10","EventType":"Event","SubCategory":"Control","EventId":"405","MemberId":"466","Severity":"OK","Action":"None","MessageId":"RCDU005","Description":"CDU is on runing state"}],"Id":"405","Events@odata.count":1,"@odata.context":"/redfish/v1/$metadata#Event.Event","Name":"Event Array","Context":"Subscription_SSE_10"}`

const turningOffEventJSON_cdu = `{"@odata.type":"#Event.v1_4_1.Event","Events":[{"EventTimestamp":"2026-02-18T12:58:42-05:00","Category":"System","Message":"CDU is on Halted state","Context":"Subscription_SSE_10","EventType":"Event","SubCategory":"State","EventId":"408","MemberId":"469","Severity":"Critical","Action":"checking log to find and replace the failed sensor or device","MessageId":"RCDU011","Description":"CDU is on Halted state"}],"Id":"408","Events@odata.count":1,"@odata.context":"/redfish/v1/$metadata#Event.Event","Name":"Event Array","Context":"Subscription_SSE_10"}`

type portAssignment struct {
	Port       int
	DeviceType string
}

func parseDeviceTypes(primary string, extras []string) ([]string, error) {
	allowed := map[string]string{
		"idrac":       "idrac",
		"nvlink":      "nvlink",
		"ami":         "ami",
		"cdu_lantern": "cdu_lantern",
		"nvbmc":       "nvbmc",
	}

	var out []string
	add := func(raw string) error {
		for _, part := range strings.Split(raw, ",") {
			dt := strings.ToLower(strings.TrimSpace(part))
			if dt == "" {
				continue
			}
			canonical, ok := allowed[dt]
			if !ok {
				return fmt.Errorf("unsupported deviceType %q", dt)
			}
			out = append(out, canonical)
		}
		return nil
	}

	if primary != "" {
		if err := add(primary); err != nil {
			return nil, err
		}
	}
	for _, extra := range extras {
		if err := add(extra); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parsePortMap(value string) ([]portAssignment, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("portMap cannot be empty")
	}

	allowed := map[string]string{
		"idrac":       "idrac",
		"nvlink":      "nvlink",
		"ami":         "ami",
		"cdu_lantern": "cdu_lantern",
		"nvbmc":       "nvbmc",
	}

	entries := strings.Split(value, ",")
	var assignments []portAssignment
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid portMap entry %q (expected deviceType:ports)", entry)
		}

		deviceType := strings.TrimSpace(parts[0])
		if deviceType == "" {
			return nil, fmt.Errorf("invalid portMap entry %q: missing device type", entry)
		}
		canonicalType, ok := allowed[strings.ToLower(deviceType)]
		if !ok {
			return nil, fmt.Errorf("invalid portMap entry %q: unsupported device type %q", entry, deviceType)
		}

		portSpec := strings.TrimSpace(parts[1])
		if portSpec == "" {
			return nil, fmt.Errorf("invalid portMap entry %q: missing port spec", entry)
		}

		ports, err := expandPortSpec(portSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid port spec %q: %w", portSpec, err)
		}

		for _, port := range ports {
			assignments = append(assignments, portAssignment{Port: port, DeviceType: canonicalType})
		}
	}

	if len(assignments) == 0 {
		return nil, fmt.Errorf("portMap produced no assignments")
	}

	return assignments, nil
}

func expandPortSpec(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if strings.Contains(spec, "-") {
		start, end, err := parsePortRange(spec)
		if err != nil {
			return nil, err
		}
		ports := make([]int, 0, end-start+1)
		for p := start; p <= end; p++ {
			ports = append(ports, p)
		}
		return ports, nil
	}

	port, err := strconv.Atoi(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid port")
	}
	if port <= 0 {
		return nil, fmt.Errorf("port must be >0")
	}
	return []int{port}, nil
}

func parsePortRange(s string) (start, end int, err error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}
	start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start port")
	}
	end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end port")
	}
	if start <= 0 || end <= 0 || end < start {
		return 0, 0, fmt.Errorf("invalid range")
	}
	return start, end, nil
}

func startOne(ctx context.Context, port int, useTLS bool, tlsCertFile, tlsKeyFile string, username, password string, deviceType string) error {
	fmt.Println("Starting server on device type", deviceType, "on port", port)
	eventJSON := startupEventJSON
	switch deviceType {
	case "nvlink":
		eventJSON = startupEventJSON_nvlink
	case "ami":
		eventJSON = startupEventJSON_ami
	case "irc":
		// irc uses the default idrac-like behavior in this simulator.
	case "cdu_lantern":
		eventJSON = startupEventJSON_cdu
	case "nvbmc":
		// nvbmc does not support power on/off SSE events
	default: // idrac
	}
	srv := &server{
		port:       port,
		hub:        newSSEHub([]byte(eventJSON)),
		username:   username,
		password:   password,
		powerState: "On",
		deviceType: deviceType,
	}

	h := srv.routes()
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%d", port)
	var err error
	if useTLS {
		err = httpSrv.ListenAndServeTLS(tlsCertFile, tlsKeyFile)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func main() {
	var port int
	var portRange string
	var portMap string
	var useTLS bool
	var tlsCertFile string
	var tlsKeyFile string
	var username string
	var password string
	var deviceType string

	flag.IntVar(&port, "port", 8080, "single port to listen on")
	flag.StringVar(&portRange, "portRange", "", "port range to listen on (e.g. 8000-8010). If set, overrides -port")
	flag.BoolVar(&useTLS, "tls", false, "enable TLS (HTTPS)")
	flag.StringVar(&tlsCertFile, "tlsCert", "", "path to TLS certificate file (PEM). Required when -tls is set")
	flag.StringVar(&tlsKeyFile, "tlsKey", "", "path to TLS private key file (PEM). Required when -tls is set")
	flag.StringVar(&username, "username", "", "HTTP Basic Auth username (if set, enables Basic Auth)")
	flag.StringVar(&password, "password", "", "HTTP Basic Auth password (if set, enables Basic Auth)")
	flag.StringVar(&deviceType, "deviceType", "idrac", "Device type (idrac, nvlink, ami, cdu_lantern, nvbmc)")
	flag.StringVar(&portMap, "portMap", "", "Comma-separated list of deviceType:portSpec pairs (e.g. 'idrac:6000-6002,nvlink:6003')")
	flag.Parse()

	if useTLS {
		if strings.TrimSpace(tlsCertFile) == "" || strings.TrimSpace(tlsKeyFile) == "" {
			log.Fatal("-tls requires -tlsCert and -tlsKey")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if strings.TrimSpace(portMap) != "" {
		assignments, err := parsePortMap(portMap)
		if err != nil {
			log.Fatalf("invalid -portMap: %v", err)
		}

		errCh := make(chan error, len(assignments))
		for _, assignment := range assignments {
			assignment := assignment
			go func() {
				errCh <- startOne(ctx, assignment.Port, useTLS, tlsCertFile, tlsKeyFile, username, password, assignment.DeviceType)
			}()
		}

		if err := <-errCh; err != nil {
			log.Fatal(err)
		}
		return
	}

	if portRange != "" {
		start, end, err := parsePortRange(portRange)
		if err != nil {
			log.Fatalf("invalid -portRange: %v", err)
		}

		errCh := make(chan error, end-start+1)
		for p := start; p <= end; p++ {
			p := p
			go func() {
				errCh <- startOne(ctx, p, useTLS, tlsCertFile, tlsKeyFile, username, password, deviceType)
			}()
		}

		// Block until the first server errors.
		err = <-errCh
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := startOne(ctx, port, useTLS, tlsCertFile, tlsKeyFile, username, password, deviceType); err != nil {
		log.Fatal(err)
	}
}
