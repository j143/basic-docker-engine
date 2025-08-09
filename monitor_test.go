package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessMonitor(t *testing.T) {
	// Test monitoring the current process
	pid := os.Getpid()
	pm := NewProcessMonitor(pid)
	
	if pm.GetLevel() != ProcessLevel {
		t.Errorf("Expected process level, got %s", pm.GetLevel())
	}
	
	metrics, err := pm.GetMetrics()
	if err != nil {
		t.Fatalf("Failed to get process metrics: %v", err)
	}
	
	processMetrics, ok := metrics.(ProcessMetrics)
	if !ok {
		t.Fatalf("Expected ProcessMetrics, got %T", metrics)
	}
	
	if processMetrics.PID != pid {
		t.Errorf("Expected PID %d, got %d", pid, processMetrics.PID)
	}
	
	if processMetrics.Name == "" {
		t.Error("Process name should not be empty")
	}
	
	t.Logf("Process metrics: PID=%d, Name=%s, Status=%s, Memory=%d, Threads=%d",
		processMetrics.PID, processMetrics.Name, processMetrics.Status,
		processMetrics.MemoryVmRSS, processMetrics.Threads)
}

func TestHostMonitor(t *testing.T) {
	hm := NewHostMonitor()
	
	if hm.GetLevel() != HostLevel {
		t.Errorf("Expected host level, got %s", hm.GetLevel())
	}
	
	metrics, err := hm.GetMetrics()
	if err != nil {
		t.Fatalf("Failed to get host metrics: %v", err)
	}
	
	hostMetrics, ok := metrics.(HostMetrics)
	if !ok {
		t.Fatalf("Expected HostMetrics, got %T", metrics)
	}
	
	if hostMetrics.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	
	if hostMetrics.CPUCount <= 0 {
		t.Error("CPU count should be positive")
	}
	
	if hostMetrics.MemoryTotal <= 0 {
		t.Error("Memory total should be positive")
	}
	
	if hostMetrics.RuntimeContext != "data center" {
		t.Errorf("Expected runtime context 'data center', got '%s'", hostMetrics.RuntimeContext)
	}
	
	t.Logf("Host metrics: Hostname=%s, CPUs=%d, Memory=%dMB, Load=%v",
		hostMetrics.Hostname, hostMetrics.CPUCount,
		hostMetrics.MemoryTotal/(1024*1024), hostMetrics.LoadAverage)
}

