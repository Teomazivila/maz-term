package models

import (
	"time"
)

// KubernetesResource represents a type of Kubernetes resource
type KubernetesResource string

const (
	// KubernetesResourcePod represents a Kubernetes Pod
	KubernetesResourcePod KubernetesResource = "pod"

	// KubernetesResourceDeployment represents a Kubernetes Deployment
	KubernetesResourceDeployment KubernetesResource = "deployment"

	// KubernetesResourceService represents a Kubernetes Service
	KubernetesResourceService KubernetesResource = "service"

	// KubernetesResourceNode represents a Kubernetes Node
	KubernetesResourceNode KubernetesResource = "node"

	// KubernetesResourceStatefulSet represents a Kubernetes StatefulSet
	KubernetesResourceStatefulSet KubernetesResource = "statefulset"

	// KubernetesResourceDaemonSet represents a Kubernetes DaemonSet
	KubernetesResourceDaemonSet KubernetesResource = "daemonset"
)

// KubernetesMetrics represents metrics for a Kubernetes cluster
type KubernetesMetrics struct {
	// ClusterName is the name of the Kubernetes cluster
	ClusterName string `json:"cluster_name"`

	// Context is the kubeconfig context used
	Context string `json:"context"`

	// Namespaces is the list of namespaces being monitored
	Namespaces []string `json:"namespaces"`

	// Nodes is the list of nodes in the cluster
	Nodes []KubernetesNodeMetrics `json:"nodes"`

	// Pods is the list of pods in the monitored namespaces
	Pods []KubernetesPodMetrics `json:"pods"`

	// Deployments is the list of deployments in the monitored namespaces
	Deployments []KubernetesDeploymentMetrics `json:"deployments"`

	// Services is the list of services in the monitored namespaces
	Services []KubernetesServiceMetrics `json:"services"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`

	// Errors contains any errors encountered during collection
	Errors []string `json:"errors,omitempty"`
}

// KubernetesNodeMetrics represents metrics for a Kubernetes node
type KubernetesNodeMetrics struct {
	// Name is the name of the node
	Name string `json:"name"`

	// Status is the status of the node (Ready, NotReady, etc.)
	Status string `json:"status"`

	// Conditions are the current node conditions
	Conditions map[string]string `json:"conditions"`

	// Taints are the taints applied to the node
	Taints []string `json:"taints"`

	// AllocatableCPU is the amount of allocatable CPU cores
	AllocatableCPU string `json:"allocatable_cpu"`

	// AllocatableMemory is the amount of allocatable memory
	AllocatableMemory string `json:"allocatable_memory"`

	// AllocatablePods is the maximum number of pods that can be scheduled to the node
	AllocatablePods int `json:"allocatable_pods"`

	// CPUUsage is the current CPU usage percentage
	CPUUsage float64 `json:"cpu_usage"`

	// MemoryUsage is the current memory usage percentage
	MemoryUsage float64 `json:"memory_usage"`

	// PodCount is the number of pods running on the node
	PodCount int `json:"pod_count"`

	// KernelVersion is the node's kernel version
	KernelVersion string `json:"kernel_version"`

	// OSImage is the node's OS image
	OSImage string `json:"os_image"`

	// KubeletVersion is the node's kubelet version
	KubeletVersion string `json:"kubelet_version"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`
}

// KubernetesPodMetrics represents metrics for a Kubernetes pod
type KubernetesPodMetrics struct {
	// Name is the name of the pod
	Name string `json:"name"`

	// Namespace is the namespace the pod is in
	Namespace string `json:"namespace"`

	// Status is the status of the pod (Running, Pending, etc.)
	Status string `json:"status"`

	// Phase is the current lifecycle phase of the pod
	Phase string `json:"phase"`

	// Node is the node the pod is running on
	Node string `json:"node"`

	// IP is the pod's IP address
	IP string `json:"ip"`

	// StartTime is when the pod was created
	StartTime time.Time `json:"start_time"`

	// RestartCount is the number of restarts across all containers
	RestartCount int `json:"restart_count"`

	// Containers is the list of containers in the pod
	Containers []KubernetesContainerMetrics `json:"containers"`

	// ResourceUsage contains resource usage metrics
	ResourceUsage KubernetesResourceUsage `json:"resource_usage"`

	// Labels are the pod's labels
	Labels map[string]string `json:"labels"`

	// OwnerReferences contains information about the pod's owner
	OwnerReferences []KubernetesOwnerReference `json:"owner_references"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`
}

