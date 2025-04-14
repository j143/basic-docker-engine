package main

import (
	"os"
	"testing"
	"fmt"
)

// Test Scenarios Documentation
//
// TestInitDirectories:
// - Verifies that the initDirectories function creates the required directories.
// - Setup: Ensures the directories do not exist before the test.
// - Expected Outcome: The directories should be created successfully.

func TestInitDirectories(t *testing.T) {
	// Setup: Remove directories if they exist
	dirs := []string{
		"/tmp/basic-docker/containers",
		"/tmp/basic-docker/images",
		"/tmp/basic-docker/layers",
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

// TestRun:
// - Verifies that the run function initializes a container correctly.
// - Setup: Creates a mock image and ensures the required directories exist.
// - Expected Outcome: A container directory is created with the correct structure.

func TestRun(t *testing.T) {
	// Setup: Create required directories and a mock image
	imageDir := "/tmp/basic-docker/images/test-image"
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("Failed to create mock image directory: %v", err)
	}
	defer os.RemoveAll("/tmp/basic-docker") // Cleanup

	// Create a mock executable file for the image
	mockExecutable := imageDir + "/test-image"
	if err := os.WriteFile(mockExecutable, []byte("#!/bin/sh\necho Hello"), 0755); err != nil {
		t.Fatalf("Failed to create mock executable: %v", err)
	}

	// Append the mock image directory to the PATH
	os.Setenv("PATH", os.Getenv("PATH")+":"+imageDir)

	// Verify that the mock image directory is in the PATH
	path := os.Getenv("PATH")
	if !contains(path, imageDir) {
		t.Fatalf("Mock image directory not found in PATH: %s", path)
	}

	// Set the TEST_ENV environment variable to true for testing
	os.Setenv("TEST_ENV", "true")

	// Mock command-line arguments
	os.Args = []string{"basic-docker", "run", "test-image", "sh"}

	// Call the run function
	run()

	// Verify that the container directory was created
	containerDir := "/tmp/basic-docker/containers"
	entries, err := os.ReadDir(containerDir)
	if err != nil {
		t.Fatalf("Failed to read container directory: %v", err)
	}
	if len(entries) == 0 {
		t.Errorf("No container directory was created")
	}
}

// TestListContainers:
// - Verifies that the listContainers function lists running containers correctly.
// - Setup: Creates mock container directories and PID files.
// - Expected Outcome: The output includes the container IDs and their statuses.

func TestListContainers(t *testing.T) {
	// Setup: Create mock container directories and PID files
	containerDir := "/tmp/basic-docker/containers"
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("Failed to create container directory: %v", err)
	}
	defer os.RemoveAll(containerDir) // Cleanup

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
	containerDir := "/tmp/basic-docker/containers/test-container"
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("Failed to create container directory: %v", err)
	}
	defer os.RemoveAll(containerDir) // Cleanup

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