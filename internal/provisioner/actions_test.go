package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetSSHHostNameReplacesOnlyTheTargetBlock(t *testing.T) {
	config := `# personal hosts
Host other
    HostName 9.9.9.9
    User root

Host ghost-node-jp1
    HostName 1.2.3.4
    User ubuntu
    IdentityFile ~/.ssh/id_rsa

Host last
    HostName 8.8.8.8
`
	got, err := setSSHHostName(config, "ghost-node-jp1", "5.6.7.8")
	if err != nil {
		t.Fatalf("setSSHHostName: %v", err)
	}
	if !strings.Contains(got, "    HostName 5.6.7.8") {
		t.Errorf("target HostName was not updated:\n%s", got)
	}
	if !strings.Contains(got, "HostName 9.9.9.9") || !strings.Contains(got, "HostName 8.8.8.8") {
		t.Errorf("a neighbouring block was modified:\n%s", got)
	}
	if !strings.Contains(got, "# personal hosts") || !strings.Contains(got, "IdentityFile ~/.ssh/id_rsa") {
		t.Errorf("unrelated lines were lost:\n%s", got)
	}
}

func TestSetSSHHostNameHandlesMultipleAliasesAndTabs(t *testing.T) {
	config := "Host jp1 ghost-node-jp1\n\tHostName 1.2.3.4\n"
	got, err := setSSHHostName(config, "ghost-node-jp1", "5.6.7.8")
	if err != nil {
		t.Fatalf("setSSHHostName: %v", err)
	}
	if !strings.Contains(got, "\tHostName 5.6.7.8") {
		t.Errorf("tab indentation was not preserved:\n%q", got)
	}
}

func TestSetSSHHostNameInsertsMissingHostName(t *testing.T) {
	config := "Host ghost-node-jp1\n    User ubuntu\n"
	got, err := setSSHHostName(config, "ghost-node-jp1", "5.6.7.8")
	if err != nil {
		t.Fatalf("setSSHHostName: %v", err)
	}
	if !strings.Contains(got, "HostName 5.6.7.8") {
		t.Errorf("HostName was not inserted:\n%s", got)
	}
	if !strings.Contains(got, "User ubuntu") {
		t.Errorf("existing directives were lost:\n%s", got)
	}
}

func TestSetSSHHostNameUnknownAlias(t *testing.T) {
	if _, err := setSSHHostName("Host other\n    HostName 1.1.1.1\n", "missing", "5.6.7.8"); err == nil {
		t.Fatal("expected an error for an alias that is not in the config")
	}
}

// A prefix match must not count: "ghost" should not match "ghost-node-jp1".
func TestSetSSHHostNameRequiresExactAlias(t *testing.T) {
	if _, err := setSSHHostName("Host ghost-node-jp1\n    HostName 1.1.1.1\n", "ghost", "5.6.7.8"); err == nil {
		t.Fatal("expected an error: 'ghost' is not an alias in the config")
	}
}

func TestSSHConfigActionWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("Host jp1\n    HostName 1.2.3.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	action := SSHConfigAction{Path: path}
	if err := action.Apply(context.Background(), Target{SSHHost: "jp1"}, "1.2.3.4", "5.6.7.8"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "HostName 5.6.7.8") {
		t.Errorf("file not updated: %s", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 600", perm)
	}
}

func TestSSHConfigActionSkipsWhenNoAliasConfigured(t *testing.T) {
	action := SSHConfigAction{Path: filepath.Join(t.TempDir(), "does-not-exist")}
	if err := action.Apply(context.Background(), Target{}, "1.2.3.4", "5.6.7.8"); err != nil {
		t.Errorf("Apply should be a no-op without an ssh_host, got: %v", err)
	}
}

func TestControlPlaneActionPutsAddress(t *testing.T) {
	var gotPath, gotAuth string
	var payload map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	action := ControlPlaneAction{BaseURL: srv.URL + "/", AdminToken: "tok"}
	target := Target{NodeID: "0f8fad5b-d9cb-469f-a165-70867728950e"}
	if err := action.Apply(context.Background(), target, "1.2.3.4", "5.6.7.8"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if want := "/api/v1/admin/nodes/0f8fad5b-d9cb-469f-a165-70867728950e/address"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if payload["address"] != "5.6.7.8" {
		t.Errorf("address = %q, want 5.6.7.8", payload["address"])
	}
}

func TestControlPlaneActionReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	action := ControlPlaneAction{BaseURL: srv.URL, AdminToken: "tok"}
	err := action.Apply(context.Background(), Target{NodeID: "abc"}, "", "5.6.7.8")
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	if !strings.Contains(err.Error(), "node not found") {
		t.Errorf("error should include the server detail, got: %v", err)
	}
}

func TestControlPlaneActionSkipsWithoutNodeID(t *testing.T) {
	action := ControlPlaneAction{BaseURL: "http://127.0.0.1:1", AdminToken: "tok"}
	if err := action.Apply(context.Background(), Target{}, "", "5.6.7.8"); err != nil {
		t.Errorf("Apply should be a no-op without a node_id, got: %v", err)
	}
}
