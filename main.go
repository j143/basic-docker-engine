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
	
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}