package watchdog

import (
	"github.com/dell/IRC-Reference-Tools/internal/httpclient"
	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

const (
	DEFAULTWATCHDOGTIMEOUT = 60

	simplePolicyURI = "/redfish/v1/Managers/IRC/Oem/Dell/DellPolicies/SimplePolicy"
	endWatchdogURI  = "/redfish/v1/Managers/IRC/Actions/Oem/DellManager.EndWatchdog"
	petWatchdogURI  = "/redfish/v1/Managers/IRC/Actions/Oem/DellManager.PetWatchdog"
)

// SetWatchdogTimeout sets the watchdog timeout on the Service. if timeout is set to 0,
// default of 60 seconds is used
func SetWatchdogTimeout(service auth.Service, timeout int) error {
	if timeout == 0 {
		timeout = DEFAULTWATCHDOGTIMEOUT
	}
	logging.Info("Setting watchdog timeout", "timeout", timeout, "ip", service.Ip)
	resp, err := httpclient.Patch(
		"https://"+service.Ip+simplePolicyURI,
		map[string]interface{}{"WatchdogTimeout": timeout, "Policy": "CloseValves"},
		nil,
		service.Auth["username"],
		service.Auth["password"],
	)
	if err != nil {
		logging.Error("error setting watchdog", "err", err)
		return err
	}
	logging.Debug("Response", "response", resp)
	return nil
}

func EndWatchdog(service auth.Service) error {
	logging.Info("Ending watchdog", "ip", service.Ip)
	resp, err := httpclient.Post(
		"https://"+service.Ip+endWatchdogURI,
		map[string]interface{}{"Continue": true},
		nil,
		service.Auth["username"],
		service.Auth["password"],
	)
	if err != nil {
		logging.Error("error ending watchdog", "err", err)
		return err
	}
	logging.Debug("Response", "response", resp)
	return nil
}

func PetWatchdog(service auth.Service) error {
	logging.Info("Petting watchdog", "ip", service.Ip)
	resp, err := httpclient.Post(
		"https://"+service.Ip+petWatchdogURI,
		map[string]interface{}{},
		nil,
		service.Auth["username"],
		service.Auth["password"],
	)
	if err != nil {
		logging.Error("error petting watchdog", "err", err)
		return err
	}
	logging.Debug("Response", "response", resp)
	return nil
}
