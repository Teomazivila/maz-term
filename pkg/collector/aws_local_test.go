package collector

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalAWSProfilesReadsBothSharedFiles(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(configPath, []byte(`
[default]
region = eu-west-1

[profile staging]
region = us-east-1
sso_session = corp

[sso-session corp]
sso_region = us-east-1

[profile production]
region = us-east-1
`), 0o600))

	credentialsPath := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credentialsPath, []byte(`
[legacy]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = secret
`), 0o600))

	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)

	profiles := LocalAWSProfiles()

	// The "profile " prefix is stripped, credentials-file sections are included,
	// and sso-session blocks are not profiles.
	assert.Equal(t, []string{"default", "legacy", "production", "staging"}, profiles)
	assert.NotContains(t, profiles, "sso-session corp")
}

// TestLocalAWSProfilesReadsNoSecrets is the important property: only section
// headers are parsed, so no credential value can leak into a report.
func TestLocalAWSProfilesReadsNoSecrets(t *testing.T) {
	dir := t.TempDir()
	credentialsPath := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credentialsPath, []byte(
		"[default]\naws_secret_access_key = TOPSECRETVALUE\n"), 0o600))

	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "absent"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)

	for _, profile := range LocalAWSProfiles() {
		assert.NotContains(t, profile, "TOPSECRETVALUE")
		assert.NotContains(t, profile, "aws_secret_access_key")
	}
}

func TestLocalAWSProfilesWithNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "absent-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "absent-credentials"))

	assert.Empty(t, LocalAWSProfiles())
}

func TestLooksLikeExpiredSSO(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "real SSO refresh failure",
			err: errors.New("failed to refresh cached credentials, refresh cached SSO token failed, " +
				"unable to refresh SSO token, operation error SSO OIDC: CreateToken, " +
				"InvalidGrantException"),
			want: true,
		},
		{name: "expired token", err: errors.New("the security token included in the request is expired"), want: true},
		{name: "unrelated", err: errors.New("AccessDenied: not authorised to perform ec2:DescribeInstances"), want: false},
		{name: "no region", err: errors.New("aws: no region found"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LooksLikeExpiredSSO(tt.err))
		})
	}
}

// TestAWSLoadOptionsOmitsRegionWhenUnset pins the local-usage behaviour: with no
// region configured the SDK is left to resolve one from the environment or the
// shared profile, rather than maz-term demanding a duplicate setting.
func TestAWSLoadOptionsOmitsRegionWhenUnset(t *testing.T) {
	c := NewAWSCollector(AWSConfig{})
	assert.Empty(t, c.loadOptions(""), "no region and no profile means no overrides")

	withRegion := c.loadOptions("eu-west-1")
	assert.Len(t, withRegion, 1)

	withProfile := NewAWSCollector(AWSConfig{Profile: "staging"})
	assert.Len(t, withProfile.loadOptions(""), 1, "the profile is applied without pinning a region")
	assert.Len(t, withProfile.loadOptions("eu-west-1"), 2)
}

// TestAWSResolveRegionsPrefersConfiguration checks the configured value wins and
// no environment lookup happens.
func TestAWSResolveRegionsPrefersConfiguration(t *testing.T) {
	c := NewAWSCollector(AWSConfig{Region: "eu-west-1", AdditionalRegions: []string{"us-east-1"}})

	regions, err := c.resolveRegions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"eu-west-1", "us-east-1"}, regions)
}

// TestAWSResolveRegionsFromEnvironment pins the zero-configuration path.
func TestAWSResolveRegionsFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "absent"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "absent"))
	t.Setenv("AWS_REGION", "ap-southeast-2")

	c := NewAWSCollector(AWSConfig{})

	regions, err := c.resolveRegions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"ap-southeast-2"}, regions)
}

// TestAWSResolveRegionsReportsActionableFailure checks the message names every
// place a region can come from.
func TestAWSResolveRegionsReportsActionableFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "absent"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "absent"))
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	c := NewAWSCollector(AWSConfig{})

	_, err := c.resolveRegions(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloud.aws.region")
	assert.Contains(t, err.Error(), "AWS_REGION")
	assert.Contains(t, err.Error(), ".aws/config")
}
