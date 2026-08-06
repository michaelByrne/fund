package hooksweb

import (
	"context"
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

// stubDeliveries reports every transmission as new unless told otherwise, which
// is the ordinary case.
type stubDeliveries struct {
	seen map[string]bool
	err  error
}

func (s *stubDeliveries) RecordDelivery(_ context.Context, transmissionID, _ string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}

	if s.seen == nil {
		s.seen = map[string]bool{}
	}

	if s.seen[transmissionID] {
		return false, nil
	}

	s.seen[transmissionID] = true

	return true, nil
}

func newTestHandlers(pub publisher) WebhooksHandlers {
	return newTestHandlersWith(pub, &stubDeliveries{})
}

func newTestHandlersWith(pub publisher, seen deliveries) WebhooksHandlers {
	return WebhooksHandlers{
		publisher:  pub,
		deliveries: seen,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		webhookID:  "WH-TEST",
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

// A valid signature says PayPal sent this. It does not say we have not already
// handled it: a captured request replays for as long as its certificate
// verifies. Recorded before publishing, so a replay never reaches the stream.
func TestAcceptRefusesToPublishAReplay(t *testing.T) {
	body := []byte(`{"event_type":"PAYMENT.SALE.COMPLETED","resource":{}}`)

	t.Run("a first delivery is published", func(t *testing.T) {
		pub := &stubPublisher{}
		rec := httptest.NewRecorder()

		newTestHandlersWith(pub, &stubDeliveries{}).
			accept(context.Background(), rec, "tx-1", body)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}

		if len(pub.published) != 1 {
			t.Fatalf("published %v, want one event", pub.published)
		}
	})

	t.Run("the same transmission again is not", func(t *testing.T) {
		seen := &stubDeliveries{}
		pub := &stubPublisher{}

		first := httptest.NewRecorder()
		newTestHandlersWith(pub, seen).accept(context.Background(), first, "tx-2", body)

		second := httptest.NewRecorder()
		newTestHandlersWith(pub, seen).accept(context.Background(), second, "tx-2", body)

		// 200, because a replay is not something PayPal should send again.
		if second.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", second.Code)
		}

		if len(pub.published) != 1 {
			t.Errorf("published %d times, want 1 -- the replay reached the stream", len(pub.published))
		}
	})

	t.Run("a different transmission of the same event still is", func(t *testing.T) {
		// PayPal sends distinct transmission ids for distinct events, so keying on
		// the id must not collapse two real payments into one.
		seen := &stubDeliveries{}
		pub := &stubPublisher{}

		newTestHandlersWith(pub, seen).accept(context.Background(), httptest.NewRecorder(), "tx-3", body)
		newTestHandlersWith(pub, seen).accept(context.Background(), httptest.NewRecorder(), "tx-4", body)

		if len(pub.published) != 2 {
			t.Errorf("published %d, want 2", len(pub.published))
		}
	})
}

// The record is what makes the replay check meaningful, so failing to write it is
// a reason to be asked again rather than to press on unprotected.
func TestAcceptAsksForRedeliveryWhenItCannotRecord(t *testing.T) {
	pub := &stubPublisher{}
	rec := httptest.NewRecorder()

	newTestHandlersWith(pub, &stubDeliveries{err: errors.New("connection refused")}).
		accept(context.Background(), rec, "tx-5", []byte(`{"event_type":"PAYMENT.SALE.COMPLETED"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	if len(pub.published) != 0 {
		t.Error("nothing should be published when we cannot tell a replay from a first delivery")
	}
}
