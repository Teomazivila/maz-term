package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	// awsCollectTimeout bounds one full sweep across every configured region.
	awsCollectTimeout = 60 * time.Second

	// awsMaxResults bounds how much of an account is enumerated per region, so a
	// large estate cannot make a refresh cycle unbounded in time or API cost.
	awsMaxResults = 200

	// cloudWatchLookback is the window utilisation is averaged over. EC2 sends
	// basic monitoring datapoints every five minutes.
	cloudWatchLookback = 15 * time.Minute

	// cloudWatchBatchSize is the maximum queries per GetMetricData call.
	cloudWatchBatchSize = 100
)

// AWSConfig configures the AWS collector.
type AWSConfig struct {
	Region            string
	Profile           string
	AdditionalRegions []string

	// Resources selects which services to enumerate. An empty slice means all
	// supported services.
	Resources []string
}

// regions returns the distinct regions to collect from, primary first.
func (c AWSConfig) regions() []string {
	seen := make(map[string]struct{}, len(c.AdditionalRegions)+1)
	var out []string

	for _, region := range append([]string{c.Region}, c.AdditionalRegions...) {
		region = strings.TrimSpace(region)
		if region == "" {
			continue
		}
		if _, dup := seen[region]; dup {
			continue
		}
		seen[region] = struct{}{}
		out = append(out, region)
	}

	return out
}

// wants reports whether a service should be collected.
func (c AWSConfig) wants(service string) bool {
	if len(c.Resources) == 0 {
		return true
	}
	for _, want := range c.Resources {
		if strings.EqualFold(strings.TrimSpace(want), service) {
			return true
		}
	}
	return false
}

// AWSCollector collects EC2, S3 and RDS inventory with CloudWatch utilisation.
//
// Every call it makes is read-only. Credentials are never taken from
// configuration: resolution is delegated to the SDK's default chain, which reads
// the environment, the shared profile, SSO and instance metadata.
type AWSCollector struct {
	*BaseCollector

	config AWSConfig

	mu             sync.RWMutex
	metrics        models.CloudProviderMetrics
	identity       AWSIdentity
	resolvedRegion string

	// clientsFor builds the per-region API clients. It is a field so tests can
	// substitute fakes without reaching AWS.
	clientsFor func(ctx context.Context, region string) (*awsClients, error)
}

// awsClients groups the per-region service clients.
type awsClients struct {
	ec2        ec2API
	s3         s3API
	rds        rdsAPI
	cloudwatch cloudWatchAPI
}

