package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CapsuleStatus represents the lifecycle state of a Capsule.
type CapsuleStatus string

const (
	CapsuleActive      CapsuleStatus = "active"
	CapsuleTerminating CapsuleStatus = "terminating"
	CapsuleTerminated  CapsuleStatus = "terminated"
)

// Capsule is the central ownership and lifecycle boundary for resources
// managed by the engine. A Capsule owns resources, tracks active members,
// and cannot be deleted while members remain attached.
type Capsule struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Status    CapsuleStatus     `json:"status"`
	Owner     string            `json:"owner,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	Resources map[string]string `json:"resources,omitempty"`
	Members   map[string]string `json:"members,omitempty"`
}

// CapsuleStore provides concurrency-safe persistence and lifecycle operations
// for Capsules.
type CapsuleStore struct {
	mu       sync.RWMutex
	capsules map[string]*Capsule
	file     string
}

// NewCapsuleStore creates a CapsuleStore backed by baseDir/capsules.json.
func NewCapsuleStore() *CapsuleStore {
	return &CapsuleStore{
		capsules: make(map[string]*Capsule),
		file:     filepath.Join(baseDir, "capsules.json"),
	}
}

// Load reads persisted Capsules. A missing file is not an error.
func (s *CapsuleStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read capsule store: %w", err)
	}

	var capsules []*Capsule
	if err := json.Unmarshal(data, &capsules); err != nil {
		return fmt.Errorf("decode capsule store: %w", err)
	}

	s.capsules = make(map[string]*Capsule, len(capsules))
	for _, capsule := range capsules {
		s.capsules[capsule.ID] = capsule
	}
	return nil
}

func (s *CapsuleStore) persistLocked() error {
	capsules := make([]*Capsule, 0, len(s.capsules))
	for _, capsule := range s.capsules {
		capsules = append(capsules, capsule)
	}

	data, err := json.MarshalIndent(capsules, "", "  ")
	if err != nil {
		return fmt.Errorf("encode capsule store: %w", err)
	}

	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write capsule store: %w", err)
	}
	if err := os.Rename(tmp, s.file); err != nil {
		return fmt.Errorf("replace capsule store: %w", err)
	}
	return nil
}

// Create creates an active Capsule.
func (s *CapsuleStore) Create(name, version, owner string) (*Capsule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		return nil, errors.New("capsule name is required")
	}

	capsule := &Capsule{
		ID:        fmt.Sprintf("cap-%d", time.Now().UnixNano()),
		Name:      name,
		Version:   version,
		Status:    CapsuleActive,
		Owner:     owner,
		CreatedAt: time.Now().UTC(),
		Resources: make(map[string]string),
		Members:   make(map[string]string),
	}

	s.capsules[capsule.ID] = capsule
	if err := s.persistLocked(); err != nil {
		delete(s.capsules, capsule.ID)
		return nil, err
	}
	return capsule, nil
}

// Get returns a Capsule by ID.
func (s *CapsuleStore) Get(id string) (*Capsule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	capsule, ok := s.capsules[id]
	if !ok {
		return nil, fmt.Errorf("capsule %q not found", id)
	}
	return capsule, nil
}

// Attach records a member, such as a container, in a Capsule.
func (s *CapsuleStore) Attach(capsuleID, memberID, memberType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	capsule, ok := s.capsules[capsuleID]
	if !ok {
		return fmt.Errorf("capsule %q not found", capsuleID)
	}
	if capsule.Status != CapsuleActive {
		return fmt.Errorf("capsule %q is not active", capsuleID)
	}
	if memberID == "" || memberType == "" {
		return errors.New("capsule member ID and type are required")
	}

	capsule.Members[memberID] = memberType
	return s.persistLocked()
}

// Detach removes a member from a Capsule.
func (s *CapsuleStore) Detach(capsuleID, memberID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	capsule, ok := s.capsules[capsuleID]
	if !ok {
		return fmt.Errorf("capsule %q not found", capsuleID)
	}
	if _, exists := capsule.Members[memberID]; !exists {
		return fmt.Errorf("member %q is not attached to capsule %q", memberID, capsuleID)
	}

	delete(capsule.Members, memberID)
	return s.persistLocked()
}

// Delete removes a Capsule only after all members have been detached.
func (s *CapsuleStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	capsule, ok := s.capsules[id]
	if !ok {
		return fmt.Errorf("capsule %q not found", id)
	}
	if len(capsule.Members) != 0 {
		return fmt.Errorf(
			"capsule %q has %d active member(s); detach them before deletion",
			id, len(capsule.Members),
		)
	}

	capsule.Status = CapsuleTerminating
	delete(s.capsules, id)
	return s.persistLocked()
}
