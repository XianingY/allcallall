package collaboration

import "unicode/utf8"

// SearchIndexPolicy 控制消息被推入搜索索引时正文的最小化程度。
// 搜索服务通常运行在信任边界之外（第三方 ES / 托管检索），不应长期持有完整消息正文。
// 因此默认只索引一条短摘要（snippet）+ 元数据，完整内容按需从消息库回取。
// 这直接对应《个人信息保护法》第六条「收集、使用个人信息应当最小化」。
// SearchIndexPolicy controls how much of a message body the search indexer ever sees.
type SearchIndexPolicy struct {
	// Enabled 为 false 时完全不向索引推送正文，仅保留元数据（最严格）。
	// 开启时按 BodySnippetMaxRunes 截断索引一条摘要。
	Enabled bool
	// BodySnippetMaxRunes 索引摘要的最大字符数（按 rune 计）。<=0 视为只索引元数据。
	BodySnippetMaxRunes int
}

// DefaultSearchIndexPolicy 返回隐私优先的默认策略：最小化开启（Enabled=true），
// 索引 64 字符摘要。与留存/撤回默认关闭不同——搜索最小化不触及客户端渲染兼容，
// 因此默认就应当开启，避免索引服务在配置缺失时持有完整正文。
// DefaultSearchIndexPolicy is the privacy-first default: minimization on, 64-rune snippet.
func DefaultSearchIndexPolicy() SearchIndexPolicy {
	return SearchIndexPolicy{Enabled: true, BodySnippetMaxRunes: 64}
}

// Normalized 返回补齐默认值后的策略副本。
// 只修复摘要长度下限，绝不改动 Enabled——显式关闭（如 config 中 enabled:false）
// 必须被如实保留，不能因为归一化而被悄悄重新打开。
// Normalized returns a copy with defaults applied; it never flips Enabled.
func (p SearchIndexPolicy) Normalized() SearchIndexPolicy {
	if p.BodySnippetMaxRunes <= 0 {
		p.BodySnippetMaxRunes = 64
	}
	return p
}

// IndexBody 将正文转换为应推入索引的内容：在 Enabled 时返回截断摘要，
// 否则返回空串（索引只保留元数据）。摘要过短不补点；被截断的语义由调用方（如高亮）处理。
// 保留输入原文可能含多字节字符，按 rune 截断以避免切断 UTF-8 序列。
// IndexBody reduces a body to what the search indexer may store.
func (p SearchIndexPolicy) IndexBody(body string) string {
	if !p.Enabled {
		return ""
	}
	runes := []rune(body)
	if len(runes) <= p.BodySnippetMaxRunes {
		return string(runes)
	}
	return string(runes[:p.BodySnippetMaxRunes])
}

// BodyLength 返回正文的字符数（按 rune 计），作为索引中的元数据信号，
// 让检索层在不持有内容的前提下仍能感知「该消息有 N 字内容」。
// BodyLength reports the rune count of a body for metadata-only indexing.
func (p SearchIndexPolicy) BodyLength(body string) int {
	return utf8.RuneCountInString(body)
}

// WithSearchIndexPolicy 注入搜索索引最小化策略（由 runtime 依据 config 装配）。
// WithSearchIndexPolicy injects the search index minimization policy.
func (s *Service) WithSearchIndexPolicy(policy SearchIndexPolicy) *Service {
	s.searchIndex = policy.Normalized()
	s.searchIndex.Enabled = policy.Enabled
	return s
}