// The four interfaces below cover only the operations used, so tests can provide
// fakes without depending on the concrete SDK clients.
type ec2API interface {
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type s3API interface {
	ListBuckets(ctx context.Context, in *s3.ListBucketsInput, opts ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
}

type rdsAPI interface {
	DescribeDBInstances(ctx context.Context, in *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

type cloudWatchAPI interface {
	GetMetricData(ctx context.Context, in *cloudwatch.GetMetricDataInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

// NewAWSCollector creates an AWS collector.
func NewAWSCollector(cfg AWSConfig) *AWSCollector {
	c := &AWSCollector{
		BaseCollector: NewBaseCollector("aws"),
		config:        cfg,
		metrics: models.CloudProviderMetrics{
			ProviderType: "aws",
			Name:         "AWS",
			Regions:      cfg.regions(),
		},
	}
	c.clientsFor = c.newClients
	return c
}

// AWSIdentity describes which local credentials were resolved, so an operator can
// confirm the dashboard is pointed at the account they expect.
type AWSIdentity struct {
	// Source names the credential provider the SDK chose, for example
	// "SharedConfigCredentials: /home/me/.aws/credentials" or "SSOProvider".
	Source string

	Profile string
	Region  string

	// Account and ARN are only populated by CheckAccess, which calls STS.
	Account string
	ARN     string
}

// loadOptions returns the SDK load options for a region.
//
// A region is only pinned when one is configured. Otherwise the SDK resolves it
// from AWS_REGION, AWS_DEFAULT_REGION or the shared profile, which is what makes
// local usage work with no maz-term configuration beyond enabling the provider.
func (c *AWSCollector) loadOptions(region string) []func(*awsconfig.LoadOptions) error {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	// An empty profile leaves the SDK's own resolution intact, which honours
	// AWS_PROFILE and falls back to "default".
	if c.config.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(c.config.Profile))
	}
	return opts
}

// newClients builds real SDK clients for a region.
func (c *AWSCollector) newClients(ctx context.Context, region string) (*awsClients, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, c.loadOptions(region)...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS configuration for %s: %w", region, err)
	}

	return &awsClients{
		ec2:        ec2.NewFromConfig(cfg),
		s3:         s3.NewFromConfig(cfg),
		rds:        rds.NewFromConfig(cfg),
		cloudwatch: cloudwatch.NewFromConfig(cfg),
	}, nil
}

// resolveRegions returns the regions to collect from.
//
// When none is configured it asks the SDK, which reads AWS_REGION,
// AWS_DEFAULT_REGION and the shared profile. Requiring cloud.aws.region in
// maz-term's own file duplicated a setting the operator already has locally.
func (c *AWSCollector) resolveRegions(ctx context.Context) ([]string, error) {
	if configured := c.config.regions(); len(configured) > 0 {
		return configured, nil
	}

	c.mu.RLock()
	cached := c.resolvedRegion
	c.mu.RUnlock()
	if cached != "" {
		return []string{cached}, nil
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, c.loadOptions("")...)
	if err != nil {
		return nil, fmt.Errorf("resolving AWS region from the environment: %w", err)
	}
	if cfg.Region == "" {
		return nil, errors.New(
			"aws: no region found. Set cloud.aws.region, AWS_REGION, " +
				"or a region in your shared profile (~/.aws/config)")
	}

	c.mu.Lock()
	c.resolvedRegion = cfg.Region
	c.mu.Unlock()

	return []string{cfg.Region}, nil
}

// recordIdentity notes which profile and region the SDK resolved.
//
// It deliberately does not retrieve the credentials themselves. Doing so is a
// network call for SSO and web-identity providers, and putting it on the
// collection path cost seconds per cycle purely to populate a display field. The
// credential source is filled in by CheckAccess, which is already making a call.
// If the credentials are broken, this cycle's real API calls will say so.
func (c *AWSCollector) recordIdentity(ctx context.Context, region string) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, c.loadOptions(region)...)
	if err != nil {
		return
	}

	identity := AWSIdentity{
		Profile: c.config.Profile,
		Region:  cfg.Region,
	}
	if identity.Profile == "" {
		identity.Profile = os.Getenv("AWS_PROFILE")
	}
	if identity.Profile == "" {
		identity.Profile = "default"
	}

	c.mu.Lock()
	// A source already established by CheckAccess is not overwritten.
	if c.identity.Source != "" {
		identity.Source = c.identity.Source
	}
	c.identity = identity
	c.mu.Unlock()
}

// Identity reports the resolved credentials, for the UI and the -check report.
func (c *AWSCollector) Identity() AWSIdentity {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identity
}

// CheckAccess resolves local credentials and calls STS GetCallerIdentity, proving
// they work and naming the account they belong to.
func (c *AWSCollector) CheckAccess(ctx context.Context) (AWSIdentity, error) {
	regions, err := c.resolveRegions(ctx)
	if err != nil {
		return c.Identity(), err
	}

	c.recordIdentity(ctx, regions[0])
	identity := c.Identity()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, c.loadOptions(regions[0])...)
	if err != nil {
		return identity, err
	}

	// Retrieving the credentials is what names the provider, and may be a network
	// call. That is acceptable here: -check exists to verify exactly this.
	if creds, credErr := cfg.Credentials.Retrieve(ctx); credErr == nil {
		identity.Source = creds.Source
	} else {
		identity.Source = "unresolved: " + credErr.Error()
	}

	c.mu.Lock()
	c.identity.Source = identity.Source
	c.mu.Unlock()

	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return identity, fmt.Errorf("verifying credentials with STS: %w", err)
	}

	identity.Account = aws.ToString(out.Account)
	identity.ARN = aws.ToString(out.Arn)

	c.mu.Lock()
	c.identity = identity
	c.mu.Unlock()

	return identity, nil
}

// Collect enumerates the configured regions.
//
// A region that fails is recorded on the result's Errors field rather than
// failing the whole sweep, so a partial view is still reported as partial.
func (c *AWSCollector) Collect(ctx context.Context) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, awsCollectTimeout)
	defer cancel()

	regions, err := c.resolveRegions(ctx)
	if err != nil {
		return nil, err
	}

	// Recorded once so the UI can show which local credentials are in use. This
	// reads configuration only and makes no network call.
	if c.Identity().Region == "" {
		c.recordIdentity(ctx, regions[0])
	}

	result := models.CloudProviderMetrics{
		ProviderType: "aws",
		Name:         "AWS",
		Regions:      regions,
		LastUpdated:  time.Now(),
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, region := range regions {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()

			instances, buckets, databases, errs := c.collectRegion(ctx, region, regions[0])

			mu.Lock()
			defer mu.Unlock()
			result.InstanceMetrics = append(result.InstanceMetrics, instances...)
			result.StorageMetrics = append(result.StorageMetrics, buckets...)
			result.DatabaseMetrics = append(result.DatabaseMetrics, databases...)
			result.Errors = append(result.Errors, errs...)
		}(region)
	}
	wg.Wait()

	// Stable ordering so the table does not reshuffle between frames.
	sort.Slice(result.InstanceMetrics, func(i, j int) bool {
		return result.InstanceMetrics[i].ID < result.InstanceMetrics[j].ID
	})
	sort.Slice(result.StorageMetrics, func(i, j int) bool {
		return result.StorageMetrics[i].Name < result.StorageMetrics[j].Name
	})
	sort.Slice(result.DatabaseMetrics, func(i, j int) bool {
		return result.DatabaseMetrics[i].ID < result.DatabaseMetrics[j].ID
	})
	sort.Strings(result.Errors)

	c.mu.Lock()
	c.metrics = result
	c.mu.Unlock()

	c.UpdateData(result)
	c.store(result)

	// Every region failing is a collection failure; some failing is partial data.
	if len(result.Errors) > 0 && len(result.InstanceMetrics) == 0 &&
		len(result.StorageMetrics) == 0 && len(result.DatabaseMetrics) == 0 {
		return result, fmt.Errorf("aws: %s", strings.Join(result.Errors, "; "))
	}

	return result, nil
}

