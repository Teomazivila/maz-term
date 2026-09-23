package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// newFakeK8s wires a collector to a fake clientset, so these tests never reach a
// cluster.
func newFakeK8s(t *testing.T, cfg KubernetesConfig, objects ...any) *KubernetesCollector {
	t.Helper()

	clientset := k8sfake.NewSimpleClientset(toRuntimeObjects(objects)...)

	c := NewKubernetesCollector(cfg)
	c.clientsFor = func() (kubernetes.Interface, metricsv.Interface, string, error) {
		// A nil metrics client exercises the common case of a cluster without
		// metrics-server installed.
		return clientset, nil, "test-cluster", nil
	}
	return c
}

func TestKubernetesCollectMapsClusterState(t *testing.T) {
	started := metav1.NewTime(time.Now().Add(-time.Hour))

	c := newFakeK8s(t, KubernetesConfig{Namespaces: []string{"default"}},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
			Spec:       corev1.NodeSpec{},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.34.1"},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-b"},
			Spec:       corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-a"},
			Status: corev1.PodStatus{
				Phase:     corev1.PodRunning,
				PodIP:     "10.1.2.3",
				StartTime: &started,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:         "api",
					Image:        "api:1.0",
					Ready:        true,
					RestartCount: 2,
					State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(3))},
			Status:     appsv1.DeploymentStatus{AvailableReplicas: 3, ReadyReplicas: 3},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.96.0.10",
				Ports:     []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		},
	)

	store := &recordingStore{}
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.Equal(t, "test-cluster", metrics.ClusterName)
	assert.Empty(t, metrics.Errors)

	require.Len(t, metrics.Nodes, 2)
	assert.Equal(t, "node-a", metrics.Nodes[0].Name)
	assert.Equal(t, "Ready", metrics.Nodes[0].Status)
	assert.Equal(t, "4", metrics.Nodes[0].AllocatableCPU)
	assert.Equal(t, 110, metrics.Nodes[0].AllocatablePods)
	assert.Equal(t, "v1.34.1", metrics.Nodes[0].KubeletVersion)

	// A cordoned node is distinguished from a healthy one.
	assert.Contains(t, metrics.Nodes[1].Status, "SchedulingDisabled")

	require.Len(t, metrics.Pods, 1)
	assert.Equal(t, "api-1", metrics.Pods[0].Name)
	assert.Equal(t, "Running", metrics.Pods[0].Phase)
	assert.Equal(t, "node-a", metrics.Pods[0].Node)
	assert.Equal(t, 2, metrics.Pods[0].RestartCount)
	require.Len(t, metrics.Pods[0].Containers, 1)
	assert.True(t, metrics.Pods[0].Containers[0].Ready)

	require.Len(t, metrics.Deployments, 1)
	assert.Equal(t, int32(3), metrics.Deployments[0].DesiredReplicas)
	assert.Equal(t, int32(3), metrics.Deployments[0].AvailableReplicas)

	require.Len(t, metrics.Services, 1)
	assert.Equal(t, "10.96.0.10", metrics.Services[0].ClusterIP)
	require.Len(t, metrics.Services[0].Ports, 1)
	assert.Equal(t, int32(80), metrics.Services[0].Ports[0].Port)

	_, k8s, _ := store.infraCounts()
	assert.Equal(t, 1, k8s, "a summary must be persisted")
}

// TestKubernetesSurfacesCrashLoopBehindRunningPhase pins the useful signal: a pod
// whose phase is fine but whose container is crash-looping is not healthy.
func TestKubernetesSurfacesCrashLoopBehindRunningPhase(t *testing.T) {
	c := newFakeK8s(t, KubernetesConfig{Namespaces: []string{"default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:         "app",
					RestartCount: 7,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: "CrashLoopBackOff",
					}},
				}},
			},
		},
	)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	pod := c.GetLatestMetrics().Pods[0]
	assert.Equal(t, "Running", pod.Phase, "the phase is still Running")
	assert.Equal(t, "CrashLoopBackOff", pod.Status,
		"the container's waiting reason is what the operator needs to see")
	assert.Equal(t, 7, pod.RestartCount)
}

