package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/loginauth"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/messagebus"
	"github.com/gin-gonic/gin"
)

var configStrings = map[string]string{
	"httpport": "8082",
}

func getEnvSettings() {
	httpPort := os.Getenv("CONFIGUI_HTTP_PORT")
	if len(httpPort) > 0 {
		configStrings["httpport"] = httpPort
	}
}

type IRC struct {
	IP          string `json:"ip"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	State       string `json:"state"` // e.g., "connected", "failed"
	Valve1State string `json:"valve1state"`
	Valve2State string `json:"valve2state"`
}

type Systems struct {
	IP                 string `json:"ip"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	Type               int    `json:"type"`
	Systemtypedesc     string `json:"systemtypedesc"`
	State              string `json:"state"`
	IsForcefulshutdown bool   `json:"isforcefulshutdown"`
	Timeout            int    `json:"timeout"`
}

type MyDelSys struct {
	IP []string `json:"ips"`
}
type Service = auth.Service

func getIRCList(c *gin.Context, s *config.SystemHandler) {
	// s.AuthClient.ResendAll()
	services := s.AuthClient.GetAllServices()
	valvestatus := s.AuthClient.GetValveStatus()

	var ircList []map[string]interface{}
	for _, service := range services {
		irc := map[string]interface{}{
			"ip":          service.Ip,
			"state":       service.State,
			"valve1state": "",
			"valve2state": "",
		}

		// Match the IP with the valvestatus array
		for _, valve := range valvestatus {
			if valve.Ip == service.Ip {
				irc["valve1state"] = valve.VState1
				irc["valve2state"] = valve.VState2
			}
		}

		ircList = append(ircList, irc)
	}
	systemTypes := s.AuthClient.GetAllSystemTypes()
	logging.Info("IRC SYSTEM", "ircList", ircList, "systemTypes", systemTypes, "valvestatus", valvestatus)
	c.JSON(200, gin.H{"status": "Fetched IRC list", "data": ircList, "systemTypes": systemTypes})
}

func deleteSystem(c *gin.Context, s *config.SystemHandler) {
	var tmp MyDelSys
	err := c.ShouldBind(&tmp)
	logging.Info("Deleting Systems", "systems", tmp)
	if err != nil {
		logging.Error("Failed to parse json", "err", err)
		_ = c.AbortWithError(500, err)
		return
	}

	failed := make(map[string]string)
	for i := 0; i < len(tmp.IP); i++ {
		var service auth.Service
		service.Ip = tmp.IP[i]
		if err := s.AuthClient.DeleteService(service); err != nil {
			failed[service.Ip] = err.Error()
		}
	}

	if len(failed) > 0 {
		c.JSON(207, gin.H{"success": "partial", "failed": failed})
	} else {
		c.JSON(200, gin.H{"success": "true"})
	}
}

func deleteAssociatedSystem(c *gin.Context, s *config.SystemHandler) {
	var tmp MyDelSys
	err := c.ShouldBind(&tmp)
	if err != nil {
		logging.Error("Failed to parse json", "err", err)
		_ = c.AbortWithError(500, err)
		return
	}

	failed := make(map[string]string)
	for i := 0; i < len(tmp.IP); i++ {
		var serviceitem auth.ServiceItem
		serviceitem.Service.Ip = tmp.IP[i]
		if err := s.AuthClient.DeleteServiceItem(serviceitem); err != nil {
			failed[serviceitem.Service.Ip] = err.Error()
		}
	}

	if len(failed) > 0 {
		c.JSON(207, gin.H{"success": "partial", "failed": failed})
	} else {
		logging.Info("Deleted Associated System Successfully")
		c.JSON(200, gin.H{"success": "true"})
	}
}
func getAssociatedSystems(c *gin.Context, s *config.SystemHandler) {
	ircIP := c.Param("ip")
	logging.Info("Fetching associated systems for IRC IP", "ip", ircIP)

	serviceItems := s.AuthClient.GetServiceItems(ircIP)
	logging.Info("Displaying service items", "serviceItems", serviceItems)

	// Remove credentials from response
	serviceItemsMap := make([]map[string]interface{}, len(serviceItems))
	for i, item := range serviceItems {
		cleanedItem := map[string]interface{}{
			"ip":                 item.Service.Ip,
			"state":              item.State,
			"serviceIp":          item.ServiceIP,
			"systemtypedesc":     item.Systemtypedesc,
			"isforcefulshutdown": item.IsForcefulshutdown,
			"timeout":            item.Timeout,
		}
		serviceItemsMap[i] = cleanedItem
	}

	c.JSON(200, gin.H{
		"status": "Fetched associated systems",
		"data":   serviceItemsMap,
	})
}

