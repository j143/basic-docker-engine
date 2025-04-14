package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const networksFile = "networks.json"

// Updated Network struct to include IP addresses for containers
type Network struct {
	Name       string
	ID         string
	Containers map[string]string // Map of container IDs to their IP addresses
}

var networks = []Network{}
var capsuleManager = NewCapsuleManager()

// loadNetworks loads the networks from the JSON file
func loadNetworks() {
	filePath := filepath.Join(baseDir, networksFile)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return // No networks file exists yet
		}
		fmt.Printf("Error loading networks: %v\n", err)
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&networks); err != nil {
		fmt.Printf("Error decoding networks: %v\n", err)
	}
}

// saveNetworks saves the networks to the JSON file
func saveNetworks() {
	filePath := filepath.Join(baseDir, networksFile)
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("Error saving networks: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(networks); err != nil {
		fmt.Printf("Error encoding networks: %v\n", err)
	}
}

// CreateNetwork creates a new network capsule
func CreateNetwork(name string) {
	id := fmt.Sprintf("net-%d", len(networks)+1)
	network := Network{Name: name, ID: id, Containers: make(map[string]string)}
	networks = append(networks, network)

	// Register the network as a resource capsule
	capsuleManager.AddCapsule(name, "1.0", id)
	saveNetworks()
	fmt.Printf("Network capsule %s created with ID %s\n", name, id)
}

// ListNetworks lists all networks
func ListNetworks() {
	fmt.Println("Available Networks:")
	for _, network := range networks {
		fmt.Printf("- %s (ID: %s)\n", network.Name, network.ID)
	}
}

// DeleteNetwork deletes a network by ID
func DeleteNetwork(id string) {
	for i, network := range networks {
		if network.ID == id {
			networks = append(networks[:i], networks[i+1:]...)
			saveNetworks()
			fmt.Printf("Network with ID %s deleted\n", id)
			return
		}
	}
	fmt.Printf("Network with ID %s not found\n", id)
}

// Updated AttachContainerToNetwork to assign IP addresses
func AttachContainerToNetwork(networkID, containerID string) error {
	for i, network := range networks {
		if network.ID == networkID {
			// Check if the container is already attached
			if _, exists := network.Containers[containerID]; exists {
				return errors.New("container is already attached to the network")
			}

			// Assign an IP address to the container
			ipAddress := fmt.Sprintf("192.168.%d.%d", i+1, len(network.Containers)+2)
			networks[i].Containers[containerID] = ipAddress
			saveNetworks()
			fmt.Printf("Container %s attached to network %s with IP %s\n", containerID, networkID, ipAddress)
			return nil
		}
	}
	return errors.New("network not found")
}

// DetachContainerFromNetwork detaches a container from a network capsule
func DetachContainerFromNetwork(networkID, containerID string) error {
	for i, network := range networks {
		if network.ID == networkID {
			// Find and remove the container
			if _, exists := network.Containers[containerID]; exists {
				delete(networks[i].Containers, containerID)
				saveNetworks()
				fmt.Printf("Container %s detached from network %s\n", containerID, networkID)
				return nil
			}
			return errors.New("container not found in the network")
		}
	}
	return errors.New("network not found")
}

// New Ping function to test connectivity between containers
func Ping(networkID, sourceContainerID, targetContainerID string) error {
	for _, network := range networks {
		if network.ID == networkID {
			sourceIP, sourceExists := network.Containers[sourceContainerID]
			targetIP, targetExists := network.Containers[targetContainerID]

			if !sourceExists || !targetExists {
				return errors.New("one or both containers are not in the network")
			}

			fmt.Printf("Pinging from %s to %s: Success\n", sourceIP, targetIP)
			return nil
		}
	}
	return errors.New("network not found")
}

// Call loadNetworks during initialization
func init() {
	loadNetworks()
}