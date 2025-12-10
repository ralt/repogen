package signer

import (
	"bytes"
	"crypto"
	"fmt"
	"os"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// GPGSigner implements Signer interface using GPG
type GPGSigner struct {
	entity *openpgp.Entity
}

// NewGPGSigner creates a new GPG signer from a private key file
func NewGPGSigner(keyPath, passphrase string) (*GPGSigner, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("key path is empty")
	}

	// Read private key file
	keyFile, err := os.Open(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}
	defer keyFile.Close()

	// Try to parse as armored key first
	entityList, err := openpgp.ReadArmoredKeyRing(keyFile)
	if err != nil {
		// Try as binary key
		keyFile.Seek(0, 0)
		entityList, err = openpgp.ReadKeyRing(keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read key: %w", err)
		}
	}

	if len(entityList) == 0 {
		return nil, fmt.Errorf("no keys found in key file")
	}

	entity := entityList[0]

	// Decrypt private key if passphrase provided
	if passphrase != "" {
		if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
			err = entity.PrivateKey.Decrypt([]byte(passphrase))
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt private key: %w", err)
			}
		}

		// Decrypt subkeys as well
		for _, subkey := range entity.Subkeys {
			if subkey.PrivateKey != nil && subkey.PrivateKey.Encrypted {
				err = subkey.PrivateKey.Decrypt([]byte(passphrase))
				if err != nil {
					return nil, fmt.Errorf("failed to decrypt subkey: %w", err)
				}
			}
		}
	}

	return &GPGSigner{entity: entity}, nil
}

// SignCleartext creates a cleartext signature (for Debian InRelease)
func (s *GPGSigner) SignCleartext(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	// Create armored writer for cleartext signature
	w, err := armor.Encode(&buf, "PGP SIGNED MESSAGE", map[string]string{"Hash": "SHA512"})
	if err != nil {
		return nil, err
	}

	// Write the data
	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	// Now sign it
	var sigBuf bytes.Buffer
	err = openpgp.ArmoredDetachSignText(&sigBuf, s.entity, bytes.NewReader(data), &packet.Config{
		DefaultHash: crypto.SHA512,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Combine into cleartext signature format
	return createCleartextSignature(data, sigBuf.Bytes()), nil
}

// SignDetached creates a detached signature (for Release.gpg, repomd.xml.asc)
func (s *GPGSigner) SignDetached(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	err := openpgp.ArmoredDetachSign(&buf, s.entity, bytes.NewReader(data), &packet.Config{
		DefaultHash: crypto.SHA512,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create detached signature: %w", err)
	}

	return buf.Bytes(), nil
}

// GetPublicKey returns the public key in armored format
func (s *GPGSigner) GetPublicKey() ([]byte, error) {
	var buf bytes.Buffer

	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, err
	}

	err = s.entity.Serialize(w)
	if err != nil {
		w.Close()
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// createCleartextSignature creates a PGP cleartext signature format
func createCleartextSignature(message, signature []byte) []byte {
	var buf bytes.Buffer

	buf.WriteString("-----BEGIN PGP SIGNED MESSAGE-----\n")
	buf.WriteString("Hash: SHA512\n")
	buf.WriteString("\n")
	buf.Write(message)
	if !bytes.HasSuffix(message, []byte("\n")) {
		buf.WriteString("\n")
	}
	buf.Write(signature)

	return buf.Bytes()
}

// NewNilSigner returns a nil signer (for unsigned repositories)
func NewNilSigner() Signer {
	return nil
}
