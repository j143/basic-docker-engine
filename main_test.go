package main

import (
	"os"
	"testing"
	"fmt"
	"path/filepath"
)

// Test Scenarios Documentation
//
// TestInitDirectories:
// - Verifies that the initDirectories function creates the required directories.
// - Setup: Ensures the directories do not exist before the test.
// - Expected Outcome: The directories should be created successfully.

func TestInitDirectories(t *testing.T) {
	// Setup: Remove directories if they exist
	baseDir := filepath.Join(os.TempDir(), "basic-docker")
	dirs := []string{
		filepath.Join(baseDir, "containers"),
		filepath.Join(baseDir, "images"),
		filepath.Join(baseDir, "layers"),
	}
	for _, dir := range dirs {
		os.RemoveAll(dir)
	}

	// Call initDirectories
	err := initDirectories()
	if err != nil {
		t.Fatalf("initDirectories failed: %v", err)
	}

	// Verify that the directories were created
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}
}

// TestListContainers:
// - Verifies that the listContainers function lists running containers correctly.
// - Setup: Creates mock container directories and PID files.
// - Expected Outcome: The output includes the container IDs and their statuses.

func TestListContainers(t *testing.T) {
	// Setup: Create mock container directories and PID files
	baseDir := filepath.Join(os.TempDir(), "basic-docker")
	containerDir := filepath.Join(baseDir, "containers")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("Failed to create container directory: %v", err)
	}
	defer os.RemoveAll(baseDir) // Cleanup

	containerID := "test-container"
	if err := os.MkdirAll(containerDir+"/"+containerID, 0755); err != nil {
		t.Fatalf("Failed to create mock container directory: %v", err)
	}
	pidFile := containerDir + "/" + containerID + "/pid"
	if err := os.WriteFile(pidFile, []byte("12345"), 0644); err != nil {
		t.Fatalf("Failed to create mock PID file: %v", err)
	}

	// Capture the output of listContainers
	output := captureOutput(listContainers)

	// Verify the output contains the container ID
	if !contains(output, containerID) {
		t.Errorf("Expected output to contain container ID '%s', but got: %s", containerID, output)
	}
}

// TestGetContainerStatus:
// - Verifies that the getContainerStatus function correctly identifies the status of a container.
// - Setup: Creates a mock container directory and PID file.
// - Expected Outcome: Returns "Running" if the process exists, otherwise "Stopped".

func TestGetContainerStatus(t *testing.T) {
	// Setup: Create a mock container directory and PID file
	baseDir := filepath.Join(os.TempDir(), "basic-docker")
	containerDir := filepath.Join(baseDir, "containers", "test-container")
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("Failed to create container directory: %v", err)
	}
	defer os.RemoveAll(baseDir) // Cleanup

	pidFile := containerDir + "/pid"
	pid := os.Getpid() // Use the current process PID for testing
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		t.Fatalf("Failed to create PID file: %v", err)
	}

	// Test: Check the status of the container
	status := getContainerStatus("test-container")
	if status != "Running" {
		t.Errorf("Expected status 'Running', but got '%s'", status)
	}

	// Cleanup: Remove the PID file to simulate a stopped container
	os.Remove(pidFile)

	// Test: Check the status again
	status = getContainerStatus("test-container")
	if status != "Stopped" {
		t.Errorf("Expected status 'Stopped', but got '%s'", status)
	}
}

// TestCapsuleManager:
// - Verifies the CapsuleManager's functionality, including adding, retrieving, and attaching Resource Capsules.
// - Setup: Initializes a CapsuleManager instance.
// - Expected Outcome: Capsules are added, retrieved, and attached correctly.

func TestCapsuleManager(t *testing.T) {
	cm := NewCapsuleManager()

	// Add a capsule
	cm.AddCapsule("libssl", "1.1.1", "/usr/lib/libssl.so")

	// Retrieve the capsule
	capsule, exists := cm.GetCapsule("libssl", "1.1.1")
	if !exists {
		t.Fatalf("Expected capsule libssl:1.1.1 to exist")
	}

	if capsule.Name != "libssl" || capsule.Version != "1.1.1" || capsule.Path != "/usr/lib/libssl.so" {
		t.Errorf("Capsule data mismatch: got %+v", capsule)
	}

	// Attach the capsule to a container
	err := cm.AttachCapsule("container-1234", "libssl", "1.1.1")
	if err != nil {
		t.Errorf("Failed to attach capsule: %v", err)
	}

	// Try to attach a non-existent capsule
	err = cm.AttachCapsule("container-1234", "libcrypto", "1.0.0")
	if err == nil {
		t.Errorf("Expected error when attaching non-existent capsule, got nil")
	}
}

// BenchmarkCapsuleAccess benchmarks the access time for Resource Capsules.
func BenchmarkCapsuleAccess(b *testing.B) {
	cm := NewCapsuleManager()
	cm.AddCapsule("libssl", "1.1.1", "/usr/lib/libssl.so")

	for i := 0; i < b.N; i++ {
		_, exists := cm.GetCapsule("libssl", "1.1.1")
		if !exists {
			b.Fatalf("Capsule not found")
		}
	}
}

// BenchmarkVolumeAccess benchmarks the access time for Docker volumes.
func BenchmarkVolumeAccess(b *testing.B) {
	volumePath := "/tmp/docker-volume/libssl.so"
	os.WriteFile(volumePath, []byte("dummy data"), 0644)
	defer os.Remove(volumePath)

	for i := 0; i < b.N; i++ {
		_, err := os.Stat(volumePath)
		if err != nil {
			b.Fatalf("Volume not found: %v", err)
		}
	}
}

// BenchmarkDynamicAttachment benchmarks the dynamic attachment of Resource Capsules.
func BenchmarkDynamicAttachment(b *testing.B) {
	cm := NewCapsuleManager()
	cm.AddCapsule("libssl", "1.1.1", "/usr/lib/libssl.so")

	for i := 0; i < b.N; i++ {
		err := cm.AttachCapsule("container-1234", "libssl", "1.1.1")
		if err != nil {
			b.Fatalf("Failed to attach capsule: %v", err)
		}
	}
}

// BenchmarkVolumeAttachment benchmarks the dynamic attachment of Docker volumes.
func BenchmarkVolumeAttachment(b *testing.B) {
	volumePath := "/tmp/docker-volume/libssl.so"
	// Ensure the directory exists before creating the file
	os.MkdirAll(filepath.Dir(volumePath), 0755)
	os.WriteFile(volumePath, []byte("dummy data"), 0644)
	defer os.Remove(volumePath)

	for i := 0; i < b.N; i++ {
		// Simulate volume attachment by checking its existence
		_, err := os.Stat(volumePath)
		if err != nil {
			b.Fatalf("Volume not found: %v", err)
		}
	}
}