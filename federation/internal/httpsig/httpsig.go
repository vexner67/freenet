// Package httpsig signs and checks requests between servers of the
// Fediverse: HTTP Signatures (draft-cavage-http-signatures) with RSA and
// SHA-256, the way Mastodon and the others speak it, and the RSA keys of
// the pages.
package httpsig

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
	"strings"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	keyBits = 2048
	// maxSkew is how far the Date of a request may be from now: older
	// requests are not taken, so a signed one cannot be sent again later.
	maxSkew = 12 * time.Hour
)

var errMalformed = errors.New("malformed signature")

// Keys makes the RSA key pairs of pages.
type Keys struct{}

func (Keys) NewKeys() (fedi.KeyPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return fedi.KeyPair{}, errs.Wrap("rsa.GenerateKey", err)
	}

	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fedi.KeyPair{}, errs.Wrap("x509.MarshalPKCS8PrivateKey", err)
	}

	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fedi.KeyPair{}, errs.Wrap("x509.MarshalPKIXPublicKey", err)
	}

	return fedi.KeyPair{
		Public:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})),
		Private: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})),
	}, nil
}

// Sign signs req, whose body is body (nil for a GET), with the private
// key (PEM) of keyID: it sets Host, Date, Digest of a body and Signature.
func Sign(req *http.Request, body []byte, keyID fedi.URL, privateKey string, now time.Time) error {
	key, err := parsePrivateKey(privateKey)
	if err != nil {
		return err
	}

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("Date", now.UTC().Format(http.TimeFormat))

	names := []string{"(request-target)", "host", "date"}
	if body != nil {
		req.Header.Set("Digest", digest(body))
		names = append(names, "digest", "content-type")
	}

	headers := map[string]string{"host": req.URL.Host}
	for name := range req.Header {
		headers[strings.ToLower(name)] = req.Header.Get(name)
	}

	signed := signingString(req.Method, req.URL.RequestURI(), names, headers)
	hash := sha256.Sum256([]byte(signed))

	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return errs.Wrap("rsa.SignPKCS1v15", err)
	}

	req.Header.Set("Signature", fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID, strings.Join(names, " "), base64.StdEncoding.EncodeToString(signature)))

	return nil
}

// Signatures checks the signatures of requests that came to an inbox.
type Signatures struct{}

func (Signatures) KeyID(r service.SignedRequest) (fedi.URL, error) {
	params, err := parseSignature(r.Headers["signature"])
	if err != nil {
		return "", fedi.ErrBadSignature
	}

	id, err := fedi.NewURL(params["keyid"])
	if err != nil {
		return "", fedi.ErrBadSignature
	}

	return id, nil
}

// Verify checks that the request is signed with publicKey over its
// target, host and date, and over its body when it has one, and that it
// is fresh.
func (Signatures) Verify(r service.SignedRequest, publicKey string, now time.Time) error {
	params, err := parseSignature(r.Headers["signature"])
	if err != nil {
		return fedi.ErrBadSignature
	}

	names := strings.Fields(strings.ToLower(params["headers"]))
	if !covers(names, "(request-target)", "host", "date") || (len(r.Body) > 0 && !covers(names, "digest")) {
		return fedi.ErrBadSignature
	}

	date, err := http.ParseTime(r.Headers["date"])
	if err != nil || date.Sub(now).Abs() > maxSkew {
		return fedi.ErrBadSignature
	}

	if len(r.Body) > 0 && !digestMatches(r.Headers["digest"], r.Body) {
		return fedi.ErrBadSignature
	}

	key, err := parsePublicKey(publicKey)
	if err != nil {
		return fedi.ErrBadSignature
	}

	signature, err := base64.StdEncoding.DecodeString(params["signature"])
	if err != nil {
		return fedi.ErrBadSignature
	}

	hash := sha256.Sum256([]byte(signingString(r.Method, r.Path, names, r.Headers)))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature) != nil {
		return fedi.ErrBadSignature
	}

	return nil
}

// signingString is what a signature signs: one line per header named,
// in their order.
func signingString(method, target string, names []string, headers map[string]string) string {
	lines := make([]string, len(names))

	for i, name := range names {
		value := headers[name]
		if name == "(request-target)" {
			value = strings.ToLower(method) + " " + target
		}

		lines[i] = name + ": " + value
	}

	return strings.Join(lines, "\n")
}

// parseSignature reads the parameters of a Signature header, their names
// in lower case: keyId="…",headers="…",signature="…".
func parseSignature(header string) (map[string]string, error) {
	params := map[string]string{}

	for header != "" {
		name, rest, ok := strings.Cut(header, "=")
		if !ok || !strings.HasPrefix(rest, `"`) {
			return nil, errMalformed
		}

		value, after, ok := strings.Cut(rest[1:], `"`)
		if !ok {
			return nil, errMalformed
		}

		params[strings.ToLower(strings.TrimSpace(name))] = value
		header = strings.TrimPrefix(strings.TrimSpace(after), ",")
	}

	if params["keyid"] == "" || params["signature"] == "" {
		return nil, errMalformed
	}

	if params["headers"] == "" {
		params["headers"] = "date"
	}

	return params, nil
}

func covers(names []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, n := range names {
			found = found || n == w
		}

		if !found {
			return false
		}
	}

	return true
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

// digestMatches: one of the digests of the header, SHA-256, is the body's.
func digestMatches(header string, body []byte) bool {
	want := digest(body)

	for _, d := range strings.Split(header, ",") {
		d = strings.TrimSpace(d)
		if len(d) > 8 && strings.EqualFold(d[:8], "SHA-256=") && d[8:] == want[8:] {
			return true
		}
	}

	return false
}

func parsePrivateKey(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("no private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errs.Wrap("x509.ParsePKCS8PrivateKey", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an rsa key")
	}

	return rsaKey, nil
}

// parsePublicKey reads the key of an actor: PKIX, as most servers write
// it, or PKCS#1.
func parsePublicKey(s string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("no public key")
	}

	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errs.Wrap("x509.ParsePKIXPublicKey", err)
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an rsa key")
	}

	return rsaKey, nil
}
