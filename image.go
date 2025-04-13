package main

import (
	"fmt"
	"io"
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
			fmt.Printf("%s\tN/A\n", entry.Name())
		}
	}
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
	// Split the image name into repository and tag
	parts := strings.Split(name, ":")
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}

	// Fetch the image manifest
	manifest, err := registry.FetchManifest(repo, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Download and extract layers
	rootfs := filepath.Join("/tmp/basic-docker/images", name, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs: %w", err)
	}

	for _, layer := range manifest.Layers {
		layerReader, err := registry.FetchLayer(repo, layer.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to download layer %s: %w", layer.Digest, err)
		}
		defer layerReader.Close()

		if err := extractLayer(layerReader, rootfs); err != nil {
			return nil, fmt.Errorf("failed to extract layer %s: %w", layer.Digest, err)
		}
	}

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