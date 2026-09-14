package cli

import (
	"fmt"
	"net"
	"strconv"
)

// ValidateListenAddr checks an HTTP listen address given as host:port. The
// host is required: an empty host binds all interfaces, which triggers firewall
// prompts on Windows. The port must be 1-65535.
func ValidateListenAddr(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("invalid address %q: host is required, e.g. localhost:8000", addr)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("invalid address %q: port: %w", addr, err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid address %q: port %d out of range 1-65535", addr, port)
	}
	return nil
}
