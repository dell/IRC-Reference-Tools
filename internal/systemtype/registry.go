package systemtype

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"gopkg.in/yaml.v3"
)

// Registry holds all registered system type configurations
type Registry struct {
	configs map[string]*SystemTypeConfig
	mu      sync.RWMutex
}

// NewRegistry creates a new empty registry
func NewRegistry() *Registry {
	return &Registry{
		configs: make(map[string]*SystemTypeConfig),
	}
}

// Register adds a config to the registry
func (r *Registry) Register(cfg *SystemTypeConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[cfg.Name] = cfg
}

// Get retrieves a config by name, returns nil if not found
func (r *Registry) Get(name string) *SystemTypeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.configs[name]
}

// GetAll returns all registered configs
func (r *Registry) GetAll() []*SystemTypeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*SystemTypeConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		result = append(result, cfg)
	}
	return result
}

// GetNames returns all registered config names
func (r *Registry) GetNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.configs))
	for name := range r.configs {
		names = append(names, name)
	}
	return names
}

// LoadFromFile loads a single YAML config file and registers it
func (r *Registry) LoadFromFile(path string) (*SystemTypeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg SystemTypeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Validate mandatory fields
	if err := validateConfig(&cfg, path); err != nil {
		return nil, err
	}

	// Apply defaults
	applyDefaults(&cfg)

	r.Register(&cfg)
	return &cfg, nil
}

// validatePowerState validates the PowerStateConfig configuration
func validatePowerState(cfg *SystemTypeConfig, path string) error {
	if cfg.PowerState == nil {
		return fmt.Errorf("config file %s missing required 'powerState' field", path)
	}

	// Validate PowerStateConfig
	switch cfg.PowerState.MonitorType {
	case MonitorTypeRedfishSSE:
		if cfg.PowerState.URI == "" || cfg.PowerState.Path == "" {
			return fmt.Errorf("config file %s: uri and path required when monitorType is '%s'", path, MonitorTypeRedfishSSE)
		}
		// IRC uses SSE strictly for leak/valve alerts, so power transition message IDs are optional.
		if strings.EqualFold(cfg.Name, "irc") {
			break
		}
		if cfg.PowerState.PowerOnMsgId == "" || cfg.PowerState.PowerOffMsgId == "" {
			return fmt.Errorf("config file %s: powerOnMsgId and powerOffMsgId required when monitorType is '%s'", path, MonitorTypeRedfishSSE)
		}
		// Validate message args format if provided
		if cfg.PowerState.PowerOnMessageArgs != "" {
			if err := validateMessageArgsFormat(cfg.PowerState.PowerOnMessageArgs); err != nil {
				return fmt.Errorf("config file %s: invalid powerOnMessageArgs: %v", path, err)
			}
		}
		if cfg.PowerState.PowerOffMessageArgs != "" {
			if err := validateMessageArgsFormat(cfg.PowerState.PowerOffMessageArgs); err != nil {
				return fmt.Errorf("config file %s: invalid powerOffMessageArgs: %v", path, err)
			}
		}
	case MonitorTypeRedfishGet:
		if cfg.PowerState.URI == "" || cfg.PowerState.Path == "" {
			return fmt.Errorf("config file %s: uri and path required when monitorType is '%s'", path, MonitorTypeRedfishGet)
		}
	case MonitorTypeTCPPing:
		// No additional validation needed for tcp-ping
	case "":
		// Default to tcp-ping if not specified
		cfg.PowerState.MonitorType = MonitorTypeTCPPing
	default:
		return fmt.Errorf("config file %s: invalid monitorType '%s', must be '%s', '%s', or '%s'", path, cfg.PowerState.MonitorType, MonitorTypeRedfishSSE, MonitorTypeRedfishGet, MonitorTypeTCPPing)
	}

	return nil
}

// validateMessageArgsFormat validates that message args are in the format "index-value"
func validateMessageArgsFormat(messageArgs string) error {
	parts := strings.Split(messageArgs, "-")
	if len(parts) != 2 {
		return fmt.Errorf("messageArgs must be in format 'index-value'")
	}

	// Validate that index is a number
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return fmt.Errorf("messageArgs index must be a number")
	}

	return nil
}

// validateConfig checks that mandatory fields are present
func validateConfig(cfg *SystemTypeConfig, path string) error {
	if cfg.Name == "" {
		return fmt.Errorf("config file %s missing required 'name' field", path)
	}
	if cfg.SystemID == nil {
		return fmt.Errorf("config file %s missing required 'systemId' field", path)
	}
	// powerState and shutdown are only required for non-parent configs
	if !cfg.IsParent {
		// Validate power state configuration
		if err := validatePowerState(cfg, path); err != nil {
			return err
		}
		if cfg.Shutdown == nil {
			return fmt.Errorf("config file %s missing required 'shutdown' field", path)
		}
		if cfg.HasValves || cfg.HasWatchdog {
			return fmt.Errorf("config file %s has valves or watchdog enabled, but these features are not implemented for this system type", path)
		}
	}
	return nil
}

// applyDefaults sets default values for optional fields
func applyDefaults(cfg *SystemTypeConfig) {
	if cfg.Shutdown != nil {
		if cfg.Shutdown.Graceful != nil && cfg.Shutdown.Graceful.Protocol == "" {
			cfg.Shutdown.Graceful.Protocol = "https"
		}
		if cfg.Shutdown.Force != nil && cfg.Shutdown.Force.Protocol == "" {
			cfg.Shutdown.Force.Protocol = "https"
		}
	}
}

// LoadFromDirectory loads all YAML config files from a directory
func (r *Registry) LoadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if _, err := r.LoadFromFile(path); err != nil {
			return err
		}
	}

	return nil
}

// LoadFromDirectories loads configs from multiple directories (e.g., default + custom)
func (r *Registry) LoadFromDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue // Skip non-existent directories
		}
		if err := r.LoadFromDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

const (
	// ConfigBaseDir is the base path for system type configs in containers
	ConfigBaseDir = "/configs/systemtypes"
	// DefaultConfigSubdir is the subdirectory for built-in configs
	DefaultConfigSubdir = "default"
	// CustomConfigSubdir is the subdirectory for user-provided configs
	CustomConfigSubdir = "custom"
	// InternalConfigSubdir is the subdirectory for internal configs (e.g., IRC with isParent: true)
	InternalConfigSubdir = "internal"
)

// LoadConfigs loads system type configurations from the default, custom, and internal subdirectories.
// Returns a new Registry populated with configs. Each service should store this in its own cache.
func LoadConfigs() (*Registry, error) {
	registry := NewRegistry()

	// Load from default subdirectory (built-in configs)
	defaultDir := filepath.Join(ConfigBaseDir, DefaultConfigSubdir)
	if err := registry.LoadFromDirectory(defaultDir); err != nil {
		logging.Warn("failed to load default configs", "dir", defaultDir, "err", err)
	}

	// Load from custom subdirectory (user-provided configs)
	customDir := filepath.Join(ConfigBaseDir, CustomConfigSubdir)
	if err := registry.LoadFromDirectory(customDir); err != nil {
		logging.Warn("failed to load custom configs", "dir", customDir, "err", err)
	}

	// Load from internal subdirectory (internal configs like IRC)
	internalDir := filepath.Join(ConfigBaseDir, InternalConfigSubdir)
	if err := registry.LoadFromDirectory(internalDir); err != nil {
		logging.Warn("failed to load internal configs", "dir", internalDir, "err", err)
	}

	logging.Info("Loaded system type configs", "count", len(registry.GetNames()), "names", registry.GetNames())
	return registry, nil
}
