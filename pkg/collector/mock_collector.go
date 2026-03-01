package collector

import (
	"context"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// MockCollector provides mock implementations for multiple collector types
type MockCollector struct {
	*BaseCollector
	mockType string
	data     interface{}
}

// NewMockCloudCollector creates a new mock cloud collector
func NewMockCloudCollector() *MockCollector {
	return &MockCollector{
		BaseCollector: NewBaseCollector("mock_cloud"),
		mockType:      "cloud",
		data: models.CloudProviderMetrics{
			ProviderType: "aws",
			Name:         "Mock AWS",
			Regions:      []string{"us-west-2", "us-east-1"},
			InstanceMetrics: []models.CloudInstanceMetrics{
				{
					ID:                "i-0123456789abcdef0",
					Name:              "web-server-1",
					Type:              "t3.medium",
					Region:            "us-west-2",
					Status:            models.ResourceStatusRunning,
					CPUUtilization:    35.2,
					MemoryUtilization: 67.8,
					NetworkIn:         1548576,
					NetworkOut:        687924,
					DiskIO:            12345,
					UptimeHours:       168.5,
				},
				{
					ID:                "i-0123456789abcdef1",
					Name:              "api-server-1",
					Type:              "t3.large",
					Region:            "us-west-2",
					Status:            models.ResourceStatusRunning,
					CPUUtilization:    42.7,
					MemoryUtilization: 55.3,
					NetworkIn:         2548576,
					NetworkOut:        1687924,
					DiskIO:            23456,
					UptimeHours:       336.2,
				},
			},
			StorageMetrics: []models.CloudStorageMetrics{
				{
					ID:             "my-app-static-assets",
					Name:           "my-app-static-assets",
					Type:           "S3 Bucket",
					Region:         "us-west-2",
					TotalSizeBytes: 5368709120, // 5GB
					ObjectCount:    12345,
					RequestCount:   67890,
					ReadBytes:      1073741824, // 1GB
					WriteBytes:     536870912,  // 512MB
				},
			},
			DatabaseMetrics: []models.CloudDatabaseMetrics{
				{
					ID:                 "db-prod-01",
					Name:               "production-db",
					Type:               "RDS",
					Engine:             "PostgreSQL",
					Region:             "us-west-2",
					Status:             models.ResourceStatusRunning,
					CPUUtilization:     62.3,
					MemoryUtilization:  75.8,
					StorageUtilization: 68.5,
					ConnectionCount:    145,
					ReadIOPS:           512.7,
					WriteIOPS:          123.4,
					Latency:            5.2,
				},
			},
			LastUpdated: time.Now(),
		},
	}
}

// NewMockKubernetesCollector creates a new mock Kubernetes collector
func NewMockKubernetesCollector() *MockCollector {
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)

	return &MockCollector{
		BaseCollector: NewBaseCollector("mock_kubernetes"),
		mockType:      "kubernetes",
		data: models.KubernetesMetrics{
			ClusterName: "minikube",
			Context:     "minikube",
			Namespaces:  []string{"default", "kube-system"},
			Nodes: []models.KubernetesNodeMetrics{
				{
					Name:              "worker-node-1",
					Status:            "Ready",
					AllocatableCPU:    "4",
					AllocatableMemory: "8Gi",
					AllocatablePods:   110,
					CPUUsage:          65.4,
					MemoryUsage:       72.3,
					PodCount:          45,
					KubeletVersion:    "v1.25.4",
					LastUpdated:       time.Now(),
				},
			},
			Pods: []models.KubernetesPodMetrics{
				{
					Name:         "nginx-deployment-6b474476c4-x8zn2",
					Namespace:    "default",
					Status:       "Running",
					Phase:        "Running",
					Node:         "worker-node-1",
					IP:           "10.244.1.42",
					StartTime:    startTime,
					RestartCount: 0,
					ResourceUsage: models.KubernetesResourceUsage{
						CPUUsage:       25,
						MemoryUsage:    52428800, // 50MB
						CPURequests:    100,
						MemoryRequests: 104857600, // 100MB
						CPULimits:      200,
						MemoryLimits:   209715200, // 200MB
					},
				},
			},
			Deployments: []models.KubernetesDeploymentMetrics{
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
				},
			},
			Services: []models.KubernetesServiceMetrics{
				{
					Name:      "nginx-service",
					Namespace: "default",
					Type:      "ClusterIP",
					ClusterIP: "10.96.45.67",
					Ports: []models.KubernetesServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: 80,
						},
					},
				},
			},
			LastUpdated: time.Now(),
		},
	}
}

// NewMockCICDCollector creates a new mock CI/CD collector
func NewMockCICDCollector() *MockCollector {
	now := time.Now()

	workflows := []models.CICDWorkflowMetrics{
		{
			ID:              "ci-workflow",
			Name:            "CI Workflow",
			Repository:      "Teomazivila/maz-term",
			Enabled:         true,
			AverageDuration: 3*time.Minute + 45*time.Second,
			SuccessRate:     92.5,
			LastRunStatus:   models.CICDStatusSuccess,
			LastRunTime:     now.Add(-15 * time.Minute),
			LastSuccessTime: now.Add(-15 * time.Minute),
			RunCount:        156,
			RecentRuns: []models.CICDRunMetrics{
				{
					ID:        "run-1",
					Status:    models.CICDStatusSuccess,
					StartTime: now.Add(-15 * time.Minute),
					EndTime:   now.Add(-12 * time.Minute),
					Duration:  3 * time.Minute,
					Trigger:   "push",
					Branch:    "main",
					Commit:    "abc123def",
					Jobs: []models.CICDJobMetrics{
						{
							ID:       "job-1",
							Name:     "build",
							Status:   models.CICDStatusSuccess,
							Duration: 2 * time.Minute,
						},
					},
				},
			},
		},
	}

	return &MockCollector{
		BaseCollector: NewBaseCollector("mock_cicd"),
		mockType:      "cicd",
		data: models.CICDMetrics{
			ProviderType: models.CICDProviderGitHubActions,
			ProviderName: "GitHub Actions",
			Workflows:    workflows,
			LastUpdated:  now,
			Summary: models.CICDSummaryMetrics{
				TotalWorkflows:  len(workflows),
				ActiveWorkflows: len(workflows),
				RunningJobs:     0,
				SuccessRate:     92.5,
				AverageDuration: 3*time.Minute + 45*time.Second,
				FailedWorkflows: 0,
			},
		},
	}
}

// Name returns the name of the collector
func (c *MockCollector) Name() string {
	return "Mock " + c.mockType + " Collector"
}

// Collect returns mock data
func (c *MockCollector) Collect(ctx context.Context) (interface{}, error) {
	return c.data, nil
}

// GetLatestMetrics returns the latest mock metrics based on type
func (c *MockCollector) GetLatestMetrics() interface{} {
	return c.data
}

// Custom cloud metrics accessor
func (c *MockCollector) GetCloudMetrics() models.CloudProviderMetrics {
	if c.mockType == "cloud" {
		return c.data.(models.CloudProviderMetrics)
	}
	return models.CloudProviderMetrics{}
}

// Custom Kubernetes metrics accessor
func (c *MockCollector) GetKubernetesMetrics() models.KubernetesMetrics {
	if c.mockType == "kubernetes" {
		return c.data.(models.KubernetesMetrics)
	}
	return models.KubernetesMetrics{}
}

// Custom CI/CD metrics accessor
func (c *MockCollector) GetCICDMetrics() models.CICDMetrics {
	if c.mockType == "cicd" {
		return c.data.(models.CICDMetrics)
	}
	return models.CICDMetrics{}
}
