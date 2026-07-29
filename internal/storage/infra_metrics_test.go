package storage

import (
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudSummaryRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{
		Timestamp:          time.Now(),
		ProviderType:       "aws",
		InstancesTotal:     5,
		InstancesRunning:   4,
		InstancesStopped:   1,
		MeanCPUUtilization: 42.5,
		BucketsTotal:       3,
		DatabasesTotal:     2,
	}))

	running, err := db.GetCloudInstanceCountHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, running, 1)
	assert.InDelta(t, 4, running[0].Value, 0.001)

	cpu, err := db.GetCloudCPUHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, cpu, 1)
	assert.InDelta(t, 42.5, cpu[0].Value, 0.001)
}

func TestKubernetesSummaryRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreKubernetesSummary(models.KubernetesSummary{
		Timestamp:   time.Now(),
		ClusterName: "prod",
		NodesTotal:  6,
		NodesReady:  5,
		PodsTotal:   80,
		PodsRunning: 74,
		PodsPending: 4,
		PodsFailed:  2,
	}))

	pods, err := db.GetKubernetesPodCountHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, pods, 1)
	assert.InDelta(t, 74, pods[0].Value, 0.001)

	nodes, err := db.GetKubernetesNodeReadyHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.InDelta(t, 5, nodes[0].Value, 0.001)
}

func TestCICDSummaryRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreCICDSummary(models.CICDSummary{
		Timestamp:           time.Now(),
		ProviderType:        "github_actions",
		WorkflowsTotal:      4,
		WorkflowsFailed:     1,
		RunsRunning:         2,
		SuccessRate:         87.5,
		MeanDurationSeconds: 195,
	}))

	rate, err := db.GetCICDSuccessRateHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, rate, 1)
	assert.InDelta(t, 87.5, rate[0].Value, 0.001)
}

// TestInfraSummaryZeroTimestampIsQueryable pins the same defect that hid HTTP
// samples: a zero timestamp written as year 1 is excluded by every time window.
func TestInfraSummaryZeroTimestampIsQueryable(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{ProviderType: "aws", InstancesRunning: 2}))
	require.NoError(t, db.StoreKubernetesSummary(models.KubernetesSummary{ClusterName: "c", PodsRunning: 3}))
	require.NoError(t, db.StoreCICDSummary(models.CICDSummary{ProviderType: "gha", SuccessRate: 100}))

	cloud, err := db.GetCloudInstanceCountHistory(time.Hour, 100)
	require.NoError(t, err)
	assert.Len(t, cloud, 1)

	pods, err := db.GetKubernetesPodCountHistory(time.Hour, 100)
	require.NoError(t, err)
	assert.Len(t, pods, 1)

	rate, err := db.GetCICDSuccessRateHistory(time.Hour, 100)
	require.NoError(t, err)
	assert.Len(t, rate, 1)
}

func TestInfraSeriesRejectsNonPositivePeriod(t *testing.T) {
	db := newTestDB(t)

	_, err := db.GetCloudInstanceCountHistory(0, 10)
	require.Error(t, err)

	_, err = db.GetKubernetesPodCountHistory(-time.Hour, 10)
	require.Error(t, err)
}

func TestInfraSeriesExcludesRowsOutsideThePeriod(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{
		Timestamp: time.Now().Add(-48 * time.Hour), InstancesRunning: 9,
	}))
	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{
		Timestamp: time.Now(), InstancesRunning: 3,
	}))

	recent, err := db.GetCloudInstanceCountHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assert.InDelta(t, 3, recent[0].Value, 0.001)

	all, err := db.GetCloudInstanceCountHistory(72*time.Hour, 100)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestInfraSeriesDownsamples(t *testing.T) {
	db := newTestDB(t)

	for i := range 40 {
		require.NoError(t, db.StoreCloudSummary(models.CloudSummary{
			Timestamp:        time.Now().Add(-time.Duration(i) * time.Minute),
			InstancesRunning: i,
		}))
	}

	points, err := db.GetCloudInstanceCountHistory(2*time.Hour, 10)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(points), 10)
	assert.NotEmpty(t, points)
}

// TestInfraSummariesAreSubjectToRetention keeps the summary tables from growing
// without bound.
func TestInfraSummariesAreSubjectToRetention(t *testing.T) {
	db := newTestDB(t)
	db.retentionPeriod = time.Hour

	old := time.Now().Add(-5 * time.Hour)
	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{Timestamp: old}))
	require.NoError(t, db.StoreKubernetesSummary(models.KubernetesSummary{Timestamp: old}))
	require.NoError(t, db.StoreCICDSummary(models.CICDSummary{Timestamp: old}))
	require.NoError(t, db.StoreCloudSummary(models.CloudSummary{Timestamp: time.Now()}))

	require.NoError(t, db.cleanupOldData())

	for _, table := range []string{"kubernetes_summary", "cicd_summary"} {
		var count int
		require.NoError(t, db.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Zero(t, count, "%s rows past the cutoff must be removed", table)
	}

	var cloud int
	require.NoError(t, db.db.QueryRow("SELECT COUNT(*) FROM cloud_summary").Scan(&cloud))
	assert.Equal(t, 1, cloud, "only the row within retention survives")
}

func TestAdapterInfraPassthrough(t *testing.T) {
	adapter := NewAdapter(newTestDB(t))

	require.NoError(t, adapter.StoreCloudSummary(models.CloudSummary{InstancesRunning: 1}))
	require.NoError(t, adapter.StoreKubernetesSummary(models.KubernetesSummary{PodsRunning: 2}))
	require.NoError(t, adapter.StoreCICDSummary(models.CICDSummary{SuccessRate: 3}))

	cloud, err := adapter.GetCloudInstanceCountHistory(time.Hour, 10)
	require.NoError(t, err)
	assert.Len(t, cloud, 1)

	cpu, err := adapter.GetCloudCPUHistory(time.Hour, 10)
	require.NoError(t, err)
	assert.Len(t, cpu, 1)

	pods, err := adapter.GetKubernetesPodCountHistory(time.Hour, 10)
	require.NoError(t, err)
	assert.Len(t, pods, 1)

	nodes, err := adapter.GetKubernetesNodeReadyHistory(time.Hour, 10)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)

	rate, err := adapter.GetCICDSuccessRateHistory(time.Hour, 10)
	require.NoError(t, err)
	assert.Len(t, rate, 1)
}
