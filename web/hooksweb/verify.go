package hooksweb

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func downloadAndCache(url, cacheKey string) (string, error) {
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

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download from URL: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if err = os.WriteFile(filePath, body, 0644); err != nil {
		return "", fmt.Errorf("failed to write to cache: %w", err)
	}

	return string(body), nil
}

func verifySignature(r *http.Request, webhookID string) error {
	transmissionID := r.Header.Get("paypal-transmission-id")
	timestamp := r.Header.Get("paypal-transmission-time")
	certURL := r.Header.Get("paypal-cert-url")
	sig := r.Header.Get("paypal-transmission-sig")

	if transmissionID == "" || timestamp == "" {
		return fmt.Errorf("missing required PayPal headers")
	}

	body := r.Body

	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	crc := crc32.ChecksumIEEE(bodyBytes)

	message := fmt.Sprintf("%s|%s|%s|%d", transmissionID, timestamp, webhookID, crc)

	certPem, err := downloadAndCache(certURL, "pp-cert.pem")
	if err != nil {
		return fmt.Errorf("failed to fetch certificate: %w", err)
	}

	block, _ := pem.Decode([]byte(certPem))
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Decode the signature from base64
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	return parsed.CheckSignature(x509.SHA256WithRSA, []byte(message)[:], sigBytes)
}
