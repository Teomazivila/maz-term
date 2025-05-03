package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/models"
)

func main() {
	fmt.Println("DevOps Terminal Dashboard - Extended Integrations Test")
	fmt.Println("=====================================================")

	// Create mock collectors
	cloudCollector := collector.NewMockCloudCollector()
	k8sCollector := collector.NewMockKubernetesCollector()
	cicdCollector := collector.NewMockCICDCollector()

	// Start collectors
	ctx := context.Background()
	cloudCollector.Start(ctx, 5*time.Second)
	k8sCollector.Start(ctx, 5*time.Second)
	cicdCollector.Start(ctx, 5*time.Second)

	// Collect data
	fmt.Println("\n1. Cloud Provider (AWS) Integration:")
	fmt.Println("-----------------------------------")
	cloudData, _ := cloudCollector.Collect(ctx)
	cloudMetrics := cloudData.(models.CloudProviderMetrics)

	fmt.Printf("Provider: %s\n", cloudMetrics.ProviderType)
	fmt.Printf("Regions: %v\n", cloudMetrics.Regions)
	fmt.Printf("Instances: %d\n", len(cloudMetrics.InstanceMetrics))
	for i, instance := range cloudMetrics.InstanceMetrics {
		fmt.Printf("  Instance %d: %s (%s) - CPU: %.1f%%, MEM: %.1f%%\n",
			i+1, instance.Name, instance.Type, instance.CPUUtilization, instance.MemoryUtilization)
	}
	fmt.Printf("Storage Resources: %d\n", len(cloudMetrics.StorageMetrics))
	fmt.Printf("Database Resources: %d\n", len(cloudMetrics.DatabaseMetrics))

	fmt.Println("\n2. Kubernetes Integration:")
	fmt.Println("-------------------------")
	k8sData, _ := k8sCollector.Collect(ctx)
	k8sMetrics := k8sData.(models.KubernetesMetrics)

	fmt.Printf("Cluster: %s\n", k8sMetrics.ClusterName)
	fmt.Printf("Context: %s\n", k8sMetrics.Context)
	fmt.Printf("Namespaces: %v\n", k8sMetrics.Namespaces)
	fmt.Printf("Nodes: %d\n", len(k8sMetrics.Nodes))
	fmt.Printf("Pods: %d\n", len(k8sMetrics.Pods))
	fmt.Printf("Deployments: %d\n", len(k8sMetrics.Deployments))
	fmt.Printf("Services: %d\n", len(k8sMetrics.Services))

	// Print pod details
	for i, pod := range k8sMetrics.Pods {
		fmt.Printf("  Pod %d: %s - Status: %s, Node: %s\n",
			i+1, pod.Name, pod.Status, pod.Node)
	}

	fmt.Println("\n3. CI/CD Integration (GitHub Actions):")
	fmt.Println("-------------------------------------")
	cicdData, _ := cicdCollector.Collect(ctx)
	cicdMetrics := cicdData.(models.CICDMetrics)

	fmt.Printf("Provider: %s\n", cicdMetrics.ProviderName)
	fmt.Printf("Workflows: %d\n", len(cicdMetrics.Workflows))
	fmt.Printf("Success Rate: %.1f%%\n", cicdMetrics.Summary.SuccessRate)

	// Print workflow details
	for i, workflow := range cicdMetrics.Workflows {
		fmt.Printf("  Workflow %d: %s - Success Rate: %.1f%%, Runs: %d\n",
			i+1, workflow.Name, workflow.SuccessRate, workflow.RunCount)

		// Print recent runs
		fmt.Printf("    Recent runs:\n")
		for j, run := range workflow.RecentRuns {
			fmt.Printf("      Run %d: Status: %s, Duration: %s, Trigger: %s\n",
				j+1, run.Status, run.Duration, run.Trigger)
		}
	}

	fmt.Println("\nIntegrations test completed successfully.")
}
