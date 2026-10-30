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
