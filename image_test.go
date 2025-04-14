package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test Scenarios Documentation
//
// TestListImages:
// - Verifies that the ListImages function correctly lists available images.
// - Setup: Creates a temporary directory and a mock image directory.
// - Expected Outcome: The output of ListImages should include the mock image name.

// Additional test scenarios can be documented here as new tests are added.

func TestListImages(t *testing.T) {
	baseDir := filepath.Join(os.TempDir(), "basic-docker")
	imagesDir := filepath.Join(baseDir, "images")

	// Setup: Create the images directory
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(baseDir) // Cleanup

	// Create a mock image directory
	imageName := "test-image"
	if err := os.MkdirAll(filepath.Join(imagesDir, imageName), 0755); err != nil {
		t.Fatalf("Failed to create mock image directory: %v", err)
	}

	// Capture the output of ListImages
	output := captureOutput(ListImages)

	// Verify the output contains the mock image name
	if !contains(output, imageName) {
		t.Errorf("Expected output to contain image name '%s', but got: %s", imageName, output)
	}
}

func captureOutput(f func()) string {
	// Redirect stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the function
	f()

	// Capture the output
	w.Close()
	os.Stdout = old
	var buf [1024]byte
	n, _ := r.Read(buf[:])
	return string(buf[:n])
}

func contains(output, substring string) bool {
	return strings.Contains(output, substring)
}