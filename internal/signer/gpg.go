package signer

import (
	"bytes"
	"crypto"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// GPGSigner implements Signer interface using GPG
type GPGSigner struct {
	entity  *openpgp.Entity
	keyPath string // Path to the private key file for GPG command-line operations
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

	return &GPGSigner{
		entity:  entity,
		keyPath: keyPath,
	}, nil
}

// SignCleartext creates a cleartext signature (for Debian InRelease)
func (s *GPGSigner) SignCleartext(data []byte) ([]byte, error) {
	// Use GPG command-line for cleartext signing since go-crypto's implementation
	// doesn't produce signatures that APT can verify correctly

	// Create a temporary GPG home directory
	tmpDir, err := os.MkdirTemp("", "repogen-gpg-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Import the key
	keyPath, err := filepath.Abs(s.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute key path: %w", err)
	}

	cmd := exec.Command("gpg", "--homedir", tmpDir, "--import", keyPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to import key: %w\nOutput: %s", err, output)
	}

	// Create temp file for input data
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}

	// Sign with GPG
	cmd = exec.Command("gpg", "--homedir", tmpDir, "--clearsign", "--armor",
		"--digest-algo", "SHA512", "--batch", "--yes", inputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to sign with GPG: %w\nOutput: %s", err, output)
	}

	// Read the signed output
	signedFile := inputFile + ".asc"
	signedData, err := os.ReadFile(signedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read signed file: %w", err)
	}

	return signedData, nil
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

// canonicalizeText implements RFC 4880 text canonicalization for signing
// Removes trailing whitespace from each line and uses CRLF line endings
func canonicalizeText(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	var buf bytes.Buffer

	for i, line := range lines {
		// Remove trailing spaces and tabs (but not the content)
		line = bytes.TrimRight(line, " \t\r")
		buf.Write(line)
		// Add CRLF for all lines except the last empty one
		if i < len(lines)-1 || len(line) > 0 {
			buf.WriteString("\r\n")
		}
	}

	return buf.Bytes()
}

// dashEscape performs dash-escaping required by OpenPGP cleartext signatures
// Any line starting with '-' must be prefixed with "- " (RFC 4880 section 7.1)
func dashEscape(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	var buf bytes.Buffer

	for i, line := range lines {
		// Dash-escape lines starting with '-'
		if bytes.HasPrefix(line, []byte("-")) {
			buf.WriteString("- ")
		}
		buf.Write(line)
		// Add newline except for the last line if it was empty
		if i < len(lines)-1 {
			buf.WriteString("\n")
		}
	}

	return buf.Bytes()
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
