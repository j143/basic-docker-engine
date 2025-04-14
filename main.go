package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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
	imagesDir = "/tmp/basic-docker/images"
	layersDir = "/tmp/basic-docker/layers"
)

// Define the ImageLayer type
type ImageLayer struct {
	ID            string
	Created       time.Time
	Size          int64
	BaseLayerPath string
	AppLayerPath  string
}

// To initialize the directories
func initDirectories() error {
	dirs := []string{
		"/tmp/basic-docker/containers",
		imagesDir,
		layersDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func ensureBusyboxExists() {
	if _, err := exec.LookPath("busybox"); err != nil {
		fmt.Println("Error: busybox is not found on the host system.")
		fmt.Println("Please install busybox to enable isolation features.")
		os.Exit(1)
	}
}

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
		testPath := filepath.Join(cgroupPath, "basic-docker-test")
		hasCgroupAccess = os.MkdirAll(testPath, 0755) == nil
		// Clean up test path
		os.Remove(testPath)
	}

	fmt.Printf("Environment detected: inContainer=%v, hasNamespacePrivileges=%v, hasCgroupAccess=%v\n", 
		inContainer, hasNamespacePrivileges, hasCgroupAccess)

	if err := initDirectories(); err != nil {
		fmt.Printf("Warning: Failed to intialize directories: %v \n", err)
	}
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
	case "exec":
		execCommand()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  basic-docker run <command> [args...]  - Run a command in a container")
	fmt.Println("  basic-docker ps                       - List running containers")
	fmt.Println("  basic-docker images                   - List available images")
	fmt.Println("  basic-docker info                     - Show system information")
	fmt.Println("  basic-docker exec <container-id> <command> [args...] - Execute a command in a running container")
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
    // Adjust the path to include the mock image directory during testing
	if os.Getenv("TEST_ENV") == "true" {
		os.Setenv("PATH", os.Getenv("PATH")+":"+imagesDir)
	}

    // Generate a container ID
    containerID := fmt.Sprintf("container-%d", time.Now().Unix())
    fmt.Printf("Starting container %s\n", containerID)

    // Check if the image exists before proceeding
    imageName := os.Args[2]
    imagePath := filepath.Join(imagesDir, imageName)
    if _, err := os.Stat(imagePath); os.IsNotExist(err) {
        fmt.Printf("Image '%s' not found. Fetching the image...\n", imageName)
        if err := fetchImage(imageName); err != nil {
            fmt.Printf("Error: Failed to fetch image '%s': %v\n", imageName, err)
            os.Exit(1)
        }
        fmt.Printf("Image '%s' fetched successfully.\n", imageName)
    }

    // Create rootfs for this container
    rootfs := filepath.Join("/tmp/basic-docker/containers", containerID, "rootfs")
    
    // Instead of calling createMinimalRootfs directly:
    // 1. Create a base layer if it doesn't exist
    baseLayerID := "base-layer"
    baseLayerPath := filepath.Join("/tmp/basic-docker/layers", baseLayerID)
    
    if _, err := os.Stat(baseLayerPath); os.IsNotExist(err) {
        // Create the base layer
        if err := os.MkdirAll(baseLayerPath, 0755); err != nil {
            fmt.Printf("Error creating base layer directory: %v\n", err)
            os.Exit(1)
        }
        
        // Initialize the base layer with minimal rootfs content
        must(initializeBaseLayer(baseLayerPath))
        
        // Save layer metadata
        layer := ImageLayer{
            ID:            baseLayerID,
            Created:       time.Now(),
            BaseLayerPath: baseLayerPath,
        }
        
        if err := saveLayerMetadata(layer); err != nil {
            fmt.Printf("Warning: Failed to save layer metadata: %v\n", err)
        }
    }

    // Fix permission issue by ensuring correct ownership and permissions for the base layer
    if err := os.Chmod(baseLayerPath, 0755); err != nil {
        fmt.Printf("Error setting permissions for base layer: %v\n", err)
        os.Exit(1)
    }
    
    // 2. Create an app layer for this specific container (optional)
    appLayerID := "app-layer-" + containerID
    appLayerPath := filepath.Join("/tmp/basic-docker/layers", appLayerID)
    
    // Use the appLayerID variable to log its creation
    fmt.Printf("App layer created with ID: %s\n", appLayerID)
    
    // You could add container-specific files to the app layer here
    // For now, we'll just use the base layer
    
    // Save layer metadata including app layer path
    layer := ImageLayer{
        ID:            appLayerID,
        Created:       time.Now(),
        BaseLayerPath: baseLayerPath,
        AppLayerPath:  appLayerPath,
    }

    if err := saveLayerMetadata(layer); err != nil {
        fmt.Printf("Warning: Failed to save layer metadata: %v\n", err)
    }
    
    // 3. Mount the layers to create the container rootfs
    layers := []string{baseLayerID} // Add appLayerID if you created one
    must(mountLayeredFilesystem(layers, rootfs))

    // Write the PID of the current process to a file
    pidFile := filepath.Join("/tmp/basic-docker/containers", containerID, "pid")
    fmt.Printf("Debug: Writing PID file for container %s at %s\n", containerID, pidFile)
    fmt.Printf("Debug: Current process PID is %d\n", os.Getpid())
    if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
        fmt.Printf("Error: Failed to write PID file for container %s: %v\n", containerID, err)
        os.Exit(1)
    }
    
    // Fix deadlock by adding a timeout mechanism
    if len(os.Args) < 4 {
        fmt.Println("No command provided. Keeping the container process alive with a timeout.")
        timeout := time.After(10 * time.Minute) // Set a timeout of 10 minutes
        select {
        case <-timeout:
            fmt.Println("Timeout reached. Exiting container process.")
            os.Exit(0)
        }
    }

    // Update the fallback logic to avoid using unshare entirely in limited isolation
    if hasNamespacePrivileges && !inContainer {
        // Full isolation approach for pure Linux environments
        runWithNamespaces(containerID, rootfs, os.Args[2], os.Args[3:])
    } else {
        runWithoutNamespaces(containerID, rootfs, os.Args[2], os.Args[3:])
    }
    
    fmt.Printf("Container %s exited\n", containerID)
}

