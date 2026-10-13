package main

import (
	"path/filepath"
	"testing"
)

func newTestCapsuleStore(t *testing.T) *CapsuleStore {
	t.Helper()
	oldBaseDir := baseDir
	baseDir = t.TempDir()
	t.Cleanup(func() { baseDir = oldBaseDir })

	store := NewCapsuleStore()
	if err := store.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	return store
}

func TestCapsuleLifecycle(t *testing.T) {
	store := newTestCapsuleStore(t)

	capsule, err := store.Create("research", "1.0", "janardhan")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if capsule.Status != CapsuleActive {
		t.Fatalf("expected active capsule, got %q", capsule.Status)
	}

	if err := store.Attach(capsule.ID, "container-1", "container"); err != nil {
		t.Fatalf("Attach() failed: %v", err)
	}

	if err := store.Delete(capsule.ID); err == nil {
		t.Fatal("expected deletion to fail while a member is attached")
	}

	if err := store.Detach(capsule.ID, "container-1"); err != nil {
		t.Fatalf("Detach() failed: %v", err)
	}

	if err := store.Delete(capsule.ID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	if _, err := store.Get(capsule.ID); err == nil {
		t.Fatal("expected capsule to be absent after deletion")
	}
}

func TestCapsuleStorePersistsMembers(t *testing.T) {
	base := t.TempDir()
	oldBaseDir := baseDir
	baseDir = base
	t.Cleanup(func() { baseDir = oldBaseDir })

	store := NewCapsuleStore()
	if err := store.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	capsule, err := store.Create("research", "1.0", "janardhan")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if err := store.Attach(capsule.ID, "container-1", "container"); err != nil {
		t.Fatalf("Attach() failed: %v", err)
	}

	reloaded := NewCapsuleStore()
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload Load() failed: %v", err)
	}

	got, err := reloaded.Get(capsule.ID)
	if err != nil {
		t.Fatalf("Get() after reload failed: %v", err)
	}
	if got.Members["container-1"] != "container" {
		t.Fatalf("expected persisted member, got %#v", got.Members)
	}
	if _, err := filepath.Abs(base); err != nil {
		t.Fatalf("temp base dir invalid: %v", err)
	}
}
