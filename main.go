package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

var baseDir = filepath.Join(os.TempDir(), "basic-docker")
var imagesDir = filepath.Join(baseDir, "images")
var layersDir = filepath.Join(baseDir, "layers")

// Define the ImageLayer type
type ImageLayer struct {
	ID            string
	Created       time.Time
	Size          int64
	BaseLayerPath string
	AppLayerPath  string
}

// ResourceCapsule represents a self-contained, versioned resource unit (legacy)
type ResourceCapsule struct {
	Name    string
	Version string
	Path    string
}

// CapsuleManager handles the lifecycle of Resource Capsules.
type CapsuleManager struct {
	Capsules map[string]ResourceCapsule
}

// NewCapsuleManager initializes a new CapsuleManager.
func NewCapsuleManager() *CapsuleManager {
	return &CapsuleManager{
		Capsules: make(map[string]ResourceCapsule),
	}
}

// AddCapsule adds a new Resource Capsule to the manager.
func (cm *CapsuleManager) AddCapsule(name, version, path string) {
	key := name + ":" + version
	cm.Capsules[key] = ResourceCapsule{Name: name, Version: version, Path: path}
}

// GetCapsule retrieves a Resource Capsule by name and version.
func (cm *CapsuleManager) GetCapsule(name, version string) (ResourceCapsule, bool) {
	key := name + ":" + version
	capsule, exists := cm.Capsules[key]
	return capsule, exists
}

// AttachCapsule attaches a capsule to a container.
func (cm *CapsuleManager) AttachCapsule(containerID, name, version string) error {
	key := name + ":" + version

	capsule, exists := cm.Capsules[key]

	if !exists {
		return fmt.Errorf("capsule %s:%s not found", name, version)
	}
	// Logic to attach the capsule to the container's filesystem.
	fmt.Printf("Attaching capsule %s:%s to container %s\n", name, version, containerID)

	// Simulate the attachment by creating a symbolic link in the container's directory
	containerDir := filepath.Join(baseDir, "containers", containerID)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return fmt.Errorf("failed to create container directory: %v", err)
	}
	linkPath := filepath.Join(containerDir, name+"-"+version)

	// If the symbolic link already exists, remove it
	if _, err := os.Lstat(linkPath); err == nil {
		if err := os.Remove(linkPath); err != nil {
			return fmt.Errorf("failed to remove existing symbolic link for capsule: %v", err)
		}
	}

	if err := os.Symlink(capsule.Path, linkPath); err != nil {
		return fmt.Errorf("failed to create symbolic link for capsule: %v", err)
	}

	return nil
}

// AddResourceCapsule selectively adds a resource capsule to the environment and verifies it by interacting with a Docker container or Kubernetes cluster.
func AddResourceCapsule(env string, capsuleName string, capsuleVersion string, capsulePath string) error {
	switch env {
	case "docker":
		return addDockerResourceCapsule(capsuleName, capsuleVersion, capsulePath)
	case "kubernetes", "k8s":
		return addKubernetesResourceCapsule(capsuleName, capsuleVersion, capsulePath)
	default:
		return fmt.Errorf("unsupported environment: %s", env)
	}
}

