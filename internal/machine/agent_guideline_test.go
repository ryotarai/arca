package machine

import (
	"strings"
	"testing"
)

func TestReplaceOrAppendMarkedSection_InitialGeneration(t *testing.T) {
	t.Parallel()

	managed := agentGuidelineSection("http://localhost:11030")
	got := ReplaceOrAppendMarkedSection("", managed)
	if got != managed {
		t.Fatalf("initial generation mismatch")
	}
	if !strings.Contains(got, "Run your application HTTP server on `:11030`.") {
		t.Fatalf("managed section must include listen requirement")
	}
	if !strings.Contains(got, "Endpoint URL inside this machine: `http://localhost:11030`.") {
		t.Fatalf("managed section must include endpoint URL")
	}
	if !strings.Contains(got, "Requests to the endpoint URL are delivered to port `11030` on this machine.") {
		t.Fatalf("managed section must mention endpoint requests reach port 11030")
	}
	if !strings.Contains(got, "supervised by `systemd`") {
		t.Fatalf("managed section must include systemd note")
	}
	if !strings.Contains(got, "Visibility scope (`owner only`, `specific users`, `all arca users`, `internet public`) is configured in the arca app (server).") {
		t.Fatalf("managed section must include visibility scope configuration note")
	}
}

func TestReplaceOrAppendMarkedSection_Regeneration(t *testing.T) {
	t.Parallel()

	old := agentGuidelineSection("http://localhost:11030")
	existing := "prefix\n\n" + old + "\nuser notes\n"
	newSection := agentGuidelineSection("http://localhost:8081")

	got := ReplaceOrAppendMarkedSection(existing, newSection)

	if !strings.Contains(got, "prefix\n\n") {
		t.Fatalf("prefix content was not preserved")
	}
	if !strings.Contains(got, "\nuser notes\n") {
		t.Fatalf("suffix content was not preserved")
	}
	if strings.Contains(got, "`http://localhost:11030`") {
		t.Fatalf("old managed section remained")
	}
	if !strings.Contains(got, "`http://localhost:8081`") {
		t.Fatalf("new managed section was not inserted")
	}
	if strings.Count(got, agentGuidelineMarkerStart) != 1 {
		t.Fatalf("expected exactly one start marker, got %d", strings.Count(got, agentGuidelineMarkerStart))
	}
	if strings.Count(got, agentGuidelineMarkerEnd) != 1 {
		t.Fatalf("expected exactly one end marker, got %d", strings.Count(got, agentGuidelineMarkerEnd))
	}
}

func TestReplaceOrAppendMarkedSection_RegenerationAfterManualEditOutsideMarkers(t *testing.T) {
	t.Parallel()

	managed := agentGuidelineSection("http://localhost:11030")
	existing := "my custom intro\n" + managed + "my custom footer\n"
	newSection := agentGuidelineSection("http://localhost:18080")

	got := ReplaceOrAppendMarkedSection(existing, newSection)

	if !strings.Contains(got, "my custom intro\n") {
		t.Fatalf("manual edit before markers was not preserved")
	}
	if !strings.Contains(got, "my custom footer\n") {
		t.Fatalf("manual edit after markers was not preserved")
	}
	if !strings.Contains(got, "`http://localhost:18080`") {
		t.Fatalf("managed section was not updated")
	}
}

func TestReplaceOrAppendMarkedSection_AppendsWhenMarkersMissing(t *testing.T) {
	t.Parallel()

	newSection := agentGuidelineSection("http://localhost:11030")
	existing := "legacy content without markers"

	got := ReplaceOrAppendMarkedSection(existing, newSection)

	if !strings.HasPrefix(got, existing) {
		t.Fatalf("existing content must be preserved when markers are missing")
	}
	if !strings.Contains(got, agentGuidelineMarkerStart) || !strings.Contains(got, agentGuidelineMarkerEnd) {
		t.Fatalf("managed section was not appended when markers are missing")
	}
}

func TestAssembleAgentGuideline_EmptyPrompts(t *testing.T) {
	t.Parallel()

	got := AssembleAgentGuideline("https://myvm.example.com", "", "", "")

	if !strings.Contains(got, agentGuidelineMarkerStart) {
		t.Fatal("missing start marker")
	}
	if !strings.Contains(got, agentGuidelineMarkerEnd) {
		t.Fatal("missing end marker")
	}
	if !strings.Contains(got, "Run your application HTTP server on `:11030`.") {
		t.Fatal("missing hardcoded listen instruction")
	}
	if !strings.Contains(got, "`https://myvm.example.com`") {
		t.Fatal("missing endpoint URL")
	}
	if !strings.Contains(got, "supervised by `systemd`") {
		t.Fatal("missing systemd note")
	}
}

func TestAssembleAgentGuideline_AllPromptsSet(t *testing.T) {
	t.Parallel()

	got := AssembleAgentGuideline(
		"https://myvm.example.com",
		"Global: use Go 1.22",
		"Template: always run tests",
		"User: prefer short functions",
	)

	if !strings.Contains(got, "Global: use Go 1.22") {
		t.Fatal("missing global prompt")
	}
	if !strings.Contains(got, "Template: always run tests") {
		t.Fatal("missing template prompt")
	}
	if !strings.Contains(got, "User: prefer short functions") {
		t.Fatal("missing user prompt")
	}
	// Verify order: global before template before user
	gi := strings.Index(got, "Global: use Go 1.22")
	ti := strings.Index(got, "Template: always run tests")
	ui := strings.Index(got, "User: prefer short functions")
	if gi >= ti || ti >= ui {
		t.Fatalf("prompts not in expected order: global=%d, template=%d, user=%d", gi, ti, ui)
	}
}

func TestAssembleAgentGuideline_PartialPrompts(t *testing.T) {
	t.Parallel()

	got := AssembleAgentGuideline("https://myvm.example.com", "", "Template only", "")

	if !strings.Contains(got, "Template only") {
		t.Fatal("missing template prompt")
	}
	// Whitespace-only prompts should not appear
	got2 := AssembleAgentGuideline("https://myvm.example.com", "  ", "", "  \n  ")
	if strings.Contains(got2, "  \n  ") {
		t.Fatal("whitespace-only prompt should not appear in output")
	}
}

func TestAssembleAgentGuideline_MarkersPresent(t *testing.T) {
	t.Parallel()

	got := AssembleAgentGuideline("http://x", "g", "t", "u")

	if strings.Count(got, agentGuidelineMarkerStart) != 1 {
		t.Fatalf("expected exactly one start marker, got %d", strings.Count(got, agentGuidelineMarkerStart))
	}
	if strings.Count(got, agentGuidelineMarkerEnd) != 1 {
		t.Fatalf("expected exactly one end marker, got %d", strings.Count(got, agentGuidelineMarkerEnd))
	}
	if !strings.HasPrefix(got, agentGuidelineMarkerStart) {
		t.Fatal("output should start with start marker")
	}
	if !strings.HasSuffix(got, agentGuidelineMarkerEnd+"\n") {
		t.Fatal("output should end with end marker followed by newline")
	}
}
