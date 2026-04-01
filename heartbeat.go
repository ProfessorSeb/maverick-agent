package main

import (
	"net"
	"os"
	"runtime"
	"time"
)

// HeartbeatPayload matches the server-side protocol definition.
type HeartbeatPayload struct {
	Hostname       string `json:"hostname"`
	IPAddress      string `json:"ip_address"`
	OSInfo         string `json:"os_info"`
	Arch           string `json:"arch"`
	AgentVersion   string `json:"agent_version"`
	CloudProvider  string `json:"cloud_provider"`
	Region         string `json:"region"`
	GeoLocation    string `json:"geo_location"`
	CPUCores       int    `json:"cpu_cores"`
	MemoryMB       int    `json:"memory_mb"`
	ActiveBackends int    `json:"active_backends"`
	RequestCount   int64  `json:"request_count"`
	ErrorCount     int64  `json:"error_count"`
	AvgLatencyMs   int    `json:"avg_latency_ms"`
	Uptime         int64  `json:"uptime_seconds"`
}

var startTime = time.Now()

// CollectHeartbeat gathers system information for a heartbeat message.
func CollectHeartbeat(proc *ProcessManager) HeartbeatPayload {
	hostname, _ := os.Hostname()
	ip := getOutboundIP()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return HeartbeatPayload{
		Hostname:     hostname,
		IPAddress:    ip,
		OSInfo:       runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: version,
		CPUCores:     runtime.NumCPU(),
		MemoryMB:     int(memStats.Sys / (1024 * 1024)),
		Uptime:       int64(time.Since(startTime).Seconds()),
	}
}

// getOutboundIP returns the preferred outbound IP of this machine.
func getOutboundIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 2*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}
