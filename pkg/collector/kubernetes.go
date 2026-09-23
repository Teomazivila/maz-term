package collector

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	// k8sCollectTimeout bounds one full sweep across every namespace.
	k8sCollectTimeout = 45 * time.Second

	// k8sMaxItems bounds how much of a namespace is listed, so a large cluster
	// cannot make a refresh cycle unbounded.
	k8sMaxItems = 500
)

// KubernetesConfig configures the Kubernetes collector.
type KubernetesConfig struct {
	// ConfigPath is the kubeconfig to use. Empty means $KUBECONFIG, then
	// ~/.kube/config, then in-cluster credentials.
	ConfigPath string

	// Context selects a kubeconfig context. Empty means the current context.
	Context string

	// Namespaces limits collection. Empty means all namespaces.
	Namespaces []string

	// Resources selects which kinds to list. Empty means all supported kinds.
	Resources []string
}

// wants reports whether a resource kind should be collected.
func (c KubernetesConfig) wants(resource string) bool {
	if len(c.Resources) == 0 {
		return true
	}
	for _, want := range c.Resources {
		if strings.EqualFold(strings.TrimSpace(want), resource) {
			return true
		}
	}
	return false
}

// KubernetesCollector collects cluster state through the Kubernetes API.
//
// Every call is a read: list operations only, no mutations. Credentials come
// from kubeconfig or the in-cluster service account, never from maz-term's own
// configuration file.
type KubernetesCollector struct {
	*BaseCollector

	config KubernetesConfig

	mu       sync.RWMutex
	metrics  models.KubernetesMetrics
	identity KubernetesIdentity

	// clientsFor builds the API clients. It is a field so tests can substitute a
	// fake clientset without reaching a cluster.
	clientsFor func() (kubernetes.Interface, metricsv.Interface, string, error)
}

// NewKubernetesCollector creates a Kubernetes collector.
func NewKubernetesCollector(cfg KubernetesConfig) *KubernetesCollector {
	c := &KubernetesCollector{
		BaseCollector: NewBaseCollector("kubernetes"),
		config:        cfg,
		metrics: models.KubernetesMetrics{
			Context:    cfg.Context,
			Namespaces: cfg.Namespaces,
		},
	}
	c.clientsFor = c.newClients
	return c
}

// KubernetesIdentity describes which local credentials were resolved, so an
// operator can confirm the dashboard is pointed where they expect.
type KubernetesIdentity struct {
	// Source names where the credentials came from, for example the kubeconfig
	// files that were merged, or "in-cluster service account".
	Source string

	Context string
	Cluster string
	User    string
	Server  string
}

// loadingRules returns the kubeconfig discovery rules.
//
// The defaults are used as-is unless a path is configured explicitly. They
// already cover $KUBECONFIG, including its colon-separated multi-file merge, and
// ~/.kube/config, in kubectl's own precedence order. Setting ExplicitPath
// restricts loading to a single file, so doing that as a "helpful" fallback
// silently discarded any merge the operator had set up.
func (c KubernetesConfig) loadingRules() *clientcmd.ClientConfigLoadingRules {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.ConfigPath != "" {
		rules.ExplicitPath = c.ConfigPath
	}
	return rules
}

// newClients resolves credentials and builds the clientsets.
//
// Resolution order matches kubectl: an explicitly configured file, then
// $KUBECONFIG, then ~/.kube/config, then the in-cluster service account.
// Delegating to clientcmd is what makes exec credential plugins work, which is
// how EKS, GKE and AKS authenticate locally.
func (c *KubernetesCollector) newClients() (kubernetes.Interface, metricsv.Interface, string, error) {
	rules := c.config.loadingRules()

	overrides := &clientcmd.ConfigOverrides{}
	if c.config.Context != "" {
		overrides.CurrentContext = c.config.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

	identity := KubernetesIdentity{Source: describeKubeconfigSource(rules)}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		// In-cluster credentials are the normal case when maz-term runs inside a
		// pod and no kubeconfig exists.
		inCluster, inClusterErr := rest.InClusterConfig()
		if inClusterErr != nil {
			return nil, nil, "", fmt.Errorf(
				"no usable Kubernetes credentials (tried %s): %w", identity.Source, err)
		}
		restConfig = inCluster
		identity = KubernetesIdentity{Source: "in-cluster service account"}
	}

	// Without a timeout a wedged API server would hang a collection cycle.
	restConfig.Timeout = k8sCollectTimeout
	identity.Server = restConfig.Host

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, "", fmt.Errorf("building Kubernetes client: %w", err)
	}

	// The metrics API is an optional add-on; a nil client is handled downstream.
	metricsClient, err := metricsv.NewForConfig(restConfig)
	if err != nil {
		metricsClient = nil
	}

	clusterName := restConfig.Host
	if raw, err := clientConfig.RawConfig(); err == nil {
		identity.Context = raw.CurrentContext
		if c.config.Context != "" {
			identity.Context = c.config.Context
		}
		if ctx, ok := raw.Contexts[identity.Context]; ok && ctx != nil {
			identity.Cluster = ctx.Cluster
			identity.User = ctx.AuthInfo
		}
		if identity.Context != "" {
			clusterName = identity.Context
		}
	}

	c.mu.Lock()
	c.identity = identity
	c.mu.Unlock()

	return clientset, metricsClient, clusterName, nil
}

