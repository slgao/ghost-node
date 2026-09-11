package ociclient

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Signer implements OCI's request signing scheme: draft-cavage-http-signatures
// with rsa-sha256 over a fixed set of headers.
//
// GET/DELETE sign:  date (request-target) host
// POST/PUT sign:    date (request-target) host content-length content-type x-content-sha256
type Signer struct {
	keyID string
	key   *rsa.PrivateKey
}

// NewSigner builds a signer from an OCI API key. The key must be an
// unencrypted PKCS#1 or PKCS#8 RSA private key in PEM form — the same file
// referenced by key_file in ~/.oci/config.
func NewSigner(tenancyOCID, userOCID, fingerprint string, privateKeyPEM []byte) (*Signer, error) {
	switch {
	case tenancyOCID == "":
		return nil, errors.New("oci: tenancy OCID is required")
	case userOCID == "":
		return nil, errors.New("oci: user OCID is required")
	case fingerprint == "":
		return nil, errors.New("oci: API key fingerprint is required")
	}

	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	return &Signer{
		keyID: fmt.Sprintf("%s/%s/%s", tenancyOCID, userOCID, fingerprint),
		key:   key,
	}, nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("oci: no PEM block found in private key")
	}
	// Proc-Type: 4,ENCRYPTED marks a passphrase-protected key. Decrypting those
	// uses a broken cipher and is deprecated in x509; ask for a clean key instead.
	if strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED") {
		return nil, errors.New("oci: passphrase-protected private keys are not supported — " +
			"re-export the key without a passphrase (openssl rsa -in key.pem -out key-nopass.pem)")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("oci: parsing private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("oci: private key is %T, want RSA", parsed)
	}
	return key, nil
}

// Sign adds Date, body digest headers and the Authorization signature to req.
// body must be the exact bytes that will be sent (nil for bodyless requests).
func (s *Signer) Sign(req *http.Request, body []byte) error {
	headers := []string{"date", "(request-target)", "host"}

	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}

	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		sum := sha256.Sum256(body)
		req.Header.Set("X-Content-Sha256", base64.StdEncoding.EncodeToString(sum[:]))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Length", strconv.Itoa(len(body)))
		req.ContentLength = int64(len(body))
		headers = append(headers, "content-length", "content-type", "x-content-sha256")
	}

	var sb strings.Builder
	for i, h := range headers {
		if i > 0 {
			sb.WriteByte('\n')
		}
		switch h {
		case "(request-target)":
			// e.g. "get /20160918/instances/ocid1.instance.oc1..."
			fmt.Fprintf(&sb, "(request-target): %s %s", strings.ToLower(req.Method), req.URL.RequestURI())
		case "host":
			fmt.Fprintf(&sb, "host: %s", req.URL.Host)
		default:
			fmt.Fprintf(&sb, "%s: %s", h, req.Header.Get(h))
		}
	}

	digest := sha256.Sum256([]byte(sb.String()))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return fmt.Errorf("oci: signing request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf(
		`Signature version="1",keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		s.keyID, strings.Join(headers, " "), base64.StdEncoding.EncodeToString(sig),
	))
	return nil
}
