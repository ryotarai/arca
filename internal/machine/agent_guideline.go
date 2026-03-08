package machine

import "strings"

const (
	agentGuidelineMarkerStart = "<!-- ARCA:AGENT_GUIDELINE_START -->"
	agentGuidelineMarkerEnd   = "<!-- ARCA:AGENT_GUIDELINE_END -->"
)

func agentGuidelineSection(endpointURL string) string {
	var b strings.Builder
	b.WriteString(agentGuidelineMarkerStart)
	b.WriteString("\n")
	b.WriteString("# Arca Agent Guidelines\n\n")
	b.WriteString("This section is managed by Arca and is safe to re-generate.\n\n")
	b.WriteString("- Run your application HTTP server on `:11030`.\n")
	b.WriteString("- Endpoint URL inside this machine: `" + strings.TrimSpace(endpointURL) + "`.\n")
	b.WriteString("- Requests to the endpoint URL are delivered to port `11030` on this machine.\n")
	b.WriteString("- The server process is started and supervised by `systemd`.\n")
	b.WriteString("- Visibility scope (`owner only`, `specific users`, `all arca users`, `internet public`) is configured in the arca app (server).\n")
	b.WriteString("\n")
	b.WriteString("You can add your own notes outside this managed block.\n")
	b.WriteString(agentGuidelineMarkerEnd)
	b.WriteString("\n")
	return b.String()
}

func replaceOrAppendMarkedSection(existing, managedSection string) string {
	start := strings.Index(existing, agentGuidelineMarkerStart)
	if start < 0 {
		return appendManagedSection(existing, managedSection)
	}

	searchFrom := start + len(agentGuidelineMarkerStart)
	endRel := strings.Index(existing[searchFrom:], agentGuidelineMarkerEnd)
	if endRel < 0 {
		return appendManagedSection(existing, managedSection)
	}
	end := searchFrom + endRel + len(agentGuidelineMarkerEnd)

	var b strings.Builder
	b.WriteString(existing[:start])
	b.WriteString(managedSection)
	b.WriteString(existing[end:])
	return b.String()
}

func appendManagedSection(existing, managedSection string) string {
	if strings.TrimSpace(existing) == "" {
		return managedSection
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if !strings.HasSuffix(existing, "\n\n") {
		existing += "\n"
	}
	return existing + managedSection
}
