package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Environment detection
var (
	// Set to true if we're running in a container environment
	inContainer = false
	// Set to true if we have full namespace privileges
	hasNamespacePrivileges = false
	// Set to true if we have cgroup access
	hasCgroupAccess = false
)

func init() {
	// Detect if we're running in a container
	if _, err := os.Stat("/.dockerenv"); err == nil {
		inContainer = true
	} else if os.Getenv("CODESPACES") == "true" {
		inContainer = true
	} else {
		// Check if /proc/self/cgroup contains docker or containerd
		data, err := os.ReadFile("/proc/self/cgroup")
		if err == nil && (strings.Contains(string(data), "docker") || 
						  strings.Contains(string(data), "containerd")) {
			inContainer = true
		}
	}

	// Test namespace privileges
	cmd := exec.Command("unshare", "--user", "echo", "test")
	hasNamespacePrivileges = cmd.Run() == nil

	// Test cgroup access
	cgroupPath := "/sys/fs/cgroup/memory"
	_, err := os.Stat(cgroupPath)
	hasCgroupAccess = err == nil
	if hasCgroupAccess {
		// Try to create a test cgroup
		testPath := filepath.Join(cgroupPath, "lean-docker-test")
		hasCgroupAccess = os.MkdirAll(testPath, 0755) == nil
		// Clean up test path
		os.Remove(testPath)
	}

	fmt.Printf("Environment detected: inContainer=%v, hasNamespacePrivileges=%v, hasCgroupAccess=%v\n", 
		inContainer, hasNamespacePrivileges, hasCgroupAccess)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Error: Command required for run")
			os.Exit(1)
		}
		run()
	case "ps":
		listContainers()
	case "images":
		listImages()
	case "info":
		printSystemInfo()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  lean-docker run <command> [args...]  - Run a command in a container")
	fmt.Println("  lean-docker ps                       - List running containers")
	fmt.Println("  lean-docker images                   - List available images")
	fmt.Println("  lean-docker info                     - Show system information")
}

func printSystemInfo() {
	fmt.Println("Lean Docker Engine - System Information")
	fmt.Println("=======================================")
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Running in container: %v\n", inContainer)
	fmt.Printf("Namespace privileges: %v\n", hasNamespacePrivileges)
	fmt.Printf("Cgroup access: %v\n", hasCgroupAccess)
	fmt.Println("Available features:")
	fmt.Printf("  - Process isolation: %v\n", hasNamespacePrivileges)
	fmt.Printf("  - Network isolation: %v\n", hasNamespacePrivileges)
	fmt.Printf("  - Resource limits: %v\n", hasCgroupAccess)
	fmt.Printf("  - Filesystem isolation: true\n")
}

func run() {
	// Generate a container ID
	containerID := fmt.Sprintf("container-%d", time.Now().Unix())
	fmt.Printf("Starting container %s\n", containerID)

	// Create rootfs for this container
	rootfs := filepath.Join("/tmp/lean-docker/containers", containerID, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		fmt.Printf("Error creating rootfs: %v\n", err)
		os.Exit(1)
	}

	// Create a minimal rootfs
	must(createMinimalRootfs(rootfs))

	if hasNamespacePrivileges && !inContainer {
		// Full isolation approach for pure Linux environments
		runWithNamespaces(containerID, rootfs, os.Args[2], os.Args[3:])
	} else {
		// Limited isolation for container environments
		runWithoutNamespaces(containerID, rootfs, os.Args[2], os.Args[3:])
	}
	
	fmt.Printf("Container %s exited\n", containerID)
}

