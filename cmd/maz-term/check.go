package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
)

// checkTimeout bounds the whole report so a wedged endpoint cannot hang it.
const checkTimeout = 45 * time.Second

// runCheck prints what maz-term resolved from the local environment for every
// configured source, and whether it actually works.
//
// It exists because the common local failure is not a bug but a credential that
// resolved to something other than the operator expected: the wrong AWS profile,
// a stale kubeconfig context, an unset token. Starting a full-screen dashboard is
// a poor way to discover that, so this reports it as plain text on stdout.
//
// Every check is read-only.
func runCheck(w io.Writer, cfg *config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	fmt.Fprintln(w, "maz-term configuration check")
	fmt.Fprintln(w)

	failures := 0

	// Local sources, which need no credentials at all.
	fmt.Fprintln(w, "Local")
	reportSystem(ctx, w)
	if !reportGit(ctx, w, cfg) {
		failures++
	}
	reportEndpoints(w, cfg)

	// Remote providers, each off unless enabled.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Providers")
	if !reportAWS(ctx, w, cfg) {
		failures++
	}
	if !reportKubernetes(ctx, w, cfg) {
		failures++
	}
	if !reportGitHub(ctx, w, cfg) {
		failures++
	}

	fmt.Fprintln(w)
	switch failures {
	case 0:
		fmt.Fprintln(w, "All configured sources are reachable.")
		return 0
	case 1:
		fmt.Fprintln(w, "1 configured source is not usable; see above.")
		return 1
	default:
		fmt.Fprintf(w, "%d configured sources are not usable; see above.\n", failures)
		return 1
	}
}

// ok and bad prefix a report line consistently.
func ok(w io.Writer, label, detail string) {
	fmt.Fprintf(w, "  [ok]      %-12s %s\n", label, detail)
}

func bad(w io.Writer, label, detail string) {
	fmt.Fprintf(w, "  [FAILED]  %-12s %s\n", label, detail)
}

func skip(w io.Writer, label, detail string) {
	fmt.Fprintf(w, "  [off]     %-12s %s\n", label, detail)
}

// hint prints an indented remediation line.
func hint(w io.Writer, text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(w, "                          %s\n", line)
	}
}

func reportSystem(ctx context.Context, w io.Writer) {
	c := collector.NewSystemMetricsCollector()

	if _, err := c.Collect(ctx); err != nil {
		bad(w, "system", err.Error())
		return
	}

	metrics := c.GetLatestMetrics()
	ok(w, "system", fmt.Sprintf("cpu %.1f%%, memory %.1f%%, %d filesystems",
		metrics.CPU.UsagePercent, metrics.Memory.UsagePercent, len(metrics.Disk.Filesystems)))
}

func reportGit(ctx context.Context, w io.Writer, cfg *config.Config) bool {
	if len(cfg.Git.Repositories) == 0 {
		skip(w, "git", "no repositories configured")
		return true
	}

	healthy := true
	for _, repo := range cfg.Git.Repositories {
		c := collector.NewGitStatusCollectorWithRemote(repo.Path, repo.Remote)

		if _, err := c.Collect(ctx); err != nil {
			bad(w, "git", fmt.Sprintf("%s: %v", repo.Path, err))
			healthy = false
			continue
		}

		metrics := c.GetLatestMetrics()
		detail := fmt.Sprintf("%s on %s, %d commits", metrics.Path, metrics.Branch, metrics.CommitCount)
		if metrics.RemoteURL != "" {
			detail += fmt.Sprintf(", %s -> %s", metrics.Remote, metrics.RemoteURL)
		}
		ok(w, "git", detail)

		// Uses the local git installation, so the operator's own config, SSH
		// agent and credential helpers apply.
		if !metrics.HasUpstream {
			hint(w, "no upstream for this branch, so unpushed commits are not counted")
		}
	}

	return healthy
}

func reportEndpoints(w io.Writer, cfg *config.Config) {
	if len(cfg.Endpoints) == 0 {
		skip(w, "endpoints", "none configured")
		return
	}
	ok(w, "endpoints", fmt.Sprintf("%d configured", len(cfg.Endpoints)))
}

