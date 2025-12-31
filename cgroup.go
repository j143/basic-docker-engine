package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CgroupVersion represents the cgroup version
type CgroupVersion int

const (
	CgroupUnknown CgroupVersion = iota
	CgroupV1
	CgroupV2
)

// CgroupInfo contains information about cgroup capabilities
type CgroupInfo struct {
	Version         CgroupVersion
	Available       bool
	MemorySupported bool
	CPUSupported    bool
	BasePath        string
	ErrorMessage    string
}

// DetectCgroupVersion detects whether the system uses cgroup v1 or v2
func DetectCgroupVersion() CgroupInfo {
	info := CgroupInfo{
		Version:   CgroupUnknown,
		Available: false,
	}

	// Check for cgroup v2 (unified hierarchy)
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		info.Version = CgroupV2
		info.BasePath = "/sys/fs/cgroup"
		info.Available = true
		
		// Check if memory controller is available
		controllersData, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
		if err == nil {
			controllers := string(controllersData)
			info.MemorySupported = strings.Contains(controllers, "memory")
			info.CPUSupported = strings.Contains(controllers, "cpu")
		}
		
		return info
	}

	// Check for cgroup v1
	if _, err := os.Stat("/sys/fs/cgroup/memory"); err == nil {
		info.Version = CgroupV1
		info.BasePath = "/sys/fs/cgroup"
		info.Available = true
		
		// For v1, check if we can access the memory subsystem
		memoryPath := "/sys/fs/cgroup/memory"
		if stat, err := os.Stat(memoryPath); err == nil && stat.IsDir() {
			info.MemorySupported = true
		}
		
		// Check CPU subsystem
		cpuPath := "/sys/fs/cgroup/cpu"
		if stat, err := os.Stat(cpuPath); err == nil && stat.IsDir() {
			info.CPUSupported = true
		}
		
		return info
	}

	info.ErrorMessage = "No cgroup filesystem detected"
	return info
}

// SetupCgroupsV2 sets up cgroups for v2 (unified hierarchy)
func SetupCgroupsV2(containerID string, memoryLimit int64) error {
	cgroupPath := filepath.Join("/sys/fs/cgroup/basic-docker", containerID)
	
	// Create the cgroup directory
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup v2 directory: %w", err)
	}

	// Enable memory controller in subtree_control
	parentControl := "/sys/fs/cgroup/basic-docker/cgroup.subtree_control"
	// First ensure parent cgroup exists
	parentPath := "/sys/fs/cgroup/basic-docker"
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		return fmt.Errorf("failed to create parent cgroup: %w", err)
	}
	
	// Try to enable memory controller
	if err := os.WriteFile(parentControl, []byte("+memory"), 0644); err != nil {
		// It's OK if this fails - may not have permission
		// Continue without memory limits
		return nil
	}

	// Set memory limit if supported
	memoryMaxFile := filepath.Join(cgroupPath, "memory.max")
	if err := os.WriteFile(memoryMaxFile, []byte(strconv.FormatInt(memoryLimit, 10)), 0644); err != nil {
		// Not fatal - just means we can't set limits
		return nil
	}

	// Add current process to cgroup
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	pid := os.Getpid()
	if err := os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to add process to cgroup: %w", err)
	}

	return nil
}

// SetupCgroupsV1 sets up cgroups for v1
func SetupCgroupsV1(containerID string, memoryLimit int64) error {
	cgroupPath := filepath.Join("/sys/fs/cgroup/memory/basic-docker", containerID)
	
	// Create the cgroup directory
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup v1 directory: %w", err)
	}

	// Set memory limit
	memoryLimitFile := filepath.Join(cgroupPath, "memory.limit_in_bytes")
	if err := os.WriteFile(memoryLimitFile, []byte(strconv.FormatInt(memoryLimit, 10)), 0644); err != nil {
		// Not fatal - just means we can't set limits
		return nil
	}

	// Add current process to cgroup
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	pid := os.Getpid()
	if err := os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to add process to cgroup: %w", err)
	}

	return nil
}

// SetupCgroupsWithDetection automatically detects cgroup version and sets up accordingly
func SetupCgroupsWithDetection(containerID string, memoryLimit int64) error {
	info := DetectCgroupVersion()
	
	if !info.Available {
		// Cgroups not available - degrade gracefully
		return nil
	}

	switch info.Version {
	case CgroupV2:
		return SetupCgroupsV2(containerID, memoryLimit)
	case CgroupV1:
		return SetupCgroupsV1(containerID, memoryLimit)
	default:
		return nil
	}
}

// CleanupCgroup removes the cgroup directory for a container
func CleanupCgroup(containerID string) error {
	info := DetectCgroupVersion()
	
	if !info.Available {
		return nil
	}

	var cgroupPath string
	switch info.Version {
	case CgroupV2:
		cgroupPath = filepath.Join("/sys/fs/cgroup/basic-docker", containerID)
	case CgroupV1:
		cgroupPath = filepath.Join("/sys/fs/cgroup/memory/basic-docker", containerID)
	default:
		return nil
	}

	// Remove the cgroup directory
	if err := os.Remove(cgroupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cgroup: %w", err)
	}

	return nil
}
