package signer

// Signer interface for signing repository metadata
type Signer interface {
	// SignCleartext creates a cleartext signature (for Debian InRelease)
	SignCleartext(data []byte) ([]byte, error)

	// SignDetached creates a detached signature (for Release.gpg, repomd.xml.asc)
	SignDetached(data []byte) ([]byte, error)

	// GetPublicKey returns the public key
	GetPublicKey() ([]byte, error)
}

// RSASigner interface for RSA signing (Alpine APK)
type RSASigner interface {
	// SignRSA creates an RSA PKCS1v15 signature
	SignRSA(data []byte) ([]byte, error)

	// GetPublicKey returns the public key
	GetPublicKey() ([]byte, error)
}
