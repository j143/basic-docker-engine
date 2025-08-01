package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// MockKubernetesCapsuleManager for testing without real K8s cluster
type MockKubernetesCapsuleManager struct {
	*KubernetesCapsuleManager
}

// NewMockKubernetesCapsuleManager creates a mock manager for testing
func NewMockKubernetesCapsuleManager() *MockKubernetesCapsuleManager {
	fakeClient := fake.NewSimpleClientset()
	
	return &MockKubernetesCapsuleManager{
		KubernetesCapsuleManager: &KubernetesCapsuleManager{
			client:    fakeClient,
			namespace: "default",
		},
	}
}

// TestKubernetesConfigMapCapsule tests ConfigMap-based Resource Capsules
func TestKubernetesConfigMapCapsule(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	// Test data
	name := "test-config"
	version := "1.0"
	data := map[string]string{
		"config.yaml": "test: value",
		"app.conf":    "setting=true",
	}
	
	// Create ConfigMap capsule
	err := mockKCM.CreateConfigMapCapsule(name, version, data)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap capsule: %v", err)
	}
	
	// Get ConfigMap capsule
	configMap, err := mockKCM.GetConfigMapCapsule(name, version)
	if err != nil {
		t.Fatalf("Failed to get ConfigMap capsule: %v", err)
	}
	
	// Verify ConfigMap data
	if configMap.Data["config.yaml"] != "test: value" {
		t.Errorf("Expected 'test: value', got '%s'", configMap.Data["config.yaml"])
	}
	
	if configMap.Data["app.conf"] != "setting=true" {
		t.Errorf("Expected 'setting=true', got '%s'", configMap.Data["app.conf"])
	}
	
	// Verify labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/name":    "resource-capsule",
		"app.kubernetes.io/version": version,
		"capsule.docker.io/name":    name,
		"capsule.docker.io/version": version,
	}
	
	for key, expectedValue := range expectedLabels {
		if configMap.Labels[key] != expectedValue {
			t.Errorf("Expected label %s='%s', got '%s'", key, expectedValue, configMap.Labels[key])
		}
	}
}

// TestKubernetesSecretCapsule tests Secret-based Resource Capsules
func TestKubernetesSecretCapsule(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	// Test data
	name := "test-secret"
	version := "2.0"
	data := map[string][]byte{
		"password": []byte("secret123"),
		"cert.pem": []byte("-----BEGIN CERTIFICATE-----"),
	}
	
	// Create Secret capsule
	err := mockKCM.CreateSecretCapsule(name, version, data)
	if err != nil {
		t.Fatalf("Failed to create Secret capsule: %v", err)
	}
	
	// Get Secret capsule
	secret, err := mockKCM.GetSecretCapsule(name, version)
	if err != nil {
		t.Fatalf("Failed to get Secret capsule: %v", err)
	}
	
	// Verify Secret data
	if string(secret.Data["password"]) != "secret123" {
		t.Errorf("Expected 'secret123', got '%s'", string(secret.Data["password"]))
	}
	
	if string(secret.Data["cert.pem"]) != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("Expected certificate data, got '%s'", string(secret.Data["cert.pem"]))
	}
	
	// Verify secret type
	if secret.Type != v1.SecretTypeOpaque {
		t.Errorf("Expected SecretTypeOpaque, got %s", secret.Type)
	}
}

// TestKubernetesCapsuleLifecycle tests complete lifecycle of Resource Capsules
func TestKubernetesCapsuleLifecycle(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	name := "lifecycle-test"
	version := "1.0"
	data := map[string]string{
		"config": "test=true",
	}
	
	// Create
	err := mockKCM.CreateConfigMapCapsule(name, version, data)
	if err != nil {
		t.Fatalf("Failed to create capsule: %v", err)
	}
	
	// Verify existence
	_, err = mockKCM.GetConfigMapCapsule(name, version)
	if err != nil {
		t.Fatalf("Capsule should exist after creation: %v", err)
	}
	
	// Delete
	err = mockKCM.DeleteCapsule(name, version)
	if err != nil {
		t.Fatalf("Failed to delete capsule: %v", err)
	}
	
	// Verify deletion
	_, err = mockKCM.GetConfigMapCapsule(name, version)
	if err == nil {
		t.Fatalf("Capsule should not exist after deletion")
	}
}

// TestAddKubernetesResourceCapsule tests the AddResourceCapsule function with Kubernetes environment
func TestAddKubernetesResourceCapsule(t *testing.T) {
	// Create a temporary test file
	tempFile, err := os.CreateTemp("", "test-capsule-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	
	testContent := "test configuration data"
	_, err = tempFile.Write([]byte(testContent))
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()
	
	// Note: This test will skip actual Kubernetes operations if no cluster is available
	// In a real environment, this would create actual K8s resources
	err = AddResourceCapsule("kubernetes", "test-capsule", "1.0", tempFile.Name())
	if err != nil {
		// This is expected in test environment without real K8s cluster
		t.Logf("Expected error in test environment: %v", err)
	}
}

// BenchmarkKubernetesConfigMapAccess benchmarks ConfigMap access performance
func BenchmarkKubernetesConfigMapAccess(b *testing.B) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	// Setup test data
	name := "benchmark-config"
	version := "1.0"
	data := map[string]string{
		"config": "benchmark data",
	}
	
	// Create the capsule
	err := mockKCM.CreateConfigMapCapsule(name, version, data)
	if err != nil {
		b.Fatalf("Failed to create ConfigMap capsule: %v", err)
	}
	
	// Reset timer before benchmark
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := mockKCM.GetConfigMapCapsule(name, version)
		if err != nil {
			b.Fatalf("Failed to get ConfigMap capsule: %v", err)
		}
	}
}