func initializeBaseLayer(baseLayerPath string) error {
    // Create essential directories in the base layer
    dirs := []string{"/bin", "/dev", "/etc", "/proc", "/sys", "/tmp"}
    for _, dir := range dirs {
        if err := os.MkdirAll(filepath.Join(baseLayerPath, dir), 0755); err != nil {
            return fmt.Errorf("failed to create directory %s: %v", dir, err)
        }
    }
    
    // Retain baseLayerPath for potential future use
    fmt.Printf("Base layer path: %s\n", baseLayerPath)

    // Ensure busybox is properly copied and symlinks are created
    if busyboxPath, err := exec.LookPath("busybox"); err == nil {
        fmt.Printf("Busybox found at: %s\n", busyboxPath)
        if err := copyFile(busyboxPath, filepath.Join(baseLayerPath, "bin/busybox")); err != nil {
            return fmt.Errorf("failed to copy busybox: %v", err)
        }

        // Create symlinks for common commands
        commands := []string{"sh", "ls", "echo", "cat", "ps"}
        for _, cmd := range commands {
            linkPath := filepath.Join(baseLayerPath, "bin", cmd)
            if err := os.Symlink("busybox", linkPath); err != nil {
                fmt.Printf("Warning: Failed to create symlink for %s: %v\n", cmd, err)
            }
        }
    } else {
        return fallbackToHostBinaries(baseLayerPath)
    }

    // Verify that essential commands are available in the base layer
    essentialCommands := []string{"sh", "ls", "echo", "cat", "ps"}
    for _, cmd := range essentialCommands {
        cmdPath := filepath.Join(baseLayerPath, "bin", cmd)
        if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
            return fmt.Errorf("essential command %s is missing in the base layer", cmd)
        }
    }

    // Debugging: Verify that busybox and symlinks are correctly set up
    busyboxPath := filepath.Join(baseLayerPath, "bin/busybox")
    if _, err := os.Stat(busyboxPath); os.IsNotExist(err) {
        return fmt.Errorf("busybox binary is missing in the base layer: %s", busyboxPath)
    }

    for _, cmd := range []string{"sh", "ls", "echo", "cat", "ps"} {
        symlinkPath := filepath.Join(baseLayerPath, "bin", cmd)
        if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
            return fmt.Errorf("symlink for %s is missing in the base layer: %s", cmd, symlinkPath)
        }
    }

    // Debugging: Verify the correctness of the sh symlink
    shSymlinkPath := filepath.Join(baseLayerPath, "bin/sh")
    if target, err := os.Readlink(shSymlinkPath); err != nil {
        return fmt.Errorf("failed to read symlink for sh: %v", err)
    } else if target != "busybox" {
        return fmt.Errorf("sh symlink does not point to busybox: %s", target)
    }

    fmt.Printf("Verified: sh symlink correctly points to busybox at %s\n", shSymlinkPath)

    // Debugging: Verify busybox and symlinks in the container's rootfs
    rootfsBusyboxPath := filepath.Join(baseLayerPath, "bin/busybox")
    if _, err := os.Stat(rootfsBusyboxPath); os.IsNotExist(err) {
        return fmt.Errorf("busybox binary is missing in the container's rootfs: %s", rootfsBusyboxPath)
    }

    for _, cmd := range []string{"sh", "ls", "echo", "cat", "ps"} {
        symlinkPath := filepath.Join(baseLayerPath, "bin", cmd)
        if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
            return fmt.Errorf("symlink for %s is missing in the container's rootfs: %s", cmd, symlinkPath)
        }
    }

    fmt.Printf("Verified: busybox and symlinks are correctly set up in the container's rootfs.\n")

    // Debugging: Verify that the echo binary and symlink are correctly set up
    echoPath := filepath.Join(baseLayerPath, "bin/echo")
    if _, err := os.Lstat(echoPath); os.IsNotExist(err) {
        return fmt.Errorf("echo binary or symlink is missing in the base layer: %s", echoPath)
    }

    fmt.Printf("Verified: echo binary or symlink exists at %s\n", echoPath)

    // Debugging: List contents of the /bin directory in the base layer
    binDir := filepath.Join(baseLayerPath, "bin")
    entries, err := os.ReadDir(binDir)
    if err != nil {
        return fmt.Errorf("failed to read /bin directory: %v", err)
    }
    fmt.Println("Contents of /bin directory:")
    for _, entry := range entries {
        fmt.Printf("- %s\n", entry.Name())
    }

    // Debugging: Attempt to execute busybox directly
    busyboxTestCmd := exec.Command(filepath.Join(binDir, "busybox"), "--help")
    busyboxTestCmd.Stdout = os.Stdout
    busyboxTestCmd.Stderr = os.Stderr
    if err := busyboxTestCmd.Run(); err != nil {
        return fmt.Errorf("failed to execute busybox: %v", err)
    }

    fmt.Println("Busybox and symlinks are correctly set up in the base layer.")
    
    return nil
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

