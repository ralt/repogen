package models

import "fmt"

// ErrorType represents different categories of errors
type ErrorType int

const (
	ErrPackageParse ErrorType = iota
	ErrMetadataGen
	ErrSigning
	ErrFileOp
	ErrInvalidConfig
)

// String returns the string representation of ErrorType
func (e ErrorType) String() string {
	switch e {
	case ErrPackageParse:
		return "PackageParse"
	case ErrMetadataGen:
		return "MetadataGen"
	case ErrSigning:
		return "Signing"
	case ErrFileOp:
		return "FileOp"
	case ErrInvalidConfig:
		return "InvalidConfig"
	default:
		return "Unknown"
	}
}

// RepoGenError represents an error during repository generation
type RepoGenError struct {
	Type    ErrorType
	Package string
	Err     error
}

// Error implements the error interface
func (e *RepoGenError) Error() string {
	if e.Package != "" {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Package, e.Err)
	}
	return fmt.Sprintf("[%s] %v", e.Type, e.Err)
}

// Unwrap returns the wrapped error
func (e *RepoGenError) Unwrap() error {
	return e.Err
}
