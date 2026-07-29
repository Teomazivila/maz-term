package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEC2, fakeS3, fakeRDS and fakeCloudWatch stand in for the SDK clients, so
// these tests never reach AWS.
type fakeEC2 struct {
	out *ec2.DescribeInstancesOutput
	err error
}

func (f fakeEC2) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type fakeS3 struct {
	out *s3.ListBucketsOutput
	err error
}

func (f fakeS3) ListBuckets(context.Context, *s3.ListBucketsInput, ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type fakeRDS struct {
	out *rds.DescribeDBInstancesOutput
	err error
}

func (f fakeRDS) DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

type fakeCloudWatch struct {
	out   *cloudwatch.GetMetricDataOutput
	err   error
	calls int
}

func (f *fakeCloudWatch) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

// newFakeAWS builds a collector wired to the given fakes.
func newFakeAWS(t *testing.T, cfg AWSConfig, clients *awsClients) *AWSCollector {
	t.Helper()

	c := NewAWSCollector(cfg)
	c.clientsFor = func(context.Context, string) (*awsClients, error) { return clients, nil }
	return c
}

func TestAWSCollectMapsInventory(t *testing.T) {
	launched := time.Now().Add(-48 * time.Hour)

	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
				{
					InstanceId:   aws.String("i-111"),
					InstanceType: ec2types.InstanceTypeT3Medium,
					State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
					LaunchTime:   aws.Time(launched),
					Tags: []ec2types.Tag{
						{Key: aws.String("Name"), Value: aws.String("web-1")},
						{Key: aws.String("env"), Value: aws.String("prod")},
					},
				},
				{
					InstanceId:   aws.String("i-222"),
					InstanceType: ec2types.InstanceTypeT3Small,
					State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
				},
			}}},
		}},
		s3: fakeS3{out: &s3.ListBucketsOutput{Buckets: []s3types.Bucket{
			{Name: aws.String("assets")},
		}}},
		rds: fakeRDS{out: &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{{
			DBInstanceIdentifier: aws.String("orders"),
			DBInstanceClass:      aws.String("db.t3.medium"),
			Engine:               aws.String("postgres"),
			EngineVersion:        aws.String("16.3"),
			DBInstanceStatus:     aws.String("available"),
		}}}},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{}},
	}

	store := &recordingStore{}
	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1"}, clients)
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	require.Len(t, metrics.InstanceMetrics, 2)
	assert.Empty(t, metrics.Errors)

	// Sorted by ID for a stable table.
	assert.Equal(t, "i-111", metrics.InstanceMetrics[0].ID)
	assert.Equal(t, "web-1", metrics.InstanceMetrics[0].Name, "the Name tag becomes the display name")
	assert.Equal(t, "t3.medium", metrics.InstanceMetrics[0].Type)
	assert.Equal(t, models.ResourceStatusRunning, metrics.InstanceMetrics[0].Status)
	assert.InDelta(t, 48, metrics.InstanceMetrics[0].UptimeHours, 1)
	assert.Equal(t, "prod", metrics.InstanceMetrics[0].Tags["env"])

	// An instance without a Name tag falls back to its ID.
	assert.Equal(t, "i-222", metrics.InstanceMetrics[1].Name)
	assert.Equal(t, models.ResourceStatusStopped, metrics.InstanceMetrics[1].Status)

	require.Len(t, metrics.StorageMetrics, 1)
	assert.Equal(t, "assets", metrics.StorageMetrics[0].Name)

	require.Len(t, metrics.DatabaseMetrics, 1)
	assert.Equal(t, "orders", metrics.DatabaseMetrics[0].ID)
	assert.Equal(t, "postgres 16.3", metrics.DatabaseMetrics[0].Engine)
	assert.Equal(t, models.ResourceStatusRunning, metrics.DatabaseMetrics[0].Status)

	// The persisted summary is an aggregate, not per-resource rows.
	cloud, _, _ := store.infraCounts()
	assert.Equal(t, 1, cloud)

	summary := store.lastCloud()
	assert.Equal(t, 2, summary.InstancesTotal)
	assert.Equal(t, 1, summary.InstancesRunning)
	assert.Equal(t, 1, summary.InstancesStopped)
	assert.Equal(t, 1, summary.BucketsTotal)
	assert.Equal(t, 1, summary.DatabasesTotal)
	assert.Zero(t, summary.ErrorCount)
}

