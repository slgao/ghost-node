package ociclient

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFromFile reads credentials from an OCI CLI config file (~/.oci/config).
// Reusing that file means ghostctl works with whatever `oci setup config`
// already produced, with no credentials duplicated into our own config.
func LoadFromFile(path, profile string) (Config, error) {
	if profile == "" {
		profile = "DEFAULT"
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("oci: locating home directory: %w", err)
		}
		path = filepath.Join(home, ".oci", "config")
	}
	path = expandHome(path)

	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("oci: opening %s: %w", path, err)
	}
	defer f.Close()

	values := map[string]string{}
	current := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		if current != profile {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("oci: reading %s: %w", path, err)
	}
	if len(values) == 0 {
		return Config{}, fmt.Errorf("oci: profile [%s] not found in %s", profile, path)
	}

	return Config{
		TenancyOCID:    values["tenancy"],
		UserOCID:       values["user"],
		Fingerprint:    values["fingerprint"],
		Region:         values["region"],
		PrivateKeyPath: expandHome(values["key_file"]),
	}, nil
}

func expandHome(path string) string {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
