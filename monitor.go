package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// MonitoringLevel represents the different levels of monitoring
type MonitoringLevel string

const (
	ProcessLevel   MonitoringLevel = "process"
	ContainerLevel MonitoringLevel = "container" 
	HostLevel      MonitoringLevel = "host"
)

// Monitor represents the main monitoring interface
type Monitor interface {
	GetMetrics() (interface{}, error)
	GetLevel() MonitoringLevel
}

// ProcessMetrics represents process-level monitoring data
type ProcessMetrics struct {
	PID          int    `json:"pid"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	MemoryVmRSS  int64  `json:"memory_vm_rss"`  // Resident Set Size
	MemoryVmSize int64  `json:"memory_vm_size"` // Virtual Memory Size
	CPUTime      int64  `json:"cpu_time"`
	CPUPercent   float64 `json:"cpu_percent"`
	OpenFiles    int    `json:"open_files"`
	Threads      int    `json:"threads"`
	StartTime    int64  `json:"start_time"`
	Socket       string `json:"socket"` // Network socket info
}

// ContainerMetrics represents container-level monitoring data  
type ContainerMetrics struct {
	ContainerID      string         `json:"container_id"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	Image            string         `json:"image"`
	Created          time.Time      `json:"created"`
	StartedAt        time.Time      `json:"started_at"`
	MemoryUsage      int64          `json:"memory_usage"`
	MemoryLimit      int64          `json:"memory_limit"`
	CPUUsage         float64        `json:"cpu_usage"`
	NetworkRx        int64          `json:"network_rx"`
	NetworkTx        int64          `json:"network_tx"`
	BlockRead        int64          `json:"block_read"`
	BlockWrite       int64          `json:"block_write"`
	PIDNamespace     string         `json:"pid_namespace"`
	NetworkNamespace string         `json:"network_namespace"`
	VethInterfaces   []string       `json:"veth_interfaces"` // veth* interfaces
	Processes        []ProcessMetrics `json:"processes"`
	DockerPath       string         `json:"docker_path"` // /var/lib/docker path
}

// HostMetrics represents host-level monitoring data
type HostMetrics struct {
	Hostname         string          `json:"hostname"`
	Uptime           time.Duration   `json:"uptime"`
	LoadAverage      []float64       `json:"load_average"`
	MemoryTotal      int64           `json:"memory_total"`
	MemoryAvailable  int64           `json:"memory_available"`
	MemoryUsed       int64           `json:"memory_used"`
	CPUCount         int             `json:"cpu_count"`
	CPUUsage         []float64       `json:"cpu_usage"`
	DiskTotal        int64           `json:"disk_total"`
	DiskUsed         int64           `json:"disk_used"`
	DiskAvailable    int64           `json:"disk_available"`
	NetworkInterfaces []NetworkInterface `json:"network_interfaces"` // eth* interfaces
	Containers       []ContainerMetrics  `json:"containers"`
	RuntimeContext   string          `json:"runtime_context"` // data center context
	KernelVersion    string          `json:"kernel_version"`
	OSRelease        string          `json:"os_release"`
}