// addDockerResourceCapsule handles Docker-specific resource capsule logic
func addDockerResourceCapsule(capsuleName, capsuleVersion, capsulePath string) error {
	// Docker-specific logic: Bind mount the capsule to a container
	containerDir := filepath.Join(baseDir, "containers")
	capsuleTargetPath := filepath.Join(containerDir, capsuleName+"-"+capsuleVersion)

	// Ensure the capsule path exists
	if _, err := os.Stat(capsulePath); os.IsNotExist(err) {
		return fmt.Errorf("capsule path does not exist: %s", capsulePath)
	}

	// Create a symbolic link to simulate binding the capsule
	if err := os.Symlink(capsulePath, capsuleTargetPath); err != nil {
		return fmt.Errorf("failed to bind capsule in Docker: %v", err)
	}

	// Log interaction with Docker
	fmt.Printf("[Docker] Capsule %s:%s added at %s\n", capsuleName, capsuleVersion, capsuleTargetPath)

	// Create a temporary Docker container to verify the capsule
	containerName := "test-container-" + capsuleName
	cmd := exec.Command("docker", "run", "--name", containerName, "-v", capsuleTargetPath+":"+capsuleTargetPath, "busybox", "ls", capsuleTargetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to verify capsule in Docker container: %v, output: %s", err, string(output))
	}

	fmt.Printf("[Docker] Verification output:\n%s\n", string(output))

	 // Show docker ps output
	psCmd := exec.Command("docker", "ps", "-a")
	psOutput, psErr := psCmd.CombinedOutput()
	if psErr != nil {
		fmt.Printf("[Docker] Failed to fetch 'docker ps' output: %v\n", psErr)
	} else {
		fmt.Printf("[Docker] 'docker ps' output:\n%s\n", string(psOutput))
	}

	// Show docker inspect output for the container
	inspectCmd := exec.Command("docker", "inspect", containerName)
	inspectOutput, inspectErr := inspectCmd.CombinedOutput()
	if inspectErr != nil {
		fmt.Printf("[Docker] Failed to fetch 'docker inspect' output: %v\n", inspectErr)
	} else {
		fmt.Printf("[Docker] 'docker inspect' output:\n%s\n", string(inspectOutput))
	}

	fmt.Printf("Successfully added and verified resource capsule %s:%s in Docker environment\n", capsuleName, capsuleVersion)
	return nil
}

// addKubernetesResourceCapsule handles Kubernetes-specific resource capsule logic
func addKubernetesResourceCapsule(capsuleName, capsuleVersion, capsulePath string) error {
	// Create a Kubernetes capsule manager
	kcm, err := NewKubernetesCapsuleManager("default")
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes capsule manager: %v", err)
	}

	// Read the capsule data
	capsuleData, err := os.ReadFile(capsulePath)
	if err != nil {
		return fmt.Errorf("failed to read capsule file: %v", err)
	}

	// Determine if we should create a ConfigMap or Secret based on the file content
	// For this example, we'll create a ConfigMap if it's text data, Secret if binary
	isTextData := isTextFile(capsuleData)
	
	if isTextData {
		// Create as ConfigMap
		data := map[string]string{
			filepath.Base(capsulePath): string(capsuleData),
		}
		err = kcm.CreateConfigMapCapsule(capsuleName, capsuleVersion, data)
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap capsule: %v", err)
		}

		// Verify the capsule was created
		configMap, err := kcm.GetConfigMapCapsule(capsuleName, capsuleVersion)
		if err != nil {
			return fmt.Errorf("failed to verify ConfigMap capsule: %v", err)
		}
		fmt.Printf("[Kubernetes] ConfigMap capsule verified: %s (keys: %v)\n", configMap.Name, getKeys(configMap.Data))
	} else {
		// Create as Secret
		data := map[string][]byte{
			filepath.Base(capsulePath): capsuleData,
		}
		err = kcm.CreateSecretCapsule(capsuleName, capsuleVersion, data)
		if err != nil {
			return fmt.Errorf("failed to create Secret capsule: %v", err)
		}

		// Verify the capsule was created
		secret, err := kcm.GetSecretCapsule(capsuleName, capsuleVersion)
		if err != nil {
			return fmt.Errorf("failed to verify Secret capsule: %v", err)
		}
		fmt.Printf("[Kubernetes] Secret capsule verified: %s (keys: %v)\n", secret.Name, getKeysBytes(secret.Data))
	}

	fmt.Printf("Successfully added and verified resource capsule %s:%s in Kubernetes environment\n", capsuleName, capsuleVersion)
	return nil
}

