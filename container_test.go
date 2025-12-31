package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectCgroupVersion(t *testing.T) {
	info := DetectCgroupVersion()
	
	// We should be able to detect some cgroup version
	if info.Version == CgroupUnknown && info.Available {
		t.Error("Cgroup detected as available but version is unknown")
	}
	
	// If v2 is detected, controllers file should exist
	if info.Version == CgroupV2 {
		if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
			t.Error("Cgroup v2 detected but controllers file doesn't exist")
		}
	}
	
	// If v1 is detected, memory subsystem should exist
	if info.Version == CgroupV1 {
		if _, err := os.Stat("/sys/fs/cgroup/memory"); err != nil {
			t.Error("Cgroup v1 detected but memory subsystem doesn't exist")
		}
	}
	
	t.Logf("Cgroup info: Version=%d, Available=%v, MemorySupported=%v, CPUSupported=%v",
		info.Version, info.Available, info.MemorySupported, info.CPUSupported)
}

func TestSetupCgroupsWithDetection(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Skipping cgroup setup test (requires root)")
	}
	
	containerID := "test-container-cgroup"
	memoryLimit := int64(100 * 1024 * 1024) // 100MB
	
	// Clean up before test
	CleanupCgroup(containerID)
	defer CleanupCgroup(containerID)
	
	err := SetupCgroupsWithDetection(containerID, memoryLimit)
	if err != nil {
		t.Logf("Warning: Cgroup setup returned error (may be expected): %v", err)
	}
	
	// Test should not panic even if cgroups are not available
}

func TestSaveAndLoadContainerState(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	// Create test metadata
	testID := "test-container-123"
	createdAt := time.Now()
	startedAt := time.Now().Add(1 * time.Second)
	exitCode := 0
	
	metadata := ContainerMetadata{
		ID:         testID,
		State:      StateRunning,
		Image:      "test-image",
		Command:    "/bin/sh",
		Args:       []string{"-c", "echo test"},
		CreatedAt:  createdAt,
		StartedAt:  &startedAt,
		ExitCode:   &exitCode,
		RootfsPath: "/tmp/test/rootfs",
	}
	
	// Test save
	if err := SaveContainerState(metadata); err != nil {
		t.Fatalf("Failed to save container state: %v", err)
	}
	
	// Test load
	loaded, err := LoadContainerState(testID)
	if err != nil {
		t.Fatalf("Failed to load container state: %v", err)
	}
	
	// Verify loaded data
	if loaded.ID != testID {
		t.Errorf("Expected ID %s, got %s", testID, loaded.ID)
	}
	
	if loaded.State != StateRunning {
		t.Errorf("Expected state %s, got %s", StateRunning, loaded.State)
	}
	
	if loaded.Image != "test-image" {
		t.Errorf("Expected image %s, got %s", "test-image", loaded.Image)
	}
	
	if loaded.Command != "/bin/sh" {
		t.Errorf("Expected command %s, got %s", "/bin/sh", loaded.Command)
	}
}

func TestUpdateContainerState(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker-update")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	// Create initial metadata
	testID := "test-container-update"
	metadata := ContainerMetadata{
		ID:         testID,
		State:      StateCreated,
		Image:      "test-image",
		Command:    "/bin/echo",
		Args:       []string{"test"},
		CreatedAt:  time.Now(),
		RootfsPath: "/tmp/test/rootfs",
	}
	
	// Save initial state
	if err := SaveContainerState(metadata); err != nil {
		t.Fatalf("Failed to save initial state: %v", err)
	}
	
	// Update state to running
	err := UpdateContainerState(testID, func(m *ContainerMetadata) {
		m.State = StateRunning
		now := time.Now()
		m.StartedAt = &now
	})
	
	if err != nil {
		t.Fatalf("Failed to update container state: %v", err)
	}
	
	// Verify update
	updated, err := LoadContainerState(testID)
	if err != nil {
		t.Fatalf("Failed to load updated state: %v", err)
	}
	
	if updated.State != StateRunning {
		t.Errorf("Expected state %s after update, got %s", StateRunning, updated.State)
	}
	
	if updated.StartedAt == nil {
		t.Error("Expected StartedAt to be set after update")
	}
}

