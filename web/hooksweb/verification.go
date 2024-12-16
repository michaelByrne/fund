package hooksweb

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
)

const (
	IEEE = 0xedb88320
)

func verifySignature(r *http.Request, webhookID string) error {
	transmissionID := r.Header.Get("paypal-transmission-id")
	if transmissionID == "" {
		return fmt.Errorf("missing transmission ID")
	}

	timestamp := r.Header.Get("paypal-transmission-time")
	if timestamp == "" {
		return fmt.Errorf("missing transmission time")
	}

	signature := r.Header.Get("paypal-transmission-sig")
	if signature == "" {
		return fmt.Errorf("missing transmission signature")
	}

	certURL := r.Header.Get("paypal-cert-url")
	if certURL == "" {
		return fmt.Errorf("missing cert URL")
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	defer r.Body.Close()

	crc32q := crc32.MakeTable(IEEE)
	bodyHash := fmt.Sprintf("%08x\n", crc32.Checksum(bodyBytes, crc32q))

	message := fmt.Sprintf("%s|%s|%s|%s", transmissionID, timestamp, webhookID, bodyHash)

	pemBytes, err := downloadAndCacheCertPEM(certURL)
	if err != nil {
		return err
	}

	return verifySignatureWithPEM(signature, message, pemBytes)
}

func verifySignatureWithPEM(signature string, message string, pemBytes []byte) error {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return fmt.Errorf("failed to parse PEM block containing the public key")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	hash := sha256.Sum256([]byte(message))

	sigBytes, err := decodeBase64(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}

	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		err = rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sigBytes)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, hash[:], sigBytes) {
			err = fmt.Errorf("ECDSA verification failed")
		}
	case ed25519.PublicKey:
		if !ed25519.Verify(key, hash[:], sigBytes) {
			err = fmt.Errorf("ed25519 verification failed")
		}
	default:
		err = fmt.Errorf("unsupported public key type")
	}

	return err
}

func decodeBase64(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func downloadAndCacheCertPEM(url, hookID string) ([]byte, error) {
	sigBytes, err := os.ReadFile(fmt.Sprintf("/tmp/%s", hookID))
	if err == nil {
		return sigBytes, nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	sigBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/tmp/%s", hookID)
	err = os.WriteFile(path, sigBytes, 0644)
	if err != nil {
		return nil, err
	}

	return sigBytes, nil
}
