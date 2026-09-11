package ociclient

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func testKeyPEM(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return pem.EncodeToMemory(block), key
}

var authParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

func parseAuthorization(t *testing.T, header string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(header, "Signature ") {
		t.Fatalf("Authorization does not start with the Signature scheme: %q", header)
	}
	out := map[string]string{}
	for _, m := range authParam.FindAllStringSubmatch(header, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// verify rebuilds the signing string the way OCI does and checks the signature.
func verify(t *testing.T, req *http.Request, pub *rsa.PublicKey, headerList string) {
	t.Helper()

	var lines []string
	for _, h := range strings.Split(headerList, " ") {
		switch h {
		case "(request-target)":
			lines = append(lines, "(request-target): "+strings.ToLower(req.Method)+" "+req.URL.RequestURI())
		case "host":
			lines = append(lines, "host: "+req.URL.Host)
		default:
			lines = append(lines, h+": "+req.Header.Get(h))
		}
	}

	params := parseAuthorization(t, req.Header.Get("Authorization"))
	sig, err := base64.StdEncoding.DecodeString(params["signature"])
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}

	digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify over %q: %v", strings.Join(lines, "\n"), err)
	}
}

func TestSignGETSignsRequiredHeaders(t *testing.T) {
	pemBytes, key := testKeyPEM(t)
	signer, err := NewSigner("ocid1.tenancy.oc1..t", "ocid1.user.oc1..u", "aa:bb:cc", pemBytes)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://iaas.ap-tokyo-1.oraclecloud.com/20160918/vnicAttachments?compartmentId=c&instanceId=i", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(req, nil); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	params := parseAuthorization(t, req.Header.Get("Authorization"))
	if got, want := params["headers"], "date (request-target) host"; got != want {
		t.Errorf("headers = %q, want %q", got, want)
	}
	if got, want := params["keyId"], "ocid1.tenancy.oc1..t/ocid1.user.oc1..u/aa:bb:cc"; got != want {
		t.Errorf("keyId = %q, want %q", got, want)
	}
	if got, want := params["algorithm"], "rsa-sha256"; got != want {
		t.Errorf("algorithm = %q, want %q", got, want)
	}
	if req.Header.Get("Date") == "" {
		t.Error("Date header was not set")
	}
	// The signature must cover the query string, not just the path.
	if !strings.Contains(req.URL.RequestURI(), "?") {
		t.Fatal("test URL lost its query string")
	}
	verify(t, req, &key.PublicKey, params["headers"])
}

func TestSignPOSTIncludesBodyHeaders(t *testing.T) {
	pemBytes, key := testKeyPEM(t)
	signer, err := NewSigner("t", "u", "f", pemBytes)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	body := []byte(`{"privateIpId":"ocid1.privateip.oc1..p"}`)
	req, err := http.NewRequest(http.MethodPost, "https://iaas.ap-tokyo-1.oraclecloud.com/20160918/publicIps", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(req, body); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	params := parseAuthorization(t, req.Header.Get("Authorization"))
	want := "date (request-target) host content-length content-type x-content-sha256"
	if params["headers"] != want {
		t.Errorf("headers = %q, want %q", params["headers"], want)
	}

	sum := sha256.Sum256(body)
	if got, want := req.Header.Get("X-Content-Sha256"), base64.StdEncoding.EncodeToString(sum[:]); got != want {
		t.Errorf("x-content-sha256 = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Content-Length"), strconv.Itoa(len(body)); got != want {
		t.Errorf("content-length = %q, want %q", got, want)
	}
	if req.ContentLength != int64(len(body)) {
		t.Errorf("req.ContentLength = %d, want %d", req.ContentLength, len(body))
	}
	verify(t, req, &key.PublicKey, params["headers"])
}

func TestSignEmptyPOSTBodyStillHashed(t *testing.T) {
	pemBytes, key := testKeyPEM(t)
	signer, err := NewSigner("t", "u", "f", pemBytes)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://iaas.x.oraclecloud.com/20160918/publicIps", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(req, nil); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// sha256 of the empty string, base64-encoded.
	const emptyDigest = "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="
	if got := req.Header.Get("X-Content-Sha256"); got != emptyDigest {
		t.Errorf("x-content-sha256 = %q, want %q", got, emptyDigest)
	}
	params := parseAuthorization(t, req.Header.Get("Authorization"))
	verify(t, req, &key.PublicKey, params["headers"])
}

func TestParsePrivateKeyRejectsEncrypted(t *testing.T) {
	encrypted := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-128-CBC,XX"},
		Bytes:   []byte("not-a-real-key"),
	})
	if _, err := parsePrivateKey(encrypted); err == nil {
		t.Fatal("expected an error for a passphrase-protected key")
	} else if !strings.Contains(err.Error(), "passphrase") {
		t.Errorf("error should mention the passphrase, got: %v", err)
	}
}

func TestParsePrivateKeyAcceptsPKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	parsed, err := parsePrivateKey(pemBytes)
	if err != nil {
		t.Fatalf("parsePrivateKey: %v", err)
	}
	if !parsed.Equal(key) {
		t.Error("parsed key differs from the original")
	}
}

func TestNewSignerRequiresIdentifiers(t *testing.T) {
	pemBytes, _ := testKeyPEM(t)
	if _, err := NewSigner("", "u", "f", pemBytes); err == nil {
		t.Error("expected an error when the tenancy OCID is missing")
	}
	if _, err := NewSigner("t", "", "f", pemBytes); err == nil {
		t.Error("expected an error when the user OCID is missing")
	}
	if _, err := NewSigner("t", "u", "", pemBytes); err == nil {
		t.Error("expected an error when the fingerprint is missing")
	}
}
