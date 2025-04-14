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