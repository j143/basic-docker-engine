package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesCapsuleManager handles Resource Capsules in Kubernetes environments
type KubernetesCapsuleManager struct {
	client        kubernetes.Interface
	dynamicClient dynamic.Interface
	namespace     string
}

// NewKubernetesCapsuleManager creates a new Kubernetes-enabled capsule manager
func NewKubernetesCapsuleManager(namespace string) (*KubernetesCapsuleManager, error) {
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

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	if namespace == "" {
		namespace = "default"
	}

	return &KubernetesCapsuleManager{
		client:        client,
		dynamicClient: dynamicClient,
		namespace:     namespace,
	}, nil
}

// CreateConfigMapCapsule creates a ConfigMap-based Resource Capsule
func (kcm *KubernetesCapsuleManager) CreateConfigMapCapsule(name, version string, data map[string]string) error {
	configMapName := fmt.Sprintf("%s-%s", name, version)
	
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: kcm.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":    "resource-capsule",
				"app.kubernetes.io/version": version,
				"capsule.docker.io/name":    name,
				"capsule.docker.io/version": version,
			},
		},
		Data: data,
	}

	_, err := kcm.client.CoreV1().ConfigMaps(kcm.namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap capsule: %v", err)
	}

	fmt.Printf("[Kubernetes] ConfigMap capsule %s:%s created successfully\n", name, version)
	return nil
}

// CreateSecretCapsule creates a Secret-based Resource Capsule  
func (kcm *KubernetesCapsuleManager) CreateSecretCapsule(name, version string, data map[string][]byte) error {
	secretName := fmt.Sprintf("%s-%s", name, version)
	
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: kcm.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":    "resource-capsule",
				"app.kubernetes.io/version": version,
				"capsule.docker.io/name":    name,
				"capsule.docker.io/version": version,
			},
		},
		Data: data,
		Type: v1.SecretTypeOpaque,
	}

	_, err := kcm.client.CoreV1().Secrets(kcm.namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Secret capsule: %v", err)
	}

	fmt.Printf("[Kubernetes] Secret capsule %s:%s created successfully\n", name, version)
	return nil
}

// GetConfigMapCapsule retrieves a ConfigMap-based Resource Capsule
func (kcm *KubernetesCapsuleManager) GetConfigMapCapsule(name, version string) (*v1.ConfigMap, error) {
	configMapName := fmt.Sprintf("%s-%s", name, version)
	
	configMap, err := kcm.client.CoreV1().ConfigMaps(kcm.namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap capsule: %v", err)
	}

	return configMap, nil
}

// GetSecretCapsule retrieves a Secret-based Resource Capsule
func (kcm *KubernetesCapsuleManager) GetSecretCapsule(name, version string) (*v1.Secret, error) {
	secretName := fmt.Sprintf("%s-%s", name, version)
	
	secret, err := kcm.client.CoreV1().Secrets(kcm.namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret capsule: %v", err)
	}

	return secret, nil
}

// ListCapsules lists all Resource Capsules in the namespace
func (kcm *KubernetesCapsuleManager) ListCapsules() error {
	fmt.Printf("[Kubernetes] Resource Capsules in namespace '%s':\n", kcm.namespace)
	
	// List ConfigMap capsules
	configMaps, err := kcm.client.CoreV1().ConfigMaps(kcm.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=resource-capsule",
	})
	if err != nil {
		return fmt.Errorf("failed to list ConfigMap capsules: %v", err)
	}

	fmt.Println("ConfigMap Capsules:")
	for _, cm := range configMaps.Items {
		capsuleName := cm.Labels["capsule.docker.io/name"]
		capsuleVersion := cm.Labels["capsule.docker.io/version"]
		fmt.Printf("  - %s:%s (ConfigMap: %s)\n", capsuleName, capsuleVersion, cm.Name)
	}

	// List Secret capsules
	secrets, err := kcm.client.CoreV1().Secrets(kcm.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=resource-capsule",
	})
	if err != nil {
		return fmt.Errorf("failed to list Secret capsules: %v", err)
	}

	fmt.Println("Secret Capsules:")
	for _, secret := range secrets.Items {
		capsuleName := secret.Labels["capsule.docker.io/name"]
		capsuleVersion := secret.Labels["capsule.docker.io/version"]
		fmt.Printf("  - %s:%s (Secret: %s)\n", capsuleName, capsuleVersion, secret.Name)
	}

	return nil
}

