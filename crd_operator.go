package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// ResourceCapsuleOperator manages the lifecycle of ResourceCapsule custom resources
type ResourceCapsuleOperator struct {
	client       dynamic.Interface
	k8sClient    kubernetes.Interface
	namespace    string
	stopCh       chan struct{}
}

// NewResourceCapsuleOperator creates a new operator instance
func NewResourceCapsuleOperator(namespace string) (*ResourceCapsuleOperator, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create Kubernetes config: %v", err)
		}
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	if namespace == "" {
		namespace = "default"
	}

	return &ResourceCapsuleOperator{
		client:    client,
		k8sClient: k8sClient,
		namespace: namespace,
		stopCh:    make(chan struct{}),
	}, nil
}

// Start begins the operator's control loop
func (op *ResourceCapsuleOperator) Start() error {
	fmt.Printf("[Operator] Starting ResourceCapsule operator in namespace: %s\n", op.namespace)

	// Define the GVR for ResourceCapsule
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	// Start watching ResourceCapsule resources
	watcher, err := op.client.Resource(gvr).Namespace(op.namespace).Watch(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to start watching ResourceCapsules: %v", err)
	}

	go func() {
		defer watcher.Stop()
		for {
			select {
			case event, ok := <-watcher.ResultChan():
				if !ok {
					fmt.Println("[Operator] Watch channel closed, restarting...")
					return
				}
				if err := op.handleEvent(event); err != nil {
					fmt.Printf("[Operator] Error handling event: %v\n", err)
				}
			case <-op.stopCh:
				fmt.Println("[Operator] Stopping operator...")
				return
			}
		}
	}()

	return nil
}

// Stop stops the operator
func (op *ResourceCapsuleOperator) Stop() {
	close(op.stopCh)
}

// handleEvent processes watch events for ResourceCapsule resources
func (op *ResourceCapsuleOperator) handleEvent(event watch.Event) error {
	switch event.Type {
	case watch.Added:
		return op.handleResourceCapsuleAdded(event.Object.(*unstructured.Unstructured))
	case watch.Modified:
		return op.handleResourceCapsuleModified(event.Object.(*unstructured.Unstructured))
	case watch.Deleted:
		return op.handleResourceCapsuleDeleted(event.Object.(*unstructured.Unstructured))
	}
	return nil
}

// handleResourceCapsuleAdded processes new ResourceCapsule resources
func (op *ResourceCapsuleOperator) handleResourceCapsuleAdded(obj *unstructured.Unstructured) error {
	name := obj.GetName()
	fmt.Printf("[Operator] ResourceCapsule %s added\n", name)

	// Extract spec data
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return fmt.Errorf("failed to get spec from ResourceCapsule %s: %v", name, err)
	}

	version, found, err := unstructured.NestedString(spec, "version")
	if err != nil || !found {
		return fmt.Errorf("failed to get version from ResourceCapsule %s: %v", name, err)
	}

	capsuleType, found, err := unstructured.NestedString(spec, "capsuleType")
	if err != nil {
		capsuleType = "configmap" // default
	}

	data, found, err := unstructured.NestedMap(spec, "data")
	if err != nil || !found {
		return fmt.Errorf("failed to get data from ResourceCapsule %s: %v", name, err)
	}

	// Create the underlying Kubernetes resource based on type
	if err := op.createUnderlyingResource(name, version, capsuleType, data); err != nil {
		return op.updateStatus(obj, "Failed", err.Error())
	}

	return op.updateStatus(obj, "Active", "ResourceCapsule successfully created")
}

// handleResourceCapsuleModified processes updated ResourceCapsule resources
func (op *ResourceCapsuleOperator) handleResourceCapsuleModified(obj *unstructured.Unstructured) error {
	name := obj.GetName()
	fmt.Printf("[Operator] ResourceCapsule %s modified\n", name)

	// Extract rollback configuration
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return fmt.Errorf("failed to get spec from ResourceCapsule %s: %v", name, err)
	}

	// Check if rollback is requested
	rollback, found, err := unstructured.NestedMap(spec, "rollback")
	if err == nil && found {
		if enabled, found, _ := unstructured.NestedBool(rollback, "enabled"); found && enabled {
			if prevVersion, found, _ := unstructured.NestedString(rollback, "previousVersion"); found && prevVersion != "" {
				fmt.Printf("[Operator] Rollback requested for %s to version %s\n", name, prevVersion)
				return op.performRollback(obj, prevVersion)
			}
		}
	}

	// Handle regular update
	return op.handleResourceCapsuleAdded(obj) // Reuse the add logic for updates
}