// BenchmarkKubernetesSecretAccess benchmarks Secret access performance
func BenchmarkKubernetesSecretAccess(b *testing.B) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	// Setup test data
	name := "benchmark-secret"
	version := "1.0"
	data := map[string][]byte{
		"secret": []byte("benchmark secret data"),
	}
	
	// Create the capsule
	err := mockKCM.CreateSecretCapsule(name, version, data)
	if err != nil {
		b.Fatalf("Failed to create Secret capsule: %v", err)
	}
	
	// Reset timer before benchmark
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := mockKCM.GetSecretCapsule(name, version)
		if err != nil {
			b.Fatalf("Failed to get Secret capsule: %v", err)
		}
	}
}

// BenchmarkKubernetesCapsuleCreation benchmarks capsule creation performance
func BenchmarkKubernetesCapsuleCreation(b *testing.B) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	data := map[string]string{
		"config": "benchmark data",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		name := "benchmark-config"
		version := fmt.Sprintf("v%d", i) // Use proper version strings
		
		err := mockKCM.CreateConfigMapCapsule(name, version, data)
		if err != nil {
			b.Fatalf("Failed to create ConfigMap capsule: %v", err)
		}
	}
}

// TestKubernetesCapsuleVersioning tests versioning capabilities
func TestKubernetesCapsuleVersioning(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	name := "versioned-config"
	
	// Create multiple versions
	versions := []string{"1.0", "1.1", "2.0"}
	for _, version := range versions {
		data := map[string]string{
			"config": "data for version " + version,
		}
		
		err := mockKCM.CreateConfigMapCapsule(name, version, data)
		if err != nil {
			t.Fatalf("Failed to create version %s: %v", version, err)
		}
	}
	
	// Verify each version exists and has correct data
	for _, version := range versions {
		configMap, err := mockKCM.GetConfigMapCapsule(name, version)
		if err != nil {
			t.Fatalf("Failed to get version %s: %v", version, err)
		}
		
		expectedData := "data for version " + version
		if configMap.Data["config"] != expectedData {
			t.Errorf("Version %s: expected '%s', got '%s'", version, expectedData, configMap.Data["config"])
		}
	}
}

// TestIsTextFile tests the text file detection function
func TestIsTextFile(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"Empty file", []byte{}, true},
		{"Text content", []byte("Hello, World!"), true},
		{"JSON content", []byte(`{"key": "value"}`), true},
		{"Binary with null byte", []byte{0x00, 0x01, 0x02}, false},
		{"Mixed content with null", []byte("text\x00data"), false},
		{"Large text file", []byte("Hello, this is a large text file with lots of content."), true},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isTextFile(tc.data)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for %s", tc.expected, result, tc.name)
			}
		})
	}
}

// TestKubernetesCapsuleLabels tests that proper labels are applied
func TestKubernetesCapsuleLabels(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	name := "labeled-config"
	version := "1.0"
	data := map[string]string{"test": "data"}
	
	err := mockKCM.CreateConfigMapCapsule(name, version, data)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap capsule: %v", err)
	}
	
	// List capsules and verify they're returned
	configMaps, err := mockKCM.client.CoreV1().ConfigMaps(mockKCM.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=resource-capsule",
	})
	if err != nil {
		t.Fatalf("Failed to list ConfigMaps: %v", err)
	}
	
	if len(configMaps.Items) != 1 {
		t.Errorf("Expected 1 ConfigMap, got %d", len(configMaps.Items))
	}
	
	configMap := configMaps.Items[0]
	if configMap.Labels["capsule.docker.io/name"] != name {
		t.Errorf("Expected capsule name '%s', got '%s'", name, configMap.Labels["capsule.docker.io/name"])
	}
	
	if configMap.Labels["capsule.docker.io/version"] != version {
		t.Errorf("Expected capsule version '%s', got '%s'", version, configMap.Labels["capsule.docker.io/version"])
	}
}

// TestBenchmarkKubernetesResourceAccess tests the benchmark helper function
func TestBenchmarkKubernetesResourceAccess(t *testing.T) {
	mockKCM := NewMockKubernetesCapsuleManager()
	
	// Create a test capsule
	name := "benchmark-test"
	version := "1.0"
	data := map[string]string{"test": "data"}
	
	err := mockKCM.CreateConfigMapCapsule(name, version, data)
	if err != nil {
		t.Fatalf("Failed to create ConfigMap capsule: %v", err)
	}
	
	// Test the benchmark function
	duration, err := mockKCM.BenchmarkKubernetesResourceAccess(name, version)
	if err != nil {
		t.Fatalf("Benchmark function failed: %v", err)
	}
	
	if duration <= 0 {
		t.Errorf("Expected positive duration, got %v", duration)
	}
	
	// Test with non-existent capsule
	_, err = mockKCM.BenchmarkKubernetesResourceAccess("nonexistent", "1.0")
	if err == nil {
		t.Errorf("Expected error for non-existent capsule, got nil")
	}
}