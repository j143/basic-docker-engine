package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

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
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  basic-docker run <command> [args...]  - Run a command in a container")
	fmt.Println("  basic-docker ps                      - List running containers")
	fmt.Println("  basic-docker images                  - List available images")
}

func run() {
	// Generate a container ID
	containerID := fmt.Sprintf("container-%d", time.Now().Unix())
	fmt.Printf("Starting container %s\n", containerID)

	// Create rootfs for this container
	rootfs := filepath.Join("/tmp/basic-docker/containers", containerID, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		fmt.Printf("Error creating rootfs: %v\n", err)
		os.Exit(1)
	}

	// In a real implementation, we'd copy from an image
	// Here we'll just create a minimal filesystem
	must(createMinimalRootfs(rootfs))

	// This is the process that will run inside our container
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	
	// Set up namespaces for isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // Hostname isolation
			syscall.CLONE_NEWPID | // Process ID isolation
			syscall.CLONE_NEWNS | // Mount isolation
			syscall.CLONE_NEWNET, // Network isolation
		Chroot: rootfs, // Use our container's rootfs
	}
	
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Set up resource constraints (memory: 100MB)
	must(setupCgroups(containerID, 100*1024*1024))
	
	// Run the container process
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	
	fmt.Printf("Container %s exited\n", containerID)
}

func createMinimalRootfs(rootfs string) error {
	// Create essential directories
	dirs := []string{"/bin", "/dev", "/etc", "/proc", "/sys", "/tmp"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(rootfs, dir), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	
	// Copy a minimal set of binaries (in a real implementation,
	// this would come from an image)
	// This assumes busybox is available on your system
	busyboxPath, err := exec.LookPath("busybox")
	if err != nil {
		return fmt.Errorf("busybox not found: %v", err)
	}
	
	if err := copyFile(busyboxPath, filepath.Join(rootfs, "bin/busybox")); err != nil {
		return fmt.Errorf("failed to copy busybox: %v", err)
	}
	
	// Create symlinks for common commands
	for _, cmd := range []string{"sh", "ls", "echo", "cat"} {
		linkPath := filepath.Join(rootfs, "bin", cmd)
		if err := os.Symlink("busybox", linkPath); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %v", cmd, err)
		}
	}
	
	return nil
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

func setupCgroups(containerID string, memoryLimit int) error {
	// Create cgroup
	cgroupPath := fmt.Sprintf("/sys/fs/cgroup/memory/basic-docker/%s", containerID)
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
	// In a full implementation, we'd track running containers
	// For now, just look at the filesystem
	containerDir := "/tmp/basic-docker/containers"
	
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
	// In a full implementation, we'd have image metadata
	// For now, just look at the filesystem
	imageDir := "/tmp/basic-docker/images"
	
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

func must(err error) {
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}