func reportAWS(ctx context.Context, w io.Writer, cfg *config.Config) bool {
	if !enabled(cfg.Cloud.Enabled, "aws") {
		skip(w, "aws", "not enabled (cloud.enabled: [\"aws\"])")
		return true
	}

	c := collector.NewAWSCollector(collector.AWSConfig{
		Region:            cfg.Cloud.AWS.Region,
		Profile:           cfg.Cloud.AWS.Profile,
		AdditionalRegions: cfg.Cloud.AWS.AdditionalRegions,
		Resources:         cfg.Cloud.AWS.Resources,
	})

	identity, err := c.CheckAccess(ctx)
	if err != nil {
		bad(w, "aws", err.Error())
		if identity.Source != "" {
			hint(w, "credentials resolved from: "+identity.Source)
		}

		// An expired SSO session needs a login, not a configuration change, so it
		// is called out separately from everything else.
		if collector.LooksLikeExpiredSSO(err) {
			target := cfg.Cloud.AWS.Profile
			if target == "" {
				target = os.Getenv("AWS_PROFILE")
			}
			if target != "" {
				hint(w, "the SSO session has expired. Run: aws sso login --profile "+target)
			} else {
				hint(w, "the SSO session has expired. Run: aws sso login")
			}
			return false
		}

		// Naming the profiles that exist locally saves the operator going to look.
		if profiles := collector.LocalAWSProfiles(); len(profiles) > 0 {
			hint(w, "profiles found locally: "+strings.Join(profiles, ", "))
			hint(w, "select one with cloud.aws.profile, or AWS_PROFILE")
		}

		hint(w, "maz-term uses the standard AWS credential chain:\n"+
			"environment, ~/.aws/credentials, ~/.aws/config, SSO, then instance metadata.\n"+
			"Try: aws sts get-caller-identity")
		return false
	}

	ok(w, "aws", fmt.Sprintf("account %s in %s", identity.Account, identity.Region))
	hint(w, fmt.Sprintf("profile %s, credentials from %s", identity.Profile, identity.Source))
	hint(w, identity.ARN)
	return true
}

func reportKubernetes(ctx context.Context, w io.Writer, cfg *config.Config) bool {
	if !cfg.Kubernetes.Enabled {
		skip(w, "kubernetes", "not enabled (kubernetes.enabled: true)")
		return true
	}

	c := collector.NewKubernetesCollector(collector.KubernetesConfig{
		ConfigPath: cfg.Kubernetes.ConfigPath,
		Context:    cfg.Kubernetes.Context,
		Namespaces: cfg.Kubernetes.Namespaces,
		Resources:  cfg.Kubernetes.Resources,
	})

	identity, version, err := c.CheckAccess(ctx)
	if err != nil {
		bad(w, "kubernetes", err.Error())
		if identity.Source != "" {
			hint(w, "kubeconfig searched: "+identity.Source)
		}
		hint(w, "maz-term uses the same resolution as kubectl:\n"+
			"$KUBECONFIG, then ~/.kube/config, then the in-cluster service account.\n"+
			"Try: kubectl cluster-info")
		return false
	}

	ok(w, "kubernetes", fmt.Sprintf("%s (server %s)", identity.Server, version))
	hint(w, fmt.Sprintf("context %s, cluster %s, user %s", identity.Context, identity.Cluster, identity.User))
	hint(w, "from "+identity.Source)
	return true
}

func reportGitHub(ctx context.Context, w io.Writer, cfg *config.Config) bool {
	if !enabled(cfg.CICD.Enabled, "github") {
		skip(w, "github", "not enabled (cicd.enabled: [\"github\"])")
		return true
	}

	token := config.GitHubToken()
	c := collector.NewGitHubActionsCollector(collector.GitHubConfig{
		Owner:        cfg.CICD.GitHub.Owner,
		Repositories: cfg.CICD.GitHub.Repositories,
		Workflows:    cfg.CICD.GitHub.Workflows,
		Token:        token,
	})

	if _, err := c.Collect(ctx); err != nil {
		bad(w, "github", err.Error())
		if token == "" {
			hint(w, "set MAZTERM_GITHUB_TOKEN or GITHUB_TOKEN.\n"+
				"A fine-grained token needs read access to Actions.\n"+
				"If you use the gh CLI: export GITHUB_TOKEN=$(gh auth token)")
		}
		return false
	}

	metrics := c.GetLatestMetrics()
	ok(w, "github", fmt.Sprintf("%d workflows across %d repositories",
		metrics.Summary.TotalWorkflows, len(cfg.CICD.GitHub.Repositories)))

	if remaining, limit, reset := c.RateLimit(); limit > 0 {
		hint(w, fmt.Sprintf("api quota %d of %d, resets %s",
			remaining, limit, reset.Format(time.Kitchen)))
	}
	return true
}

// enabled reports whether name appears in the provider list.
func enabled(list []string, name string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return true
		}
	}
	return false
}

// checkOutput is stdout, replaceable in tests.
var checkOutput io.Writer = os.Stdout
