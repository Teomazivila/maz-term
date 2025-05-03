package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
)

// KubernetesMetricsCollector collects metrics from Kubernetes clusters
type KubernetesMetricsCollector struct {
	*BaseCollector
	config  config.KubernetesConfig
	metrics models.KubernetesMetrics
	mu      sync.RWMutex
}

// NewKubernetesMetricsCollector creates a new Kubernetes metrics collector
func NewKubernetesMetricsCollector(cfg config.KubernetesConfig) *KubernetesMetricsCollector {
	return &KubernetesMetricsCollector{
		BaseCollector: NewBaseCollector("kubernetes"),
		config:        cfg,
		metrics: models.KubernetesMetrics{
			ClusterName: "kubernetes",
			Context:     cfg.Context,
			Namespaces:  cfg.Namespaces,
			LastUpdated: time.Now(),
		},
	}
}

// Name returns the name of the collector
func (c *KubernetesMetricsCollector) Name() string {
	return "Kubernetes Metrics Collector"
}

// Collect gathers metrics from the Kubernetes cluster
func (c *KubernetesMetricsCollector) Collect(ctx context.Context) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// For the MVP, we'll simulate Kubernetes data collection with demo data
	// In a real implementation, we would use the Kubernetes client-go to collect actual metrics

	// Reset errors
	c.metrics.Errors = []string{}

	// Update collection timestamp
	c.metrics.LastUpdated = time.Now()

	// Use a waitgroup to collect metrics concurrently
	var wg sync.WaitGroup
	var nodesErr, podsErr, deploymentsErr, servicesErr error

	// Collect nodes
	if contains(c.config.Resources, "nodes") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodes, err := c.collectNodes(ctx)
			if err != nil {
				nodesErr = err
				return
			}
			c.metrics.Nodes = nodes
		}()
	}

	// Collect pods
	if contains(c.config.Resources, "pods") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pods, err := c.collectPods(ctx)
			if err != nil {
				podsErr = err
				return
			}
			c.metrics.Pods = pods
		}()
	}

	// Collect deployments
	if contains(c.config.Resources, "deployments") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deployments, err := c.collectDeployments(ctx)
			if err != nil {
				deploymentsErr = err
				return
			}
			c.metrics.Deployments = deployments
		}()
	}

	// Collect services
	if contains(c.config.Resources, "services") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			services, err := c.collectServices(ctx)
			if err != nil {
				servicesErr = err
				return
			}
			c.metrics.Services = services
		}()
	}

	// Wait for all collectors to finish
	wg.Wait()

	// Check for errors
	if nodesErr != nil {
		c.metrics.Errors = append(c.metrics.Errors, fmt.Sprintf("Nodes error: %v", nodesErr))
	}
	if podsErr != nil {
		c.metrics.Errors = append(c.metrics.Errors, fmt.Sprintf("Pods error: %v", podsErr))
	}
	if deploymentsErr != nil {
		c.metrics.Errors = append(c.metrics.Errors, fmt.Sprintf("Deployments error: %v", deploymentsErr))
	}
	if servicesErr != nil {
		c.metrics.Errors = append(c.metrics.Errors, fmt.Sprintf("Services error: %v", servicesErr))
	}

	return c.metrics, nil
}

