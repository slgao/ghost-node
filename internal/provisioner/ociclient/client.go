package ociclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds the credentials from ~/.oci/config (DEFAULT profile).
type Config struct {
	TenancyOCID    string
	UserOCID       string
	Fingerprint    string
	Region         string
	PrivateKeyPEM  []byte
	PrivateKeyPath string // used when PrivateKeyPEM is empty
	HTTPClient     *http.Client
}

// Client is a minimal OCI Core Services (iaas) REST client. It covers only the
// compute and networking calls needed for public IP rotation, which is why we
// sign requests here rather than pulling in the full OCI SDK.
type Client struct {
	endpoint string
	region   string
	signer   *Signer
	http     *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.Region == "" {
		return nil, errors.New("oci: region is required (e.g. ap-tokyo-1)")
	}

	keyPEM := cfg.PrivateKeyPEM
	if len(keyPEM) == 0 {
		if cfg.PrivateKeyPath == "" {
			return nil, errors.New("oci: private key is required")
		}
		b, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("oci: reading private key %s: %w", cfg.PrivateKeyPath, err)
		}
		keyPEM = b
	}

	signer, err := NewSigner(cfg.TenancyOCID, cfg.UserOCID, cfg.Fingerprint, keyPEM)
	if err != nil {
		return nil, err
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		endpoint: fmt.Sprintf("https://iaas.%s.oraclecloud.com", cfg.Region),
		region:   cfg.Region,
		signer:   signer,
		http:     hc,
	}, nil
}

func (c *Client) Region() string { return c.region }

// APIError is a non-2xx response from the OCI API.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	s := fmt.Sprintf("oci: %d %s: %s", e.Status, e.Code, msg)
	if e.RequestID != "" {
		s += fmt.Sprintf(" (opc-request-id: %s)", e.RequestID)
	}
	return s
}

// IsNotFound reports whether err is a 404 from the OCI API. Callers use it to
// distinguish "this instance has no public IP" from a real failure.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// IsConflict reports whether err is a 409 — typically a public IP operation
// racing against one that has not settled yet.
func IsConflict(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}

const maxAttempts = 4

// do issues a signed request, decoding a JSON response into out (may be nil).
// Throttling and transient server errors are retried with linear backoff.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("oci: encoding request body: %w", err)
		}
		body = b
	} else if method == http.MethodPost || method == http.MethodPut {
		body = []byte("{}")
	}

	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt-1) * 2 * time.Second):
			}
		}

		err := c.doOnce(ctx, method, u, body, out)
		if err == nil {
			return nil
		}
		lastErr = err

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			// Network-level failure — worth another try.
			continue
		}
		if apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500 {
			continue
		}
		return err // 4xx other than 429: retrying will not help
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, method, u string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return fmt.Errorf("oci: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if err := c.signer.Sign(req, body); err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("oci: %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("oci: reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{Status: resp.StatusCode, RequestID: resp.Header.Get("opc-request-id")}
		var payload struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &payload) == nil {
			apiErr.Code, apiErr.Message = payload.Code, payload.Message
		}
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(respBody))
		}
		return apiErr
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("oci: decoding response: %w", err)
	}
	return nil
}
