package mock

import (
	"context"
	"sync"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
)

type MockAuthorizationClient struct {
	Mu sync.Mutex

	Services     []auth.Service
	ServiceItems []auth.ServiceItem
	SystemTypes  map[string]string
}

func (mac *MockAuthorizationClient) AddService(service auth.Service) error                         { return nil }
func (mac *MockAuthorizationClient) AddServiceItem(si auth.ServiceItem) error                      { return nil }
func (mac *MockAuthorizationClient) DeleteService(service auth.Service) error                      { return nil }
func (mac *MockAuthorizationClient) DeleteServiceItem(si auth.ServiceItem) error                   { return nil }
func (mac *MockAuthorizationClient) GetHECConfig()                                                 {}
func (mac *MockAuthorizationClient) GetService(ctx context.Context, services chan<- *auth.Service) {}
func (mac *MockAuthorizationClient) GetAllServices() []auth.Service {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	return mac.Services
}
func (mac *MockAuthorizationClient) GetServiceItems(sip string) []auth.ServiceItem {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	sItems := []auth.ServiceItem{}
	for _, s := range mac.ServiceItems {
		if s.ServiceIP == sip {
			sItems = append(sItems, s)
		}
	}
	return sItems
}

func (mac *MockAuthorizationClient) GetServiceItemWithIP(siip string) auth.ServiceItem {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	for i := len(mac.ServiceItems) - 1; i >= 0; i-- {
		si := mac.ServiceItems[i]
		if si.Ip == siip {
			return si
		}
	}
	return auth.ServiceItem{}
}
func (mac *MockAuthorizationClient) GetServiceWithIP(ip string) auth.Service {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	for i := len(mac.Services) - 1; i >= 0; i-- {
		s := mac.Services[i]
		if s.Ip == ip {
			return s
		}
	}
	return auth.Service{}
}
func (mac *MockAuthorizationClient) ReadOneMessage(queue string, v any) error        { return nil }
func (mac *MockAuthorizationClient) ResendAll()                                      {}
func (mac *MockAuthorizationClient) SendCommand(command auth.Command) error          { return nil }
func (mac *MockAuthorizationClient) SendCommandString(command string)                {}
func (mac *MockAuthorizationClient) SplunkAddHEC(SplunkHttp auth.SplunkConfig) error { return nil }
func (mac *MockAuthorizationClient) UpdateService(s auth.Service) error {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	mac.Services = append(mac.Services, s)
	return nil
}
func (mac *MockAuthorizationClient) UpdateServiceItem(si auth.ServiceItem) error {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()
	mac.ServiceItems = append(mac.ServiceItems, si)
	return nil
}
func (mac *MockAuthorizationClient) UpdateServiceItemState(state string, siip string) error {
	return nil
}
func (mac *MockAuthorizationClient) UpdateServiceState(state string, sip string) error { return nil }
func (mac *MockAuthorizationClient) AddSystemType(sysType string) error {
	mac.SystemTypes[sysType] = sysType
	return nil
}
func (mac *MockAuthorizationClient) UpdateValveState(sip, v1, v2 string) error {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()

	var state string
	if v1 != "" {
		state = v1
	} else if v2 != "" {
		state = v2
	}

	// Update valve state in Services
	for i := range mac.Services {
		if mac.Services[i].Ip == sip {
			mac.Services[i].State = state
			break
		}
	}

	// Update valve state in ServiceItems
	for i := range mac.ServiceItems {
		if mac.ServiceItems[i].Ip == sip {
			mac.ServiceItems[i].State = state
			mac.ServiceItems[i].Service.State = state
			break
		}
	}

	return nil
}
func (mac *MockAuthorizationClient) GetAllSystemTypes() []auth.SystemType { return nil }
func (mac *MockAuthorizationClient) UpdateServiceX(s auth.Service) error  { return nil }
func (mac *MockAuthorizationClient) UpdateServiceXState(state string, ip string) error {
	mac.Mu.Lock()
	defer mac.Mu.Unlock()

	// Update Services
	for i := range mac.Services {
		if mac.Services[i].Ip == ip {
			mac.Services[i].State = state
			break
		}
	}
	if len(mac.Services) == 0 || mac.Services[len(mac.Services)-1].Ip != ip {
		mac.Services = append(mac.Services, auth.Service{Ip: ip, State: state})
	}

	// Update ServiceItems
	for i := range mac.ServiceItems {
		if mac.ServiceItems[i].Ip == ip {
			mac.ServiceItems[i].State = state
			mac.ServiceItems[i].Service.State = state
			return nil
		}
	}
	mac.ServiceItems = append(mac.ServiceItems, auth.ServiceItem{Service: auth.Service{Ip: ip, State: state}})
	return nil
}
func (mac *MockAuthorizationClient) UpdateLogin(l auth.Login) error { return nil }
func (mac *MockAuthorizationClient) GetLogin() auth.Login           { return auth.Login{} }
