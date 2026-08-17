package signaling

import (
	"context"
	"errors"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/user"
	"time"
)

func (h *Hub) registerCallInvite(ctx context.Context, callID, fromEmail, toEmail string) {
	if h.commercial == nil || h.users == nil || callID == "" {
		return
	}
	caller, err := h.users.GetByEmail(ctx, fromEmail)
	if err != nil {
		h.logger.Warn().Err(err).Str("from", fromEmail).Msg("lookup caller failed")
		return
	}
	callee, err := h.users.GetByEmail(ctx, toEmail)
	if err != nil {
		h.logger.Warn().Err(err).Str("to", toEmail).Msg("lookup callee failed")
		return
	}
	if err := h.commercial.RegisterCallInvite(ctx, callID, caller, callee); err != nil {
		h.logger.Warn().Err(err).Str("call_id", callID).Msg("register call invite failed")
	}
}

func (h *Hub) recordCallLifecycle(ctx context.Context, msg SignalMessage) {
	if h.commercial == nil || msg.CallID == "" {
		return
	}
	switch msg.Type {
	case TypeCallAccept:
		if err := h.commercial.UpdateCallStatus(ctx, msg.CallID, models.CallStatusAnswered, ""); err != nil {
			h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to update call status on accept")
		}
		if h.collab != nil {
			if err := h.collab.AppendDirectCallEventByEmail(ctx, msg.From, msg.To, msg.CallID, "call.accepted", map[string]any{
				"status": models.CallStatusAnswered,
			}); err != nil {
				h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to append call.accepted event")
			}
		}
		if h.metrics != nil {
			h.metrics.Inc("call_answer_total")
		}
	case TypeCallReject:
		if err := h.commercial.UpdateCallStatus(ctx, msg.CallID, models.CallStatusRejected, "rejected"); err != nil {
			h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to update call status on reject")
		}
		h.clearBusy(ctx, msg.From, msg.To)
		if h.collab != nil {
			if err := h.collab.AppendDirectCallEventByEmail(ctx, msg.From, msg.To, msg.CallID, "call.rejected", map[string]any{
				"status": models.CallStatusRejected,
				"reason": "rejected",
			}); err != nil {
				h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to append call.rejected event")
			}
		}
	case TypeCallEnd:
		if err := h.commercial.UpdateCallStatus(ctx, msg.CallID, models.CallStatusEnded, "ended"); err != nil {
			h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to update call status on end")
		}
		h.clearBusy(ctx, msg.From, msg.To)
		if h.collab != nil {
			if err := h.collab.AppendDirectCallEventByEmail(ctx, msg.From, msg.To, msg.CallID, "call.ended", map[string]any{
				"status": models.CallStatusEnded,
				"reason": "ended",
			}); err != nil {
				h.logger.Warn().Err(err).Str("call_id", msg.CallID).Msg("failed to append call.ended event")
			}
		}
		if h.metrics != nil {
			h.metrics.Inc("call_ended_total")
		}
	}
}

// setBusy / clearBusy 在通话生命周期内维护"忙碌"主动态。
// setBusy / clearBusy maintain the busy manual state across a call's lifetime.
func (h *Hub) setBusy(ctx context.Context, emails ...string) {
	if h.presence == nil {
		return
	}
	for _, email := range emails {
		if email == "" {
			continue
		}
		if err := h.presence.SetManualState(ctx, email, presence.StateBusy, ""); err != nil {
			h.logger.Debug().Err(err).Str("email", email).Msg("presence: set busy failed")
		}
	}
}

func (h *Hub) clearBusy(ctx context.Context, emails ...string) {
	if h.presence == nil {
		return
	}
	for _, email := range emails {
		if email == "" {
			continue
		}
		if err := h.presence.ClearManualState(ctx, email); err != nil {
			h.logger.Debug().Err(err).Str("email", email).Msg("presence: clear busy failed")
		}
	}
}

// sendCallNotification 发送来电推送通知
// sendCallNotification sends push notification for incoming call
func (h *Hub) sendCallNotification(ctx context.Context, toEmail string, fromEmail string, callID string) {
	// 如果没有 FCM 管理器或用户服务，跳过
	// Skip if FCM manager or user service not available
	if h.fcmManager == nil || h.users == nil {
		h.logger.Debug().
			Str("to", toEmail).
			Str("from", fromEmail).
			Msg("fcm manager or user service not available, skipping notification")
		return
	}

	// 不阻塞地发送推送通知，使用 goroutine
	// Send notification asynchronously to avoid blocking
	go func() {
		// 调用上上下文以取消悠斶
		// Create a new context with timeout
		notifCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 获取接收者的用户信息
		// Get recipient user info
		toUser, err := h.users.GetByEmail(notifCtx, toEmail)
		if err != nil {
			h.logger.Warn().Err(err).Str("email", toEmail).Msg("failed to get recipient user info")
			return
		}

		// 获取勳者的用户信息
		// Get initiator user info
		fromUser, err := h.users.GetByEmail(notifCtx, fromEmail)
		if err != nil {
			h.logger.Warn().Err(err).Str("email", fromEmail).Msg("failed to get initiator user info")
			return
		}

		tokens := make([]string, 0, 4)
		seen := make(map[string]struct{})
		devices, err := h.users.ListPushDevices(notifCtx, toUser.ID)
		if err != nil {
			h.logger.Warn().Err(err).Str("email", toEmail).Msg("failed to list push devices")
		}
		for _, device := range devices {
			if device.Token == "" {
				continue
			}
			if _, ok := seen[device.Token]; ok {
				continue
			}
			seen[device.Token] = struct{}{}
			tokens = append(tokens, device.Token)
		}
		if toUser.FCMToken != "" {
			if _, ok := seen[toUser.FCMToken]; !ok {
				tokens = append(tokens, toUser.FCMToken)
			}
		}
		if len(tokens) == 0 {
			h.logger.Debug().Str("email", toEmail).Msg("recipient has no fcm token")
			return
		}

		var sent int
		for _, token := range tokens {
			if err := h.fcmManager.SendCallNotification(notifCtx, token, fromEmail, fromUser.DisplayName, callID); err != nil {
				if fcm.IsInvalidTokenError(err) {
					if cleanupErr := h.users.DeletePushDeviceByToken(notifCtx, toUser.ID, token); cleanupErr != nil && !errors.Is(cleanupErr, user.ErrNotFound) {
						h.logger.Warn().Err(cleanupErr).Uint64("user_id", toUser.ID).Msg("failed to delete invalid push device")
					}
				}
				h.logger.Error().Err(err).
					Str("to", toEmail).
					Str("from", fromEmail).
					Str("call_id", callID).
					Msg("failed to send call notification")
				continue
			}
			sent++
		}

		h.logger.Info().
			Str("to", toEmail).
			Str("from", fromEmail).
			Str("call_id", callID).
			Int("devices", sent).
			Msg("call notification sent successfully")
	}()
}
