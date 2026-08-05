package chat

import (
	"time"

	"github.com/allcallall/backend/internal/models"
)

// GroupView 群组详情视图（含成员列表）。
type GroupView struct {
	Group   models.ChatGroup `json:"group"`
	Members []MemberView     `json:"members"`
}

// MemberView 群成员视图。
type MemberView struct {
	UserID            uint64     `json:"user_id"`
	Role              string     `json:"role"`
	Email             string     `json:"email,omitempty"`
	DisplayName       string     `json:"display_name,omitempty"`
	Muted             bool       `json:"muted"`
	LastReadMessageID *uint64    `json:"last_read_message_id,omitempty"`
	LastReadAt        *time.Time `json:"last_read_at,omitempty"`
	JoinedAt          time.Time  `json:"joined_at"`
}

// MessageView 消息视图（含发送者信息与富媒体元数据）。
type MessageView struct {
	models.ChatMessage
	SenderEmail string         `json:"sender_email,omitempty"`
	SenderName  string         `json:"sender_name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// MessagePage 漫游分页结果。
type MessagePage struct {
	Messages   []MessageView `json:"messages"`
	NextBefore *uint64       `json:"next_before,omitempty"`
	NextAfter  *uint64       `json:"next_after,omitempty"`
	HasMorePrev bool         `json:"has_more_prev,omitempty"`
	HasMoreNext bool         `json:"has_more_next,omitempty"`
}

// ReadReceiptView 单条消息的已读用户。
type ReadReceiptView struct {
	UserID      uint64    `json:"user_id"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	ReadAt      time.Time `json:"read_at"`
}

// MemberReadView 当前用户在本群的已读概览（含未读数）。
type MemberReadView struct {
	UserID            uint64     `json:"user_id"`
	LastReadMessageID *uint64    `json:"last_read_message_id,omitempty"`
	LastReadAt        *time.Time `json:"last_read_at,omitempty"`
	UnreadCount       int64     `json:"unread_count"`
}
