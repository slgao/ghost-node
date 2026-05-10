package agent

import (
	"net"
	"os"
)

func getHostname() (string, error) {
	return os.Hostname()
}

// getOutboundIP returns the local IP used to reach the internet.
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