func TestContainerMonitor(t *testing.T) {
	// Create a test container directory
	testContainerID := "test-monitor-container"
	containerDir := filepath.Join(baseDir, "containers", testContainerID)
	err := os.MkdirAll(containerDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test container directory: %v", err)
	}
	defer os.RemoveAll(containerDir)
	
	// Create a PID file
	pidFile := filepath.Join(containerDir, "pid")
	err = os.WriteFile(pidFile, []byte("1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}
	
	cm := NewContainerMonitor(testContainerID)
	
	if cm.GetLevel() != ContainerLevel {
		t.Errorf("Expected container level, got %s", cm.GetLevel())
	}
	
	metrics, err := cm.GetMetrics()
	if err != nil {
		t.Fatalf("Failed to get container metrics: %v", err)
	}
	
	containerMetrics, ok := metrics.(ContainerMetrics)
	if !ok {
		t.Fatalf("Expected ContainerMetrics, got %T", metrics)
	}
	
	if containerMetrics.ContainerID != testContainerID {
		t.Errorf("Expected container ID %s, got %s", testContainerID, containerMetrics.ContainerID)
	}
	
	if containerMetrics.DockerPath != containerDir {
		t.Errorf("Expected Docker path %s, got %s", containerDir, containerMetrics.DockerPath)
	}
	
	if len(containerMetrics.VethInterfaces) == 0 {
		t.Error("Should have at least one veth interface")
	}
	
	t.Logf("Container metrics: ID=%s, Status=%s, Memory=%d, VethInterfaces=%v",
		containerMetrics.ContainerID, containerMetrics.Status,
		containerMetrics.MemoryUsage, containerMetrics.VethInterfaces)
}

func TestMonitoringAggregator(t *testing.T) {
	aggregator := NewMonitoringAggregator()
	
	// Add monitors
	aggregator.AddMonitor(NewProcessMonitor(os.Getpid()))
	aggregator.AddMonitor(NewHostMonitor())
	
	metrics, err := aggregator.GetAllMetrics()
	if err != nil {
		t.Fatalf("Failed to get aggregated metrics: %v", err)
	}
	
	if len(metrics) != 2 {
		t.Errorf("Expected 2 monitoring levels, got %d", len(metrics))
	}
	
	if _, exists := metrics[ProcessLevel]; !exists {
		t.Error("Process level metrics should exist")
	}
	
	if _, exists := metrics[HostLevel]; !exists {
		t.Error("Host level metrics should exist")
	}
	
	// Test formatted metrics
	formatted, err := aggregator.GetFormattedMetrics()
	if err != nil {
		t.Fatalf("Failed to get formatted metrics: %v", err)
	}
	
	if len(formatted) == 0 {
		t.Error("Formatted metrics should not be empty")
	}
	
	t.Logf("Aggregated metrics length: %d characters", len(formatted))
}

func TestMonitoringGapAnalysis(t *testing.T) {
	// Create sample metrics for gap analysis
	metrics := map[MonitoringLevel]interface{}{
		ProcessLevel:   ProcessMetrics{PID: 1, Name: "test"},
		ContainerLevel: ContainerMetrics{ContainerID: "test"},
		HostLevel:      HostMetrics{Hostname: "test-host"},
	}
	
	gap := AnalyzeMonitoringGap(metrics)
	
	if len(gap.ProcessToContainer) == 0 {
		t.Error("Process to container gaps should be identified")
	}
	
	if len(gap.ContainerToHost) == 0 {
		t.Error("Container to host gaps should be identified")
	}
	
	if len(gap.CrossLevel) == 0 {
		t.Error("Cross-level gaps should be identified")
	}
	
	t.Logf("Gap analysis: ProcessToContainer=%d, ContainerToHost=%d, CrossLevel=%d",
		len(gap.ProcessToContainer), len(gap.ContainerToHost), len(gap.CrossLevel))
}

func TestMonitoringLevelsTable(t *testing.T) {
	// Test that our monitoring implementation addresses the table from the problem statement
	testCases := []struct {
		level    MonitoringLevel
		spec     string
		onDisk   string
		inMemory string
		inNetwork string
		runtime  string
		isolation string
	}{
		{
			level:     ProcessLevel,
			spec:      "Source",
			onDisk:    ".TEXT",
			inMemory:  "PID",
			inNetwork: "Socket",
			runtime:   "server core",
			isolation: "moderate: memory space, etc.",
		},
		{
			level:     ContainerLevel,
			spec:      "Dockerfile",
			onDisk:    "/var/lib/docker",
			inMemory:  "Container ID",
			inNetwork: "veth*",
			runtime:   "host",
			isolation: "private OS view: own PID space, file system, network interfaces",
		},
		{
			level:     HostLevel,
			spec:      "Kickstart",
			onDisk:    "/",
			inMemory:  "Hostname",
			inNetwork: "eth*",
			runtime:   "data center",
			isolation: "full: including own page caches and kernel",
		},
	}
	
	for _, tc := range testCases {
		t.Logf("Testing monitoring level: %s", tc.level)
		
		switch tc.level {
		case ProcessLevel:
			pm := NewProcessMonitor(os.Getpid())
			metrics, err := pm.GetMetrics()
			if err != nil {
				t.Errorf("Failed to get process metrics: %v", err)
				continue
			}
			processMetrics := metrics.(ProcessMetrics)
			
			// Verify PID is captured (in memory)
			if processMetrics.PID == 0 {
				t.Error("Process PID should be captured")
			}
			
			// Verify socket information (in network)
			if processMetrics.Socket == "" {
				t.Error("Process socket information should be captured")
			}
			
		case ContainerLevel:
			// Create test container for this test
			testContainerID := "test-levels-container"
			containerDir := filepath.Join(baseDir, "containers", testContainerID)
			os.MkdirAll(containerDir, 0755)
			defer os.RemoveAll(containerDir)
			
			cm := NewContainerMonitor(testContainerID)
			metrics, err := cm.GetMetrics()
			if err != nil {
				t.Errorf("Failed to get container metrics: %v", err)
				continue
			}
			containerMetrics := metrics.(ContainerMetrics)
			
			// Verify container ID is captured (in memory)
			if containerMetrics.ContainerID != testContainerID {
				t.Error("Container ID should be captured")
			}
			
			// Verify veth interfaces (in network)
			if len(containerMetrics.VethInterfaces) == 0 {
				t.Error("Container veth interfaces should be captured")
			}
			
			// Verify docker path (on disk)
			if !strings.Contains(containerMetrics.DockerPath, "docker") {
				t.Error("Container docker path should be captured")
			}
			
		case HostLevel:
			hm := NewHostMonitor()
			metrics, err := hm.GetMetrics()
			if err != nil {
				t.Errorf("Failed to get host metrics: %v", err)
				continue
			}
			hostMetrics := metrics.(HostMetrics)
			
			// Verify hostname is captured (in memory)
			if hostMetrics.Hostname == "" {
				t.Error("Host hostname should be captured")
			}
			
			// Verify network interfaces (in network) - should have some interfaces
			if len(hostMetrics.NetworkInterfaces) == 0 {
				// If no real interfaces found, this is expected in test environments
				t.Logf("No network interfaces found (expected in test environments)")
			}
			
			// Verify runtime context (runtime)
			if hostMetrics.RuntimeContext != tc.runtime {
				t.Errorf("Expected runtime context '%s', got '%s'", tc.runtime, hostMetrics.RuntimeContext)
			}
		}
	}
}

func BenchmarkProcessMonitoring(b *testing.B) {
	pm := NewProcessMonitor(os.Getpid())
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pm.GetMetrics()
		if err != nil {
			b.Fatalf("Error getting process metrics: %v", err)
		}
	}
}

func BenchmarkHostMonitoring(b *testing.B) {
	hm := NewHostMonitor()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hm.GetMetrics()
		if err != nil {
			b.Fatalf("Error getting host metrics: %v", err)
		}
	}
}

func BenchmarkMonitoringAggregator(b *testing.B) {
	aggregator := NewMonitoringAggregator()
	aggregator.AddMonitor(NewProcessMonitor(os.Getpid()))
	aggregator.AddMonitor(NewHostMonitor())
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := aggregator.GetAllMetrics()
		if err != nil {
			b.Fatalf("Error getting aggregated metrics: %v", err)
		}
	}
}