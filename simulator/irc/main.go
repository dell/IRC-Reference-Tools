package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SSEEvent struct {
	ID    string          `json:"id,omitempty"`
	Event string          `json:"event,omitempty"`
	Retry int             `json:"retry,omitempty"`
	Data  json.RawMessage `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]chan SSEEvent
	seq     uint64
}

type WatchdogState struct {
	mu              sync.RWMutex
	WatchdogTimeout int
	LastPetUTC      time.Time
	Ended           bool
	Continue        bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uint64]chan SSEEvent)}
}

func (h *Hub) AddClient(ch chan SSEEvent) uint64 {
	id := atomic.AddUint64(&h.seq, 1)
	h.mu.Lock()
	h.clients[id] = ch
	h.mu.Unlock()
	return id
}

func (h *Hub) RemoveClient(id uint64) {
	h.mu.Lock()
	ch, ok := h.clients[id]
	if ok {
		delete(h.clients, id)
	}
	h.mu.Unlock()
	if ok {
		close(ch)
	}
}

type patchSimplePolicyRequest struct {
	WatchdogTimeout int `json:"WatchdogTimeout"`
}

func patchSimplePolicyHandler(wd *WatchdogState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.Header().Set("Allow", http.MethodPatch)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req patchSimplePolicyRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if req.WatchdogTimeout <= 0 {
			http.Error(w, "WatchdogTimeout must be > 0", http.StatusBadRequest)
			return
		}

		wd.mu.Lock()
		wd.WatchdogTimeout = req.WatchdogTimeout
		wd.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

type endWatchdogRequest struct {
	Continue bool `json:"Continue"`
}

func endWatchdogHandler(wd *WatchdogState, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req endWatchdogRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}

		wd.mu.Lock()
		wd.Ended = true
		wd.Continue = req.Continue
		wd.mu.Unlock()

		go sendValveCloseEvents(hub)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func petWatchdogHandler(wd *WatchdogState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		wd.mu.Lock()
		wd.LastPetUTC = time.Now().UTC()
		wd.Ended = false
		wd.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func redfishRootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"EventService": {"@odata.id": "/redfish/v1/EventService"}}`))
}

func redfishEventServiceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"Status": {"State": "Enabled"},"ServerSentEventUri": "/redfish/v1/SSE"}`))
}

func serviceTagFromRequest(r *http.Request) string {
	// r.Host is typically host:port for non-default ports.
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "SVC"
	}
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		// Might be missing port; best effort.
		return "SVC"
	}
	port = strings.TrimSpace(port)
	if port == "" {
		return "SVC"
	}
	return "SVC" + port
}

func redfishManagersIRCHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"Oem": map[string]any{
			"Dell": map[string]any{
				"ServiceTag": serviceTagFromRequest(r),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *Hub) Broadcast(ev SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		select {
		case ch <- ev:
		default:
			// If a client is slow, drop this event instead of blocking the whole hub.
		}
	}
}

func parsePortRangeString(v string) (start int, end int, ok bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, false
	}
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lo, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	hi, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if lo <= 0 || hi <= 0 || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

type AuthConfig struct {
	Enabled  bool
	Username string
	Password string
}

func requireBasicAuth(cfg AuthConfig, next http.HandlerFunc) http.HandlerFunc {
	if !cfg.Enabled {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="SSE"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

var samples = []json.RawMessage{
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43332","EventTimestamp":"2026-01-06T17:10:22+00:00","MessageSeverity":"Informational","Message":"IRC is Opening Valve:1","MessageId":"Dell.0.1.LCR0006","MemberId":"0","MessageArgs":["1"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43332"}}],"Id":"128","Context":"SSE-Subscription-5"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43332","EventTimestamp":"2026-01-06T17:10:22+00:00","MessageSeverity":"Informational","Message":"IRC is Opening Valve:2","MessageId":"Dell.0.1.LCR0006","MemberId":"0","MessageArgs":["2"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43332"}}],"Id":"128","Context":"SSE-Subscription-5"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43333","EventTimestamp":"2026-01-06T17:10:37+00:00","MessageSeverity":"Informational","Message":"IRC has Opened Valve:1","MessageId":"Dell.0.1.LCR0001","MemberId":"0","MessageArgs":["1"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43333"}}],"Id":"132","Context":"SSE-Subscription-5"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43333","EventTimestamp":"2026-01-06T17:10:37+00:00","MessageSeverity":"Informational","Message":"IRC has Opened Valve:2","MessageId":"Dell.0.1.LCR0001","MemberId":"0","MessageArgs":["2"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43333"}}],"Id":"132","Context":"SSE-Subscription-5"}`),

	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43335","EventTimestamp":"2026-01-06T17:11:11+00:00","MessageSeverity":"Critical","Message":"The IRC detected a leak on External front leak sensor 1 in the rack with Service Tag 1234568.","MessageId":"Dell.0.1.LCPO009","MemberId":"0","MessageArgs":["IRC","External front leak sensor 1","1234568"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43335"}}],"Id":"141","Context":"SSE-Subscription-7"}`),

	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43336","EventTimestamp":"2026-01-06T17:11:11+00:00","MessageSeverity":"Informational","Message":"IRC is closing Valve:1","MessageId":"Dell.0.1.LCR0007","MemberId":"0","MessageArgs":["1"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43336"}}],"Id":"142","Context":"SSE-Subscription-7"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43330","EventTimestamp":"2026-01-06T17:09:47+00:00","MessageSeverity":"Informational","Message":"IRC is closing Valve:2","MessageId":"Dell.0.1.LCR0007","MemberId":"0","MessageArgs":["2"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43330"}}],"Id":"117","Context":"SSE-Subscription-3"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43337","EventTimestamp":"2026-01-06T17:11:26+00:00","MessageSeverity":"Informational","Message":"IRC has Closed Valve:1","MessageId":"Dell.0.1.LCR0002","MemberId":"0","MessageArgs":["1"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43337"}}],"Id":"147","Context":"SSE-Subscription-7"}`),
	json.RawMessage(`{"@odata.context":"/redfish/v1/$metadata#Event.Event","@odata.type":"#Event.v1_3_0.Event","Name":"Event Array","Events":[{"EventType":"Alert","EventId":"EventLog.43331","EventTimestamp":"2026-01-06T17:10:02+00:00","MessageSeverity":"Informational","Message":"IRC has Closed Valve:2","MessageId":"Dell.0.1.LCR0002","MemberId":"0","MessageArgs":["2"],"OriginOfCondition":{"@odata.id":"/redfish/v1/Managers/IRC/LogServices/EventLog/43331"}}],"Id":"122","Context":"SSE-Subscription-3"}`),
}