// isTextFile determines if the data is likely text (not binary)
func isTextFile(data []byte) bool {
	// Simple heuristic: if first 512 bytes contain no null bytes and are mostly printable, consider it text
	if len(data) == 0 {
		return true
	}
	
	sample := data
	if len(data) > 512 {
		sample = data[:512]
	}
	
	for _, b := range sample {
		if b == 0 {
			return false // null byte suggests binary
		}
	}
	
	return true
}

// getKeys extracts keys from a string map
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getKeysBytes extracts keys from a byte map
func getKeysBytes(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// To initialize the directories
func initDirectories() error {
	dirs := []string{
		filepath.Join(baseDir, "containers"),
		imagesDir,
		layersDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
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
	case "network-create":
		if len(os.Args) < 3 {
			fmt.Println("Usage: basic-docker network-create <network-name>")
			return
		}
		CreateNetwork(os.Args[2])
	case "network-list":
		ListNetworks()
	case "network-delete":
		if len(os.Args) < 3 {
			fmt.Println("Usage: basic-docker network-delete <network-id>")
			return
		}
		DeleteNetwork(os.Args[2])
	case "network-attach":
		if len(os.Args) < 4 {
			fmt.Println("Usage: basic-docker network-attach <network-id> <container-id>")
			return
		}
		err := AttachContainerToNetwork(os.Args[2], os.Args[3])
		if err != nil {
			fmt.Printf("Error: %s\n", err)
		}
	case "network-detach":
		if len(os.Args) < 4 {
			fmt.Println("Usage: basic-docker network-detach <network-id> <container-id>")
			return
		}
		err := DetachContainerFromNetwork(os.Args[2], os.Args[3])
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return
		}
	case "network-ping":
		if len(os.Args) < 5 {
			fmt.Println("Usage: basic-docker network-ping <network-id> <source-container-id> <target-container-id>")
			return
		}
		err := Ping(os.Args[2], os.Args[3], os.Args[4])
		if err != nil {
			fmt.Printf("Error: %s\n", err)
		}
	case "load":
		if len(os.Args) < 3 {
			fmt.Println("Error: Tar file path required for load")
			os.Exit(1)
		}
		tarFilePath := os.Args[2]
		imageName := strings.TrimSuffix(filepath.Base(tarFilePath), ".tar")

		fmt.Printf("Loading image from '%s'...\n", tarFilePath)
		image, err := LoadImageFromTar(tarFilePath, imageName)
		if err != nil {
			fmt.Printf("Error: Failed to load image from '%s': %v\n", tarFilePath, err)
			os.Exit(1)
		}
		fmt.Printf("Image '%s' loaded successfully.\n", image.Name)
	case "image":
		if len(os.Args) < 3 {
			fmt.Println("Error: Subcommand required for image")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "rm":
			if len(os.Args) < 4 {
				fmt.Println("Error: Image name required for rm")
				os.Exit(1)
			}
			imageName := os.Args[3]
			imagePath := filepath.Join(imagesDir, imageName)

			if _, err := os.Stat(imagePath); os.IsNotExist(err) {
				fmt.Printf("Error: Image '%s' does not exist.\n", imageName)
				os.Exit(1)
			}

			if err := os.RemoveAll(imagePath); err != nil {
				fmt.Printf("Error: Failed to delete image '%s': %v\n", imageName, err)
				os.Exit(1)
			}

			fmt.Printf("Image '%s' deleted successfully.\n", imageName)
		default:
			fmt.Println("Error: Unknown subcommand for image")
			os.Exit(1)
		}
	case "k8s-capsule":
		if len(os.Args) < 3 {
			fmt.Println("Usage: basic-docker k8s-capsule <command>")
			fmt.Println("Commands: create, list, get, delete")
			os.Exit(1)
		}
		handleKubernetesCapsuleCommand()
	case "k8s-crd":
		if len(os.Args) < 3 {
			fmt.Println("Usage: basic-docker k8s-crd <command>")
			fmt.Println("Commands: create, list, get, delete, rollback")
			os.Exit(1)
		}
		handleKubernetesCRDCommand()
	case "capsule-benchmark":
		if len(os.Args) < 3 {
			fmt.Println("Usage: basic-docker capsule-benchmark <environment>")
			fmt.Println("Environments: docker, kubernetes")
			os.Exit(1)
		}
		handleCapsuleBenchmark(os.Args[2])
	default:
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
	fmt.Println("  basic-docker network-create <network-name>  Create a new network")
	fmt.Println("  basic-docker network-list                   List all networks")
	fmt.Println("  basic-docker network-delete <network-id>   Delete a network by ID")
	fmt.Println("  basic-docker network-attach <network-id> <container-id> Attach a container to a network")
	fmt.Println("  basic-docker network-detach <network-id> <container-id> Detach a container from a network")
	fmt.Println("  basic-docker network-ping <network-id> <source-container-id> <target-container-id> Test connectivity between containers")
	fmt.Println("  basic-docker load <tar-file-path>          Load an image from a tar file")
	fmt.Println("  basic-docker image rm <image-name>         Remove an image by name")
	fmt.Println("  basic-docker k8s-capsule <command>         Manage Kubernetes Resource Capsules")
	fmt.Println("  basic-docker k8s-crd <command>             Manage ResourceCapsule CRDs")
	fmt.Println("  basic-docker capsule-benchmark <env>       Benchmark Resource Capsules (docker|kubernetes)")
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
	if len(os.Args) < 3 {
		fmt.Println("Error: Image name required for run")
		os.Exit(1)
	}

	imageName := os.Args[2]
	imagePath := filepath.Join(imagesDir, imageName, "rootfs")

	// Check if the image exists locally
	if _, err := os.Stat(imagePath); err == nil {
		fmt.Printf("Using locally loaded image '%s'.\n", imageName)
	} else {
		fmt.Printf("Fetching image '%s' from registry...\n", imageName)
		// Extract registry URL and repository from image name
		parts := strings.SplitN(imageName, "/", 2)
		registryURL := "https://registry-1.docker.io/v2/" // Default to Docker Hub
		repo := imageName
		if len(parts) > 1 {
			registryURL = fmt.Sprintf("http://%s/v2/", parts[0])
			repo = parts[1]
		}

		registry := NewDockerHubRegistry(registryURL)
		image, err := Pull(registry, repo)
		if err != nil {
			fmt.Printf("Error: Failed to fetch image '%s': %v\n", imageName, err)
			os.Exit(1)
		}
		fmt.Printf("Image '%s' fetched successfully.\n", imageName)
		imagePath = image.RootFS
	}

	// Create rootfs for this container
	containerID := fmt.Sprintf("container-%d", time.Now().Unix())
	rootfs := filepath.Join(baseDir, "containers", containerID, "rootfs")

	if err := os.MkdirAll(rootfs, 0755); err != nil {
		fmt.Printf("Error: Failed to create rootfs for container '%s': %v\n", containerID, err)
		os.Exit(1)
	}

	if err := copyDir(imagePath, rootfs); err != nil {
		fmt.Printf("Error: Failed to copy rootfs for container '%s': %v\n", containerID, err)
		os.Exit(1)
	}

	fmt.Printf("Starting container %s\n", containerID)

	// Execute the command in the container
	if len(os.Args) < 4 {
		fmt.Println("Error: Command required for run")
		os.Exit(1)
	}

	command := os.Args[3]
	args := os.Args[4:]
	runWithoutNamespaces(containerID, rootfs, command, args)
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

	fmt.Printf("Verified: sh symlink correctly points to busybox at %s\n", filepath.Join(baseLayerPath, "bin/sh"))

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
			syscall.CLONE_NEWNS, // Mount isolation
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
	defer file.Close()
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %v", err)
	}

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
	pidFile := filepath.Join(baseDir, "containers", containerID, "pid")
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
	containerDir := filepath.Join(baseDir, "containers")
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
	imageDir := "/tmp/basic-docker/images"
	fmt.Println("IMAGE NAME\tSIZE\tCONTENT VERIFIED")

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
			imageName := entry.Name()
			rootfsPath := filepath.Join(imageDir, imageName, "rootfs")

			// Check if the rootfs contains content
			contentVerified := "No"
			totalSize := int64(0)
			if files, err := os.ReadDir(rootfsPath); err == nil && len(files) > 0 {
				contentVerified = "Yes"
				// Calculate the total size of the rootfs
				filepath.Walk(rootfsPath, func(_ string, info os.FileInfo, err error) error {
					if err == nil && !info.IsDir() {
						totalSize += info.Size()
					}
					return nil
				})
			}

			fmt.Printf("%s\t%d bytes\t%s\n", imageName, totalSize, contentVerified)
		}
	}
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
	baseLayerPath := filepath.Join(baseDir, "layers", baseLayerID)
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
	appLayerPath := filepath.Join(baseDir, "layers", appLayerID)
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
	targetPath := filepath.Join(baseDir, "test-mount")

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
	containerDir := filepath.Join(baseDir, "containers", containerID)
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		fmt.Printf("Error: Container %s does not exist. Please ensure the container is running.\n", containerID)
		os.Exit(1)
	}

	// Locate the PID of the container
	pidFile := filepath.Join(baseDir, "containers", containerID, "pid")
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

