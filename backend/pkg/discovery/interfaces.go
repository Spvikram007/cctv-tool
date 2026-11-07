package discovery

import (
	"fmt"
	"net"
	"strings"
)

// NetworkInterface represents a local network interface available for discovery
type NetworkInterface struct {
	Index             int      `json:"index"`
	Name              string   `json:"name"`
	HardwareAddr      string   `json:"hardware_addr"`
	IPv4Addresses     []string `json:"ipv4_addresses"`
	IPv6Addresses     []string `json:"ipv6_addresses"`
	Subnets           []string `json:"subnets"` // CIDRs, e.g. "192.168.1.0/24"
	IsUp              bool     `json:"is_up"`
	IsLoopback        bool     `json:"is_loopback"`
	SupportsMulticast bool     `json:"supports_multicast"`
}

// GetAvailableInterfaces enumerates all network interfaces and returns suitable ones for CCTV discovery
func GetAvailableInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate network interfaces: %w", err)
	}

	result := make([]NetworkInterface, 0, len(ifaces))

	for _, iface := range ifaces {
		// Only consider interfaces that are UP
		isUp := (iface.Flags & net.FlagUp) != 0
		isLoopback := (iface.Flags & net.FlagLoopback) != 0
		supportsMulticast := (iface.Flags & net.FlagMulticast) != 0

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		ipv4s := make([]string, 0)
		ipv6s := make([]string, 0)
		subnets := make([]string, 0)

		for _, addr := range addrs {
			var ip net.IP
			var mask net.IPMask

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
				mask = v.Mask
			case *net.IPAddr:
				ip = v.IP
				mask = ip.DefaultMask()
			}

			if ip == nil {
				continue
			}

			if ip4 := ip.To4(); ip4 != nil {
				ipv4s = append(ipv4s, ip4.String())
				if mask != nil {
					network := ip4.Mask(mask)
					ones, bits := mask.Size()
					if bits == 32 && ones > 0 {
						subnets = append(subnets, fmt.Sprintf("%s/%d", network.String(), ones))
					}
				}
			} else if ip.To16() != nil {
				ipv6s = append(ipv6s, ip.String())
			}
		}

		result = append(result, NetworkInterface{
			Index:             iface.Index,
			Name:              iface.Name,
			HardwareAddr:      iface.HardwareAddr.String(),
			IPv4Addresses:     ipv4s,
			IPv6Addresses:     ipv6s,
			Subnets:           subnets,
			IsUp:              isUp,
			IsLoopback:        isLoopback,
			SupportsMulticast: supportsMulticast,
		})
	}

	return result, nil
}

// FindInterfaceByName locates an interface by name
func FindInterfaceByName(name string) (*net.Interface, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	return net.InterfaceByName(name)
}

