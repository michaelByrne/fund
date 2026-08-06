package hooksweb

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
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

type stubPublisher struct {
	published []string
	err       error
}

func (s *stubPublisher) Publish(event string, _ []byte) error {
	s.published = append(s.published, event)

	return s.err
}

func newTestHandlers(pub publisher) WebhooksHandlers {
	return WebhooksHandlers{
		publisher: pub,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		webhookID: "WH-TEST",
	}
}

func webhookRequest(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/webhooks",
		strings.NewReader(`{"event_type":"PAYMENT.SALE.COMPLETED","resource":{}}`))

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req
}

// A settled rejection and an inability to check need different answers. Answering
// 200 to both means a fault at our end -- a certificate we cannot fetch, or one
// that stops chaining -- silently discards real events, because PayPal marks them
// delivered and never sends them again.
func TestUnverifiableRequestsAskForRedelivery(t *testing.T) {
	validHeaders := map[string]string{
		"paypal-transmission-id":   "tx-1",
		"paypal-transmission-time": time.Now().Format(time.RFC3339),
		"paypal-cert-url":          "https://api.paypal.com/v1/notifications/certs/CERT-abc",
		"paypal-transmission-sig":  "c2ln",
	}

	withHeader := func(key, value string) map[string]string {
		out := map[string]string{}
		for k, v := range validHeaders {
			out[k] = v
		}
		out[key] = value

		return out
	}

	cases := []struct {
		name    string
		headers map[string]string
		// failFetch makes the certificate download fail, standing in for PayPal
		// being unreachable or serving an error.
		failFetch  bool
		wantStatus int
		reason     string
	}{
		{
			name:       "missing headers",
			headers:    map[string]string{},
			wantStatus: http.StatusOK,
			reason:     "a request with no signature headers will not grow them on a retry",
		},
		{
			name:       "certificate url is not PayPal",
			headers:    withHeader("paypal-cert-url", "https://evil.example.com/cert.pem"),
			wantStatus: http.StatusOK,
			reason:     "a forged request is settled, and redelivering it achieves nothing",
		},
		{
			name:       "signature is not base64",
			headers:    withHeader("paypal-transmission-sig", "!!!not base64!!!"),
			wantStatus: http.StatusOK,
			reason:     "a malformed signature decodes no better the second time",
		},
		{
			name:       "certificate cannot be fetched",
			headers:    validHeaders,
			failFetch:  true,
			wantStatus: http.StatusInternalServerError,
			reason:     "the request may be perfectly valid; we simply could not check it",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.failFetch {
				original := certFetchClient
				certFetchClient = &http.Client{Transport: failingTransport{}}
				t.Cleanup(func() { certFetchClient = original })
			}

			pub := &stubPublisher{}
			rec := httptest.NewRecorder()

			newTestHandlers(pub).webhooks(rec, webhookRequest(t, c.headers))

			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, c.wantStatus, c.reason)
			}

			if len(pub.published) != 0 {
				t.Errorf("an unverified event must not be published, got %v", pub.published)
			}
		})
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("certificate host unreachable")
}
