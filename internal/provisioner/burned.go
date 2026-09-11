package provisioner

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// BurnedStore remembers addresses that failed verification so a rotation does
// not hand them straight back. Oracle draws from a regional pool that VPN users
// have been cycling through for years, so repeat draws are common — and a
// neighbour in the same /24 is often blocked along with it.
type BurnedStore struct {
	path string

	mu      sync.Mutex
	entries map[string]time.Time
}

// LoadBurnedStore reads the store from disk. A missing file is not an error.
func LoadBurnedStore(path string) (*BurnedStore, error) {
	s := &BurnedStore{path: path, entries: map[string]time.Time{}}
	if path == "" {
		return s, nil
	}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading burned-IP store %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &s.entries); err != nil {
		return nil, fmt.Errorf("parsing burned-IP store %s: %w", path, err)
	}
	return s, nil
}

// Add records an address as burned as of now.
func (s *BurnedStore) Add(ip string) {
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[ip] = time.Now().UTC()
}

// Burned reports whether ip, or any address in its /24, was burned within the
// given window. An empty window disables the check.
func (s *BurnedStore) Burned(ip string, within time.Duration) (bool, string) {
	if ip == "" || within <= 0 {
		return false, ""
	}
	candidate, err := netip.ParseAddr(ip)
	if err != nil {
		return false, ""
	}
	// /24 neighbour matching only makes sense for IPv4; for anything else
	// fall back to exact matches.
	var prefix netip.Prefix
	if candidate.Is4() {
		if p, err := candidate.Prefix(24); err == nil {
			prefix = p
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-within)
	for raw, when := range s.entries {
		if when.Before(cutoff) {
			continue
		}
		if raw == ip {
			return true, fmt.Sprintf("burned %s ago", roughAge(when))
		}
		if !prefix.IsValid() {
			continue
		}
		other, err := netip.ParseAddr(raw)
		if err == nil && prefix.Contains(other) {
			return true, fmt.Sprintf("same /24 as %s, burned %s ago", raw, roughAge(when))
		}
	}
	return false, ""
}

// Save writes the store back to disk, dropping entries older than retain.
func (s *BurnedStore) Save(retain time.Duration) error {
	if s.path == "" {
		return nil
	}

	s.mu.Lock()
	cutoff := time.Now().Add(-retain)
	for ip, when := range s.entries {
		if retain > 0 && when.Before(cutoff) {
			delete(s.entries, ip)
		}
	}
	b, err := json.MarshalIndent(s.entries, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return fmt.Errorf("encoding burned-IP store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating burned-IP store directory: %w", err)
	}
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("writing burned-IP store %s: %w", s.path, err)
	}
	return nil
}

// List returns burned addresses, most recent first.
func (s *BurnedStore) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ips := make([]string, 0, len(s.entries))
	for ip := range s.entries {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return s.entries[ips[i]].After(s.entries[ips[j]]) })
	return ips
}

func roughAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