// describeKubeconfigSource names the files clientcmd will consult.
func describeKubeconfigSource(rules *clientcmd.ClientConfigLoadingRules) string {
	if rules.ExplicitPath != "" {
		return rules.ExplicitPath
	}

	existing := make([]string, 0, len(rules.Precedence))
	for _, path := range rules.Precedence {
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, path)
		}
	}

	if len(existing) == 0 {
		return "no kubeconfig found"
	}
	return strings.Join(existing, ", ")
}

// Identity reports the resolved credentials, for the UI and the -check report.
func (c *KubernetesCollector) Identity() KubernetesIdentity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identity
}

// CheckAccess resolves credentials and queries the server version, proving the
// cluster is genuinely reachable with what was found locally.
func (c *KubernetesCollector) CheckAccess(ctx context.Context) (KubernetesIdentity, string, error) {
	clientset, _, _, err := c.clientsFor()
	if err != nil {
		return c.Identity(), "", err
	}

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return c.Identity(), "", fmt.Errorf("reaching the API server: %w", err)
	}

	return c.Identity(), version.GitVersion, nil
}

// Collect gathers cluster state.
func (c *KubernetesCollector) Collect(ctx context.Context) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, k8sCollectTimeout)
	defer cancel()

	clientset, metricsClient, clusterName, err := c.clientsFor()
	if err != nil {
		result := models.KubernetesMetrics{
			Context:     c.config.Context,
			Namespaces:  c.config.Namespaces,
			LastUpdated: time.Now(),
			Errors:      []string{err.Error()},
		}
		c.publish(result)
		return result, err
	}

	result := models.KubernetesMetrics{
		ClusterName: clusterName,
		Context:     c.config.Context,
		Namespaces:  c.config.Namespaces,
		LastUpdated: time.Now(),
	}

	namespaces := c.config.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	listOpts := metav1.ListOptions{Limit: k8sMaxItems}

	if c.config.wants("nodes") {
		nodes, err := clientset.CoreV1().Nodes().List(ctx, listOpts)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("nodes: %v", err))
		} else {
			result.Nodes = convertNodes(nodes.Items)
		}
	}

	for _, namespace := range namespaces {
		if c.config.wants("pods") {
			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("pods in %s: %v", displayNamespace(namespace), err))
			} else {
				result.Pods = append(result.Pods, convertPods(pods.Items)...)
			}
		}

		if c.config.wants("deployments") {
			deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, listOpts)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("deployments in %s: %v", displayNamespace(namespace), err))
			} else {
				result.Deployments = append(result.Deployments, convertDeployments(deployments.Items)...)
			}
		}

		if c.config.wants("services") {
			services, err := clientset.CoreV1().Services(namespace).List(ctx, listOpts)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("services in %s: %v", displayNamespace(namespace), err))
			} else {
				result.Services = append(result.Services, convertServices(services.Items)...)
			}
		}
	}

	// Utilisation needs metrics-server, which many clusters do not run. Its
	// absence is not an error worth showing.
	if metricsClient != nil && len(result.Nodes) > 0 {
		if err := applyNodeUtilisation(ctx, metricsClient, result.Nodes); err != nil {
			c.Logger().Debug("metrics API unavailable", "error", err)
		}
	}

	// Stable ordering so tables do not reshuffle between frames.
	sort.Slice(result.Pods, func(i, j int) bool {
		if result.Pods[i].Namespace != result.Pods[j].Namespace {
			return result.Pods[i].Namespace < result.Pods[j].Namespace
		}
		return result.Pods[i].Name < result.Pods[j].Name
	})
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].Name < result.Nodes[j].Name })

	c.publish(result)

	// Nothing at all retrieved means the cluster is unreachable.
	if len(result.Errors) > 0 && len(result.Nodes) == 0 && len(result.Pods) == 0 &&
		len(result.Deployments) == 0 && len(result.Services) == 0 {
		return result, fmt.Errorf("kubernetes: %s", strings.Join(result.Errors, "; "))
	}

	return result, nil
}

