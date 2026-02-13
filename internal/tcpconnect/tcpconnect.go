package tcpconnect

import (
	"fmt"
	"net"
	"time"
)

var defaultPorts = []int{22, 80, 443}

// TryTCPConnect attempts a TCP connection to host:port with a timeout.
func TryTCPConnect(ip string, port int, timeout time.Duration) error {
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// IsHostAccessible returns true if any of the given ports accept a TCP connection.
func IsHostAccessible(ip string, ports []int, timeout time.Duration) bool {
	if len(ports) == 0 {
		ports = defaultPorts
	}
	for _, p := range ports {
		if err := TryTCPConnect(ip, p, timeout); err == nil {
			return true
		}
	}
	return false
}
