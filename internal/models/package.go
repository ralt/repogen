package models

// Package represents a software package with its metadata
type Package struct {
	// Core metadata
	Name         string
	Version      string
	Architecture string
	Description  string
	Maintainer   string
	Homepage     string
	License      string
	Dependencies []string
	Conflicts    []string
	Groups       []string

	// File information
	Filename  string
	Size      int64
	MD5Sum    string
	SHA1Sum   string
	SHA256Sum string
	SHA512Sum string

	// Type-specific metadata
	Metadata map[string]interface{}
}