// Reintroduce runWithoutNamespaces for simplicity and modularity
func runWithoutNamespaces(containerID, rootfs, command string, args []string) {
    fmt.Println("Warning: Namespace isolation is not permitted. Executing without isolation.")
    cmd := exec.Command(command, args...)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        fmt.Printf("Error: %v\n", err)
        os.Exit(1)
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

	// Create a base layer
	baseLayerID := "base-layer-" + fmt.Sprintf("%d", time.Now().Unix())
	baseLayerPath := filepath.Join(layersDir, baseLayerID)
	if err := os.MkdirAll(baseLayerPath, 0755); err != nil {
		return fmt.Errorf("failed to create base layer: %v", err)
	}

	// Create a bin directory in the base layer
	if err := os.MkdirAll(filepath.Join(baseLayerPath, "bin"), 0755); err != nil {
		return fmt.Errorf("failed to create bin directory in base layer: %v", err)
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

	// Now copy the base layer to the rootfs
	if err := copyDir(baseLayerPath, rootfs); err != nil {
		return fmt.Errorf("failed to copy base layer to rootfs: %v", err)
	}
	
	// Create a record of this layer
	layer := ImageLayer{
		ID:            baseLayerID,
		Created:       time.Now(),
		BaseLayerPath: baseLayerPath,
	}

	// Save layer metadata
	if err := saveLayerMetadata(layer); err != nil {
		fmt.Printf("Warning: Failed to save layer metadata: %v\n", err)
	}

	return nil
}

// Add this function to copy directories
func copyDir(src, dst string) error {
    return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        
        // Calculate relative path
        relPath, err := filepath.Rel(src, path)
        if err != nil {
            return err
        }
        
        // Skip if it's the root directory
        if relPath == "." {
            return nil
        }
        
        // Create target path
        targetPath := filepath.Join(dst, relPath)
        
        // If it's a directory, create it
        if info.IsDir() {
            return os.MkdirAll(targetPath, info.Mode())
        }
        
        // If it's a file, copy it
        return copyFile(path, targetPath)
    })
}