// NetworkInterface represents a network interface
type NetworkInterface struct {
	Name      string `json:"name"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	RxPackets int64  `json:"rx_packets"`
	TxPackets int64  `json:"tx_packets"`
}

// ProcessMonitor implements monitoring at the process level
type ProcessMonitor struct {
	pid int
}

// ContainerMonitor implements monitoring at the container level
type ContainerMonitor struct {
	containerID string
}

// HostMonitor implements monitoring at the host level
type HostMonitor struct{}

// NewProcessMonitor creates a new process monitor
func NewProcessMonitor(pid int) *ProcessMonitor {
	return &ProcessMonitor{pid: pid}
}

// NewContainerMonitor creates a new container monitor
func NewContainerMonitor(containerID string) *ContainerMonitor {
	return &ContainerMonitor{containerID: containerID}
}

// NewHostMonitor creates a new host monitor
func NewHostMonitor() *HostMonitor {
	return &HostMonitor{}
}

// GetLevel returns the monitoring level for ProcessMonitor
func (pm *ProcessMonitor) GetLevel() MonitoringLevel {
	return ProcessLevel
}

// GetLevel returns the monitoring level for ContainerMonitor  
func (cm *ContainerMonitor) GetLevel() MonitoringLevel {
	return ContainerLevel
}

// GetLevel returns the monitoring level for HostMonitor
func (hm *HostMonitor) GetLevel() MonitoringLevel {
	return HostLevel
}

// GetMetrics collects process-level metrics
func (pm *ProcessMonitor) GetMetrics() (interface{}, error) {
	metrics := ProcessMetrics{PID: pm.pid}
	
	// Read from /proc/[pid]/stat
	statFile := fmt.Sprintf("/proc/%d/stat", pm.pid)
	statContent, err := os.ReadFile(statFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read stat file: %v", err)
	}
	
	statFields := strings.Fields(string(statContent))
	if len(statFields) >= 24 {
		// Process name (remove parentheses)
		metrics.Name = strings.Trim(statFields[1], "()")
		
		// Process status
		metrics.Status = statFields[2]
		
		// CPU time (user + sys)
		utime, _ := strconv.ParseInt(statFields[13], 10, 64)
		stime, _ := strconv.ParseInt(statFields[14], 10, 64)
		metrics.CPUTime = utime + stime
		
		// Start time
		starttime, _ := strconv.ParseInt(statFields[21], 10, 64)
		metrics.StartTime = starttime
		
		// Number of threads
		metrics.Threads, _ = strconv.Atoi(statFields[19])
	}
	
	// Read memory info from /proc/[pid]/status
	statusFile := fmt.Sprintf("/proc/%d/status", pm.pid)
	statusContent, err := os.ReadFile(statusFile)
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(statusContent)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						metrics.MemoryVmRSS = val * 1024 // Convert from KB to bytes
					}
				}
			} else if strings.HasPrefix(line, "VmSize:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						metrics.MemoryVmSize = val * 1024 // Convert from KB to bytes
					}
				}
			}
		}
	}
	
	// Count open file descriptors
	fdDir := fmt.Sprintf("/proc/%d/fd", pm.pid)
	if entries, err := os.ReadDir(fdDir); err == nil {
		metrics.OpenFiles = len(entries)
	}
	
	// Get socket information (simplified)
	metrics.Socket = fmt.Sprintf("process-%d-socket", pm.pid)
	
	return metrics, nil
}

// GetMetrics collects container-level metrics
func (cm *ContainerMonitor) GetMetrics() (interface{}, error) {
	metrics := ContainerMetrics{
		ContainerID: cm.containerID,
		VethInterfaces: []string{},
		Processes: []ProcessMetrics{},
	}
	
	// Container directory path
	containerDir := filepath.Join(baseDir, "containers", cm.containerID)
	
	// Check if container exists
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("container %s not found", cm.containerID)
	}
	
	// Basic container info
	metrics.Name = cm.containerID
	metrics.Status = getContainerStatus(cm.containerID)
	metrics.DockerPath = containerDir
	
	// Get creation time from directory
	if info, err := os.Stat(containerDir); err == nil {
		metrics.Created = info.ModTime()
	}
	
	// Read PID file if exists
	pidFile := filepath.Join(containerDir, "pid")
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			// Get process metrics for the main container process
			pm := NewProcessMonitor(pid)
			if processMetrics, err := pm.GetMetrics(); err == nil {
				if pm, ok := processMetrics.(ProcessMetrics); ok {
					metrics.Processes = append(metrics.Processes, pm)
				}
			}
			
			// Get namespace information
			metrics.PIDNamespace = fmt.Sprintf("/proc/%d/ns/pid", pid)
			metrics.NetworkNamespace = fmt.Sprintf("/proc/%d/ns/net", pid)
		}
	}
	
	// Mock some network and resource stats (in a real implementation, 
	// these would come from cgroups and network interfaces)
	metrics.NetworkRx = 1024 * 100  // Mock 100KB received
	metrics.NetworkTx = 1024 * 50   // Mock 50KB transmitted
	metrics.MemoryUsage = 1024 * 1024 * 10  // Mock 10MB usage
	metrics.MemoryLimit = 1024 * 1024 * 100 // Mock 100MB limit
	
	// Look for veth interfaces (simplified simulation)
	metrics.VethInterfaces = append(metrics.VethInterfaces, fmt.Sprintf("veth%s", cm.containerID[:8]))
	
	return metrics, nil
}

// GetMetrics collects host-level metrics
func (hm *HostMonitor) GetMetrics() (interface{}, error) {
	metrics := HostMetrics{
		NetworkInterfaces: []NetworkInterface{},
		Containers: []ContainerMetrics{},
		RuntimeContext: "data center", // As per the table specification
	}
	
	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		metrics.Hostname = hostname
	}
	
	// Get system info
	metrics.CPUCount = runtime.NumCPU()
	
	// Get kernel version
	if kernelData, err := os.ReadFile("/proc/version"); err == nil {
		metrics.KernelVersion = strings.TrimSpace(string(kernelData))
	}
	
	// Get OS release
	if releaseData, err := os.ReadFile("/etc/os-release"); err == nil {
		metrics.OSRelease = strings.TrimSpace(string(releaseData))
	}
	
	// Get uptime
	if uptimeData, err := os.ReadFile("/proc/uptime"); err == nil {
		uptimeFields := strings.Fields(string(uptimeData))
		if len(uptimeFields) > 0 {
			if uptimeSeconds, err := strconv.ParseFloat(uptimeFields[0], 64); err == nil {
				metrics.Uptime = time.Duration(uptimeSeconds) * time.Second
			}
		}
	}
	
	// Get load average
	if loadData, err := os.ReadFile("/proc/loadavg"); err == nil {
		loadFields := strings.Fields(string(loadData))
		if len(loadFields) >= 3 {
			for i := 0; i < 3; i++ {
				if load, err := strconv.ParseFloat(loadFields[i], 64); err == nil {
					metrics.LoadAverage = append(metrics.LoadAverage, load)
				}
			}
		}
	}
	
	// Get memory info
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(memData)))
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, _ := strconv.ParseInt(fields[1], 10, 64)
				value *= 1024 // Convert from KB to bytes
				
				switch {
				case strings.HasPrefix(line, "MemTotal:"):
					metrics.MemoryTotal = value
				case strings.HasPrefix(line, "MemAvailable:"):
					metrics.MemoryAvailable = value
				}
			}
		}
		metrics.MemoryUsed = metrics.MemoryTotal - metrics.MemoryAvailable
	}
	
	// Get disk usage for root filesystem
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		metrics.DiskTotal = int64(stat.Blocks) * int64(stat.Bsize)
		metrics.DiskAvailable = int64(stat.Bavail) * int64(stat.Bsize)
		metrics.DiskUsed = metrics.DiskTotal - metrics.DiskAvailable
	}
	
	// Get network interfaces (eth* interfaces as per table)
	if err := filepath.WalkDir("/sys/class/net", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on error
		}
		
		if d.IsDir() && strings.HasPrefix(d.Name(), "eth") {
			iface := NetworkInterface{Name: d.Name()}
			
			// Read RX bytes
			if rxData, err := os.ReadFile(filepath.Join(path, "statistics/rx_bytes")); err == nil {
				if val, err := strconv.ParseInt(strings.TrimSpace(string(rxData)), 10, 64); err == nil {
					iface.RxBytes = val
				}
			}
			
			// Read TX bytes
			if txData, err := os.ReadFile(filepath.Join(path, "statistics/tx_bytes")); err == nil {
				if val, err := strconv.ParseInt(strings.TrimSpace(string(txData)), 10, 64); err == nil {
					iface.TxBytes = val
				}
			}
			
			// Read RX packets
			if rxData, err := os.ReadFile(filepath.Join(path, "statistics/rx_packets")); err == nil {
				if val, err := strconv.ParseInt(strings.TrimSpace(string(rxData)), 10, 64); err == nil {
					iface.RxPackets = val
				}
			}
			
			// Read TX packets
			if txData, err := os.ReadFile(filepath.Join(path, "statistics/tx_packets")); err == nil {
				if val, err := strconv.ParseInt(strings.TrimSpace(string(txData)), 10, 64); err == nil {
					iface.TxPackets = val
				}
			}
			
			metrics.NetworkInterfaces = append(metrics.NetworkInterfaces, iface)
		}
		return nil
	}); err != nil {
		// If we can't read network interfaces, add a mock eth0
		metrics.NetworkInterfaces = append(metrics.NetworkInterfaces, NetworkInterface{
			Name: "eth0", RxBytes: 1024 * 1024, TxBytes: 1024 * 512,
		})
	}
	
	// Get all container metrics
	containerDir := filepath.Join(baseDir, "containers")
	if entries, err := os.ReadDir(containerDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				cm := NewContainerMonitor(entry.Name())
				if containerMetrics, err := cm.GetMetrics(); err == nil {
					if cm, ok := containerMetrics.(ContainerMetrics); ok {
						metrics.Containers = append(metrics.Containers, cm)
					}
				}
			}
		}
	}
	
	return metrics, nil
}

// MonitoringAggregator aggregates metrics from all monitoring levels
type MonitoringAggregator struct {
	monitors []Monitor
}

// NewMonitoringAggregator creates a new monitoring aggregator
func NewMonitoringAggregator() *MonitoringAggregator {
	return &MonitoringAggregator{
		monitors: []Monitor{},
	}
}

// AddMonitor adds a monitor to the aggregator
func (ma *MonitoringAggregator) AddMonitor(monitor Monitor) {
	ma.monitors = append(ma.monitors, monitor)
}

// GetAllMetrics gets metrics from all monitoring levels
func (ma *MonitoringAggregator) GetAllMetrics() (map[MonitoringLevel]interface{}, error) {
	result := make(map[MonitoringLevel]interface{})
	
	for _, monitor := range ma.monitors {
		metrics, err := monitor.GetMetrics()
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics from %s monitor: %v", monitor.GetLevel(), err)
		}
		result[monitor.GetLevel()] = metrics
	}
	
	return result, nil
}

// GetFormattedMetrics returns metrics in a formatted JSON string
func (ma *MonitoringAggregator) GetFormattedMetrics() (string, error) {
	metrics, err := ma.GetAllMetrics()
	if err != nil {
		return "", err
	}
	
	jsonData, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal metrics: %v", err)
	}
	
	return string(jsonData), nil
}

// MonitoringGap represents the gap analysis between monitoring levels
type MonitoringGap struct {
	ProcessToContainer []string `json:"process_to_container"`
	ContainerToHost    []string `json:"container_to_host"`
	CrossLevel         []string `json:"cross_level"`
}

// AnalyzeMonitoringGap analyzes the gaps in monitoring coverage
func AnalyzeMonitoringGap(metrics map[MonitoringLevel]interface{}) MonitoringGap {
	gap := MonitoringGap{
		ProcessToContainer: []string{},
		ContainerToHost:    []string{},
		CrossLevel:         []string{},
	}
	
	// Analyze process to container gaps
	gap.ProcessToContainer = append(gap.ProcessToContainer, 
		"PID mapping to container ID correlation",
		"Process namespace isolation visibility",
		"Container resource limit enforcement on processes",
	)
	
	// Analyze container to host gaps  
	gap.ContainerToHost = append(gap.ContainerToHost,
		"Container network isolation vs host network visibility", 
		"Container filesystem overlay vs host filesystem access",
		"Container resource usage vs host resource allocation",
	)
	
	// Analyze cross-level monitoring gaps
	gap.CrossLevel = append(gap.CrossLevel,
		"End-to-end transaction tracing across isolation boundaries",
		"Performance correlation between process, container, and host metrics",
		"Security event correlation across all monitoring levels",
	)
	
	return gap
}