package repositorycontract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type projectDocument struct {
	Path   string
	SHA256 string
}

var projectDocuments = []projectDocument{
	{Path: "CONTRIBUTING.md", SHA256: "3e5e02b9b93d6ce486abf9895bc39fe85c59daab7eb334a560bf6a42cd3d85a9"},
	{Path: "DEVELOPMENT.md", SHA256: "396fea31b2765cb8ebe60a52a61e082f8f3fe9220d64e2ca49922c6d38f1466c"},
	{Path: "docs/architecture/00-system-boundary.md", SHA256: "89346b7ac7a96da6e17d7169b177799449f49b58f5046f87481d6243af0a3dbf"},
	{Path: "docs/cli/configuration.md", SHA256: "2de1d6000446a2ac16627c40846d53cc219685e04d24781437bee292505408f9"},
	{Path: "docs/desktop/README.md", SHA256: "b0ea67160e1fa4ca6fb9fbb64bf69a97fc20e53139dc3cf951efac630bdf04c0"},
	{Path: "docs/engineering/05-testing-standard.md", SHA256: "21d2b4f6cb1e8d07308527d195724521fc16182f726f251e5de83d92f7892251"},
	{Path: "docs/engineering/06-dependency-and-change-policy.md", SHA256: "871a18dbbabf77790f542ce6eeeed1491915bc48293556648c2924efd5442a80"},
	{Path: "docs/engineering/08-threat-model.md", SHA256: "789e7f8ebef4a9c02c2faf92744144e1ed9aa8187758e9400059c5163569d3cd"},
	{Path: "docs/engineering/09-data-integrity.md", SHA256: "bbe12ba5b2e68e1f211d936b6bb45198bbad1499851996c03db26e9c1a061717"},
}

var genericScaffoldMarkers = []string{
	"This document captures the durable design contract",
	"It is intentionally a scaffold",
	"Product decision: UNDECIDED",
	"Technical owner: UNASSIGNED",
	"Related ADR: UNDECIDED",
	"Operability standard connects code changes",
}

type scaffoldManifest struct {
	Files []scaffoldManifestFile `json:"files"`
}

type scaffoldManifestFile struct {
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Ownership         string `json:"ownership"`
	Status            string `json:"status"`
	GeneratedChecksum string `json:"generatedChecksum"`
}

func TestProjectDocumentsMatchApprovedContracts(t *testing.T) {
	root := repositoryRoot(t)
	manifest := readScaffoldManifest(t, root)
	registered := make(map[string]scaffoldManifestFile, len(manifest.Files))
	for _, entry := range manifest.Files {
		registered[entry.Path] = entry
	}

	for _, document := range projectDocuments {
		document := document
		t.Run(document.Path, func(t *testing.T) {
			entry, ok := registered[document.Path]
			if !ok {
				t.Fatalf("project document is missing from .ssealed/manifest.json")
			}
			if entry.Kind != "document" || entry.Ownership != "seeded" || entry.Status != "active" {
				t.Fatalf(
					"unexpected scaffold registration: kind=%q ownership=%q status=%q",
					entry.Kind,
					entry.Ownership,
					entry.Status,
				)
			}

			content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(document.Path)))
			if err != nil {
				t.Fatal(err)
			}
			sum := fmt.Sprintf("%x", sha256.Sum256(content))
			if sum != document.SHA256 {
				t.Fatalf("approved content drift: got %s want %s", sum, document.SHA256)
			}
			if entry.GeneratedChecksum == "sha256:"+sum {
				t.Fatalf("project document still matches the generic generated scaffold")
			}

			text := string(content)
			for _, marker := range genericScaffoldMarkers {
				if strings.Contains(text, marker) {
					t.Fatalf("generic scaffold marker remains: %q", marker)
				}
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func readScaffoldManifest(t *testing.T, root string) scaffoldManifest {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, ".ssealed", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest scaffoldManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode .ssealed/manifest.json: %v", err)
	}
	return manifest
}