// handleKubernetesCapsuleCommand handles Kubernetes capsule-related CLI commands
func handleKubernetesCapsuleCommand() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: basic-docker k8s-capsule <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  create <name> <version> <file-path>  - Create a new Resource Capsule")
		fmt.Println("  list                                 - List all Resource Capsules")
		fmt.Println("  get <name> <version>                 - Get a specific Resource Capsule")
		fmt.Println("  delete <name> <version>              - Delete a Resource Capsule")
		os.Exit(1)
	}

	command := os.Args[3]
	
	kcm, err := NewKubernetesCapsuleManager("default")
	if err != nil {
		fmt.Printf("Error: Failed to create Kubernetes client: %v\n", err)
		fmt.Println("Make sure you have access to a Kubernetes cluster and kubectl is configured.")
		os.Exit(1)
	}

	switch command {
	case "create":
		if len(os.Args) < 7 {
			fmt.Println("Usage: basic-docker k8s-capsule create <name> <version> <file-path>")
			os.Exit(1)
		}
		name := os.Args[4]
		version := os.Args[5]
		filePath := os.Args[6]
		
		err := AddResourceCapsule("kubernetes", name, version, filePath)
		if err != nil {
			fmt.Printf("Error: Failed to create Kubernetes capsule: %v\n", err)
			os.Exit(1)
		}
		
	case "list":
		err := kcm.ListCapsules()
		if err != nil {
			fmt.Printf("Error: Failed to list capsules: %v\n", err)
			os.Exit(1)
		}
		
	case "get":
		if len(os.Args) < 6 {
			fmt.Println("Usage: basic-docker k8s-capsule get <name> <version>")
			os.Exit(1)
		}
		name := os.Args[4]
		version := os.Args[5]
		
		// Try ConfigMap first
		configMap, err := kcm.GetConfigMapCapsule(name, version)
		if err == nil {
			fmt.Printf("ConfigMap Capsule: %s:%s\n", name, version)
			fmt.Printf("Data keys: %v\n", getKeys(configMap.Data))
			return
		}
		
		// Try Secret
		secret, err := kcm.GetSecretCapsule(name, version)
		if err == nil {
			fmt.Printf("Secret Capsule: %s:%s\n", name, version)
			fmt.Printf("Data keys: %v\n", getKeysBytes(secret.Data))
			return
		}
		
		fmt.Printf("Error: Capsule %s:%s not found\n", name, version)
		os.Exit(1)
		
	case "delete":
		if len(os.Args) < 6 {
			fmt.Println("Usage: basic-docker k8s-capsule delete <name> <version>")
			os.Exit(1)
		}
		name := os.Args[4]
		version := os.Args[5]
		
		err := kcm.DeleteCapsule(name, version)
		if err != nil {
			fmt.Printf("Error: Failed to delete capsule: %v\n", err)
			os.Exit(1)
		}
		
	default:
		fmt.Printf("Error: Unknown command '%s'\n", command)
		os.Exit(1)
	}
}

