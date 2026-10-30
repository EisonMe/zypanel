package web

import (
	"strings"
	"testing"
)

const sampleIngress = `tunnel: 23525a37-aa88-4bf8-9546-f7278ab1a732
credentials-file: /root/.cloudflared/23525a37-aa88-4bf8-9546-f7278ab1a732.json
protocol: quic
ingress:
- hostname: a.example.com
  service: http://127.0.0.1:8001
- hostname: b.example.com
  service: http://127.0.0.1:8002
- service: http_status:404
`

// Removing a rule used to drop everything before the first "- " line, which
// deleted tunnel:, credentials-file:, protocol: and ingress:. cloudflared then
// failed to parse the file and crash-looped.
func TestRemoveIngressRuleKeepsFileHeader(t *testing.T) {
	out, removed := removeIngressRule(sampleIngress, "a.example.com")
	if !removed {
		t.Fatal("expected the rule to be removed")
	}
	for _, want := range []string{
		"tunnel: 23525a37-aa88-4bf8-9546-f7278ab1a732",
		"credentials-file: /root/.cloudflared/23525a37-aa88-4bf8-9546-f7278ab1a732.json",
		"protocol: quic",
		"ingress:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("header line %q was lost:\n%s", want, out)
		}
	}
	if strings.Contains(out, "a.example.com") {
		t.Fatalf("removed rule still present:\n%s", out)
	}
	if !strings.Contains(out, "b.example.com") {
		t.Fatalf("unrelated rule was dropped:\n%s", out)
	}
	if !strings.Contains(out, "http_status:404") {
		t.Fatalf("catch-all rule was dropped:\n%s", out)
	}
}

func TestRemoveIngressRuleKeepsOtherRules(t *testing.T) {
	out, removed := removeIngressRule(sampleIngress, "b.example.com")
	if !removed {
		t.Fatal("expected the rule to be removed")
	}
	if strings.Contains(out, "b.example.com") {
		t.Fatalf("removed rule still present:\n%s", out)
	}
	if !strings.Contains(out, "a.example.com") || !strings.Contains(out, "ingress:") {
		t.Fatalf("config damaged:\n%s", out)
	}
}

func TestRemoveIngressRuleUnknownHostIsNoOp(t *testing.T) {
	out, removed := removeIngressRule(sampleIngress, "nope.example.com")
	if removed {
		t.Fatal("nothing should have been removed")
	}
	if out != sampleIngress {
		t.Fatalf("text was modified:\n%s", out)
	}
}

func TestRemoveIngressRuleOnlyFirstMatch(t *testing.T) {
	dup := sampleIngress + "- hostname: a.example.com\n  service: http://127.0.0.1:9000\n"
	out, removed := removeIngressRule(dup, "a.example.com")
	if !removed {
		t.Fatal("expected removal")
	}
	if strings.Count(out, "a.example.com") != 1 {
		t.Fatalf("expected exactly one duplicate left:\n%s", out)
	}
}

// A rule inserted with a different indentation than the rest of the file makes
// the YAML invalid, so cloudflared refuses to start.
func TestInsertIngressRuleMatchesExistingIndent(t *testing.T) {
	out, ok := insertIngressRule(sampleIngress, "c.example.com", "http://127.0.0.1:8003")
	if !ok {
		t.Fatal("expected insertion to succeed")
	}
	if !strings.Contains(out, "\n- hostname: c.example.com\n  service: http://127.0.0.1:8003\n") {
		t.Fatalf("rule was not inserted flush with the other rules:\n%s", out)
	}
	if strings.Contains(out, "  - hostname: c.example.com") {
		t.Fatalf("rule was inserted with a mismatched indent:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "- service: http_status:404") {
		t.Fatalf("catch-all is no longer last:\n%s", out)
	}
}

func TestInsertIngressRuleKeepsTwoSpaceIndent(t *testing.T) {
	indented := "tunnel: t\ncredentials-file: /c.json\ningress:\n  - hostname: a.example.com\n    service: http://127.0.0.1:1\n  - service: http_status:404\n"
	out, ok := insertIngressRule(indented, "b.example.com", "http://127.0.0.1:2")
	if !ok {
		t.Fatal("expected insertion to succeed")
	}
	if !strings.Contains(out, "  - hostname: b.example.com\n    service: http://127.0.0.1:2\n") {
		t.Fatalf("indent was not preserved:\n%s", out)
	}
}

func TestInsertIngressRuleWithoutCatchAllFails(t *testing.T) {
	if _, ok := insertIngressRule("tunnel: t\ningress:\n- hostname: a.example.com\n  service: http://127.0.0.1:1\n", "b.example.com", "http://127.0.0.1:2"); ok {
		t.Fatal("expected failure when no catch-all rule exists")
	}
}