func TestAWSCollectAppliesCloudWatchUtilisation(t *testing.T) {
	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String("i-111"),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
			}}}},
		}},
		s3:  fakeS3{out: &s3.ListBucketsOutput{}},
		rds: fakeRDS{out: &rds.DescribeDBInstancesOutput{}},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{
			MetricDataResults: []cwtypes.MetricDataResult{
				// q0 is CPUUtilization for the first instance, q1 NetworkIn,
				// q2 NetworkOut, in the order the queries are built.
				{Id: aws.String("q0"), Values: []float64{37.5, 20.0}},
				{Id: aws.String("q1"), Values: []float64{2048}},
				{Id: aws.String("q2"), Values: []float64{1024}},
			},
		}},
	}

	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1"}, clients)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	instance := c.GetLatestMetrics().InstanceMetrics[0]
	assert.InDelta(t, 37.5, instance.CPUUtilization, 0.001, "the newest datapoint is used")
	assert.Equal(t, uint64(2048), instance.NetworkIn)
	assert.Equal(t, uint64(1024), instance.NetworkOut)
}

// TestAWSCloudWatchFailureKeepsInventory pins the best-effort treatment: a missing
// CloudWatch permission must not hide the instances themselves.
func TestAWSCloudWatchFailureKeepsInventory(t *testing.T) {
	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String("i-111"),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
			}}}},
		}},
		s3:         fakeS3{out: &s3.ListBucketsOutput{}},
		rds:        fakeRDS{out: &rds.DescribeDBInstancesOutput{}},
		cloudwatch: &fakeCloudWatch{err: errors.New("AccessDenied")},
	}

	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1"}, clients)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	require.Len(t, metrics.InstanceMetrics, 1)
	assert.Zero(t, metrics.InstanceMetrics[0].CPUUtilization, "unknown utilisation stays zero")
	assert.Empty(t, metrics.Errors, "a utilisation gap is not an inventory failure")
}

// TestAWSPartialFailureIsReported ensures a failing service is surfaced while the
// rest of the inventory is still shown.
func TestAWSPartialFailureIsReported(t *testing.T) {
	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String("i-111"),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
			}}}},
		}},
		s3:         fakeS3{err: errors.New("AccessDenied: s3:ListAllMyBuckets")},
		rds:        fakeRDS{out: &rds.DescribeDBInstancesOutput{}},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{}},
	}

	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1"}, clients)

	_, err := c.Collect(context.Background())
	require.NoError(t, err, "one failing service is partial data, not a failed sweep")

	metrics := c.GetLatestMetrics()
	assert.Len(t, metrics.InstanceMetrics, 1)
	require.Len(t, metrics.Errors, 1)
	assert.Contains(t, metrics.Errors[0], "s3")
}

// TestAWSTotalFailureIsAnError ensures a completely unreachable account reports an
// error rather than an empty but apparently healthy view.
func TestAWSTotalFailureIsAnError(t *testing.T) {
	clients := &awsClients{
		ec2:        fakeEC2{err: errors.New("no credentials")},
		s3:         fakeS3{err: errors.New("no credentials")},
		rds:        fakeRDS{err: errors.New("no credentials")},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{}},
	}

	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1"}, clients)

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no credentials")
	assert.NotEmpty(t, c.GetLatestMetrics().Errors)
}

func TestAWSRequiresARegion(t *testing.T) {
	c := NewAWSCollector(AWSConfig{})

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "region")
}

func TestAWSClientConstructionFailureIsReported(t *testing.T) {
	c := NewAWSCollector(AWSConfig{Region: "eu-west-1"})
	c.clientsFor = func(context.Context, string) (*awsClients, error) {
		return nil, errors.New("profile \"missing\" not found")
	}

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile")
}