func TestListAllContainers(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker-list")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	// Create multiple containers
	containers := []ContainerMetadata{
		{
			ID:         "container-1",
			State:      StateRunning,
			Image:      "test-image-1",
			Command:    "/bin/sh",
			CreatedAt:  time.Now(),
			RootfsPath: "/tmp/test/rootfs1",
		},
		{
			ID:         "container-2",
			State:      StateExited,
			Image:      "test-image-2",
			Command:    "/bin/echo",
			CreatedAt:  time.Now(),
			RootfsPath: "/tmp/test/rootfs2",
		},
	}
	
	for _, c := range containers {
		if err := SaveContainerState(c); err != nil {
			t.Fatalf("Failed to save container %s: %v", c.ID, err)
		}
	}
	
	// List all containers
	listed, err := ListAllContainers()
	if err != nil {
		t.Fatalf("Failed to list containers: %v", err)
	}
	
	if len(listed) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(listed))
	}
	
	// Verify containers are in the list
	foundIDs := make(map[string]bool)
	for _, c := range listed {
		foundIDs[c.ID] = true
	}
	
	if !foundIDs["container-1"] || !foundIDs["container-2"] {
		t.Error("Not all containers were listed")
	}
}

func TestRemoveContainer(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker-remove")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	// Create a stopped container
	testID := "container-to-remove"
	metadata := ContainerMetadata{
		ID:         testID,
		State:      StateExited,
		Image:      "test-image",
		Command:    "/bin/echo",
		CreatedAt:  time.Now(),
		RootfsPath: "/tmp/test/rootfs",
	}
	
	if err := SaveContainerState(metadata); err != nil {
		t.Fatalf("Failed to save container: %v", err)
	}
	
	// Remove the container
	if err := RemoveContainer(testID); err != nil {
		t.Fatalf("Failed to remove container: %v", err)
	}
	
	// Verify it's gone
	containerDir := filepath.Join(baseDir, "containers", testID)
	if _, err := os.Stat(containerDir); !os.IsNotExist(err) {
		t.Error("Container directory still exists after removal")
	}
	
	// Verify we can't load it
	if _, err := LoadContainerState(testID); err == nil {
		t.Error("Should not be able to load removed container")
	}
}

func TestCannotRemoveRunningContainer(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker-remove-running")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	// Create a running container
	testID := "running-container"
	metadata := ContainerMetadata{
		ID:         testID,
		State:      StateRunning,
		Image:      "test-image",
		Command:    "/bin/sleep",
		CreatedAt:  time.Now(),
		RootfsPath: "/tmp/test/rootfs",
	}
	
	if err := SaveContainerState(metadata); err != nil {
		t.Fatalf("Failed to save container: %v", err)
	}
	
	// Try to remove the running container - should fail
	err := RemoveContainer(testID)
	if err == nil {
		t.Error("Should not be able to remove running container")
	}
	
	// Verify it still exists
	if _, err := LoadContainerState(testID); err != nil {
		t.Error("Running container was removed when it shouldn't be")
	}
}

func TestGetContainerLogs(t *testing.T) {
	// Create a temporary base directory for testing
	tempDir := filepath.Join(os.TempDir(), "test-basic-docker-logs")
	defer os.RemoveAll(tempDir)
	
	// Temporarily override baseDir for testing
	originalBaseDir := baseDir
	baseDir = tempDir
	defer func() { baseDir = originalBaseDir }()
	
	testID := "container-with-logs"
	containerDir := filepath.Join(baseDir, "containers", testID)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("Failed to create container directory: %v", err)
	}
	
	// Create a log file
	logFile := filepath.Join(containerDir, "stdout.log")
	testLogs := "Test log output\nLine 2\nLine 3\n"
	if err := os.WriteFile(logFile, []byte(testLogs), 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}
	
	// Get logs
	logs, err := GetContainerLogs(testID)
	if err != nil {
		t.Fatalf("Failed to get container logs: %v", err)
	}
	
	if logs != testLogs {
		t.Errorf("Expected logs:\n%s\nGot:\n%s", testLogs, logs)
	}
	
	// Test non-existent container logs
	noLogs, err := GetContainerLogs("non-existent")
	if err != nil {
		t.Fatalf("Getting logs for non-existent container should return empty, not error: %v", err)
	}
	
	if noLogs != "" {
		t.Error("Expected empty logs for non-existent container")
	}
}