// KubernetesContainerMetrics represents metrics for a container in a pod
type KubernetesContainerMetrics struct {
	// Name is the name of the container
	Name string `json:"name"`

	// Image is the container image
	Image string `json:"image"`

	// Ready is whether the container is ready
	Ready bool `json:"ready"`

	// RestartCount is the number of times the container has restarted
	RestartCount int `json:"restart_count"`

	// State is the current state of the container (running, waiting, terminated)
	State string `json:"state"`

	// CPUUsage is the CPU usage in millicores
	CPUUsage int64 `json:"cpu_usage"`

	// MemoryUsage is the memory usage in bytes
	MemoryUsage int64 `json:"memory_usage"`

	// CPURequests is the CPU requests in millicores
	CPURequests int64 `json:"cpu_requests"`

	// MemoryRequests is the memory requests in bytes
	MemoryRequests int64 `json:"memory_requests"`

	// CPULimits is the CPU limits in millicores
	CPULimits int64 `json:"cpu_limits"`

	// MemoryLimits is the memory limits in bytes
	MemoryLimits int64 `json:"memory_limits"`
}

// KubernetesResourceUsage represents resource usage metrics
type KubernetesResourceUsage struct {
	// CPUUsage is the CPU usage in millicores
	CPUUsage int64 `json:"cpu_usage"`

	// MemoryUsage is the memory usage in bytes
	MemoryUsage int64 `json:"memory_usage"`

	// CPURequests is the total CPU requests in millicores
	CPURequests int64 `json:"cpu_requests"`

	// MemoryRequests is the total memory requests in bytes
	MemoryRequests int64 `json:"memory_requests"`

	// CPULimits is the total CPU limits in millicores
	CPULimits int64 `json:"cpu_limits"`

	// MemoryLimits is the total memory limits in bytes
	MemoryLimits int64 `json:"memory_limits"`
}

// KubernetesOwnerReference represents the owner of a Kubernetes resource
type KubernetesOwnerReference struct {
	// Kind is the kind of the owner (Deployment, ReplicaSet, etc.)
	Kind string `json:"kind"`

	// Name is the name of the owner
	Name string `json:"name"`
}

// KubernetesDeploymentMetrics represents metrics for a Kubernetes deployment
type KubernetesDeploymentMetrics struct {
	// Name is the name of the deployment
	Name string `json:"name"`

	// Namespace is the namespace the deployment is in
	Namespace string `json:"namespace"`

	// DesiredReplicas is the number of desired replicas
	DesiredReplicas int32 `json:"desired_replicas"`

	// AvailableReplicas is the number of available replicas
	AvailableReplicas int32 `json:"available_replicas"`

	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32 `json:"ready_replicas"`

	// UpdatedReplicas is the number of updated replicas
	UpdatedReplicas int32 `json:"updated_replicas"`

	// UnavailableReplicas is the number of unavailable replicas
	UnavailableReplicas int32 `json:"unavailable_replicas"`

	// Strategy is the deployment strategy
	Strategy string `json:"strategy"`

	// Age is the age of the deployment
	Age time.Duration `json:"age"`

	// Conditions are the current deployment conditions
	Conditions map[string]string `json:"conditions"`

	// Labels are the deployment's labels
	Labels map[string]string `json:"labels"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`
}

// KubernetesServiceMetrics represents metrics for a Kubernetes service
type KubernetesServiceMetrics struct {
	// Name is the name of the service
	Name string `json:"name"`

	// Namespace is the namespace the service is in
	Namespace string `json:"namespace"`

	// Type is the service type (ClusterIP, NodePort, etc.)
	Type string `json:"type"`

	// ClusterIP is the cluster IP of the service
	ClusterIP string `json:"cluster_ip"`

	// ExternalIPs are the external IPs of the service
	ExternalIPs []string `json:"external_ips"`

	// Ports are the ports exposed by the service
	Ports []KubernetesServicePort `json:"ports"`

	// Selector is the service selector
	Selector map[string]string `json:"selector"`

	// Labels are the service's labels
	Labels map[string]string `json:"labels"`

	// Age is the age of the service
	Age time.Duration `json:"age"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`
}

// KubernetesServicePort represents a port exposed by a Kubernetes service
type KubernetesServicePort struct {
	// Name is the name of the port
	Name string `json:"name"`

	// Protocol is the protocol (TCP, UDP)
	Protocol string `json:"protocol"`

	// Port is the port number
	Port int32 `json:"port"`

	// TargetPort is the target port
	TargetPort int32 `json:"target_port"`

	// NodePort is the node port (for NodePort services)
	NodePort int32 `json:"node_port"`
}
