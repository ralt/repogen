package homebrew

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralt/repogen/internal/models"
)

// ParseExistingMetadata reads Formula/*.rb files
func (g *Generator) ParseExistingMetadata(config *models.RepositoryConfig) ([]models.Package, error) {
	formulaDir := filepath.Join(config.OutputDir, "Formula")

	formulaFiles, err := filepath.Glob(filepath.Join(formulaDir, "*.rb"))
	if err != nil {
		return nil, err
	}

	if len(formulaFiles) == 0 {
		return nil, fmt.Errorf("no existing Homebrew metadata found in %s", config.OutputDir)
	}

	var packages []models.Package
	for _, formulaPath := range formulaFiles {
		pkgs, err := parseFormula(formulaPath)
		if err != nil {
			continue
		}
		packages = append(packages, pkgs...)
	}

	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages found in Formula files")
	}

	return packages, nil
}

func parseFormula(path string) ([]models.Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var packages []models.Package

	// Regex patterns
	versionRe := regexp.MustCompile(`version\s+"([^"]+)"`)
	descRe := regexp.MustCompile(`desc\s+"([^"]+)"`)
	homepageRe := regexp.MustCompile(`homepage\s+"([^"]+)"`)
	urlRe := regexp.MustCompile(`url\s+"([^"]+)"`)
	sha256Re := regexp.MustCompile(`sha256\s+"([^"]+)"`)

	scanner := bufio.NewScanner(f)
	var version, desc, homepage, url, sha256 string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if matches := versionRe.FindStringSubmatch(line); len(matches) > 1 {
			version = matches[1]
		}
		if matches := descRe.FindStringSubmatch(line); len(matches) > 1 {
			desc = matches[1]
		}
		if matches := homepageRe.FindStringSubmatch(line); len(matches) > 1 {
			homepage = matches[1]
		}
		if matches := urlRe.FindStringSubmatch(line); len(matches) > 1 {
			url = matches[1]
		}
		if matches := sha256Re.FindStringSubmatch(line); len(matches) > 1 {
			sha256 = matches[1]

			// URL + SHA256 = one package/bottle
			if url != "" {
				pkg := models.Package{
					Name:        extractPackageNameFromURL(url),
					Version:     version,
					Description: desc,
					Homepage:    homepage,
					Filename:    url,
					SHA256Sum:   sha256,
					Metadata:    make(map[string]interface{}),
				}
				packages = append(packages, pkg)

				// Reset for next bottle
				url = ""
				sha256 = ""
			}
		}
	}

	return packages, scanner.Err()
}

func extractPackageNameFromURL(url string) string {
	// Extract from URL like "bottles/package--1.0.0.platform.bottle.tar.gz"
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return ""
	}
	filename := parts[len(parts)-1]

	// Remove .bottle.tar.gz
	filename = strings.TrimSuffix(filename, ".bottle.tar.gz")
	filename = strings.TrimSuffix(filename, ".bottle.tar")

	// Split by --
	nameParts := strings.Split(filename, "--")
	if len(nameParts) > 0 {
		return nameParts[0]
	}
	return filename
}