// TestKubernetesUnreachableClusterIsAnError pins the honest failure: no data plus
// no error would look like a healthy empty cluster.
func TestKubernetesUnreachableClusterIsAnError(t *testing.T) {
	c := NewKubernetesCollector(KubernetesConfig{})
	c.clientsFor = func() (kubernetes.Interface, metricsv.Interface, string, error) {
		return nil, nil, "", errors.New("no usable Kubernetes credentials")
	}

	_, err := c.Collect(context.Background())
	require.Error(t, err)

	metrics := c.GetLatestMetrics()
	require.Len(t, metrics.Errors, 1)
	assert.Contains(t, metrics.Errors[0], "credentials")
	assert.Empty(t, metrics.Nodes)
}

// TestKubernetesUnreachableClusterStillPersistsSummary records the outage rather
// than leaving a gap that looks like the collector was not running.
func TestKubernetesUnreachableClusterStillPersistsSummary(t *testing.T) {
	store := &recordingStore{}
	c := NewKubernetesCollector(KubernetesConfig{})
	c.SetStorageProvider(store)
	c.clientsFor = func() (kubernetes.Interface, metricsv.Interface, string, error) {
		return nil, nil, "", errors.New("connection refused")
	}

	_, _ = c.Collect(context.Background())

	_, k8s, _ := store.infraCounts()
	assert.Equal(t, 1, k8s)
}

func TestKubernetesResourceSelection(t *testing.T) {
	c := newFakeK8s(t, KubernetesConfig{
		Namespaces: []string{"default"},
		Resources:  []string{"pods"},
	},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "default"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
	)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.Len(t, metrics.Pods, 1)
	assert.Empty(t, metrics.Nodes, "nodes were not requested")
	assert.Empty(t, metrics.Deployments, "deployments were not requested")
}

func TestKubernetesMultipleNamespaces(t *testing.T) {
	c := newFakeK8s(t, KubernetesConfig{Namespaces: []string{"a", "b"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "a"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "b"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "c"}},
	)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	pods := c.GetLatestMetrics().Pods
	require.Len(t, pods, 2, "only the configured namespaces are listed")
	// Sorted by namespace then name for a stable table.
	assert.Equal(t, "a", pods[0].Namespace)
	assert.Equal(t, "b", pods[1].Namespace)
}

func TestKubernetesDeploymentReplicaDefault(t *testing.T) {
	// Replicas unset means one, matching the API's own defaulting.
	deployments := convertDeployments([]appsv1.Deployment{{
		ObjectMeta: metav1.ObjectMeta{Name: "api", CreationTimestamp: metav1.Now()},
		Spec:       appsv1.DeploymentSpec{},
	}})

	require.Len(t, deployments, 1)
	assert.Equal(t, int32(1), deployments[0].DesiredReplicas)
}

func TestKubernetesServiceExternalAddressFromStatus(t *testing.T) {
	services := convertServices([]corev1.Service{{
		ObjectMeta: metav1.ObjectMeta{Name: "lb", CreationTimestamp: metav1.Now()},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{IP: "203.0.113.10"},
				{Hostname: "lb.example.com"},
			},
		}},
	}})

	require.Len(t, services, 1)
	assert.Contains(t, services[0].ExternalIPs, "203.0.113.10")
	assert.Contains(t, services[0].ExternalIPs, "lb.example.com")
}

