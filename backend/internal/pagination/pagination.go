// Package pagination 提供面向用户列表端点的统一分页抽象，作为仓库层
// "ListWithPaging" 的统一入口，避免各列表查询各自裸写 .Find(&...) 而无
// Limit/Offset，导致数据增长后成为慢查询与 OOM 源。
package pagination

import (
	"gorm.io/gorm"
)

const (
	// DefaultLimit 面向用户列表端点的默认每页大小。
	DefaultLimit = 50
	// MaxLimit 单页硬上限，防止客户端通过超大 limit 触发全表扫描 / OOM。
	MaxLimit = 500
	// MaxOffset 深分页偏移上限，超过则截断，避免 OFFSET 深翻页性能悬崖。
	MaxOffset = 10000
)

// Page 描述一次列表查询的分页参数。
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// Normalize 将客户端传入的分页参数收敛到合法区间，并应用默认/上限。
func (p Page) Normalize() Page {
	limit := p.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > MaxOffset {
		offset = MaxOffset
	}
	return Page{Limit: limit, Offset: offset}
}

// Scope 将分页条件（Limit/Offset）应用到 gorm 查询。调用方负责先写好
// Where/Order 等过滤与排序条件。
func (p Page) Scope(db *gorm.DB) *gorm.DB {
	np := p.Normalize()
	return db.Offset(np.Offset).Limit(np.Limit)
}

// Result 是分页列表的统一返回结构。
type Result[T any] struct {
	Items   []T   `json:"items"`
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

// NewResult 根据已取回的当前页与命中总数组装分页结果。
func NewResult[T any](items []T, total int64, page Page) Result[T] {
	np := page.Normalize()
	hasMore := int64(np.Offset+np.Limit) < total
	return Result[T]{
		Items:   items,
		Total:   total,
		Limit:   np.Limit,
		Offset:  np.Offset,
		HasMore: hasMore,
	}
}
