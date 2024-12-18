package hooksweb

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
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
	timeStamp := r.Header.Get("paypal-transmission-time")
	certURL := r.Header.Get("paypal-cert-url")
	sig := r.Header.Get("paypal-transmission-sig")

	if transmissionID == "" || timeStamp == "" {
		return fmt.Errorf("missing required PayPal headers")
	}

	body := r.Body
	defer body.Close()

	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	crc := crc32Checksum(bodyBytes)

	message := fmt.Sprintf("%s|%s|%s|%d", transmissionID, timeStamp, "7SG759247G748343R", crc)

	certStr, err := downloadAndCache(certURL, "paypal_cert.pem")
	if err != nil {
		return err
	}

	block, _ := pem.Decode([]byte(certStr))
	if block == nil {
		return fmt.Errorf("failed to parse PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	decodedSig, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	hasher := crypto.SHA256.New()
	hasher.Write([]byte(message))
	return cert.CheckSignature(x509.SHA256WithRSA, hasher.Sum(nil), decodedSig)
}

func crc32Checksum(data []byte) uint32 {
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc = (crc >> 8) ^ (uint32(b) ^ crc) // Example CRC logic
	}
	return ^crc
}
