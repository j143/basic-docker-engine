package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ContainerState represents the state of a container
type ContainerState string

const (
	StateCreated ContainerState = "created"
	StateRunning ContainerState = "running"
	StateExited  ContainerState = "exited"
	StateFailed  ContainerState = "failed"
)

// ContainerMetadata contains the state and metadata of a container
type ContainerMetadata struct {
	ID          string         `json:"id"`
	State       ContainerState `json:"state"`
	Image       string         `json:"image"`
	Command     string         `json:"command"`
	Args        []string       `json:"args"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	ExitCode    *int           `json:"exit_code,omitempty"`
	Error       string         `json:"error,omitempty"`
	PID         int            `json:"pid,omitempty"`
	RootfsPath  string         `json:"rootfs_path"`
}

// SaveContainerState saves the container state to disk
func SaveContainerState(metadata ContainerMetadata) error {
	containerDir := filepath.Join(baseDir, "containers", metadata.ID)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return fmt.Errorf("failed to create container directory: %w", err)
	}

	stateFile := filepath.Join(containerDir, "state.json")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal container state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write container state: %w", err)
	}

	return nil
}

// LoadContainerState loads the container state from disk
func LoadContainerState(containerID string) (*ContainerMetadata, error) {
	stateFile := filepath.Join(baseDir, "containers", containerID, "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("container %s not found", containerID)
		}
		return nil, fmt.Errorf("failed to read container state: %w", err)
	}

	var metadata ContainerMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal container state: %w", err)
	}

	return &metadata, nil
}

// UpdateContainerState updates specific fields of the container state
func UpdateContainerState(containerID string, updateFn func(*ContainerMetadata)) error {
	metadata, err := LoadContainerState(containerID)
	if err != nil {
		return err
	}

	updateFn(metadata)

	return SaveContainerState(*metadata)
}

// ListAllContainers lists all containers with their states
func ListAllContainers() ([]ContainerMetadata, error) {
	containerDir := filepath.Join(baseDir, "containers")
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		return []ContainerMetadata{}, nil
	}

	entries, err := os.ReadDir(containerDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read containers directory: %w", err)
	}

	var containers []ContainerMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		containerID := entry.Name()
		metadata, err := LoadContainerState(containerID)
		if err != nil {
			// If we can't load state, create a minimal metadata
			metadata = &ContainerMetadata{
				ID:    containerID,
				State: StateExited,
			}
		}

		containers = append(containers, *metadata)
	}

	return containers, nil
}

// RemoveContainer removes a container and its associated resources
func RemoveContainer(containerID string) error {
	// Load container state first
	metadata, err := LoadContainerState(containerID)
	if err != nil {
		// If state doesn't exist, still try to remove directory
		containerDir := filepath.Join(baseDir, "containers", containerID)
		if err := os.RemoveAll(containerDir); err != nil {
			return fmt.Errorf("failed to remove container directory: %w", err)
		}
		return nil
	}

	// Can't remove running containers
	if metadata.State == StateRunning {
		return fmt.Errorf("cannot remove running container %s (stop it first)", containerID)
	}

	// Clean up cgroup if it exists
	if err := CleanupCgroup(containerID); err != nil {
		fmt.Printf("Warning: failed to cleanup cgroup: %v\n", err)
	}

	// Remove container directory
	containerDir := filepath.Join(baseDir, "containers", containerID)
	if err := os.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("failed to remove container directory: %w", err)
	}

	return nil
}

// GetContainerLogs reads the logs from a container
func GetContainerLogs(containerID string) (string, error) {
	logFile := filepath.Join(baseDir, "containers", containerID, "stdout.log")
	
	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No logs yet
		}
		return "", fmt.Errorf("failed to read container logs: %w", err)
	}

	return string(data), nil
}
