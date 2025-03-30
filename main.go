package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	switch os.Args[1] {
	case "run":
		run()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
	}
}

func run() {
	// This is the process that will run inside our container
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	
	// Set up namespaces for isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // Hostname isolation
			syscall.CLONE_NEWPID | // Process ID isolation
			syscall.CLONE_NEWNS | // Mount isolation
			syscall.CLONE_NEWNET, // Network isolation
	}
	
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Create a new root filesystem
	cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNS
	
	// Set up filesystem
	must(setupContainerFs())

	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	
}

func setupContainerFs() error {
	// This is a simplified version
	// In a real implementation, you'd:
	// 1. Unpack an image to a directory
	// 2. Use pivot_root to change the root directory
	
	// Create a minimal filesystem structure
	rootfs := "/tmp/lean-docker-rootfs"
	os.MkdirAll(rootfs, 0755)
	
	// Mount proc filesystem
	if err := syscall.Mount("proc", rootfs+"/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount proc: %w", err)
	}
	
	// Chroot into the new filesystem
	if err := syscall.Chroot(rootfs); err != nil {
		return fmt.Errorf("failed to chroot: %w", err)
	}
	
	// Change working directory
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}
	
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