// publish records the sample and persists its summary.
func (c *KubernetesCollector) publish(result models.KubernetesMetrics) {
	c.mu.Lock()
	c.metrics = result
	c.mu.Unlock()

	c.UpdateData(result)

	store := c.Storage()
	if store == nil {
		return
	}
	if err := store.StoreKubernetesSummary(models.KubernetesSummaryFrom(result)); err != nil {
		c.Logger().Error("failed to store kubernetes summary", "error", err)
	}
}

// convertNodes maps API nodes to the dashboard model.
func convertNodes(items []corev1.Node) []models.KubernetesNodeMetrics {
	nodes := make([]models.KubernetesNodeMetrics, 0, len(items))

	for _, node := range items {
		entry := models.KubernetesNodeMetrics{
			Name:              node.Name,
			Status:            "NotReady",
			Conditions:        make(map[string]string, len(node.Status.Conditions)),
			AllocatableCPU:    node.Status.Allocatable.Cpu().String(),
			AllocatableMemory: node.Status.Allocatable.Memory().String(),
			KernelVersion:     node.Status.NodeInfo.KernelVersion,
			OSImage:           node.Status.NodeInfo.OSImage,
			KubeletVersion:    node.Status.NodeInfo.KubeletVersion,
			LastUpdated:       time.Now(),
		}

		if pods, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			entry.AllocatablePods = int(pods.Value())
		}

		for _, condition := range node.Status.Conditions {
			entry.Conditions[string(condition.Type)] = string(condition.Status)
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				entry.Status = "Ready"
			}
		}

		// A cordoned node is worth distinguishing from a healthy one.
		if node.Spec.Unschedulable {
			entry.Status += ",SchedulingDisabled"
		}

		for _, taint := range node.Spec.Taints {
			entry.Taints = append(entry.Taints, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
		}

		nodes = append(nodes, entry)
	}

	return nodes
}

// convertPods maps API pods to the dashboard model.
func convertPods(items []corev1.Pod) []models.KubernetesPodMetrics {
	pods := make([]models.KubernetesPodMetrics, 0, len(items))

	for _, pod := range items {
		entry := models.KubernetesPodMetrics{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Phase:       string(pod.Status.Phase),
			Status:      string(pod.Status.Phase),
			Node:        pod.Spec.NodeName,
			IP:          pod.Status.PodIP,
			Labels:      pod.Labels,
			LastUpdated: time.Now(),
		}

		if pod.Status.StartTime != nil {
			entry.StartTime = pod.Status.StartTime.Time
		}

		// A pod stuck pulling an image or crash-looping reports Phase Pending or
		// Running, which hides the actual problem; the container's waiting reason
		// is the useful signal.
		for _, container := range pod.Status.ContainerStatuses {
			entry.RestartCount += int(container.RestartCount)

			state := "Unknown"
			switch {
			case container.State.Running != nil:
				state = "Running"
			case container.State.Waiting != nil:
				state = container.State.Waiting.Reason
				entry.Status = container.State.Waiting.Reason
			case container.State.Terminated != nil:
				state = container.State.Terminated.Reason
			}

			entry.Containers = append(entry.Containers, models.KubernetesContainerMetrics{
				Name:         container.Name,
				Image:        container.Image,
				Ready:        container.Ready,
				RestartCount: int(container.RestartCount),
				State:        state,
			})
		}

		for _, owner := range pod.OwnerReferences {
			entry.OwnerReferences = append(entry.OwnerReferences, models.KubernetesOwnerReference{
				Kind: owner.Kind,
				Name: owner.Name,
			})
		}

		pods = append(pods, entry)
	}

	return pods
}