// GetLatestMetrics returns the most recently collected metrics
func (c *KubernetesMetricsCollector) GetLatestMetrics() models.KubernetesMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Start starts the collector
func (c *KubernetesMetricsCollector) Start(ctx context.Context, interval time.Duration) error {
	// Call the parent Start method
	if err := c.BaseCollector.Start(ctx, interval); err != nil {
		return err
	}

	if !c.running {
		return nil
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Collect initial data
		data, err := c.Collect(ctx)
		if err == nil {
			c.UpdateData(data)
		}

		for {
			select {
			case <-ticker.C:
				data, err := c.Collect(ctx)
				if err == nil {
					c.UpdateData(data)
				}
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop stops the collector
func (c *KubernetesMetricsCollector) Stop() error {
	return c.BaseCollector.Stop()
}

// collectNodes collects metrics from Kubernetes nodes
func (c *KubernetesMetricsCollector) collectNodes(ctx context.Context) ([]models.KubernetesNodeMetrics, error) {
	// In a real implementation, we would use the Kubernetes client-go to collect actual metrics
	// For the MVP, we'll return demo data

	nodes := []models.KubernetesNodeMetrics{
		{
			Name:   "worker-node-1",
			Status: "Ready",
			Conditions: map[string]string{
				"Ready":              "True",
				"NetworkUnavailable": "False",
				"DiskPressure":       "False",
				"MemoryPressure":     "False",
				"PIDPressure":        "False",
			},
			AllocatableCPU:    "4",
			AllocatableMemory: "8Gi",
			AllocatablePods:   110,
			CPUUsage:          65.4,
			MemoryUsage:       72.3,
			PodCount:          45,
			KernelVersion:     "5.15.0-67-generic",
			OSImage:           "Ubuntu 22.04.1 LTS",
			KubeletVersion:    "v1.25.4",
			LastUpdated:       time.Now(),
		},
		{
			Name:   "worker-node-2",
			Status: "Ready",
			Conditions: map[string]string{
				"Ready":              "True",
				"NetworkUnavailable": "False",
				"DiskPressure":       "False",
				"MemoryPressure":     "False",
				"PIDPressure":        "False",
			},
			AllocatableCPU:    "4",
			AllocatableMemory: "8Gi",
			AllocatablePods:   110,
			CPUUsage:          42.1,
			MemoryUsage:       56.7,
			PodCount:          38,
			KernelVersion:     "5.15.0-67-generic",
			OSImage:           "Ubuntu 22.04.1 LTS",
			KubeletVersion:    "v1.25.4",
			LastUpdated:       time.Now(),
		},
		{
			Name:   "master-node",
			Status: "Ready",
			Conditions: map[string]string{
				"Ready":              "True",
				"NetworkUnavailable": "False",
				"DiskPressure":       "False",
				"MemoryPressure":     "False",
				"PIDPressure":        "False",
			},
			Taints: []string{
				"node-role.kubernetes.io/control-plane:NoSchedule",
			},
			AllocatableCPU:    "2",
			AllocatableMemory: "4Gi",
			AllocatablePods:   110,
			CPUUsage:          23.5,
			MemoryUsage:       45.6,
			PodCount:          15,
			KernelVersion:     "5.15.0-67-generic",
			OSImage:           "Ubuntu 22.04.1 LTS",
			KubeletVersion:    "v1.25.4",
			LastUpdated:       time.Now(),
		},
	}

	return nodes, nil
}

// collectPods collects metrics from Kubernetes pods
func (c *KubernetesMetricsCollector) collectPods(ctx context.Context) ([]models.KubernetesPodMetrics, error) {
	// In a real implementation, we would use the Kubernetes client-go to collect actual metrics
	// For the MVP, we'll return demo data

	now := time.Now()
	startTime := now.Add(-24 * time.Hour)

	pods := []models.KubernetesPodMetrics{
		{
			Name:         "nginx-deployment-6b474476c4-x8zn2",
			Namespace:    "default",
			Status:       "Running",
			Phase:        "Running",
			Node:         "worker-node-1",
			IP:           "10.244.1.42",
			StartTime:    startTime,
			RestartCount: 0,
			Containers: []models.KubernetesContainerMetrics{
				{
					Name:           "nginx",
					Image:          "nginx:1.21",
					Ready:          true,
					RestartCount:   0,
					State:          "running",
					CPUUsage:       25,
					MemoryUsage:    52428800, // 50MB
					CPURequests:    100,
					MemoryRequests: 104857600, // 100MB
					CPULimits:      200,
					MemoryLimits:   209715200, // 200MB
				},
			},
			ResourceUsage: models.KubernetesResourceUsage{
				CPUUsage:       25,
				MemoryUsage:    52428800, // 50MB
				CPURequests:    100,
				MemoryRequests: 104857600, // 100MB
				CPULimits:      200,
				MemoryLimits:   209715200, // 200MB
			},
			Labels: map[string]string{
				"app":               "nginx",
				"pod-template-hash": "6b474476c4",
			},
			OwnerReferences: []models.KubernetesOwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "nginx-deployment-6b474476c4",
				},
			},
			LastUpdated: now,
		},
		{
			Name:         "nginx-deployment-6b474476c4-2a3b4",
			Namespace:    "default",
			Status:       "Running",
			Phase:        "Running",
			Node:         "worker-node-2",
			IP:           "10.244.2.76",
			StartTime:    startTime,
			RestartCount: 0,
			Containers: []models.KubernetesContainerMetrics{
				{
					Name:           "nginx",
					Image:          "nginx:1.21",
					Ready:          true,
					RestartCount:   0,
					State:          "running",
					CPUUsage:       22,
					MemoryUsage:    48234567, // ~46MB
					CPURequests:    100,
					MemoryRequests: 104857600, // 100MB
					CPULimits:      200,
					MemoryLimits:   209715200, // 200MB
				},
			},
			ResourceUsage: models.KubernetesResourceUsage{
				CPUUsage:       22,
				MemoryUsage:    48234567, // ~46MB
				CPURequests:    100,
				MemoryRequests: 104857600, // 100MB
				CPULimits:      200,
				MemoryLimits:   209715200, // 200MB
			},
			Labels: map[string]string{
				"app":               "nginx",
				"pod-template-hash": "6b474476c4",
			},
			OwnerReferences: []models.KubernetesOwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "nginx-deployment-6b474476c4",
				},
			},
			LastUpdated: now,
		},
		{
			Name:         "api-deployment-5d7f9c8d4b-dc3fa",
			Namespace:    "default",
			Status:       "Running",
			Phase:        "Running",
			Node:         "worker-node-1",
			IP:           "10.244.1.87",
			StartTime:    startTime.Add(2 * time.Hour),
			RestartCount: 1,
			Containers: []models.KubernetesContainerMetrics{
				{
					Name:           "api",
					Image:          "app/api:v2.1",
					Ready:          true,
					RestartCount:   1,
					State:          "running",
					CPUUsage:       156,
					MemoryUsage:    268435456, // 256MB
					CPURequests:    200,
					MemoryRequests: 536870912, // 512MB
					CPULimits:      500,
					MemoryLimits:   1073741824, // 1GB
				},
				{
					Name:           "sidecar",
					Image:          "app/sidecar:v1.0",
					Ready:          true,
					RestartCount:   0,
					State:          "running",
					CPUUsage:       15,
					MemoryUsage:    41943040, // 40MB
					CPURequests:    50,
					MemoryRequests: 67108864, // 64MB
					CPULimits:      100,
					MemoryLimits:   134217728, // 128MB
				},
			},
			ResourceUsage: models.KubernetesResourceUsage{
				CPUUsage:       171,
				MemoryUsage:    310378496, // 296MB
				CPURequests:    250,
				MemoryRequests: 603979776, // 576MB
				CPULimits:      600,
				MemoryLimits:   1207959552, // 1.128GB
			},
			Labels: map[string]string{
				"app":               "api",
				"pod-template-hash": "5d7f9c8d4b",
			},
			OwnerReferences: []models.KubernetesOwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "api-deployment-5d7f9c8d4b",
				},
			},
			LastUpdated: now,
		},
	}

	return pods, nil
}

