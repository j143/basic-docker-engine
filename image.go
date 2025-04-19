package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ListImages lists all available images
func ListImages() {
	imageDir := "/tmp/basic-docker/images"
	fmt.Println("IMAGE NAME\tSIZE")

	if _, err := os.Stat(imageDir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(imageDir)
	if err != nil {
		fmt.Printf("Error reading images: %v\n", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			size, err := calculateDirSize(filepath.Join(imageDir, entry.Name()))
			if err != nil {
				fmt.Printf("%s\tError calculating size\n", entry.Name())
			} else {
				fmt.Printf("%s\t%d bytes\n", entry.Name(), size)
			}
		}
	}
}

// calculateDirSize calculates the total size of a directory
func calculateDirSize(dirPath string) (int64, error) {
	var totalSize int64
	err := filepath.Walk(dirPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, err
	}
	return totalSize, nil
}

// Image represents a container image
type Image struct {
	Name    string
	RootFS  string
	Layers  []string
}

// Registry represents a generic interface for interacting with container registries
type Registry interface {
	FetchManifest(repo, tag string) (*Manifest, error)
	FetchLayer(repo, digest string) (io.ReadCloser, error)
}

// DockerHubRegistry is a default implementation of the Registry interface for Docker Hub or custom registries.
type DockerHubRegistry struct {
	BaseURL string
}

// NewDockerHubRegistry creates a new instance of DockerHubRegistry with an optional custom registry URL.
func NewDockerHubRegistry(customURL string) *DockerHubRegistry {
	if customURL == "" {
		customURL = "https://registry-1.docker.io/v2/"
	}
	return &DockerHubRegistry{
		BaseURL: customURL,
	}
}

// FetchManifest fetches the manifest for a given repository and tag.
func (r *DockerHubRegistry) FetchManifest(repo, tag string) (*Manifest, error) {
	url := fmt.Sprintf("%s%s/manifests/%s", r.BaseURL, repo, tag)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &manifest, nil
}

// FetchLayer fetches a specific layer by its digest.
func (r *DockerHubRegistry) FetchLayer(repo, digest string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s%s/blobs/%s", r.BaseURL, repo, digest)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch layer: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Manifest represents the structure of an image manifest
type Manifest struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		Digest string `json:"digest"`
	} `json:"layers"`
}

// Pull downloads an image using the provided registry
func Pull(registry Registry, name string) (*Image, error) {
	fmt.Printf("[DEBUG] Starting to pull image '%s'\n", name)

	// Split the image name into repository and tag
	parts := strings.Split(name, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	fmt.Printf("[DEBUG] Fetching manifest for repo '%s' and tag '%s'\n", repo, tag)
	// Fetch the image manifest
	manifest, err := registry.FetchManifest(repo, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	fmt.Printf("[DEBUG] Manifest fetched successfully. Number of layers: %d\n", len(manifest.Layers))

	// Download and extract layers
	rootfs := filepath.Join("/tmp/basic-docker/images", name, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs: %w", err)
	}

	for _, layer := range manifest.Layers {
		fmt.Printf("[DEBUG] Downloading layer with digest '%s'\n", layer.Digest)
		layerReader, err := registry.FetchLayer(repo, layer.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to download layer %s: %w", layer.Digest, err)
		}
		defer layerReader.Close()

		fmt.Printf("[DEBUG] Extracting layer '%s'\n", layer.Digest)
		if err := extractLayer(layerReader, rootfs); err != nil {
			return nil, fmt.Errorf("failed to extract layer %s: %w", layer.Digest, err)
		}
	}

	fmt.Printf("[DEBUG] Image '%s' pulled successfully. RootFS path: %s\n", name, rootfs)
	return &Image{
		Name:   name,
		RootFS: rootfs,
		Layers: []string{"base"},
	}, nil
}

// extractLayer extracts a tar archive to the specified rootfs directory
func extractLayer(reader io.Reader, rootfs string) error {
	// Use tar to extract the layer
	cmd := exec.Command("tar", "-x", "-C", rootfs)
	cmd.Stdin = reader
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract layer: %w", err)
	}
	return nil
}

// LoadImageFromTar loads a container image from a .tar file
func LoadImageFromTar(tarFilePath string, imageName string) (*Image, error) {
	rootfs := filepath.Join("/tmp/basic-docker/images", imageName, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs: %w", err)
	}

	// Extract the tar file to the rootfs directory
	tarFile, err := os.Open(tarFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tar file: %w", err)
	}
	defer tarFile.Close()

	cmd := exec.Command("tar", "-x", "-C", rootfs, "-f", tarFilePath)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to extract tar file: %w", err)
	}

	return &Image{
		Name:   imageName,
		RootFS: rootfs,
		Layers: []string{"base"},
	}, nil
}