// handleCapsuleBenchmark handles benchmarking commands
func handleCapsuleBenchmark(environment string) {
	switch environment {
	case "docker":
		runDockerCapsuleBenchmark()
	case "kubernetes", "k8s":
		runKubernetesCapsuleBenchmark()
	default:
		fmt.Printf("Error: Unsupported environment '%s'\n", environment)
		fmt.Println("Supported environments: docker, kubernetes")
		os.Exit(1)
	}
}

// runDockerCapsuleBenchmark runs benchmarks for Docker-based Resource Capsules
func runDockerCapsuleBenchmark() {
	fmt.Println("=== Docker Resource Capsule Benchmark ===")
	
	cm := NewCapsuleManager()
	cm.AddCapsule("benchmark-capsule", "1.0", "/tmp/benchmark-file")
	
	// Create a test file
	testFile := "/tmp/benchmark-file"
	err := os.WriteFile(testFile, []byte("benchmark data"), 0644)
	if err != nil {
		fmt.Printf("Error: Failed to create test file: %v\n", err)
		return
	}
	defer os.Remove(testFile)
	
	// Benchmark capsule access
	iterations := 10000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, exists := cm.GetCapsule("benchmark-capsule", "1.0")
		if !exists {
			fmt.Println("Error: Capsule not found during benchmark")
			return
		}
	}
	duration := time.Since(start)
	
	fmt.Printf("Docker Capsule Access: %d iterations in %v\n", iterations, duration)
	fmt.Printf("Average per operation: %v\n", duration/time.Duration(iterations))
}

