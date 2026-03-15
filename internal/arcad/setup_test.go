package arcad

import (
	"testing"
)

func TestBuildAgentGuidelineSection(t *testing.T) {
	section := buildAgentGuidelineSection("http://localhost:11030")

	if len(section) == 0 {
		t.Fatal("empty section")
	}
	if section[:len(guidelineMarkerStart)] != guidelineMarkerStart {
		t.Fatalf("section does not start with marker")
	}
	if section[len(section)-len(guidelineMarkerEnd)-1:len(section)-1] != guidelineMarkerEnd {
		t.Fatalf("section does not end with marker")
	}
	if !contains(section, "http://localhost:11030") {
		t.Fatalf("section does not contain endpoint URL")
	}
}

func TestReplaceOrAppendGuideline_Empty(t *testing.T) {
	section := buildAgentGuidelineSection("http://localhost:11030")
	result := replaceOrAppendGuideline("", section)
	if result != section {
		t.Fatalf("expected section only, got: %q", result)
	}
}

func TestReplaceOrAppendGuideline_Append(t *testing.T) {
	existing := "# My Notes\n\nSome content here.\n"
	section := buildAgentGuidelineSection("http://localhost:11030")
	result := replaceOrAppendGuideline(existing, section)

	if !contains(result, "# My Notes") {
		t.Fatalf("lost existing content")
	}
	if !contains(result, guidelineMarkerStart) {
		t.Fatalf("missing managed section")
	}
}

func TestReplaceOrAppendGuideline_Replace(t *testing.T) {
	oldSection := guidelineMarkerStart + "\nold content\n" + guidelineMarkerEnd
	existing := "# My Notes\n\n" + oldSection + "\n\n# After\n"
	section := buildAgentGuidelineSection("http://localhost:11030")
	result := replaceOrAppendGuideline(existing, section)

	if contains(result, "old content") {
		t.Fatalf("old managed content should be replaced")
	}
	if !contains(result, "# My Notes") {
		t.Fatalf("lost prefix content")
	}
	if !contains(result, "# After") {
		t.Fatalf("lost suffix content")
	}
	if !contains(result, "http://localhost:11030") {
		t.Fatalf("new section not included")
	}
}

func TestSetupConfigFromEnv_Defaults(t *testing.T) {
	cfg := SetupConfigFromEnv()
	if cfg.DaemonUser != "arcad" {
		t.Fatalf("DaemonUser = %q, want %q", cfg.DaemonUser, "arcad")
	}
	if cfg.InteractiveUser != "arcauser" {
		t.Fatalf("InteractiveUser = %q, want %q", cfg.InteractiveUser, "arcauser")
	}
	if cfg.ShelleyPort != "21032" {
		t.Fatalf("ShelleyPort = %q, want %q", cfg.ShelleyPort, "21032")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