// convertDeployments maps API deployments to the dashboard model.
func convertDeployments(items []appsv1.Deployment) []models.KubernetesDeploymentMetrics {
	deployments := make([]models.KubernetesDeploymentMetrics, 0, len(items))

	for _, deployment := range items {
		entry := models.KubernetesDeploymentMetrics{
			Name:                deployment.Name,
			Namespace:           deployment.Namespace,
			AvailableReplicas:   deployment.Status.AvailableReplicas,
			ReadyReplicas:       deployment.Status.ReadyReplicas,
			UpdatedReplicas:     deployment.Status.UpdatedReplicas,
			UnavailableReplicas: deployment.Status.UnavailableReplicas,
			Strategy:            string(deployment.Spec.Strategy.Type),
			Age:                 time.Since(deployment.CreationTimestamp.Time),
			Conditions:          make(map[string]string, len(deployment.Status.Conditions)),
			Labels:              deployment.Labels,
			LastUpdated:         time.Now(),
		}

		// Replicas defaults to 1 when unset, matching the API's own defaulting.
		entry.DesiredReplicas = 1
		if deployment.Spec.Replicas != nil {
			entry.DesiredReplicas = *deployment.Spec.Replicas
		}

		for _, condition := range deployment.Status.Conditions {
			entry.Conditions[string(condition.Type)] = string(condition.Status)
		}

		deployments = append(deployments, entry)
	}

	return deployments
}

// convertServices maps API services to the dashboard model.
func convertServices(items []corev1.Service) []models.KubernetesServiceMetrics {
	services := make([]models.KubernetesServiceMetrics, 0, len(items))

	for _, service := range items {
		entry := models.KubernetesServiceMetrics{
			Name:        service.Name,
			Namespace:   service.Namespace,
			Type:        string(service.Spec.Type),
			ClusterIP:   service.Spec.ClusterIP,
			ExternalIPs: service.Spec.ExternalIPs,
			Selector:    service.Spec.Selector,
			Labels:      service.Labels,
			Age:         time.Since(service.CreationTimestamp.Time),
			LastUpdated: time.Now(),
		}

		// A LoadBalancer's assigned address lives in status, not spec.
		for _, ingress := range service.Status.LoadBalancer.Ingress {
			switch {
			case ingress.IP != "":
				entry.ExternalIPs = append(entry.ExternalIPs, ingress.IP)
			case ingress.Hostname != "":
				entry.ExternalIPs = append(entry.ExternalIPs, ingress.Hostname)
			}
		}

		for _, port := range service.Spec.Ports {
			entry.Ports = append(entry.Ports, models.KubernetesServicePort{
				Name:     port.Name,
				Port:     port.Port,
				Protocol: string(port.Protocol),
				NodePort: port.NodePort,
			})
		}

		services = append(services, entry)
	}

	return services
}

// applyNodeUtilisation fills node CPU and memory percentages from the metrics
// API, which is an optional cluster add-on.
func applyNodeUtilisation(ctx context.Context, client metricsv.Interface, nodes []models.KubernetesNodeMetrics) error {
	usage, err := client.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	byName := make(map[string]int, len(nodes))
	for i, node := range nodes {
		byName[node.Name] = i
	}

	for _, item := range usage.Items {
		index, ok := byName[item.Name]
		if !ok {
			continue
		}

		node := &nodes[index]

		// Allocatable is a quantity string on the model, so it is reparsed here
		// to turn absolute usage into a percentage.
		if allocatable, err := resource.ParseQuantity(node.AllocatableCPU); err == nil && allocatable.MilliValue() > 0 {
			node.CPUUsage = float64(item.Usage.Cpu().MilliValue()) / float64(allocatable.MilliValue()) * 100
		}
		if allocatable, err := resource.ParseQuantity(node.AllocatableMemory); err == nil && allocatable.Value() > 0 {
			node.MemoryUsage = float64(item.Usage.Memory().Value()) / float64(allocatable.Value()) * 100
		}
	}

	return nil
}

// displayNamespace renders the all-namespaces sentinel readably.
func displayNamespace(namespace string) string {
	if namespace == metav1.NamespaceAll {
		return "all namespaces"
	}
	return namespace
}

// GetLatestMetrics returns the most recent sample.
func (c *KubernetesCollector) GetLatestMetrics() models.KubernetesMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Start begins periodic collection.
func (c *KubernetesCollector) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("kubernetes collection failed", "error", err)
		}
	})
}