// runKubernetesCapsuleBenchmark runs benchmarks for Kubernetes-based Resource Capsules
func runKubernetesCapsuleBenchmark() {
	fmt.Println("=== Kubernetes Resource Capsule Benchmark ===")
	
	kcm, err := NewKubernetesCapsuleManager("default")
	if err != nil {
		fmt.Printf("Error: Failed to create Kubernetes client: %v\n", err)
		return
	}
	
	// Create a test capsule
	testData := map[string]string{
		"benchmark-file": "benchmark data",
	}
	
	err = kcm.CreateConfigMapCapsule("benchmark-capsule", "1.0", testData)
	if err != nil {
		fmt.Printf("Error: Failed to create test capsule: %v\n", err)
		return
	}
	
	// Clean up after benchmark
	defer kcm.DeleteCapsule("benchmark-capsule", "1.0")
	
	// Benchmark capsule access
	iterations := 100 // Lower iterations for K8s API calls
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := kcm.BenchmarkKubernetesResourceAccess("benchmark-capsule", "1.0")
		if err != nil {
			fmt.Printf("Error during benchmark iteration %d: %v\n", i, err)
			return
		}
	}
	duration := time.Since(start)
	
	fmt.Printf("Kubernetes Capsule Access: %d iterations in %v\n", iterations, duration)
	fmt.Printf("Average per operation: %v\n", duration/time.Duration(iterations))
}