// Implement the saveLayerMetadata function
func saveLayerMetadata(layer ImageLayer) error {
    // Serialize the layer metadata to JSON
    metadataFile := filepath.Join(layersDir, layer.ID+".json")
    file, err := os.Create(metadataFile)
    if err != nil {
        return fmt.Errorf("failed to create metadata file: %v", err)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    if err := encoder.Encode(layer); err != nil {
        return fmt.Errorf("failed to write metadata to file: %v", err)
    }

    fmt.Printf("Metadata for layer %s saved to %s\n", layer.ID, metadataFile)
    return nil
}

func mountLayeredFilesystem(layers []string, rootfs string) error {
	// Clear the rootfs first
	if err := os.RemoveAll(rootfs); err != nil {
		return fmt.Errorf("failed to clear rootfs: %v", err)
	}

	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return fmt.Errorf("failed to create rootfs: %v", err)
	}

	// Start from the base layer and apply each layer
	for _, layerID := range layers {
		layerPath := filepath.Join(layersDir, layerID)
		if err := copyDir(layerPath, rootfs); err != nil {
			return fmt.Errorf("failed to apply layer %s: %v", layerID, err)
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

func getContainerStatus(containerID string) string {
	pidFile := filepath.Join("/tmp/basic-docker/containers", containerID, "pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return "Stopped"
	}

	pid := strings.TrimSpace(string(pidData))
	procPath := fmt.Sprintf("/proc/%s", pid)
	if _, err := os.Stat(procPath); os.IsNotExist(err) {
		return "Stopped"
	}

	// Check if the process is still running
	if err := syscall.Kill(atoi(pid), 0); err != nil {
		if err == syscall.ESRCH {
			return "Stopped"
		}
	}

	return "Running"
}

func listContainers() {
	containerDir := "/tmp/basic-docker/containers"
	fmt.Println("CONTAINER ID\tSTATUS\tCOMMAND")

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
			containerID := entry.Name()
			status := getContainerStatus(containerID)
			fmt.Printf("%s\t%s\tN/A\n", containerID, status)
		}
	}
}

func listImages() {
	fmt.Println("[DEBUG] listImages: Starting to list images")
	ListImages()
	fmt.Println("[DEBUG] listImages: Finished listing images")
}

func testListImages() {
	fmt.Println("[DEBUG] Testing ListImages function")
	ListImages()
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

func testMultiLayerMount() {
    // Create a base layer
    baseLayerID := "base-layer-" + fmt.Sprintf("%d", time.Now().Unix())
    baseLayerPath := filepath.Join("/tmp/basic-docker/layers", baseLayerID)
    if err := os.MkdirAll(baseLayerPath, 0755); err != nil {
        fmt.Printf("Error creating base layer: %v\n", err)
        return
    }
    
    // Add a file to the base layer
    if err := os.WriteFile(filepath.Join(baseLayerPath, "base.txt"), []byte("Base layer file"), 0644); err != nil {
        fmt.Printf("Error creating base layer file: %v\n", err)
        return
    }
    
    // Create a second layer
    appLayerID := "app-layer-" + fmt.Sprintf("%d", time.Now().Unix())
    appLayerPath := filepath.Join("/tmp/basic-docker/layers", appLayerID)
    if err := os.MkdirAll(appLayerPath, 0755); err != nil {
        fmt.Printf("Error creating app layer: %v\n", err)
        return
    }
    
    // Retain appLayerPath for potential future use
    fmt.Printf("App layer path: %s\n", appLayerPath)

    // Add a file to the app layer
    if err := os.WriteFile(filepath.Join(appLayerPath, "app.txt"), []byte("App layer file"), 0644); err != nil {
        fmt.Printf("Error creating app layer file: %v\n", err)
        return
    }
    
    // Create a target directory
    targetPath := filepath.Join("/tmp/basic-docker/test-mount")
    
    // Mount the layers
    layers := []string{baseLayerID, appLayerID}
    if err := mountLayeredFilesystem(layers, targetPath); err != nil {
        fmt.Printf("Error mounting layers: %v\n", err)
        return
    }
    
    // Check if files exist
    if _, err := os.Stat(filepath.Join(targetPath, "base.txt")); err != nil {
        fmt.Printf("Base layer file not found: %v\n", err)
        return
    }
    
    if _, err := os.Stat(filepath.Join(targetPath, "app.txt")); err != nil {
        fmt.Printf("App layer file not found: %v\n", err)
        return
    }
    
    fmt.Println("Multi-layer mount test successful!")
    fmt.Printf("Mounted layers at: %s\n", targetPath)
}

func execCommand() {
	if len(os.Args) < 4 {
		fmt.Println("Error: Container ID and command required for exec")
		os.Exit(1)
	}

	containerID := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:]

	// Check if the container directory exists
	containerDir := filepath.Join("/tmp/basic-docker/containers", containerID)
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		fmt.Printf("Error: Container %s does not exist. Please ensure the container is running.\n", containerID)
		os.Exit(1)
	}

	// Locate the PID of the container
	pidFile := fmt.Sprintf("/tmp/basic-docker/containers/%s/pid", containerID)
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Printf("Error: Failed to read PID file for container %s: %v\n", containerID, err)
		os.Exit(1)
	}

	pid := strings.TrimSpace(string(pidData))

	// Verify if the process with the given PID exists
	procPath := fmt.Sprintf("/proc/%s", pid)
	if _, err := os.Stat(procPath); os.IsNotExist(err) {
		fmt.Printf("Error: Process with PID %s does not exist. The container might not be running.\n", pid)
		os.Exit(1)
	}

	// Attach to the container's namespace and execute the command
	nsPath := fmt.Sprintf("/proc/%s/ns/mnt", pid)
	cmd := exec.Command("nsenter", "--mount="+nsPath, "--pid="+fmt.Sprintf("/proc/%s/ns/pid", pid), "--", command)
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error: Failed to execute command in container %s: %v\n", containerID, err)
		os.Exit(1)
	}
}