// TestAWSBucketsListedOnceAcrossRegions pins the guard against repeating the
// global S3 inventory per region.
func TestAWSBucketsListedOnceAcrossRegions(t *testing.T) {
	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{}},
		s3: fakeS3{out: &s3.ListBucketsOutput{Buckets: []s3types.Bucket{
			{Name: aws.String("assets")},
		}}},
		rds:        fakeRDS{out: &rds.DescribeDBInstancesOutput{}},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{}},
	}

	c := newFakeAWS(t, AWSConfig{
		Region:            "eu-west-1",
		AdditionalRegions: []string{"us-east-1", "eu-central-1"},
	}, clients)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.Len(t, c.GetLatestMetrics().StorageMetrics, 1,
		"ListBuckets is global, so the bucket must appear once")
	assert.Len(t, c.GetLatestMetrics().Regions, 3)
}

func TestAWSResourceSelection(t *testing.T) {
	clients := &awsClients{
		ec2: fakeEC2{out: &ec2.DescribeInstancesOutput{
			Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String("i-111"),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
			}}}},
		}},
		s3: fakeS3{out: &s3.ListBucketsOutput{Buckets: []s3types.Bucket{
			{Name: aws.String("assets")},
		}}},
		rds: fakeRDS{out: &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{{
			DBInstanceIdentifier: aws.String("orders"),
			DBInstanceStatus:     aws.String("available"),
		}}}},
		cloudwatch: &fakeCloudWatch{out: &cloudwatch.GetMetricDataOutput{}},
	}

	c := newFakeAWS(t, AWSConfig{Region: "eu-west-1", Resources: []string{"ec2"}}, clients)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.Len(t, metrics.InstanceMetrics, 1)
	assert.Empty(t, metrics.StorageMetrics, "s3 was not requested")
	assert.Empty(t, metrics.DatabaseMetrics, "rds was not requested")
}

func TestAWSRegionsDeduplicated(t *testing.T) {
	cfg := AWSConfig{
		Region:            "eu-west-1",
		AdditionalRegions: []string{"eu-west-1", " us-east-1 ", "", "us-east-1"},
	}

	assert.Equal(t, []string{"eu-west-1", "us-east-1"}, cfg.regions())
}

func TestEC2StateMapping(t *testing.T) {
	tests := []struct {
		name ec2types.InstanceStateName
		want models.ResourceStatus
	}{
		{ec2types.InstanceStateNameRunning, models.ResourceStatusRunning},
		{ec2types.InstanceStateNameStopped, models.ResourceStatusStopped},
		{ec2types.InstanceStateNameTerminated, models.ResourceStatusStopped},
		{ec2types.InstanceStateNamePending, models.ResourceStatusPending},
		{ec2types.InstanceStateNameStopping, models.ResourceStatusPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.name), func(t *testing.T) {
			assert.Equal(t, tt.want, ec2StateToStatus(&ec2types.InstanceState{Name: tt.name}))
		})
	}

	assert.Equal(t, models.ResourceStatusPending, ec2StateToStatus(nil),
		"a missing state must not panic")
}

func TestRDSStatusMapping(t *testing.T) {
	assert.Equal(t, models.ResourceStatusRunning, rdsStatusToStatus("available"))
	assert.Equal(t, models.ResourceStatusStopped, rdsStatusToStatus("stopped"))
	assert.Equal(t, models.ResourceStatusPending, rdsStatusToStatus("backing-up"))
	assert.Equal(t, models.ResourceStatusError, rdsStatusToStatus("failed"))
}

// TestCloudSummaryMeanExcludesUnreportedInstances pins the guard against an
// account without CloudWatch access showing a mean CPU of zero.
func TestCloudSummaryMeanExcludesUnreportedInstances(t *testing.T) {
	summary := models.CloudSummaryFrom(models.CloudProviderMetrics{
		InstanceMetrics: []models.CloudInstanceMetrics{
			{Status: models.ResourceStatusRunning, CPUUtilization: 40},
			{Status: models.ResourceStatusRunning, CPUUtilization: 60},
			{Status: models.ResourceStatusRunning, CPUUtilization: 0},
		},
	})

	assert.InDelta(t, 50.0, summary.MeanCPUUtilization, 0.001,
		"only instances that reported a datapoint contribute")
	assert.Equal(t, 3, summary.InstancesRunning)
}

func TestCloudSummaryTimestampNeverZero(t *testing.T) {
	summary := models.CloudSummaryFrom(models.CloudProviderMetrics{})
	assert.False(t, summary.Timestamp.IsZero(),
		"a zero timestamp would be written as year 1 and excluded from every query")
}
