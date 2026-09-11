package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const cloudflareAPI = "https://api.cloudflare.com/client/v4"

// Cloudflare updates A records through the Cloudflare API.
//
// The API token needs only Zone:DNS:Edit on the zone holding your node records.
type Cloudflare struct {
	Token  string
	ZoneID string // optional; looked up from the record name when empty
	TTL    int
	// Proxied puts the record behind Cloudflare's proxy. Leave false for
	// REALITY and Hysteria2 nodes — the proxy only carries HTTP(S), and
	// proxying would hide the address the client actually needs to reach.
	Proxied bool

	HTTPClient *http.Client

	zoneCache map[string]string
}

func NewCloudflare(token, zoneID string) *Cloudflare {
	return &Cloudflare{
		Token:      token,
		ZoneID:     zoneID,
		TTL:        60,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
		zoneCache:  map[string]string{},
	}
}

func (c *Cloudflare) Name() string { return "cloudflare" }

type cfResponse struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *cfResponse) err() error {
	if c.Success {
		return nil
	}
	if len(c.Errors) == 0 {
		return errors.New("cloudflare: request failed with no error detail")
	}
	msgs := make([]string, 0, len(c.Errors))
	for _, e := range c.Errors {
		msgs = append(msgs, fmt.Sprintf("%d: %s", e.Code, e.Message))
	}
	return fmt.Errorf("cloudflare: %s", strings.Join(msgs, "; "))
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// Upsert points fqdn at ip, creating the A record when it does not exist.
func (c *Cloudflare) Upsert(ctx context.Context, fqdn, ip string) error {
	if c.Token == "" {
		return errors.New("cloudflare: API token is required")
	}

	zoneID, err := c.zoneFor(ctx, fqdn)
	if err != nil {
		return err
	}

	existing, err := c.findRecord(ctx, zoneID, fqdn)
	if err != nil {
		return err
	}

	ttl := c.TTL
	if ttl <= 0 {
		ttl = 60
	}
	record := cfDNSRecord{Type: "A", Name: fqdn, Content: ip, TTL: ttl, Proxied: c.Proxied}

	if existing == nil {
		var out cfDNSRecord
		return c.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", record, &out)
	}
	if existing.Content == ip {
		return nil
	}
	var out cfDNSRecord
	return c.do(ctx, http.MethodPatch, "/zones/"+zoneID+"/dns_records/"+existing.ID, record, &out)
}

// zoneFor resolves the zone holding fqdn by walking up its parent domains,
// so a config only has to name the record, not the zone.
func (c *Cloudflare) zoneFor(ctx context.Context, fqdn string) (string, error) {
	if c.ZoneID != "" {
		return c.ZoneID, nil
	}
	if c.zoneCache == nil {
		c.zoneCache = map[string]string{}
	}

	labels := strings.Split(strings.TrimSuffix(fqdn, "."), ".")
	for i := 0; i+1 < len(labels); i++ {
		candidate := strings.Join(labels[i:], ".")
		if id, ok := c.zoneCache[candidate]; ok {
			return id, nil
		}

		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := c.do(ctx, http.MethodGet, "/zones?name="+candidate, nil, &zones); err != nil {
			return "", err
		}
		if len(zones) > 0 {
			c.zoneCache[candidate] = zones[0].ID
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("cloudflare: no zone found for %s — set zone_id explicitly", fqdn)
}

func (c *Cloudflare) findRecord(ctx context.Context, zoneID, fqdn string) (*cfDNSRecord, error) {
	var records []cfDNSRecord
	path := fmt.Sprintf("/zones/%s/dns_records?type=A&name=%s", zoneID, fqdn)
	if err := c.do(ctx, http.MethodGet, path, nil, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return &records[0], nil
}

func (c *Cloudflare) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("cloudflare: encoding request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, cloudflareAPI+path, body)
	if err != nil {
		return fmt.Errorf("cloudflare: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("cloudflare: reading response: %w", err)
	}

	var parsed cfResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("cloudflare: %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := parsed.err(); err != nil {
		return err
	}
	if out == nil || len(parsed.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(parsed.Result, out); err != nil {
		return fmt.Errorf("cloudflare: decoding result: %w", err)
	}
	return nil
}