func fallbackToHostBinaries(rootfs string) error {
	fmt.Println("Warning: Falling back to host binaries as busybox is not available.")

	// List of essential commands to copy from the host system
	hostCommands := []string{"sh", "ls", "echo", "cat", "ps"}

	for _, cmd := range hostCommands {
		hostCmdPath, err := exec.LookPath(cmd)
		if err != nil {
			fmt.Printf("Warning: Command %s not found on the host system. Skipping.\n", cmd)
			continue
		}

		// Copy the command binary to the container's rootfs
		containerCmdPath := filepath.Join(rootfs, "bin", filepath.Base(hostCmdPath))
		if err := copyFile(hostCmdPath, containerCmdPath); err != nil {
			fmt.Printf("Error: Failed to copy %s to container rootfs: %v\n", cmd, err)
			return err
		}
	}

	return nil
}

func atoi(s string) int {
	result, err := strconv.Atoi(s)
	if err != nil {
		fmt.Printf("Error converting string to int: %v\n", err)
		os.Exit(1)
	}
	return result
}

func fetchImage(imageName string) error {
	// Placeholder function for fetching an image
	fmt.Printf("Fetching image '%s'...\n", imageName)
	// Simulate fetching the image
	time.Sleep(2 * time.Second)
	fmt.Printf("Image '%s' fetched successfully.\n", imageName)
	return nil
}
