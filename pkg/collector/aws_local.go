package collector

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalAWSProfiles returns the profile names configured on this machine.
//
// Only section headers are read, never credential values. It exists so that a
// failure to resolve credentials can tell the operator which profiles they
// actually have, instead of leaving them to go and look.
//
// The files consulted are the SDK's: $AWS_CONFIG_FILE or ~/.aws/config, and
// $AWS_SHARED_CREDENTIALS_FILE or ~/.aws/credentials.
func LocalAWSProfiles() []string {
	seen := make(map[string]struct{})

	for _, source := range awsSharedFiles() {
		for _, name := range parseINISections(source.path) {
			// Profiles are written "[profile name]" in the config file and
			// "[name]" in the credentials file.
			if source.stripProfilePrefix {
				name = strings.TrimSpace(strings.TrimPrefix(name, "profile"))
			}
			if name == "" || strings.HasPrefix(name, "sso-session") {
				continue
			}
			seen[name] = struct{}{}
		}
	}

	profiles := make([]string, 0, len(seen))
	for name := range seen {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)

	return profiles
}

// awsSharedFile is one file to scan for profile names.
type awsSharedFile struct {
	path               string
	stripProfilePrefix bool
}

// awsSharedFiles returns the shared config and credentials paths, honouring the
// SDK's environment overrides.
func awsSharedFiles() []awsSharedFile {
	configPath := os.Getenv("AWS_CONFIG_FILE")
	credentialsPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")

	if configPath == "" || credentialsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		if configPath == "" {
			configPath = filepath.Join(home, ".aws", "config")
		}
		if credentialsPath == "" {
			credentialsPath = filepath.Join(home, ".aws", "credentials")
		}
	}

	return []awsSharedFile{
		{path: configPath, stripProfilePrefix: true},
		{path: credentialsPath},
	}
}

// parseINISections returns the bracketed section names in an INI file.
func parseINISections(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var sections []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		sections = append(sections, strings.TrimSpace(line[1:len(line)-1]))
	}

	return sections
}

// LooksLikeExpiredSSO reports whether an error is the SSO session having lapsed,
// which needs `aws sso login` rather than any change to configuration.
func LooksLikeExpiredSSO(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"expired",
		"sso session",
		"ssooidc",
		"the sso session associated",
		"refresh cached sso token",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
