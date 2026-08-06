package collaboration

import "testing"

func TestSearchIndexPolicyDefaultIsPrivacyFirst(t *testing.T) {
	// 默认策略必须隐私优先：开启最小化并给 64 字符摘要上限。
	// The default policy must be privacy-first: minimization on with a 64-rune snippet cap.
	got := DefaultSearchIndexPolicy()
	if !got.Enabled {
		t.Fatal("default policy must be enabled (privacy-first)")
	}
	if got.BodySnippetMaxRunes != 64 {
		t.Fatalf("default snippet cap = %d, want 64", got.BodySnippetMaxRunes)
	}
}

func TestSearchIndexPolicyNormalizedPreservesEnabled(t *testing.T) {
	// Normalized 只能修复摘要长度，绝不改动 Enabled，避免显式关闭被悄悄重新打开。
	// Normalized must never flip Enabled, so an explicit disable is preserved.
	disabled := SearchIndexPolicy{Enabled: false}.Normalized()
	if disabled.Enabled {
		t.Fatal("Normalized must not enable an explicitly disabled policy")
	}
	// 显式关闭但给了摘要长度时，长度应被修正而 Enabled 保持 false。
	// An explicit disable with a cap should keep the cap repaired but stay disabled.
	disabledWithCap := SearchIndexPolicy{Enabled: false, BodySnippetMaxRunes: 0}.Normalized()
	if disabledWithCap.Enabled || disabledWithCap.BodySnippetMaxRunes != 64 {
		t.Fatalf("got %+v, want Enabled=false with cap 64", disabledWithCap)
	}
}

func TestSearchIndexPolicyIndexBodyTruncates(t *testing.T) {
	policy := SearchIndexPolicy{Enabled: true, BodySnippetMaxRunes: 5}.Normalized()
	// 中文按 rune 截断，避免切断 UTF-8 多字节序列。
	// Truncate by runes so multibyte sequences are never split.
	body := "一二三四五六七八九"
	got := policy.IndexBody(body)
	if got != "一二三四五" {
		t.Fatalf("IndexBody = %q, want %q", got, "一二三四五")
	}
	if len([]rune(got)) != 5 {
		t.Fatalf("snippet rune length = %d, want 5", len([]rune(got)))
	}
}

func TestSearchIndexPolicyIndexBodyShortPassthrough(t *testing.T) {
	policy := SearchIndexPolicy{Enabled: true, BodySnippetMaxRunes: 64}.Normalized()
	body := "short message"
	if got := policy.IndexBody(body); got != body {
		t.Fatalf("short body must pass through unchanged, got %q", got)
	}
}

func TestSearchIndexPolicyDisabledDropsBody(t *testing.T) {
	policy := SearchIndexPolicy{Enabled: false}.Normalized()
	// 关闭时索引完全不持有正文，只保留元数据。
	// Disabled: the indexer stores no body at all, only metadata.
	if got := policy.IndexBody("任何内容都不应进入索引"); got != "" {
		t.Fatalf("disabled policy must not index any body, got %q", got)
	}
}

func TestSearchIndexPolicyBodyLengthCountsRunes(t *testing.T) {
	policy := SearchIndexPolicy{Enabled: true}.Normalized()
	// 字节数与字符数不同：9 个汉字 = 27 字节，但 9 个字符。
	// Rune count differs from byte count for multibyte text.
	body := "一二三四五六七八九"
	if got := policy.BodyLength(body); got != 9 {
		t.Fatalf("BodyLength = %d, want 9", got)
	}
	if got := policy.BodyLength("hello"); got != 5 {
		t.Fatalf("BodyLength = %d, want 5", got)
	}
}