// collectDeployments collects metrics from Kubernetes deployments
func (c *KubernetesMetricsCollector) collectDeployments(ctx context.Context) ([]models.KubernetesDeploymentMetrics, error) {
	// In a real implementation, we would use the Kubernetes client-go to collect actual metrics
	// For the MVP, we'll return demo data

	now := time.Now()

	deployments := []models.KubernetesDeploymentMetrics{
		{
			Name:                "nginx-deployment",
			Namespace:           "default",
			DesiredReplicas:     2,
			AvailableReplicas:   2,
			ReadyReplicas:       2,
			UpdatedReplicas:     2,
			UnavailableReplicas: 0,
			Strategy:            "RollingUpdate",
			Age:                 24 * time.Hour,
			Conditions: map[string]string{
				"Available":   "True",
				"Progressing": "True",
			},
			Labels: map[string]string{
				"app": "nginx",
			},
			LastUpdated: now,
		},
		{
			Name:                "api-deployment",
			Namespace:           "default",
			DesiredReplicas:     1,
			AvailableReplicas:   1,
			ReadyReplicas:       1,
			UpdatedReplicas:     1,
			UnavailableReplicas: 0,
			Strategy:            "RollingUpdate",
			Age:                 22 * time.Hour,
			Conditions: map[string]string{
				"Available":   "True",
				"Progressing": "True",
			},
			Labels: map[string]string{
				"app": "api",
			},
			LastUpdated: now,
		},
		{
			Name:                "db-deployment",
			Namespace:           "default",
			DesiredReplicas:     1,
			AvailableReplicas:   1,
			ReadyReplicas:       1,
			UpdatedReplicas:     1,
			UnavailableReplicas: 0,
			Strategy:            "Recreate",
			Age:                 21 * time.Hour,
			Conditions: map[string]string{
				"Available":   "True",
				"Progressing": "True",
			},
			Labels: map[string]string{
				"app": "db",
			},
			LastUpdated: now,
		},
	}

	return deployments, nil
}

// collectServices collects metrics from Kubernetes services
func (c *KubernetesMetricsCollector) collectServices(ctx context.Context) ([]models.KubernetesServiceMetrics, error) {
	// In a real implementation, we would use the Kubernetes client-go to collect actual metrics
	// For the MVP, we'll return demo data

	now := time.Now()

	services := []models.KubernetesServiceMetrics{
		{
			Name:        "nginx-service",
			Namespace:   "default",
			Type:        "ClusterIP",
			ClusterIP:   "10.96.45.67",
			ExternalIPs: []string{},
			Ports: []models.KubernetesServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 80,
				},
			},
			Selector: map[string]string{
				"app": "nginx",
			},
			Labels: map[string]string{
				"app": "nginx",
			},
			Age:         24 * time.Hour,
			LastUpdated: now,
		},
		{
			Name:        "api-service",
			Namespace:   "default",
			Type:        "ClusterIP",
			ClusterIP:   "10.96.78.90",
			ExternalIPs: []string{},
			Ports: []models.KubernetesServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       8080,
					TargetPort: 8080,
				},
				{
					Name:       "metrics",
					Protocol:   "TCP",
					Port:       8081,
					TargetPort: 8081,
				},
			},
			Selector: map[string]string{
				"app": "api",
			},
			Labels: map[string]string{
				"app": "api",
			},
			Age:         22 * time.Hour,
			LastUpdated: now,
		},
		{
			Name:        "frontend-service",
			Namespace:   "default",
			Type:        "NodePort",
			ClusterIP:   "10.96.12.34",
			ExternalIPs: []string{},
			Ports: []models.KubernetesServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 80,
					NodePort:   30080,
				},
			},
			Selector: map[string]string{
				"app": "frontend",
			},
			Labels: map[string]string{
				"app": "frontend",
			},
			Age:         20 * time.Hour,
			LastUpdated: now,
		},
	}

	return services, nil
}
