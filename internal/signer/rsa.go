package signer

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// AlpineRSASigner implements RSASigner interface for Alpine APK signing
type AlpineRSASigner struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// NewAlpineRSASigner creates a new RSA signer for Alpine from a private key file
func NewAlpineRSASigner(keyPath, passphrase string) (*AlpineRSASigner, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("key path is empty")
	}

	// Read private key file
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Check if key is encrypted
	var privateKey *rsa.PrivateKey
	if x509.IsEncryptedPEMBlock(block) {
		if passphrase == "" {
			return nil, fmt.Errorf("key is encrypted but no passphrase provided")
		}

		// Decrypt the PEM block
		decryptedData, err := x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt key: %w", err)
		}

		// Parse PKCS1 or PKCS8 private key
		privateKey, err = parseRSAPrivateKey(decryptedData)
		if err != nil {
			return nil, err
		}
	} else {
		// Parse unencrypted key
		privateKey, err = parseRSAPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	}

	return &AlpineRSASigner{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
	}, nil
}

// parseRSAPrivateKey tries to parse RSA private key in PKCS1 or PKCS8 format
func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	// Try PKCS1 first
	key, err := x509.ParsePKCS1PrivateKey(data)
	if err == nil {
		return key, nil
	}

	// Try PKCS8
	parsedKey, err := x509.ParsePKCS8PrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return rsaKey, nil
}

// SignRSA creates an RSA PKCS1v15 signature using SHA1 (Alpine APK standard)
func (s *AlpineRSASigner) SignRSA(data []byte) ([]byte, error) {
	// Calculate SHA1 hash
	h := sha1.New()
	h.Write(data)
	hashed := h.Sum(nil)

	// Sign with RSA PKCS1v15
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA1, hashed)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return signature, nil
}

// GetPublicKey returns the public key in PEM format
func (s *AlpineRSASigner) GetPublicKey() ([]byte, error) {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(s.publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}

	return pem.EncodeToMemory(block), nil
}

// NewNilRSASigner returns a nil RSA signer (for unsigned repositories)
func NewNilRSASigner() RSASigner {
	return nil
}