// collectRegion gathers one region's inventory.
func (c *AWSCollector) collectRegion(ctx context.Context, region, primaryRegion string) (
	[]models.CloudInstanceMetrics, []models.CloudStorageMetrics, []models.CloudDatabaseMetrics, []string,
) {
	clients, err := c.clientsFor(ctx, region)
	if err != nil {
		return nil, nil, nil, []string{err.Error()}
	}

	var (
		instances []models.CloudInstanceMetrics
		buckets   []models.CloudStorageMetrics
		databases []models.CloudDatabaseMetrics
		problems  []string
	)

	if c.config.wants("ec2") {
		found, err := c.describeInstances(ctx, clients, region)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s ec2: %v", region, err))
		} else {
			instances = found
		}
	}

	if c.config.wants("s3") {
		found, err := c.listBuckets(ctx, clients, region, primaryRegion)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s s3: %v", region, err))
		} else {
			buckets = found
		}
	}

	if c.config.wants("rds") {
		found, err := c.describeDatabases(ctx, clients, region)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s rds: %v", region, err))
		} else {
			databases = found
		}
	}

	return instances, buckets, databases, problems
}

// describeInstances lists EC2 instances and fills in CloudWatch utilisation.
func (c *AWSCollector) describeInstances(ctx context.Context, clients *awsClients, region string) ([]models.CloudInstanceMetrics, error) {
	out, err := clients.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		MaxResults: aws.Int32(awsMaxResults),
	})
	if err != nil {
		return nil, err
	}

	var instances []models.CloudInstanceMetrics
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			entry := models.CloudInstanceMetrics{
				ID:     aws.ToString(instance.InstanceId),
				Type:   string(instance.InstanceType),
				Region: region,
				Status: ec2StateToStatus(instance.State),
				Tags:   ec2Tags(instance.Tags),
			}

			entry.Name = entry.Tags["Name"]
			if entry.Name == "" {
				entry.Name = entry.ID
			}
			if instance.LaunchTime != nil {
				entry.UptimeHours = time.Since(*instance.LaunchTime).Hours()
			}

			instances = append(instances, entry)
		}
	}

	// Utilisation is best-effort: a missing CloudWatch permission must not hide
	// the inventory itself.
	if len(instances) > 0 {
		if err := c.applyInstanceUtilisation(ctx, clients, instances); err != nil {
			c.Logger().Warn("cloudwatch utilisation unavailable", "region", region, "error", err)
		}
	}

	return instances, nil
}

// applyInstanceUtilisation fills CPU, network and disk figures from CloudWatch.
//
// Queries are batched through GetMetricData rather than issuing one
// GetMetricStatistics call per instance per metric, which is both slower and
// billed per request.
func (c *AWSCollector) applyInstanceUtilisation(ctx context.Context, clients *awsClients, instances []models.CloudInstanceMetrics) error {
	type target struct {
		index  int
		metric string
	}

	wanted := []string{"CPUUtilization", "NetworkIn", "NetworkOut"}
	byID := make(map[string]target, len(instances)*len(wanted))
	queries := make([]cwtypes.MetricDataQuery, 0, len(instances)*len(wanted))

	for i := range instances {
		for _, metric := range wanted {
			id := fmt.Sprintf("q%d", len(queries))
			byID[id] = target{index: i, metric: metric}

			queries = append(queries, cwtypes.MetricDataQuery{
				Id: aws.String(id),
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String("AWS/EC2"),
						MetricName: aws.String(metric),
						Dimensions: []cwtypes.Dimension{{
							Name:  aws.String("InstanceId"),
							Value: aws.String(instances[i].ID),
						}},
					},
					Period: aws.Int32(300),
					Stat:   aws.String("Average"),
				},
			})
		}
	}

	end := time.Now()
	start := end.Add(-cloudWatchLookback)

	for offset := 0; offset < len(queries); offset += cloudWatchBatchSize {
		batch := queries[offset:min(offset+cloudWatchBatchSize, len(queries))]

		out, err := clients.cloudwatch.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
			StartTime:         aws.Time(start),
			EndTime:           aws.Time(end),
			MetricDataQueries: batch,
		})
		if err != nil {
			return err
		}

		for _, series := range out.MetricDataResults {
			if len(series.Values) == 0 {
				continue
			}

			t, ok := byID[aws.ToString(series.Id)]
			if !ok {
				continue
			}

			// Values are newest-first; the most recent datapoint is the useful one.
			value := series.Values[0]
			switch t.metric {
			case "CPUUtilization":
				instances[t.index].CPUUtilization = value
			case "NetworkIn":
				instances[t.index].NetworkIn = uint64(max(value, 0))
			case "NetworkOut":
				instances[t.index].NetworkOut = uint64(max(value, 0))
			}
		}
	}

	return nil
}

