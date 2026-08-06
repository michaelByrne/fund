package hooksweb

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// The header names the URL, so an unchecked one lets the caller choose the key
// that verifies their own signature: host a certificate, sign with the matching
// private key, and the webhook is indistinguishable from a real one.
func TestCertURLMustBePayPal(t *testing.T) {
	rejected := []struct {
		name string
		url  string
	}{
		{"attacker's own host", "https://evil.example.com/cert.pem"},
		// The reason the check is on the parsed host and not a prefix.
		{"paypal as a prefix of another domain", "https://api.paypal.com.evil.example.com/cert.pem"},
		{"paypal as a path segment", "https://evil.example.com/api.paypal.com/cert.pem"},
		{"paypal in userinfo", "https://api.paypal.com@evil.example.com/cert.pem"},
		{"plaintext", "http://api.paypal.com/cert.pem"},
		{"empty", ""},
	}

	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			if err := checkCertURL(c.url); err == nil {
				t.Errorf("checkCertURL(%q) accepted a non-PayPal certificate source", c.url)
			}
		})
	}

	accepted := []string{
		"https://api.paypal.com/v1/notifications/certs/CERT-abc",
		"https://api.sandbox.paypal.com/v1/notifications/certs/CERT-abc",
		"https://api-m.paypal.com/v1/notifications/certs/CERT-abc",
	}

	for _, u := range accepted {
		if err := checkCertURL(u); err != nil {
			t.Errorf("checkCertURL(%q) rejected a real PayPal URL: %v", u, err)
		}
	}
}

// selfSignedPEM is what an attacker can produce: a syntactically perfect
// certificate carrying a key they hold.
func selfSignedPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "messageverificationcerts.paypal.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// The host check is not enough on its own. Parsing a certificate says nothing
// about who issued it, and the old code verified signatures against whatever
// public key it found.
func TestCertMustChainToATrustedRoot(t *testing.T) {
	if _, err := parseAndVerifyCert(selfSignedPEM(t)); err == nil {
		t.Fatal("a self-signed certificate was accepted")
	} else if !strings.Contains(err.Error(), "chain") {
		t.Errorf("expected a chain-of-trust failure, got: %v", err)
	}
}

func TestCertPEMMustContainACertificate(t *testing.T) {
	for _, c := range []struct {
		name string
		pem  string
	}{
		{"empty", ""},
		{"not PEM at all", "definitely not a certificate"},
		{"a key rather than a certificate", "-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseAndVerifyCert(c.pem); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
