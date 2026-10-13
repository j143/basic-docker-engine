package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const networksFile = "networks.json"

// Network represents a container network owned by a Capsule.
type Network struct {
	Name       string            `json:"name"`
	ID         string            `json:"id"`
	CapsuleID  string            `json:"capsule_id"`
	Containers map[string]string `json:"containers"`
}

var networks = []Network{}
var capsuleStore = NewCapsuleStore()

func loadNetworks() {
	filePath := filepath.Join(baseDir, networksFile)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
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

func saveNetworks() error {
	filePath := filepath.Join(baseDir, networksFile)
	data, err := json.MarshalIndent(networks, "", "  ")
	if err != nil {
		return fmt.Errorf("encode networks: %w", err)
	}

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write networks: %w", err)
	}
	if err := os.Rename(tmp, filePath); err != nil {
		return fmt.Errorf("replace networks: %w", err)
	}
	return nil
}

// CreateNetwork creates a network and a Capsule that owns it.
func CreateNetwork(name string) {
	capsule, err := capsuleStore.Create(name, "1.0", "local")
	if err != nil {
		fmt.Printf("Error creating network capsule: %v\n", err)
		return
	}

	id := fmt.Sprintf("net-%d", time.Now().UnixNano())
	network := Network{
		Name:       name,
		ID:         id,
		CapsuleID:  capsule.ID,
		Containers: make(map[string]string),
	}
	networks = append(networks, network)
	capsule.Resources["network"] = id

	if err := saveNetworks(); err != nil {
		fmt.Printf("Error saving networks: %v\n", err)
		return
	}

	fmt.Printf("Network %s created and owned by capsule %s\n", id, capsule.ID)
}

// ListNetworks lists networks and their owning Capsules.
func ListNetworks() {
	fmt.Println("Available Networks:")
	for _, network := range networks {
		fmt.Printf("- %s (ID: %s, Capsule: %s)\n", network.Name, network.ID, network.CapsuleID)
	}
}

// DeleteNetwork deletes a network and its owning Capsule. It refuses to
// delete either while containers remain attached.
func DeleteNetwork(id string) {
	for i, network := range networks {
		if network.ID != id {
			continue
		}

		if err := capsuleStore.Delete(network.CapsuleID); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		networks = append(networks[:i], networks[i+1:]...)
		if err := saveNetworks(); err != nil {
			fmt.Printf("Error saving networks: %v\n", err)
			return
		}

		fmt.Printf("Network %s and its Capsule deleted\n", id)
		return
	}
	fmt.Printf("Network with ID %s not found\n", id)
}

// AttachContainerToNetwork attaches a container to a network and records it
// as a member of the network's Capsule.
func AttachContainerToNetwork(networkID, containerID string) error {
	for i, network := range networks {
		if network.ID != networkID {
			continue
		}

		if _, exists := network.Containers[containerID]; exists {
			return errors.New("container is already attached to the network")
		}

		if err := capsuleStore.Attach(network.CapsuleID, containerID, "container"); err != nil {
			return err
		}

		ipAddress := fmt.Sprintf("192.168.%d.%d", i+1, len(network.Containers)+2)
		networks[i].Containers[containerID] = ipAddress
		if err := saveNetworks(); err != nil {
			_ = capsuleStore.Detach(network.CapsuleID, containerID)
			return err
		}

		fmt.Printf("Container %s attached to network %s with IP %s\n", containerID, networkID, ipAddress)
		return nil
	}
	return errors.New("network not found")
}

// DetachContainerFromNetwork removes a container from a network and its
// owning Capsule.
func DetachContainerFromNetwork(networkID, containerID string) error {
	for i, network := range networks {
		if network.ID != networkID {
			continue
		}

		if _, exists := network.Containers[containerID]; !exists {
			return errors.New("container not found in the network")
		}

		if err := capsuleStore.Detach(network.CapsuleID, containerID); err != nil {
			return err
		}

		delete(networks[i].Containers, containerID)
		if err := saveNetworks(); err != nil {
			return err
		}

		fmt.Printf("Container %s detached from network %s\n", containerID, networkID)
		return nil
	}
	return errors.New("network not found")
}

// Ping reports whether two containers are members of the same network.
// It does not yet perform a real network probe.
func Ping(networkID, sourceContainerID, targetContainerID string) error {
	for _, network := range networks {
		if network.ID != networkID {
			continue
		}

		sourceIP, sourceExists := network.Containers[sourceContainerID]
		targetIP, targetExists := network.Containers[targetContainerID]
		if !sourceExists || !targetExists {
			return errors.New("one or both containers are not in the network")
		}

		fmt.Printf("Pinging from %s to %s: Success\n", sourceIP, targetIP)
		return nil
	}
	return errors.New("network not found")
}

func init() {
	loadNetworks()
	if err := capsuleStore.Load(); err != nil {
		fmt.Printf("Error loading capsules: %v\n", err)
	}
}
