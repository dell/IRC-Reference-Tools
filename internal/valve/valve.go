package valve

import (
	"encoding/json"
	"fmt"

	"github.com/dell/IRC-Reference-Tools/internal/httpclient"
	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

var valveSensorPaths = []string{
	"/redfish/v1/Chassis/Rack/Sensors/Valve1",
	"/redfish/v1/Chassis/Rack/Sensors/Valve2",
	// "/redfish/v1/Chassis/eRDHx/Sensors/ValveInlet",
}

func GetValveStatus(service auth.Service) (string, string, error) {
	var valveReadings []int
	valve_status := make(map[string][]int)

	for _, path := range valveSensorPaths {
		resp, err := httpclient.Get(
			"https://"+service.Ip+path,
			nil,
			service.Auth["username"],
			service.Auth["password"],
		)
		if err != nil {
			logging.Error("error getting valve status", "response", resp)
		}
		reading, err := get_reading(path, resp)
		if err != nil {
			logging.Error("error getting reading status", "err", err)
		}
		valveReadings = append(valveReadings, reading)
	}
	valve_status[service.Ip] = valveReadings
	for k, v := range valve_status {
		fmt.Println(k, v)
	}
	if valveReadings[0] == -1 && valveReadings[1] == -1 {
		logging.Warn("no valve readings found")
	}

	vstatus1, vstatus2 := calculate_status(valveReadings[0], valveReadings[1])
	return vstatus1, vstatus2, nil
}

func calculate_status(ValveReading1 int, ValveReading2 int) (string, string) {
	status1 := get_valve_status(ValveReading1)
	status2 := get_valve_status(ValveReading2)
	return status1, status2
}

func get_valve_status(reading int) string {
	if reading == -1 {
		return "Unknown"
	}
	if reading >= 0 && reading <= 5 {
		return "Closed"
	}
	if reading >= 95 && reading <= 100 {
		return "Open"
	}
	return "Unknown"
}
func get_reading(path string, resp string) (int, error) {
	var valve_response map[string]interface{}
	err := json.Unmarshal([]byte(resp), &valve_response)
	if err != nil {
		return -1, fmt.Errorf("json unmarshal error: %w", err)
	}

	reading, ok := valve_response["Reading"]
	if !ok {
		return -1, fmt.Errorf("reading field not found %v", path)
	}

	readingValue, ok := reading.(float64)
	if !ok {
		return -1, fmt.Errorf("reading field is not a number")
	}

	return int(readingValue), nil
}