// listBuckets lists S3 buckets. ListBuckets is a global operation, so it is only
// issued for the primary region to avoid repeating the same inventory per region.
func (c *AWSCollector) listBuckets(ctx context.Context, clients *awsClients, region string, primaryRegion string) ([]models.CloudStorageMetrics, error) {
	if region != primaryRegion {
		return nil, nil
	}

	out, err := clients.s3.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	buckets := make([]models.CloudStorageMetrics, 0, len(out.Buckets))
	for i, bucket := range out.Buckets {
		if i >= awsMaxResults {
			break
		}
		buckets = append(buckets, models.CloudStorageMetrics{
			ID:     aws.ToString(bucket.Name),
			Name:   aws.ToString(bucket.Name),
			Type:   "S3 bucket",
			Region: region,
		})
	}

	return buckets, nil
}

// describeDatabases lists RDS instances.
func (c *AWSCollector) describeDatabases(ctx context.Context, clients *awsClients, region string) ([]models.CloudDatabaseMetrics, error) {
	out, err := clients.rds.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		MaxRecords: aws.Int32(100),
	})
	if err != nil {
		return nil, err
	}

	databases := make([]models.CloudDatabaseMetrics, 0, len(out.DBInstances))
	for _, instance := range out.DBInstances {
		databases = append(databases, models.CloudDatabaseMetrics{
			ID:     aws.ToString(instance.DBInstanceIdentifier),
			Name:   aws.ToString(instance.DBInstanceIdentifier),
			Type:   aws.ToString(instance.DBInstanceClass),
			Engine: fmt.Sprintf("%s %s", aws.ToString(instance.Engine), aws.ToString(instance.EngineVersion)),
			Region: region,
			Status: rdsStatusToStatus(aws.ToString(instance.DBInstanceStatus)),
		})
	}

	return databases, nil
}

// ec2StateToStatus maps an EC2 instance state to the shared status vocabulary.
func ec2StateToStatus(state *ec2types.InstanceState) models.ResourceStatus {
	if state == nil {
		return models.ResourceStatusPending
	}

	switch state.Name {
	case ec2types.InstanceStateNameRunning:
		return models.ResourceStatusRunning
	case ec2types.InstanceStateNameStopped, ec2types.InstanceStateNameTerminated:
		return models.ResourceStatusStopped
	case ec2types.InstanceStateNamePending, ec2types.InstanceStateNameStopping,
		ec2types.InstanceStateNameShuttingDown:
		return models.ResourceStatusPending
	default:
		return models.ResourceStatusError
	}
}

// rdsStatusToStatus maps an RDS status string to the shared status vocabulary.
func rdsStatusToStatus(status string) models.ResourceStatus {
	switch strings.ToLower(status) {
	case "available":
		return models.ResourceStatusRunning
	case "stopped":
		return models.ResourceStatusStopped
	case "creating", "modifying", "backing-up", "starting", "stopping", "rebooting":
		return models.ResourceStatusPending
	default:
		return models.ResourceStatusError
	}
}

// ec2Tags converts EC2 tags to a map.
func ec2Tags(tags []ec2types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}

	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		out[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return out
}

// store persists the summary sample.
func (c *AWSCollector) store(metrics models.CloudProviderMetrics) {
	store := c.Storage()
	if store == nil {
		return
	}
	if err := store.StoreCloudSummary(models.CloudSummaryFrom(metrics)); err != nil {
		c.Logger().Error("failed to store cloud summary", "error", err)
	}
}

// GetLatestMetrics returns the most recent sample.
func (c *AWSCollector) GetLatestMetrics() models.CloudProviderMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Start begins periodic collection.
func (c *AWSCollector) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("aws collection failed", "error", err)
		}
	})
}