// DeleteCapsule deletes a Resource Capsule by name and version
func (kcm *KubernetesCapsuleManager) DeleteCapsule(name, version string) error {
	resourceName := fmt.Sprintf("%s-%s", name, version)
	
	// Try to delete ConfigMap first
	err := kcm.client.CoreV1().ConfigMaps(kcm.namespace).Delete(context.TODO(), resourceName, metav1.DeleteOptions{})
	if err == nil {
		fmt.Printf("[Kubernetes] ConfigMap capsule %s:%s deleted successfully\n", name, version)
		return nil
	}

	// Try to delete Secret
	err = kcm.client.CoreV1().Secrets(kcm.namespace).Delete(context.TODO(), resourceName, metav1.DeleteOptions{})
	if err == nil {
		fmt.Printf("[Kubernetes] Secret capsule %s:%s deleted successfully\n", name, version)
		return nil
	}

	return fmt.Errorf("capsule %s:%s not found", name, version)
}

// AttachCapsuleToDeployment attaches a Resource Capsule to a Kubernetes Deployment
func (kcm *KubernetesCapsuleManager) AttachCapsuleToDeployment(deploymentName, capsuleName, capsuleVersion string) error {
    // 1. Get the existing Deployment
    deployment, err := kcm.client.AppsV1().Deployments(kcm.namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
    if err != nil {
        return fmt.Errorf("failed to get deployment %s: %v", deploymentName, err)
    }

	// does capsule exists as a ConfigMap or Secret
    configMapName := fmt.Sprintf("%s-%s", capsuleName, capsuleVersion)
    secretName := configMapName
    
    // First, determine if the capsule exists as a ConfigMap or Secret
    _, configMapErr := kcm.GetConfigMapCapsule(capsuleName, capsuleVersion)
    _, secretErr := kcm.GetSecretCapsule(capsuleName, capsuleVersion)
    
	// 2. Add a volume for the ConfigMap/Secret
    var volumeName string
    var volumeSource v1.VolumeSource
    var mountPath string
    
    if configMapErr == nil {
        // It's a ConfigMap capsule
        volumeName = fmt.Sprintf("capsule-%s-%s", capsuleName, capsuleVersion)
        volumeSource = v1.VolumeSource{
            ConfigMap: &v1.ConfigMapVolumeSource{
                LocalObjectReference: v1.LocalObjectReference{
                    Name: configMapName,
                },
            },
        }
        mountPath = fmt.Sprintf("/capsules/%s/%s", capsuleName, capsuleVersion)
    } else if secretErr == nil {
        // It's a Secret capsule
        volumeName = fmt.Sprintf("capsule-%s-%s", capsuleName, capsuleVersion)
        volumeSource = v1.VolumeSource{
            Secret: &v1.SecretVolumeSource{
                SecretName: secretName,
            },
        }
        mountPath = fmt.Sprintf("/capsules/%s/%s", capsuleName, capsuleVersion)
    } else {
        return fmt.Errorf("capsule %s:%s not found", capsuleName, capsuleVersion)
    }
    
    volumeExists := false
    for _, volume := range deployment.Spec.Template.Spec.Volumes {
        if volume.Name == volumeName {
            volumeExists = true
            break
        }
    }
    
    // Add the volume if it doesn't exist
    if !volumeExists {
        deployment.Spec.Template.Spec.Volumes = append(
            deployment.Spec.Template.Spec.Volumes,
            v1.Volume{
                Name:         volumeName,
                VolumeSource: volumeSource,
            },
        )
    }
    
	// 3. Add a volumeMount to the container spec
    for i := range deployment.Spec.Template.Spec.Containers {
        container := &deployment.Spec.Template.Spec.Containers[i]
        
		// check if this container already has the mount
		mountExists := false
		for _, mount := range container.VolumeMounts {
            if mount.Name == volumeName {
                mountExists = true
                break
            }
        }

		if !mountExists {
            container.VolumeMounts = append(
                container.VolumeMounts,
                v1.VolumeMount{
                    Name:      volumeName,
                    MountPath: mountPath,
                    ReadOnly:  true,
                },
            )
        }
        
    }
    
    //4. Update the deployment
    _, err = kcm.client.AppsV1().Deployments(kcm.namespace).Update(
        context.TODO(), 
        deployment, 
        metav1.UpdateOptions{},
    )
    if err != nil {
        return fmt.Errorf("failed to update deployment %s: %v", deploymentName, err)
    }
    
    fmt.Printf("[Kubernetes] Capsule %s:%s attached to deployment %s at path %s\n", 
        capsuleName, capsuleVersion, deploymentName, mountPath)
    return nil
}

// BenchmarkKubernetesResourceAccess benchmarks access to Kubernetes resources
func (kcm *KubernetesCapsuleManager) BenchmarkKubernetesResourceAccess(name, version string) (time.Duration, error) {
	start := time.Now()
	
	// Try ConfigMap first
	_, err := kcm.GetConfigMapCapsule(name, version)
	if err == nil {
		return time.Since(start), nil
	}
	
	// Try Secret
	_, err = kcm.GetSecretCapsule(name, version)
	if err == nil {
		return time.Since(start), nil
	}
	
	return 0, fmt.Errorf("capsule %s:%s not found", name, version)
}

// CRD-related functions

// CreateCRDCapsule creates a ResourceCapsule custom resource
func (kcm *KubernetesCapsuleManager) CreateCRDCapsule(name, version string, data map[string]interface{}, capsuleType string) error {
	if capsuleType == "" {
		capsuleType = "configmap"
	}

	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	resourceCapsule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "capsules.docker.io/v1",
			"kind":       "ResourceCapsule",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": kcm.namespace,
				"labels": map[string]interface{}{
					"capsule.docker.io/name":    name,
					"capsule.docker.io/version": version,
				},
			},
			"spec": map[string]interface{}{
				"data":        data,
				"version":     version,
				"capsuleType": capsuleType,
				"rollback": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	_, err := kcm.dynamicClient.Resource(gvr).Namespace(kcm.namespace).Create(context.TODO(), resourceCapsule, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ResourceCapsule CRD: %v", err)
	}

	fmt.Printf("[Kubernetes] ResourceCapsule CRD %s:%s created successfully\n", name, version)
	return nil
}

// GetCRDCapsule retrieves a ResourceCapsule custom resource
func (kcm *KubernetesCapsuleManager) GetCRDCapsule(name string) (*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	resourceCapsule, err := kcm.dynamicClient.Resource(gvr).Namespace(kcm.namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ResourceCapsule CRD: %v", err)
	}

	return resourceCapsule, nil
}

// ListCRDCapsules lists all ResourceCapsule custom resources
func (kcm *KubernetesCapsuleManager) ListCRDCapsules() error {
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	list, err := kcm.dynamicClient.Resource(gvr).Namespace(kcm.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list ResourceCapsule CRDs: %v", err)
	}

	fmt.Printf("[Kubernetes] ResourceCapsule CRDs in namespace '%s':\n", kcm.namespace)
	for _, item := range list.Items {
		name := item.GetName()
		spec, found, _ := unstructured.NestedMap(item.Object, "spec")
		if found {
			version, found, _ := unstructured.NestedString(spec, "version")
			capsuleType, found2, _ := unstructured.NestedString(spec, "capsuleType")
			if !found2 {
				capsuleType = "configmap"
			}
			
			status, statusFound, _ := unstructured.NestedMap(item.Object, "status")
			phase := "Unknown"
			if statusFound {
				if p, found, _ := unstructured.NestedString(status, "phase"); found {
					phase = p
				}
			}
			
			if found {
				fmt.Printf("  - %s:%s (Type: %s, Status: %s)\n", name, version, capsuleType, phase)
			} else {
				fmt.Printf("  - %s (Type: %s, Status: %s)\n", name, capsuleType, phase)
			}
		}
	}

	return nil
}

// DeleteCRDCapsule deletes a ResourceCapsule custom resource
func (kcm *KubernetesCapsuleManager) DeleteCRDCapsule(name string) error {
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	err := kcm.dynamicClient.Resource(gvr).Namespace(kcm.namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete ResourceCapsule CRD: %v", err)
	}

	fmt.Printf("[Kubernetes] ResourceCapsule CRD %s deleted successfully\n", name)
	return nil
}

// RollbackCRDCapsule performs rollback for a ResourceCapsule
func (kcm *KubernetesCapsuleManager) RollbackCRDCapsule(name, previousVersion string) error {
	gvr := schema.GroupVersionResource{
		Group:    "capsules.docker.io",
		Version:  "v1",
		Resource: "resourcecapsules",
	}

	// Get the current ResourceCapsule
	resourceCapsule, err := kcm.GetCRDCapsule(name)
	if err != nil {
		return err
	}

	// Update the rollback configuration
	rollback := map[string]interface{}{
		"enabled":         true,
		"previousVersion": previousVersion,
	}

	if err := unstructured.SetNestedMap(resourceCapsule.Object, rollback, "spec", "rollback"); err != nil {
		return fmt.Errorf("failed to set rollback configuration: %v", err)
	}

	// Update the resource
	_, err = kcm.dynamicClient.Resource(gvr).Namespace(kcm.namespace).Update(context.TODO(), resourceCapsule, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ResourceCapsule for rollback: %v", err)
	}

	fmt.Printf("[Kubernetes] Rollback initiated for ResourceCapsule %s to version %s\n", name, previousVersion)
	return nil
}