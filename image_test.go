package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"net/http"
	"net/http/httptest"
	"io/ioutil"
)

// Test Scenarios Documentation
//
// TestListImages:
// - Verifies that the ListImages function correctly lists available images.
// - Setup: Creates a temporary directory and a mock image directory.
// - Expected Outcome: The output of ListImages should include the mock image name.
//
// TestDockerHubRegistry_FetchManifest:
// - Verifies the FetchManifest method of DockerHubRegistry using a mock HTTP server.
// - Setup: Creates a mock server to simulate Docker Hub API responses.
// - Expected Outcome: The manifest returned by FetchManifest should match the mock data.
//
// TestDockerHubRegistry_FetchLayer:
// - Verifies the FetchLayer method of DockerHubRegistry using a mock HTTP server.
// - Setup: Creates a mock server to simulate Docker Hub API responses.
// - Expected Outcome: The layer content returned by FetchLayer should match the mock data.

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

// TestDockerHubRegistry_FetchManifest tests the FetchManifest method of DockerHubRegistry
func TestDockerHubRegistry_FetchManifest(t *testing.T) {
	// Mock server to simulate Docker Hub API
	handler := http.NewServeMux()
	handler.HandleFunc("/v2/library/busybox/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"config": {"digest": "sha256:configdigest"},
			"layers": [
				{"digest": "sha256:layer1digest"},
				{"digest": "sha256:layer2digest"}
			]
		}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a DockerHubRegistry instance with the mock server URL
	registry := &DockerHubRegistry{BaseURL: server.URL + "/v2/"}

	// Call FetchManifest
	manifest, err := registry.FetchManifest("library/busybox", "latest")
	if err != nil {
		t.Fatalf("FetchManifest failed: %v", err)
	}

	// Verify the manifest content
	if manifest.Config.Digest != "sha256:configdigest" {
		t.Errorf("Expected config digest 'sha256:configdigest', got '%s'", manifest.Config.Digest)
	}
	if len(manifest.Layers) != 2 {
		t.Errorf("Expected 2 layers, got %d", len(manifest.Layers))
	}
	if manifest.Layers[0].Digest != "sha256:layer1digest" {
		t.Errorf("Expected first layer digest 'sha256:layer1digest', got '%s'", manifest.Layers[0].Digest)
	}
	if manifest.Layers[1].Digest != "sha256:layer2digest" {
		t.Errorf("Expected second layer digest 'sha256:layer2digest', got '%s'", manifest.Layers[1].Digest)
	}
}

// TestDockerHubRegistry_FetchLayer tests the FetchLayer method of DockerHubRegistry
func TestDockerHubRegistry_FetchLayer(t *testing.T) {
	// Mock server to simulate Docker Hub API
	handler := http.NewServeMux()
	handler.HandleFunc("/v2/library/busybox/blobs/sha256:layer1digest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("layer1content"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a DockerHubRegistry instance with the mock server URL
	registry := &DockerHubRegistry{BaseURL: server.URL + "/v2/"}

	// Call FetchLayer
	reader, err := registry.FetchLayer("library/busybox", "sha256:layer1digest")
	if err != nil {
		t.Fatalf("FetchLayer failed: %v", err)
	}
	defer reader.Close()

	// Verify the layer content
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read layer content: %v", err)
	}
	if string(content) != "layer1content" {
		t.Errorf("Expected layer content 'layer1content', got '%s'", string(content))
	}
}