func startSampleRedfishBroadcaster(hub *Hub, enabled bool, interval time.Duration) {
	if !enabled {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		//idx := 0
		for range t.C {
			// valve opening events
			sendValveOpenEvents(hub)

			time.Sleep(10 * time.Second)
			// leak detected event
			payload := withCurrentEventTimestamp(samples[4])
			hub.Broadcast(SSEEvent{Data: payload})
		}
	}()
}

func sendValveOpenEvents(hub *Hub) {
	payload := withCurrentEventTimestamp(samples[0])
	hub.Broadcast(SSEEvent{Data: payload})
	payload = withCurrentEventTimestamp(samples[1])
	hub.Broadcast(SSEEvent{Data: payload})
	time.Sleep(10 * time.Second)
	payload = withCurrentEventTimestamp(samples[2])
	hub.Broadcast(SSEEvent{Data: payload})
	payload = withCurrentEventTimestamp(samples[3])
	hub.Broadcast(SSEEvent{Data: payload})
}

func sendValveCloseEvents(hub *Hub) {
	payload := withCurrentEventTimestamp(samples[5])
	hub.Broadcast(SSEEvent{Data: payload})
	payload = withCurrentEventTimestamp(samples[6])
	hub.Broadcast(SSEEvent{Data: payload})
	time.Sleep(10 * time.Second)
	payload = withCurrentEventTimestamp(samples[7])
	hub.Broadcast(SSEEvent{Data: payload})
	payload = withCurrentEventTimestamp(samples[8])
	hub.Broadcast(SSEEvent{Data: payload})
}

func withCurrentEventTimestamp(raw json.RawMessage) json.RawMessage {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}

	// Match the original examples' "+00:00" suffix.
	now := time.Now().UTC().Format("2006-01-02T15:04:05+00:00")

	events, ok := obj["Events"].([]interface{})
	if ok {
		for _, e := range events {
			em, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			em["EventTimestamp"] = now
		}
	}

	b, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return json.RawMessage(b)
}

func writeSSE(w *bufio.Writer, ev SSEEvent) error {
	if ev.Retry > 0 {
		if _, err := fmt.Fprintf(w, "retry: %d\n", ev.Retry); err != nil {
			return err
		}
	}
	if ev.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", ev.ID); err != nil {
			return err
		}
	}
	if ev.Event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Event); err != nil {
			return err
		}
	}
	data := strings.TrimRight(string(ev.Data), "\n")
	if data == "" {
		data = "{}"
	}
	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}
	return w.Flush()
}

func sseHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		headers := w.Header()
		headers.Set("Content-Type", "text/event-stream")
		headers.Set("Cache-Control", "no-cache")
		headers.Set("Connection", "keep-alive")
		headers.Set("X-Accel-Buffering", "no")

		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events := make(chan SSEEvent, 16)
		clientID := hub.AddClient(events)
		defer hub.RemoveClient(clientID)

		bw := bufio.NewWriterSize(w, 8*1024)

		ctx := r.Context()
		keepAlive := time.NewTicker(25 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-keepAlive.C:
				// Comment line for keepalive; safe even if client ignores it.
				_, _ = fmt.Fprint(bw, ": keepalive\n\n")
				_ = bw.Flush()
				flusher.Flush()
			case ev, ok := <-events:
				if !ok {
					return
				}
				if err := writeSSE(bw, ev); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

type publishRequest struct {
	ID    string      `json:"id,omitempty"`
	Event string      `json:"event,omitempty"`
	Retry *int        `json:"retry,omitempty"`
	Data  interface{} `json:"data"`
}

func publishHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req publishRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}

		b, err := json.Marshal(req.Data)
		if err != nil {
			http.Error(w, "invalid data", http.StatusBadRequest)
			return
		}

		ev := SSEEvent{ID: req.ID, Event: req.Event, Data: b}
		if req.Retry != nil {
			ev.Retry = *req.Retry
		}

		hub.Broadcast(ev)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func makeMux(hub *Hub, authCfg AuthConfig, wd *WatchdogState) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/redfish/v1", requireBasicAuth(authCfg, redfishRootHandler))
	mux.HandleFunc("/redfish/v1/EventService", requireBasicAuth(authCfg, redfishEventServiceHandler))
	mux.HandleFunc("/redfish/v1/Managers/IRC", requireBasicAuth(authCfg, redfishManagersIRCHandler))
	mux.HandleFunc("/redfish/v1/Managers/IRC/Oem/Dell/DellPolicies/SimplePolicy", requireBasicAuth(authCfg, patchSimplePolicyHandler(wd)))
	mux.HandleFunc("/redfish/v1/Managers/IRC/Actions/Oem/DellManager.EndWatchdog", requireBasicAuth(authCfg, endWatchdogHandler(wd, hub)))
	mux.HandleFunc("/redfish/v1/Managers/IRC/Actions/Oem/DellManager.PetWatchdog", requireBasicAuth(authCfg, petWatchdogHandler(wd)))
	mux.HandleFunc("/redfish/v1/SSE", requireBasicAuth(authCfg, sseHandler(hub)))
	mux.HandleFunc("/publish", requireBasicAuth(authCfg, publishHandler(hub)))
	return mux
}

func main() {
	var (
		port        = flag.Int("port", 8080, "Single port to listen on (ignored if -port-range is set)")
		portRange   = flag.String("port-range", "", "Port range to listen on, inclusive (e.g. 8080-8085)")
		tlsCertFile = flag.String("tls-cert", "", "Path to TLS certificate (PEM). If set with -tls-key, enables HTTPS")
		tlsKeyFile  = flag.String("tls-key", "", "Path to TLS private key (PEM). If set with -tls-cert, enables HTTPS")
		basicUser   = flag.String("basic-user", "", "Basic auth username (enable auth when set with -basic-pass)")
		basicPass   = flag.String("basic-pass", "", "Basic auth password (enable auth when set with -basic-user)")
		sampleOn    = flag.Bool("sample-redfish", true, "Enable periodic sample Redfish SSE events")
		sampleEvery = flag.Duration("sample-interval", 180*time.Second, "Interval for periodic sample Redfish SSE events")
	)
	flag.Parse()

	hub := NewHub()
	startSampleRedfishBroadcaster(hub, *sampleOn, *sampleEvery)

	authCfg := AuthConfig{Enabled: false}
	if strings.TrimSpace(*basicUser) != "" && *basicPass != "" {
		authCfg = AuthConfig{Enabled: true, Username: strings.TrimSpace(*basicUser), Password: *basicPass}
	}
	wd := &WatchdogState{WatchdogTimeout: 60}

	mux := makeMux(hub, authCfg, wd)
	certFile := strings.TrimSpace(*tlsCertFile)
	keyFile := strings.TrimSpace(*tlsKeyFile)
	tlsEnabled := certFile != "" && keyFile != ""

	start, end, ok := parsePortRangeString(*portRange)
	if !ok {
		p := *port
		if p <= 0 {
			p = 8080
		}
		start, end = p, p
	}

	errCh := make(chan error, end-start+1)
	for port := start; port <= end; port++ {
		addr := ":" + strconv.Itoa(port)
		srv := &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}

		log.Printf("listening on %s", addr)
		go func(s *http.Server) {
			var err error
			if tlsEnabled {
				err = s.ListenAndServeTLS(certFile, keyFile)
			} else {
				err = s.ListenAndServe()
			}
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}(srv)
	}

	go func() {
		if err := <-errCh; err != nil {
			log.Fatal(err)
		}
	}()

	select {}
}
