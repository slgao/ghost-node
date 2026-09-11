package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vpnplatform/core/internal/provisioner/dns"
)

// DNSAction repoints a node's hostname at the new address.
type DNSAction struct {
	Updater dns.Updater
}

func (a DNSAction) Name() string {
	if a.Updater == nil {
		return "dns"
	}
	return "dns/" + a.Updater.Name()
}

func (a DNSAction) Apply(ctx context.Context, target Target, _, newIP string) error {
	if a.Updater == nil || target.DNSRecord == "" || newIP == "" {
		return nil
	}
	if err := a.Updater.Upsert(ctx, target.DNSRecord, newIP); err != nil {
		return fmt.Errorf("pointing %s at %s: %w", target.DNSRecord, newIP, err)
	}
	return nil
}

// ControlPlaneAction writes the new address back to the control plane so the
// portal, subscription endpoints and generated VLESS URIs stay correct.
type ControlPlaneAction struct {
	BaseURL    string
	AdminToken string
	HTTPClient *http.Client
}

func (a ControlPlaneAction) Name() string { return "control-plane" }

func (a ControlPlaneAction) Apply(ctx context.Context, target Target, _, newIP string) error {
	if a.BaseURL == "" || target.NodeID == "" || newIP == "" {
		return nil
	}
	if a.AdminToken == "" {
		return fmt.Errorf("admin token is required to update node %s", target.NodeID)
	}

	body, err := json.Marshal(map[string]string{"address": newIP})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/admin/nodes/%s/address", strings.TrimSuffix(a.BaseURL, "/"), target.NodeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	req.Header.Set("Content-Type", "application/json")

	hc := a.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("calling control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("control plane returned %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

// SSHConfigAction keeps ~/.ssh/config pointed at the node, so `ssh ghost-node`
// keeps working after a rotation instead of needing a manual edit.
type SSHConfigAction struct {
	// Path defaults to ~/.ssh/config.
	Path string
}

func (a SSHConfigAction) Name() string { return "ssh-config" }

func (a SSHConfigAction) Apply(_ context.Context, target Target, _, newIP string) error {
	if target.SSHHost == "" || newIP == "" {
		return nil
	}

	path := a.Path
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locating home directory: %w", err)
		}
		path = filepath.Join(home, ".ssh", "config")
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	updated, err := setSSHHostName(string(original), target.SSHHost, newIP)
	if err != nil {
		return err
	}
	if updated == string(original) {
		return nil
	}

	// Write via a temp file in the same directory so a crash cannot leave a
	// half-written ssh config behind.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ghostctl-ssh-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(updated); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// setSSHHostName rewrites the HostName of a single Host block, preserving the
// rest of the file — including comments and indentation — byte for byte.
func setSSHHostName(config, alias, ip string) (string, error) {
	lines := strings.Split(config, "\n")

	inBlock := false
	found := false
	hostLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)

		if len(fields) > 0 && strings.EqualFold(fields[0], "Host") {
			inBlock = false
			for _, name := range fields[1:] {
				if name == alias {
					inBlock = true
					hostLine = i
					break
				}
			}
			continue
		}
		if !inBlock || len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "HostName") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%s%s %s", indent, fields[0], ip)
			found = true
			inBlock = false
		}
	}

	if found {
		return strings.Join(lines, "\n"), nil
	}
	if hostLine < 0 {
		return "", fmt.Errorf("no `Host %s` block found in ssh config", alias)
	}

	// The block exists but has no HostName — add one directly beneath it.
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:hostLine+1]...)
	out = append(out, "    HostName "+ip)
	out = append(out, lines[hostLine+1:]...)
	return strings.Join(out, "\n"), nil
}
