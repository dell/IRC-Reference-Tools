package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/systemtype"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/messagebus"
)

// shutdowners is a map to enable calling different actions based on the
// type of serviceItems
var shutdowners map[string]Shutdowner

// systemTypeRegistry is the local cache of system type configs
var systemTypeRegistry *systemtype.Registry

func init() {
	shutdowners = make(map[string]Shutdowner)
}

// createShutdownersFromConfigs creates shutdowners from loaded configs and registers system types
func createShutdownersFromConfigs(authClient auth.AuthClientInterface) {
	for _, cfg := range systemTypeRegistry.GetAll() {
		// Create shutdowner from config if it has shutdown config
		shutdowner := shutdownerFromConfig(cfg)
		if shutdowner != nil {
			shutdowners[cfg.Name] = shutdowner
			logging.Info("Loaded shutdowner", "name", cfg.Name)
		}

		// Skip registering parent system types (e.g., IRC) in DB
		if cfg.IsParent {
			logging.Info("Skipping DB registration for parent system type", "name", cfg.Name)
			continue
		}

		// Register system type in DB (for UI dropdown)
		authClient.AddSystemType(cfg.Name)
		logging.Info("Registered system type", "name", cfg.Name)
	}
}

func loadActions(authClient auth.AuthClientInterface) {
	// Load system type configs
	logging.Info("Loading system type configs")
	var err error
	systemTypeRegistry, err = systemtype.LoadConfigs()
	if err != nil {
		logging.Warn("Warning: failed to load system type configs", "err", err)
	}

	// Create shutdowners from loaded configs
	createShutdownersFromConfigs(authClient)

	logging.Info("Loaded system types", "count", len(shutdowners), "names", getShutdownerNames())
}

func getShutdownerNames() []string {
	names := make([]string, 0, len(shutdowners))
	for name := range shutdowners {
		names = append(names, name)
	}
	return names
}

func handleDataGroup(group *databus.DataGroup, authClient auth.AuthClientInterface) {
	hostID := group.HostID
	if hostID == "" {
		logging.Warn("HostID is empty, using FQDN", "fqdn", group.FQDN)
		hostID = group.FQDN
	}
	for _, event := range group.Events {
		go handleEvent(event, hostID, authClient)
		logging.Debug("Event Received", "hostID", hostID, "event", event)
	}
}

func main() {
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	mb := messagebus.InitializeStompMB()
	defer mb.Close()
	authClient := auth.NewAuthClientWithBus(mb, "action")
	loadActions(authClient)
	dbClient := databus.NewDataBusClientWithBus(mb, "action")
	dbClient.Subscribe("/action")
	dbClient.Get("/action")
	groupsIn := databus.NewDataGroupChan(10)

	go dbClient.GetGroup(groupsIn, "/action")
	for {
		select {
		case group, ok := <-groupsIn:
			if !ok {
				logging.Error("Groups channel closed, shutting down")
				return
			}
			handleDataGroup(group, authClient)
		case sig := <-sigChan:
			logging.Info("Received signal, shutting down gracefully", "signal", sig)
			return
		}
	}
}