func addService(c *gin.Context, s *config.SystemHandler) {
	logging.Info("Adding IRC")
	var irc IRC
	if err := c.ShouldBindJSON(&irc); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	service := auth.Service{
		ServiceType: auth.SYSTEM_TYPE_IRC,
		Ip:          irc.IP,
		AuthType:    auth.AuthTypeUsernamePassword,
		Auth:        map[string]string{"username": irc.Username, "password": irc.Password},
	}
	if err := s.AuthClient.AddService(service); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "IRC added"})
}

func AddAssociatedSystem(c *gin.Context, s *config.SystemHandler) {
	var system Systems
	if err := c.ShouldBindJSON(&system); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	logging.Info("Adding Associated System")
	logging.Info("System details", "system", system)

	// Get IRC IP from URL
	ircIP := c.Param("ip")
	system.Type = 0

	servicewithip := auth.ServiceItem{
		Service: auth.Service{
			Ip:          system.IP, // Associated system's own IP
			AuthType:    auth.AuthTypeUsernamePassword,
			ServiceType: system.Type,
			Auth:        map[string]string{"username": system.Username, "password": system.Password},
		},
		ServiceIP:          ircIP,
		Systemtypedesc:     system.Systemtypedesc,
		IsForcefulshutdown: system.IsForcefulshutdown, // Default shutdown type
		Timeout:            system.Timeout,            // Default timeout in seconds
		// IRC IP (parent service)
	}
	logging.Info("Adding the service item", "serviceItem", servicewithip)
	if err := s.AuthClient.AddServiceItem(servicewithip); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "System added"})
}

// -------------------- Main --------------------

func main() {
	getEnvSettings()
	loginauth.InitJWT()

	mb := messagebus.InitializeStompMB()
	defer mb.Close()

	// Add signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	systemHandler := config.NewSystemHandler(
		auth.NewAuthClientWithBus(mb, "configui"),
		databus.NewDataBusClientWithBus(mb, "configui"),
		config.NewConfigClientWithBus(mb),
	)

	r := gin.Default()
	r.Static("/static", "./frontend/build/static")
	r.StaticFile("/", "./frontend/build/index.html")

	// Public auth
	r.POST("/api/v1/GetToken", loginauth.AdminLogin(systemHandler))
	r.POST("/api/v1/logout", loginauth.Logout())

	// Protected group
	pr := r.Group("/")
	pr.Use(loginauth.AuthRequiredOrBasic(systemHandler))
	{
		pr.GET("/api/v1/me", loginauth.Me())

		// Your existing APIs now protected:
		pr.POST("/api/v1/irc", func(c *gin.Context) { addService(c, systemHandler) })
		pr.GET("/api/v1/irc", func(c *gin.Context) { getIRCList(c, systemHandler) })
		pr.DELETE("/api/v1/irc", func(c *gin.Context) { deleteSystem(c, systemHandler) })

		pr.POST("/api/v1/irc/:ip/racdevice", func(c *gin.Context) { AddAssociatedSystem(c, systemHandler) })
		pr.GET("/api/v1/irc/:ip/racdevice", func(c *gin.Context) { getAssociatedSystems(c, systemHandler) })
		pr.DELETE("/api/v1/irc/:ip/racdevice", func(c *gin.Context) { deleteAssociatedSystem(c, systemHandler) })

		// Password ops:
		pr.POST("/api/v1/ChangePassword", loginauth.ChangePassword(systemHandler))
		pr.POST("/api/v1/ResetToDefault", loginauth.ResetToDefault(systemHandler))
	}

	port := configStrings["httpport"]
	certFile := "/extrabin/certs/localhost.crt"
	keyFile := "/extrabin/certs/localhost.key"

	// CAUTION: InsecureSkipVerify is enabled because this system only supports self-signed certificates
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	// Start server in goroutine
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			logging.Error("Failed to run secure webserver: %v", err)
		}
	}()

	// Wait for signal
	<-sigChan
	logging.Info("Received signal, shutting down")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logging.Error("Server forced to shutdown: %v", err)
	}
}