// runWithNamespaces uses full Linux namespace isolation
func runWithNamespaces(containerID, rootfs, command string, args []string) {
	cmd := exec.Command(command, args...)
	
	// Set up namespaces for isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // Hostname isolation
			syscall.CLONE_NEWPID | // Process ID isolation 
			syscall.CLONE_NEWNS,   // Mount isolation
	}
	
	// Add network isolation if available
	if hasNamespacePrivileges {
		cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNET
	}
	
	// Use the container's rootfs
	cmd.SysProcAttr.Chroot = rootfs
	
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Set up resource constraints if available
	if hasCgroupAccess {
		must(setupCgroups(containerID, 100*1024*1024))
	}
	
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// runWithoutNamespaces uses basic chroot-like isolation for container environments
func runWithoutNamespaces(containerID, rootfs, command string, args []string) {
	// Create a script to run the command with basic isolation
	script := fmt.Sprintf(`#!/bin/sh
cd %s
export PATH=/bin:/usr/bin:/sbin:/usr/sbin
# Try different isolation methods
if command -v chroot > /dev/null && [ $(id -u) -eq 0 ]; then
    # If we have chroot and root
    chroot . %s %s
elif command -v unshare > /dev/null; then
    # Try unshare if available
    unshare --mount --uts --ipc --pid --fork -- %s %s
else
    # Fallback without isolation
    %s %s
fi
`, rootfs, command, combineArgs(args), command, combineArgs(args), command, combineArgs(args))
	
	scriptPath := filepath.Join(rootfs, "run.sh")
	os.WriteFile(scriptPath, []byte(script), 0755)
	
	// Execute the script
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}

func createMinimalRootfs(rootfs string) error {
	// Create essential directories
	dirs := []string{"/bin", "/dev", "/etc", "/proc", "/sys", "/tmp"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(rootfs, dir), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	
	// Try copying busybox if available
	busyboxPath, err := exec.LookPath("busybox")
	if err == nil {
		if err := copyFile(busyboxPath, filepath.Join(rootfs, "bin/busybox")); err != nil {
			return fmt.Errorf("failed to copy busybox: %v", err)
		}
		
		// Create symlinks for common commands
		for _, cmd := range []string{"sh", "ls", "echo", "cat", "ps"} {
			linkPath := filepath.Join(rootfs, "bin", cmd)
			if err := os.Symlink("busybox", linkPath); err != nil {
				return fmt.Errorf("failed to create symlink for %s: %v", cmd, err)
			}
		}
	} else {
		// Copy basic binaries from host if busybox is not available
		for _, cmd := range []string{"sh", "ls", "echo", "cat"} {
			cmdPath, err := exec.LookPath(cmd)
			if err == nil {
				if err := copyFile(cmdPath, filepath.Join(rootfs, "bin", filepath.Base(cmdPath))); err != nil {
					fmt.Printf("Warning: Failed to copy %s: %v\n", cmd, err)
				}
			}
		}
	}
	
	return nil
}

func setupCgroups(containerID string, memoryLimit int) error {
	// Skip if no cgroup access
	if !hasCgroupAccess {
		return nil
	}
	
	// Create cgroup
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/memory/lean-docker/%s", containerID)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup: %v", err)
	}
	
	// Set memory limit
	if err := os.WriteFile(
		fmt.Sprintf("%s/memory.limit_in_bytes", cgroupPath), 
		[]byte(fmt.Sprintf("%d", memoryLimit)), 
		0644,
	); err != nil {
		return fmt.Errorf("failed to set memory limit: %v", err)
	}
	
	// Add current process to cgroup
	pid := os.Getpid()
	if err := os.WriteFile(
		fmt.Sprintf("%s/cgroup.procs", cgroupPath), 
		[]byte(fmt.Sprintf("%d", pid)), 
		0644,
	); err != nil {
		return fmt.Errorf("failed to add process to cgroup: %v", err)
	}
	
	return nil
}

func listContainers() {
	// Look at the filesystem for container directories
	containerDir := "/tmp/lean-docker/containers"
	
	fmt.Println("CONTAINER ID\tSTATUS\tCOMMAND")
	
	// Check if directory exists
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		return
	}
	
	entries, err := os.ReadDir(containerDir)
	if err != nil {
		fmt.Printf("Error reading containers: %v\n", err)
		return
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			fmt.Printf("%s\tN/A\tN/A\n", entry.Name())
		}
	}
}

func listImages() {
	// Look at the filesystem for image directories
	imageDir := "/tmp/lean-docker/images"
	
	fmt.Println("IMAGE NAME\tSIZE")
	
	// Check if directory exists
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

func copyFile(src, dst string) error {
	// Read the source file
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	
	// Write to the destination file
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return err
	}
	
	return nil
}

func combineArgs(args []string) string {
	result := ""
	for _, arg := range args {
		result += " " + arg
	}
	return result
}

func must(err error) {
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}