// handleKubernetesCRDCommand handles ResourceCapsule CRD-related CLI commands
func handleKubernetesCRDCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: basic-docker k8s-crd <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  create <name> <version> <file-path> [type]  Create a ResourceCapsule CRD")
		fmt.Println("  list                                        List all ResourceCapsule CRDs")
		fmt.Println("  get <name>                                  Get ResourceCapsule CRD details")
		fmt.Println("  delete <name>                               Delete a ResourceCapsule CRD")
		fmt.Println("  rollback <name> <previous-version>          Rollback a ResourceCapsule CRD")
		fmt.Println("  operator start [namespace]                  Start the ResourceCapsule operator")
		return
	}

	kcm, err := NewKubernetesCapsuleManager("")
	if err != nil {
		fmt.Printf("Error creating Kubernetes capsule manager: %v\n", err)
		return
	}

	command := os.Args[2]
	switch command {
	case "create":
		if len(os.Args) < 6 {
			fmt.Println("Usage: basic-docker k8s-crd create <name> <version> <file-path> [type]")
			return
		}
		name := os.Args[3]
		version := os.Args[4]
		filePath := os.Args[5]
		capsuleType := "configmap"
		if len(os.Args) >= 7 {
			capsuleType = os.Args[6]
		}

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			return
		}

		// Convert content to data map
		data := map[string]interface{}{
			"content": string(content),
		}

		err = kcm.CreateCRDCapsule(name, version, data, capsuleType)
		if err != nil {
			fmt.Printf("Error creating ResourceCapsule CRD: %v\n", err)
		}

	case "list":
		err := kcm.ListCRDCapsules()
		if err != nil {
			fmt.Printf("Error listing ResourceCapsule CRDs: %v\n", err)
		}

	case "get":
		if len(os.Args) < 4 {
			fmt.Println("Usage: basic-docker k8s-crd get <name>")
			return
		}
		name := os.Args[3]

		resourceCapsule, err := kcm.GetCRDCapsule(name)
		if err != nil {
			fmt.Printf("Error getting ResourceCapsule CRD: %v\n", err)
			return
		}

		fmt.Printf("ResourceCapsule CRD: %s\n", name)
		fmt.Printf("Namespace: %s\n", resourceCapsule.GetNamespace())
		
		spec, found, _ := unstructured.NestedMap(resourceCapsule.Object, "spec")
		if found {
			if version, found, _ := unstructured.NestedString(spec, "version"); found {
				fmt.Printf("Version: %s\n", version)
			}
			if capsuleType, found, _ := unstructured.NestedString(spec, "capsuleType"); found {
				fmt.Printf("Type: %s\n", capsuleType)
			}
		}
		
		status, found, _ := unstructured.NestedMap(resourceCapsule.Object, "status")
		if found {
			if phase, found, _ := unstructured.NestedString(status, "phase"); found {
				fmt.Printf("Status: %s\n", phase)
			}
			if message, found, _ := unstructured.NestedString(status, "message"); found {
				fmt.Printf("Message: %s\n", message)
			}
		}

	case "delete":
		if len(os.Args) < 4 {
			fmt.Println("Usage: basic-docker k8s-crd delete <name>")
			return
		}
		name := os.Args[3]

		err := kcm.DeleteCRDCapsule(name)
		if err != nil {
			fmt.Printf("Error deleting ResourceCapsule CRD: %v\n", err)
		}

	case "rollback":
		if len(os.Args) < 5 {
			fmt.Println("Usage: basic-docker k8s-crd rollback <name> <previous-version>")
			return
		}
		name := os.Args[3]
		previousVersion := os.Args[4]

		err := kcm.RollbackCRDCapsule(name, previousVersion)
		if err != nil {
			fmt.Printf("Error rolling back ResourceCapsule CRD: %v\n", err)
		}

	case "operator":
		if len(os.Args) < 4 {
			fmt.Println("Usage: basic-docker k8s-crd operator start [namespace]")
			return
		}
		subcommand := os.Args[3]
		if subcommand != "start" {
			fmt.Println("Usage: basic-docker k8s-crd operator start [namespace]")
			return
		}

		namespace := "default"
		if len(os.Args) >= 5 {
			namespace = os.Args[4]
		}

		operator, err := NewResourceCapsuleOperator(namespace)
		if err != nil {
			fmt.Printf("Error creating operator: %v\n", err)
			return
		}

		fmt.Println("Starting ResourceCapsule operator... (Press Ctrl+C to stop)")
		if err := operator.Start(); err != nil {
			fmt.Printf("Error starting operator: %v\n", err)
			return
		}

		// Keep the operator running
		select {}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: create, list, get, delete, rollback, operator")
	}
}