func TestKubernetesSummaryCountsPhases(t *testing.T) {
	summary := models.KubernetesSummaryFrom(models.KubernetesMetrics{
		Nodes: []models.KubernetesNodeMetrics{
			{Status: "Ready"}, {Status: "Ready"}, {Status: "NotReady"},
		},
		Pods: []models.KubernetesPodMetrics{
			{Phase: "Running", RestartCount: 1},
			{Phase: "Running"},
			{Phase: "Pending"},
			{Phase: "Failed", RestartCount: 4},
			{Phase: "Succeeded"},
		},
		Deployments: []models.KubernetesDeploymentMetrics{
			{DesiredReplicas: 2, AvailableReplicas: 2},
			{DesiredReplicas: 3, AvailableReplicas: 1},
		},
	})

	assert.Equal(t, 3, summary.NodesTotal)
	assert.Equal(t, 2, summary.NodesReady)
	assert.Equal(t, 5, summary.PodsTotal)
	assert.Equal(t, 2, summary.PodsRunning)
	assert.Equal(t, 1, summary.PodsPending)
	assert.Equal(t, 1, summary.PodsFailed)
	assert.Equal(t, 1, summary.PodsSucceeded)
	assert.Equal(t, 5, summary.RestartsTotal)
	assert.Equal(t, 2, summary.DeploymentsTotal)
	assert.Equal(t, 1, summary.DeploymentsAvailable,
		"a deployment short of its desired replicas is not available")
}

func TestDisplayNamespace(t *testing.T) {
	assert.Equal(t, "all namespaces", displayNamespace(metav1.NamespaceAll))
	assert.Equal(t, "kube-system", displayNamespace("kube-system"))
}

// toRuntimeObjects converts test fixtures to the runtime.Object slice the fake
// clientset expects.
func toRuntimeObjects(objects []any) []runtime.Object {
	out := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		if typed, ok := object.(runtime.Object); ok {
			out = append(out, typed)
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// TestKubeconfigLoadingRulesKeepDefaultPrecedence pins the fix for a regression I
// introduced: setting ExplicitPath to ~/.kube/config as a "helpful" fallback
// restricts loading to that single file and silently discards the $KUBECONFIG
// multi-file merge that clientcmd's defaults already handle.
func TestKubeconfigLoadingRulesKeepDefaultPrecedence(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/a.yaml:/tmp/b.yaml")

	rules := KubernetesConfig{}.loadingRules()

	assert.Empty(t, rules.ExplicitPath,
		"an unconfigured path must leave clientcmd's own precedence intact")
	assert.Contains(t, rules.Precedence, "/tmp/a.yaml")
	assert.Contains(t, rules.Precedence, "/tmp/b.yaml",
		"every file in KUBECONFIG must remain in the precedence chain")
}

func TestKubeconfigExplicitPathIsHonoured(t *testing.T) {
	rules := KubernetesConfig{ConfigPath: "/custom/kubeconfig"}.loadingRules()

	assert.Equal(t, "/custom/kubeconfig", rules.ExplicitPath)
}

func TestDescribeKubeconfigSource(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.yaml")
	require.NoError(t, os.WriteFile(present, []byte("apiVersion: v1\n"), 0o600))

	t.Setenv("KUBECONFIG", present+string(os.PathListSeparator)+filepath.Join(dir, "absent.yaml"))

	source := describeKubeconfigSource(KubernetesConfig{}.loadingRules())
	assert.Contains(t, source, "present.yaml")
	assert.NotContains(t, source, "absent.yaml", "only files that exist are named")

	explicit := describeKubeconfigSource(KubernetesConfig{ConfigPath: "/x/y"}.loadingRules())
	assert.Equal(t, "/x/y", explicit)
}

func TestDescribeKubeconfigSourceWithNothingPresent(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("HOME", t.TempDir())

	assert.Equal(t, "no kubeconfig found",
		describeKubeconfigSource(KubernetesConfig{}.loadingRules()))
}

// TestKubernetesCredentialFailureNamesWhatWasTried keeps the error actionable.
func TestKubernetesCredentialFailureNamesWhatWasTried(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	t.Setenv("HOME", t.TempDir())

	c := NewKubernetesCollector(KubernetesConfig{})

	_, _, _, err := c.newClients()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable Kubernetes credentials")
	assert.Contains(t, err.Error(), "tried")
}
