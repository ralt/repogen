package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

// Checksum contains various checksums for a file
type Checksum struct {
	MD5    string
	SHA1   string
	SHA256 string
	SHA512 string
	Size   int64
}

// CalculateChecksums calculates all checksums for a file in a single pass
func CalculateChecksums(path string) (*Checksum, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get file info for size
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Create all hash writers
	md5Hash := md5.New()
	sha1Hash := sha1.New()
	sha256Hash := sha256.New()
	sha512Hash := sha512.New()

	// Use MultiWriter to calculate all hashes at once
	multiWriter := io.MultiWriter(md5Hash, sha1Hash, sha256Hash, sha512Hash)

	// Stream file through all hashes
	if _, err := io.Copy(multiWriter, f); err != nil {
		return nil, err
	}

	return &Checksum{
		MD5:    hex.EncodeToString(md5Hash.Sum(nil)),
		SHA1:   hex.EncodeToString(sha1Hash.Sum(nil)),
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
		SHA512: hex.EncodeToString(sha512Hash.Sum(nil)),
		Size:   info.Size(),
	}, nil
}

// CalculateChecksum calculates a specific checksum for data
func CalculateChecksum(data []byte, hashType string) (string, error) {
	var h hash.Hash

	switch hashType {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		h = sha256.New()
	}

	h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}
