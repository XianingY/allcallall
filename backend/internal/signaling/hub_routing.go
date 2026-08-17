package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/redis/go-redis/v9"
	"time"
)

func (h *Hub) enqueue(ctx context.Context, target string, payload []byte) {
	if h.redis == nil {
		return
	}
	key := signalQueuePrefix + target
	pipe := h.redis.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.LTrim(ctx, key, 0, signalQueueMaxLen-1)
	pipe.Expire(ctx, key, time.Hour)
	_, err := pipe.Exec(ctx)
	if err != nil {
		h.logger.Debug().Err(err).Str("email", target).Msg("failed to enqueue signaling payload")
	}
}

func (h *Hub) ensureAllowedPeer(ctx context.Context, fromEmail, toEmail string) error {
	fromUser, err := h.users.GetByEmail(ctx, fromEmail)
	if err != nil {
		return err
	}
	toUser, err := h.users.GetByEmail(ctx, toEmail)
	if err != nil {
		return err
	}
	blocked, err := h.commercial.AreUsersBlocked(ctx, fromUser.ID, toUser.ID)
	if err != nil {
		return err
	}
	if blocked {
		if h.metrics != nil {
			h.metrics.Inc("blocked_interaction_total")
		}
		return commerce.ErrUserBlocked
	}
	return nil
}

func (h *Hub) sendProtocolError(fromEmail, toEmail, callID string, err error) {
	if fromEmail == "" {
		return
	}

	reason := "request rejected"
	switch {
	case errors.Is(err, commerce.ErrUserBlocked):
		reason = "user is blocked"
	}

	msg := SignalMessage{
		Type:   TypeCallError,
		CallID: callID,
		To:     fromEmail,
		From:   toEmail,
		Payload: json.RawMessage(fmt.Sprintf(
			`{"reason":%q}`,
			reason,
		)),
	}
	encoded, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		h.logger.Warn().Err(marshalErr).Msg("failed to marshal protocol error message")
		return
	}
	h.dispatchLocal(fromEmail, encoded)
	if h.redis != nil {
		h.enqueue(context.Background(), fromEmail, encoded)
	}
}

func (h *Hub) Poll(ctx context.Context, email string, timeout time.Duration) ([]byte, bool, error) {
	if h.redis == nil {
		return nil, false, nil
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	key := signalQueuePrefix + email
	res, err := h.redis.BRPop(ctx, timeout, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(res) != 2 {
		return nil, false, nil
	}
	return []byte(res[1]), true, nil
}

func (h *Hub) channelName(email string) string {
	return fmt.Sprintf("signal:%s", email)
}