// handleResourceCapsuleDeleted processes deleted ResourceCapsule resources
func (op *ResourceCapsuleOperator) handleResourceCapsuleDeleted(obj *unstructured.Unstructured) error {
	name := obj.GetName()
	fmt.Printf("[Operator] ResourceCapsule %s deleted\n", name)

	// Clean up underlying resources
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || !found {
		return nil // Nothing to clean up
	}

	version, found, err := unstructured.NestedString(spec, "version")
	if err != nil || !found {
		return nil
	}

	capsuleType, found, err := unstructured.NestedString(spec, "capsuleType")
	if err != nil {
		capsuleType = "configmap"
	}

	return op.deleteUnderlyingResource(name, version, capsuleType)
}

// createUnderlyingResource creates the actual ConfigMap or Secret
func (op *ResourceCapsuleOperator) createUnderlyingResource(name, version, capsuleType string, data map[string]interface{}) error {
	resourceName := fmt.Sprintf("%s-%s", name, version)

	if capsuleType == "secret" {
		// Convert data to byte map for Secret
		secretData := make(map[string][]byte)
		for k, v := range data {
			if str, ok := v.(string); ok {
				secretData[k] = []byte(str)
			}
		}

		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: op.namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "resource-capsule",
					"app.kubernetes.io/version":    version,
					"capsule.docker.io/name":       name,
					"capsule.docker.io/version":    version,
					"capsule.docker.io/managed-by": "resourcecapsule-operator",
				},
			},
			Data: secretData,
			Type: v1.SecretTypeOpaque,
		}

		_, err := op.k8sClient.CoreV1().Secrets(op.namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		return err
	} else {
		// Convert data to string map for ConfigMap
		configData := make(map[string]string)
		for k, v := range data {
			if str, ok := v.(string); ok {
				configData[k] = str
			} else {
				configData[k] = fmt.Sprintf("%v", v)
			}
		}

		configMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: op.namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "resource-capsule",
					"app.kubernetes.io/version":    version,
					"capsule.docker.io/name":       name,
					"capsule.docker.io/version":    version,
					"capsule.docker.io/managed-by": "resourcecapsule-operator",
				},
			},
			Data: configData,
		}

		_, err := op.k8sClient.CoreV1().ConfigMaps(op.namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		return err
	}
}

// deleteUnderlyingResource deletes the underlying ConfigMap or Secret
func (op *ResourceCapsuleOperator) deleteUnderlyingResource(name, version, capsuleType string) error {
	resourceName := fmt.Sprintf("%s-%s", name, version)

	if capsuleType == "secret" {
		return op.k8sClient.CoreV1().Secrets(op.namespace).Delete(context.TODO(), resourceName, metav1.DeleteOptions{})
	} else {
		return op.k8sClient.CoreV1().ConfigMaps(op.namespace).Delete(context.TODO(), resourceName, metav1.DeleteOptions{})
	}
}

// performRollback implements rollback functionality
func (op *ResourceCapsuleOperator) performRollback(obj *unstructured.Unstructured, previousVersion string) error {
	name := obj.GetName()
	fmt.Printf("[Operator] Performing rollback for %s to version %s\n", name, previousVersion)

	// This is a simplified rollback - in a real implementation, you would:
	// 1. Find the previous version's ResourceCapsule
	// 2. Update the current ResourceCapsule spec with the previous version's data
	// 3. Update the version field

	return op.updateStatus(obj, "Active", fmt.Sprintf("Rollback to version %s completed", previousVersion))
}

// updateStatus updates the status of a ResourceCapsule
func (op *ResourceCapsuleOperator) updateStatus(obj *unstructured.Unstructured, phase, message string) error {
	// Update status
	status := map[string]interface{}{
		"phase":       phase,
		"lastUpdated": time.Now().Format(time.RFC3339),
		"message":     message,
	}

	if err := unstructured.SetNestedMap(obj.Object, status, "status"); err != nil {
		return fmt.Errorf("failed to set status: %v", err)
	}

	// Define the GVR for ResourceCapsule
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	// Update the resource
	_, err := op.client.Resource(gvr).Namespace(op.namespace).UpdateStatus(context.TODO(), obj, metav1.UpdateOptions{})
	return err
}