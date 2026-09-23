package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a git repository with one commit and returns its path.
func initRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	run("init", "--initial-branch=main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644))
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	return dir
}

func TestGitCollectorOnRealRepository(t *testing.T) {
	dir := initRepo(t)
	c := NewGitStatusCollector(dir)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.True(t, metrics.IsRepository)
	assert.Empty(t, metrics.Error)
	assert.Equal(t, "main", metrics.Branch)
	assert.Equal(t, 1, metrics.CommitCount)
	assert.Contains(t, metrics.Branches, "main", "branches must come from the repository")
	assert.False(t, metrics.LastCommit.IsZero())

	require.Len(t, metrics.CommitHistory, 1)
	assert.Equal(t, "initial commit", metrics.CommitHistory[0].Message)
	assert.Equal(t, "Test", metrics.CommitHistory[0].Author)
	assert.NotEmpty(t, metrics.CommitHistory[0].Hash)
}

// TestGitCollectorSeparatesModifiedFromUntracked pins the distinction: counting
// untracked files as modified overstated how dirty the tree was.
func TestGitCollectorSeparatesModifiedFromUntracked(t *testing.T) {
	dir := initRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "brand-new.txt"), []byte("new\n"), 0o644))

	c := NewGitStatusCollector(dir)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.Equal(t, 1, metrics.ModifiedFiles, "only the tracked change counts as modified")
	assert.Equal(t, 1, metrics.UntrackedFiles)

	// Changed files are listed individually rather than summarised as a count.
	paths := make(map[string]string, len(metrics.ChangedFiles))
	for _, change := range metrics.ChangedFiles {
		paths[change.Path] = change.Status
	}
	assert.Contains(t, paths, "README.md")
	assert.Contains(t, paths, "brand-new.txt")
	assert.Equal(t, "??", paths["brand-new.txt"])
}

// TestGitCollectorHandlesFilenamesWithSpaces covers the -z porcelain parsing;
// git quotes such paths in its default output.
func TestGitCollectorHandlesFilenamesWithSpaces(t *testing.T) {
	dir := initRepo(t)

	name := "a file with spaces.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644))

	c := NewGitStatusCollector(dir)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	var found bool
	for _, change := range c.GetLatestMetrics().ChangedFiles {
		if change.Path == name {
			found = true
		}
	}
	assert.True(t, found, "a filename containing spaces must be parsed intact")
}

// TestGitCollectorOnNonRepository pins the honest failure: zeroes were previously
// indistinguishable from a clean repository.
func TestGitCollectorOnNonRepository(t *testing.T) {
	c := NewGitStatusCollector(t.TempDir())

	_, err := c.Collect(context.Background())
	require.Error(t, err, "a path that is not a repository must surface an error")

	metrics := c.GetLatestMetrics()
	assert.False(t, metrics.IsRepository)
	assert.NotEmpty(t, metrics.Error, "the reason must be reported to the UI")
}

func TestGitCollectorOnMissingPath(t *testing.T) {
	c := NewGitStatusCollector(filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, c.GetLatestMetrics().Error, "unavailable")
}

// TestGitCollectorDoesNotPersistNonRepository ensures a misconfigured path does
// not fill the history with zeroes.
func TestGitCollectorDoesNotPersistNonRepository(t *testing.T) {
	store := &recordingStore{}
	c := NewGitStatusCollector(t.TempDir())
	c.SetStorageProvider(store)

	_, _ = c.Collect(context.Background())

	_, _, git := store.counts()
	assert.Zero(t, git, "a path that is not a repository must not be recorded")
}

func TestGitCollectorPersistsRealRepository(t *testing.T) {
	store := &recordingStore{}
	c := NewGitStatusCollector(initRepo(t))
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	_, _, git := store.counts()
	assert.Equal(t, 1, git)
}

func TestGitCollectorPublishesToSubscribers(t *testing.T) {
	c := NewGitStatusCollector(initRepo(t))

	updates, id := c.Subscribe()
	defer c.Unsubscribe(id)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	select {
	case metrics := <-updates:
		assert.True(t, metrics.IsRepository)
	case <-time.After(time.Second):
		t.Fatal("subscriber received nothing")
	}
}

// TestGitCollectorExpandsTilde pins H-015 at the collector boundary.
func TestGitCollectorExpandsTilde(t *testing.T) {
	c := NewGitStatusCollector("~/definitely-not-a-real-directory")

	assert.NotContains(t, c.repoPath, "~")
	assert.True(t, filepath.IsAbs(c.repoPath))
}

func TestGitCollectorEmptyPathUsesWorkingDirectory(t *testing.T) {
	c := NewGitStatusCollector("")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cwd, c.repoPath)
}

func TestParsePorcelain(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		wantChanges   int
		wantModified  int
		wantUntracked int
	}{
		{name: "empty", in: ""},
		{
			name:         "single modification",
			in:           " M README.md\x00",
			wantChanges:  1,
			wantModified: 1,
		},
		{
			name:          "untracked only",
			in:            "?? new.txt\x00",
			wantChanges:   1,
			wantUntracked: 1,
		},
		{
			name:          "mixed",
			in:            " M a.txt\x00?? b.txt\x00A  c.txt\x00",
			wantChanges:   3,
			wantModified:  2,
			wantUntracked: 1,
		},
		{
			// A rename is followed by a second record holding the original path,
			// which must be consumed rather than parsed as its own entry.
			name:         "rename consumes the original path",
			in:           "R  new.txt\x00old.txt\x00",
			wantChanges:  1,
			wantModified: 1,
		},
		{
			name:         "path containing spaces",
			in:           " M a file with spaces.txt\x00",
			wantChanges:  1,
			wantModified: 1,
		},
		{
			name: "truncated entry is ignored",
			in:   "M\x00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes, modified, untracked := parsePorcelain(tt.in)

			assert.Len(t, changes, tt.wantChanges)
			assert.Equal(t, tt.wantModified, modified)
			assert.Equal(t, tt.wantUntracked, untracked)
		})
	}
}

func TestParsePorcelainRenameKeepsNewPath(t *testing.T) {
	changes, _, _ := parsePorcelain("R  new.txt\x00old.txt\x00")

	require.Len(t, changes, 1)
	assert.Equal(t, "new.txt", changes[0].Path)
	assert.Equal(t, "R ", changes[0].Status)
}

func TestParseCommitHistory(t *testing.T) {
	out := "abc123\x1fAlice\x1f1700000000\x1ffirst\n" +
		"def456\x1fBob\x1f1700000100\x1fsecond\n"

	commits := parseCommitHistory(out)

	require.Len(t, commits, 2)
	assert.Equal(t, "abc123", commits[0].Hash)
	assert.Equal(t, "Alice", commits[0].Author)
	assert.Equal(t, "first", commits[0].Message)
	assert.Equal(t, time.Unix(1700000000, 0), commits[0].Timestamp)
	assert.Equal(t, "Bob", commits[1].Author)
}

func TestParseCommitHistoryIgnoresMalformedLines(t *testing.T) {
	commits := parseCommitHistory("abc\x1fAlice\nonly-one-field\n\n")
	assert.Empty(t, commits)
}

// TestParseCommitHistoryKeepsMessagesContainingSeparators guards the split limit.
func TestParseCommitHistoryKeepsMessagesContainingSeparators(t *testing.T) {
	commits := parseCommitHistory("abc\x1fAlice\x1f1700000000\x1ffix: a, b and c\n")

	require.Len(t, commits, 1)
	assert.Equal(t, "fix: a, b and c", commits[0].Message)
}
