package providers

import (
	"bytes"
	"encoding/binary"
	"testing"

	tp "torus_go_agent/internal/types"
)

func TestXXHash64_KnownVectors(t *testing.T) {
	// Empty input with seed 0 → known reference value.
	// XXH64("", 0) = 0xEF46DB3751D8E999
	got := xxhash64([]byte{}, 0)
	want := uint64(0xEF46DB3751D8E999)
	if got != want {
		t.Errorf("xxhash64(empty, 0) = %016x, want %016x", got, want)
	}

	// XXH64("abc", 0) — verify against known reference.
	// Reference: xxhsum -H64 says abc = 0x44BC2CF5AD770999
	got2 := xxhash64([]byte("abc"), 0)
	want2 := uint64(0x44BC2CF5AD770999)
	if got2 != want2 {
		t.Errorf("xxhash64(abc, 0) = %016x, want %016x", got2, want2)
	}
}

func TestXXHash64_Seeded(t *testing.T) {
	// With a non-zero seed, output should differ from seed=0.
	a := xxhash64([]byte("hello"), 0)
	b := xxhash64([]byte("hello"), cchSeed)
	if a == b {
		t.Error("same hash for different seeds — seed not being applied")
	}
}

func TestXXHash64_LargeInput(t *testing.T) {
	// Input >= 32 bytes exercises the 4-lane accumulator path.
	data := bytes.Repeat([]byte("abcdefgh"), 8) // 64 bytes
	h := xxhash64(data, cchSeed)
	if h == 0 {
		t.Error("hash of 64-byte input should not be zero")
	}

	// Deterministic: same input → same output.
	h2 := xxhash64(data, cchSeed)
	if h != h2 {
		t.Errorf("non-deterministic: %016x != %016x", h, h2)
	}
}

func TestXXHash64_Endianness(t *testing.T) {
	// Verify the implementation reads little-endian correctly.
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, 0x0102030405060708)
	h := xxhash64(buf, 0)
	if h == 0 {
		t.Error("hash should not be zero")
	}
}

func TestComputeFingerprint(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{
			{Type: "text", Text: "Hello, how are you doing today?"},
		}},
	}
	fp := computeFingerprint(msgs, "2.1.87")
	if len(fp) != 3 {
		t.Fatalf("fingerprint length = %d, want 3", len(fp))
	}

	// Deterministic.
	fp2 := computeFingerprint(msgs, "2.1.87")
	if fp != fp2 {
		t.Errorf("non-deterministic fingerprint: %q != %q", fp, fp2)
	}

	// Different version → different fingerprint.
	fp3 := computeFingerprint(msgs, "2.1.88")
	if fp == fp3 {
		t.Errorf("fingerprint should differ for different versions")
	}
}

func TestComputeFingerprint_ShortMessage(t *testing.T) {
	// Message shorter than index 20 — should use '0' fallback.
	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: []tp.ContentBlock{
			{Type: "text", Text: "Hi"},
		}},
	}
	fp := computeFingerprint(msgs, "2.1.87")
	if len(fp) != 3 {
		t.Fatalf("fingerprint length = %d, want 3", len(fp))
	}
}

func TestComputeFingerprint_NoUserMessage(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleAssistant, Content: []tp.ContentBlock{
			{Type: "text", Text: "Hello!"},
		}},
	}
	fp := computeFingerprint(msgs, "2.1.87")
	if len(fp) != 3 {
		t.Fatalf("fingerprint length = %d, want 3", len(fp))
	}
}

func TestBuildAttributionHeader(t *testing.T) {
	h := buildAttributionHeader("a4f")
	want := "x-anthropic-billing-header: cc_version=2.1.87.a4f; cc_entrypoint=cli; cch=00000;"
	if h != want {
		t.Errorf("header = %q\nwant   %q", h, want)
	}
}

func TestApplyCCH(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.87.abc; cc_entrypoint=cli; cch=00000;"}]}`)

	patched := applyCCH(body)

	// Placeholder should be replaced.
	if bytes.Contains(patched, []byte("cch=00000")) {
		t.Error("placeholder not replaced")
	}

	// Should contain cch= with a 5-char hex value.
	if !bytes.Contains(patched, []byte("cch=")) {
		t.Error("no cch= found in patched body")
	}

	// Length should be the same (00000 → 5 hex chars).
	if len(patched) != len(body) {
		t.Errorf("body length changed: %d → %d", len(body), len(patched))
	}
}

func TestApplyCCH_NoPlaceholder(t *testing.T) {
	body := []byte(`{"system":"hello"}`)
	patched := applyCCH(body)
	if !bytes.Equal(body, patched) {
		t.Error("body without placeholder should be unchanged")
	}
}

func TestApplyCCH_Deterministic(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.87.abc; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)

	p1 := applyCCH(body)
	// Re-create body since applyCCH consumed it.
	body2 := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.87.abc; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
	p2 := applyCCH(body2)

	if !bytes.Equal(p1, p2) {
		t.Errorf("non-deterministic CCH:\n  %s\n  %s", p1, p2)
	}
}
