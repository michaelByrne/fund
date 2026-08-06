package hooksweb

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// certFetchClient bounds the PayPal certificate download. Without a timeout a
// stalled connection holds the webhook handler open indefinitely, and PayPal
// gives up and redelivers while we are still waiting.
var certFetchClient = &http.Client{Timeout: 10 * time.Second}

// certHosts are the only hosts a webhook may name as the source of its signing
// certificate.
//
// paypal-cert-url is a request header, so without this the caller chooses which
// key verifies their own signature: host a certificate anywhere, sign the message
// with the matching private key, and verification passes. The check has to be on
// the host rather than on a prefix, because a URL like
// https://api.paypal.com.example.com/cert.pem passes a naive prefix test.
var certHosts = map[string]bool{
	"api.paypal.com":           true,
	"api.sandbox.paypal.com":   true,
	"api-m.paypal.com":         true,
	"api-m.sandbox.paypal.com": true,
}

func checkCertURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("unparseable certificate url: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("certificate url is not https: %q", parsed.Scheme)
	}

	if !certHosts[parsed.Hostname()] {
		return fmt.Errorf("certificate url host %q is not PayPal", parsed.Hostname())
	}

	return nil
}

// parseAndVerifyCert reads the downloaded PEM and checks it chains to a trusted
// root, rather than trusting whatever key the certificate happens to carry.
//
// PayPal serves the leaf first and may include intermediates after it.
func parseAndVerifyCert(certPem string) (*x509.Certificate, error) {
	var leaf *x509.Certificate
	intermediates := x509.NewCertPool()

	rest := []byte(certPem)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			continue
		}

		parsed, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}

		if leaf == nil {
			leaf = parsed

			continue
		}

		intermediates.AddCert(parsed)
	}

	if leaf == nil {
		return nil, fmt.Errorf("no certificate in PEM")
	}

	if _, err := leaf.Verify(x509.VerifyOptions{Intermediates: intermediates}); err != nil {
		return nil, fmt.Errorf("certificate does not chain to a trusted root: %w", err)
	}

	return leaf, nil
}

// downloadAndCache caches by URL. Keying on a constant meant the first
// certificate fetched after a restart answered every later request whatever URL
// it named, so a rotation at PayPal broke verification until the container was
// replaced -- and a single request naming another URL poisoned it for everyone.
func downloadAndCache(ctx context.Context, certURL, cacheKey string) (string, error) {
	filePath := filepath.Join("tmp", cacheKey)

	var data []byte
	var err error

	if _, err = os.Stat(filePath); err == nil {
		data, err = os.ReadFile(filePath)
		if err == nil {
			return string(data), nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err = os.MkdirAll("tmp", 0755); err != nil {
		return "", fmt.Errorf("failed to create tmp directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build certificate request: %w", err)
	}

	resp, err := certFetchClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("certificate download returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if err = os.WriteFile(filePath, body, 0644); err != nil {
		return "", fmt.Errorf("failed to write to cache: %w", err)
	}

	return string(body), nil
}

func verifySignature(r *http.Request, webhookID string) ([]byte, error) {
	transmissionID := r.Header.Get("paypal-transmission-id")
	timestamp := r.Header.Get("paypal-transmission-time")
	certURL := r.Header.Get("paypal-cert-url")
	sig := r.Header.Get("paypal-transmission-sig")

	if transmissionID == "" || timestamp == "" {
		return nil, fmt.Errorf("missing required PayPal headers")
	}

	body := r.Body
	defer body.Close()

	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	crc := crc32.ChecksumIEEE(bodyBytes)

	message := fmt.Sprintf("%s|%s|%s|%d", transmissionID, timestamp, webhookID, crc)

	// Before anything is downloaded. The URL decides which key will verify this
	// request's signature, so an unchecked one makes the signature meaningless.
	if err = checkCertURL(certURL); err != nil {
		return nil, err
	}

	// Cache key derived from the URL, so a rotated certificate is a new entry
	// rather than a stale hit.
	sum := sha256.Sum256([]byte(certURL))
	certPem, err := downloadAndCache(r.Context(), certURL, "pp-cert-"+hex.EncodeToString(sum[:8])+".pem")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch certificate: %w", err)
	}

	parsed, err := parseAndVerifyCert(certPem)
	if err != nil {
		return nil, err
	}

	// Decode the signature from base64
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	return bodyBytes, parsed.CheckSignature(x509.SHA256WithRSA, []byte(message)[:], sigBytes)
}
