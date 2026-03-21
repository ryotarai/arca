package arcad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/machine"
)

func TestWriteGuidelineFiles_CreatesAllFiles(t *testing.T) {
	homeDir := t.TempDir()
	managed := machine.AssembleAgentGuideline("https://example.com", "", "", "")

	if err := writeGuidelineFiles(homeDir, managed); err != nil {
		t.Fatalf("writeGuidelineFiles: %v", err)
	}

	for _, target := range guidelineTargets {
		path := filepath.Join(homeDir, target.dir, target.file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected file %s to exist: %v", path, err)
			continue
		}
		if !strings.Contains(string(data), "Arca Agent Guidelines") {
			t.Errorf("file %s missing guideline content", path)
		}
	}
}

func TestWriteGuidelineFiles_ReplacesWithoutDuplicate(t *testing.T) {
	homeDir := t.TempDir()
	managed1 := machine.AssembleAgentGuideline("https://v1.example.com", "", "", "")
	managed2 := machine.AssembleAgentGuideline("https://v2.example.com", "global prompt", "", "")

	if err := writeGuidelineFiles(homeDir, managed1); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeGuidelineFiles(homeDir, managed2); err != nil {
		t.Fatalf("second write: %v", err)
	}

	for _, target := range guidelineTargets {
		path := filepath.Join(homeDir, target.dir, target.file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		if strings.Contains(content, "v1.example.com") {
			t.Errorf("file %s still contains old URL after update", path)
		}
		if !strings.Contains(content, "v2.example.com") {
			t.Errorf("file %s missing new URL", path)
		}
		if strings.Count(content, "Arca Agent Guidelines") != 1 {
			t.Errorf("file %s has duplicated guideline section", path)
		}
	}
}

func TestWriteGuidelineFiles_PreservesUserContent(t *testing.T) {
	homeDir := t.TempDir()

	// Pre-populate one file with user content
	claudeDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	userContent := "# My Custom Notes\n\nDo not remove this.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	managed := machine.AssembleAgentGuideline("https://example.com", "", "", "")
	if err := writeGuidelineFiles(homeDir, managed); err != nil {
		t.Fatalf("writeGuidelineFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "My Custom Notes") {
		t.Error("user content was removed")
	}
	if !strings.Contains(content, "Arca Agent Guidelines") {
		t.Error("managed section not appended")
	}
}
