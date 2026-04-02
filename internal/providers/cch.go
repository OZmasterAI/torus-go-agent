package providers

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/bits"

	t "torus_go_agent/internal/types"
)

// CCH constants matching Claude Code's client attestation.
const (
	cchSeed        uint64 = 0x6E52736AC806831E
	cchMask        uint64 = 0xFFFFF // 20-bit → 5 hex chars
	cchPlaceholder        = "cch=00000"
	cchVersion            = "2.1.87"
	fpSalt                = "59cf53e54c78"
)

// ── xxHash64 (seeded) ──────────────────────────────────────────────────────

const (
	xxp1 uint64 = 0x9E3779B185EBCA87
	xxp2 uint64 = 0xC2B2AE3D27D4EB4F
	xxp3 uint64 = 0x165667B19E3779F9
	xxp4 uint64 = 0x85EBCA77C2B2AE63
	xxp5 uint64 = 0x27D4EB2F165667C5
)

func xxRound(v, input uint64) uint64 {
	v += input * xxp2
	v = bits.RotateLeft64(v, 31)
	v *= xxp1
	return v
}

func xxMergeRound(h, v uint64) uint64 {
	v = xxRound(0, v)
	h ^= v
	h = h*xxp1 + xxp4
	return h
}

// xxhash64 computes XXH64 with a custom seed per the xxHash specification.
func xxhash64(data []byte, seed uint64) uint64 {
	n := len(data)
	var h uint64

	p := 0
	if n >= 32 {
		v1 := seed + xxp1 + xxp2
		v2 := seed + xxp2
		v3 := seed
		v4 := seed - xxp1

		for p+32 <= n {
			v1 = xxRound(v1, binary.LittleEndian.Uint64(data[p:]))
			v2 = xxRound(v2, binary.LittleEndian.Uint64(data[p+8:]))
			v3 = xxRound(v3, binary.LittleEndian.Uint64(data[p+16:]))
			v4 = xxRound(v4, binary.LittleEndian.Uint64(data[p+24:]))
			p += 32
		}

		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = xxMergeRound(h, v1)
		h = xxMergeRound(h, v2)
		h = xxMergeRound(h, v3)
		h = xxMergeRound(h, v4)
	} else {
		h = seed + xxp5
	}

	h += uint64(n)

	// Remaining 8-byte chunks.
	for p+8 <= n {
		k1 := xxRound(0, binary.LittleEndian.Uint64(data[p:]))
		h ^= k1
		h = bits.RotateLeft64(h, 27)*xxp1 + xxp4
		p += 8
	}

	// Remaining 4-byte chunk.
	if p+4 <= n {
		h ^= uint64(binary.LittleEndian.Uint32(data[p:])) * xxp1
		h = bits.RotateLeft64(h, 23)*xxp2 + xxp3
		p += 4
	}

	// Remaining bytes.
	for p < n {
		h ^= uint64(data[p]) * xxp5
		h = bits.RotateLeft64(h, 11) * xxp1
		p++
	}

	// Avalanche.
	h ^= h >> 33
	h *= xxp2
	h ^= h >> 29
	h *= xxp3
	h ^= h >> 32

	return h
}

// ── Fingerprint ────────────────────────────────────────────────────────────

// extractFirstUserText returns the text content of the first user message.
func extractFirstUserText(messages []t.Message) string {
	for _, m := range messages {
		if m.Role == t.RoleUser {
			for _, b := range m.Content {
				if b.Type == "text" {
					return b.Text
				}
			}
			return ""
		}
	}
	return ""
}

// computeFingerprint returns a 3-char hex fingerprint.
// Algorithm: SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
func computeFingerprint(messages []t.Message, version string) string {
	msg := extractFirstUserText(messages)
	indices := [3]int{4, 7, 20}
	var chars [3]byte
	for i, idx := range indices {
		if idx < len(msg) {
			chars[i] = msg[idx]
		} else {
			chars[i] = '0'
		}
	}
	input := fpSalt + string(chars[:]) + version
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:])[:3]
}

// ── Attribution header ─────────────────────────────────────────────────────

// buildAttributionHeader returns the billing header string with cch=00000 placeholder.
func buildAttributionHeader(fingerprint string) string {
	version := cchVersion + "." + fingerprint
	return fmt.Sprintf(
		"x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; %s;",
		version, cchPlaceholder,
	)
}

// ── CCH application ────────────────────────────────────────────────────────

// applyCCH computes xxHash64 over the serialized body and replaces the cch=00000 placeholder.
func applyCCH(body []byte) []byte {
	if !bytes.Contains(body, []byte(cchPlaceholder)) {
		return body
	}
	hash := xxhash64(body, cchSeed)
	cch := fmt.Sprintf("cch=%05x", hash&cchMask)
	return bytes.Replace(body, []byte(cchPlaceholder), []byte(cch), 1)
}
