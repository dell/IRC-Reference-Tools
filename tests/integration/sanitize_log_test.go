//go:build integration

// Package integration contains tests that run against a live docker-compose
// stack. They make real API calls, capture real container logs, and verify
// that sensitive data (passwords, tokens, secrets) is never logged in
// plaintext at any log level.
//
// Credential masking relies on the Stringer interface: Service.String() and
// ServiceItem.String() replace the Auth field with "*****".
//
// Prerequisites:
//
//	LOG_LEVEL=debug bash docker-compose-files/compose.sh up --build -d
//
// Run:
//
//	go test -tags=integration ./tests/integration -v
package integration

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const (
	baseURL = "https://localhost:443/api/v1"

	// Unique marker values — if any appear in plaintext in the logs the
	// sanitization is broken.
	markerPassIRC = "MARKER_ircpass_t3st_4829"
	markerPassRAC = "MARKER_racpass_t3st_7153"
	markerUserIRC = "ircuser_test"
	markerUserRAC = "racuser_test"
	markerIRCIP   = "192.168.254.201"
	markerRACIP   = "192.168.254.202:443"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// repoRoot returns the top-level repo directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine repo root")
	}
	// file is tests/integration/sanitize_log_test.go → go up two levels
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// httpClient returns an *http.Client that skips TLS verification (self-signed
// certs) and has a reasonable timeout.
func httpClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

// apiRequest makes an HTTP request with basic auth and returns the body.
func apiRequest(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	req.SetBasicAuth("admin", "admin")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	_, _ = fmt.Fprintf(buf, "")
	b := make([]byte, 1<<20)
	n, _ := resp.Body.Read(b)
	return resp.StatusCode, string(b[:n])
}

// composeLogs runs compose.sh logs --no-color and returns the combined output.
func composeLogs(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "docker-compose-files", "compose.sh"),
		"logs", "--no-color")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compose logs failed: %v\n%s", err, string(out))
	}
	return string(out)
}

// readEnvFile reads a KEY=VALUE file and returns a map.
func readEnvFile(path string) map[string]string {
	m := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Setup / teardown: add test devices, capture logs, clean up
// ---------------------------------------------------------------------------

// testState holds data shared across subtests in TestLogSanitization.
type testState struct {
	logs      string
	jwt       string
	envSecrets map[string]string // key → value from .env
}

func setup(t *testing.T) *testState {
	t.Helper()

	// -- Preflight: make sure the stack is reachable --
	status, _ := apiRequest(t, "GET", baseURL+"/irc", "")
	if status == 0 {
		t.Fatal("cannot reach configui — is the stack running?\n" +
			"  LOG_LEVEL=debug bash docker-compose-files/compose.sh up --build -d")
	}

	// -- Get a JWT token (also a sensitive value to check) --
	_, tokenBody := apiRequest(t, "POST", baseURL+"/GetToken",
		`{"username":"admin","password":"admin"}`)
	var tokenResp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal([]byte(tokenBody), &tokenResp)

	// -- Add IRC with marker password --
	apiRequest(t, "POST", baseURL+"/irc", fmt.Sprintf(
		`{"ip":"%s","username":"%s","password":"%s"}`,
		markerIRCIP, markerUserIRC, markerPassIRC))

	// -- Add rack device with marker password --
	apiRequest(t, "POST", baseURL+"/irc/"+markerIRCIP+"/racdevice", fmt.Sprintf(
		`{"ip":"%s","username":"%s","password":"%s","systemtypedesc":"idrac","isforcefulshutdown":true}`,
		markerRACIP, markerUserRAC, markerPassRAC))

	// Let log lines flush
	time.Sleep(3 * time.Second)

	// -- Capture logs from all containers --
	logs := composeLogs(t)

	// -- Read .env secrets --
	envFile := filepath.Join(repoRoot(t), "docker-compose-files", ".env")
	envSecrets := readEnvFile(envFile)

	return &testState{
		logs:       logs,
		jwt:        tokenResp.Token,
		envSecrets: envSecrets,
	}
}

func cleanup(t *testing.T) {
	t.Helper()
	// Remove rack device
	apiRequest(t, "DELETE",
		baseURL+"/irc/"+markerIRCIP+"/racdevice",
		fmt.Sprintf(`{"ips":["%s"]}`, markerRACIP))
	// Remove IRC
	apiRequest(t, "DELETE",
		baseURL+"/irc",
		fmt.Sprintf(`{"ips":["%s"]}`, markerIRCIP))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestLogSanitization(t *testing.T) {
	state := setup(t)
	t.Cleanup(func() { cleanup(t) })

	t.Run("irc_password_not_in_logs", func(t *testing.T) {
		if strings.Contains(state.logs, markerPassIRC) {
			lines := findLines(state.logs, markerPassIRC)
			t.Errorf("IRC password marker found in plaintext in logs:\n%s", lines)
		}
	})

	t.Run("racdevice_password_not_in_logs", func(t *testing.T) {
		if strings.Contains(state.logs, markerPassRAC) {
			lines := findLines(state.logs, markerPassRAC)
			t.Errorf("Rack device password marker found in plaintext in logs:\n%s", lines)
		}
	})

	t.Run("auth_masked_in_logs", func(t *testing.T) {
		// Service.String() and ServiceItem.String() replace credentials
		// with "*****". Verify this masking appears in the logs.
		if !strings.Contains(state.logs, "Auth: *****") {
			t.Error("Auth: ***** not found in logs — Stringer masking may not be active.\n" +
				"Hint: restart with LOG_LEVEL=debug to ensure Info-level logs are emitted.")
		}
	})

	t.Run("jwt_token_not_in_logs", func(t *testing.T) {
		if state.jwt == "" {
			t.Skip("no JWT token obtained, skipping")
		}
		if strings.Contains(state.logs, state.jwt) {
			t.Error("JWT token found in plaintext in logs")
		}
	})

	t.Run("mysql_password_not_in_logs", func(t *testing.T) {
		pw, ok := state.envSecrets["MYSQL_PASSWORD"]
		if !ok || pw == "" {
			t.Skip("MYSQL_PASSWORD not found in .env, skipping")
		}
		if strings.Contains(state.logs, pw) {
			t.Error("MYSQL_PASSWORD found in plaintext in logs")
		}
	})

	t.Run("jwt_secret_not_in_logs", func(t *testing.T) {
		secret, ok := state.envSecrets["CONFIGUI_JWT_SECRET"]
		if !ok || secret == "" {
			t.Skip("CONFIGUI_JWT_SECRET not found in .env, skipping")
		}
		if strings.Contains(state.logs, secret) {
			t.Error("CONFIGUI_JWT_SECRET found in plaintext in logs")
		}
	})
}

// findLines returns up to 5 lines from logs that contain needle.
func findLines(logs, needle string) string {
	var matches []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, needle) {
			matches = append(matches, "  "+line)
			if len(matches) >= 5 {
				break
			}
		}
	}
	return strings.Join(matches, "\n")
}
