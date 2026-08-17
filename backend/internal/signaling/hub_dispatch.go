package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func (h *Hub) handleIncoming(ctx context.Context, fromClient *client, data []byte) error {
	if h.presence != nil {
		if err := h.presence.UpdateLastSeen(ctx, fromClient.email); err != nil {
			h.logger.Debug().Err(err).Str("email", fromClient.email).Msg("failed to refresh last seen")
		}
	}

	var msg SignalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}

	// Extract trace context
	var tracer = otel.Tracer("signaling")
	if msg.Trace != "" {
		carrier := propagation.MapCarrier{"traceparent": msg.Trace}
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	}
	ctx, span := tracer.Start(ctx, "Hub.handleIncoming", trace.WithAttributes())
	defer span.End()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	msg.Trace = carrier["traceparent"]

	msg.From = fromClient.email
	if msg.Type != TypeClientPing && msg.To == "" {
		return fmt.Errorf("missing target 'to'")
	}

	if msg.Type != TypeClientPing && h.commercial != nil && h.users != nil {
		if err := h.ensureAllowedPeer(ctx, msg.From, msg.To); err != nil {
			h.sendProtocolError(msg.From, msg.To, msg.CallID, err)
			return nil
		}
	}

	ackMsg, err := h.applyProtocolRules(&msg)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if msg.Type != TypeClientPing {
		h.dispatchLocal(msg.To, encoded)
		h.enqueue(ctx, msg.To, encoded)
	}

	envBytes, err := json.Marshal(redisEnvelope{
		NodeID: h.nodeID,
		Data:   encoded,
	})
	if err != nil {
		return err
	}

	if ackMsg != nil {
		if ackBytes, err := json.Marshal(ackMsg); err == nil {
			h.dispatchLocal(msg.From, ackBytes)
			h.enqueue(ctx, msg.From, ackBytes)
		} else {
			h.logger.Warn().Err(err).Msg("failed to marshal ack message")
		}
	}

	// 如果是 call.invite 消息，需要发送推送通知
	// If this is a call.invite message, send push notification
	if msg.Type == TypeCallInvite {
		h.registerCallInvite(ctx, msg.CallID, msg.From, msg.To)
		h.sendCallNotification(ctx, msg.To, msg.From, msg.CallID)
		// 通话中自动置忙，覆盖设备推导的在线态。
		// Mark both participants busy while the call is live.
		h.setBusy(ctx, msg.From, msg.To)
		if h.metrics != nil {
			h.metrics.Inc("call_invite_total")
		}
	}
	h.recordCallLifecycle(ctx, msg)

	if msg.Type == TypeClientPing {
		// A client pong (or app-level ping reply) proves the connection is
		// alive: renew the device heartbeat so the user stays online.
		if h.presence != nil {
			if err := h.presence.Heartbeat(ctx, msg.From, "signaling", ""); err != nil {
				h.logger.Debug().Err(err).Str("email", msg.From).Msg("presence heartbeat on ping failed")
			}
		}
		return nil
	}

	return h.redis.Publish(ctx, h.channelName(msg.To), envBytes).Err()
}

func (h *Hub) HandleHTTPMessage(ctx context.Context, fromEmail string, data []byte) error {
	if h.presence != nil {
		if err := h.presence.UpdateLastSeen(ctx, fromEmail); err != nil {
			h.logger.Debug().Err(err).Str("email", fromEmail).Msg("failed to refresh last seen")
		}
	}

	var msg SignalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}

	// Extract trace context
	var tracer = otel.Tracer("signaling")
	if msg.Trace != "" {
		carrier := propagation.MapCarrier{"traceparent": msg.Trace}
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	}
	ctx, span := tracer.Start(ctx, "Hub.HandleHTTPMessage", trace.WithAttributes())
	defer span.End()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	msg.Trace = carrier["traceparent"]

	msg.From = fromEmail
	if msg.Type != TypeClientPing && msg.To == "" {
		return fmt.Errorf("missing target 'to'")
	}

	if msg.Type != TypeClientPing && h.commercial != nil && h.users != nil {
		if err := h.ensureAllowedPeer(ctx, msg.From, msg.To); err != nil {
			h.sendProtocolError(msg.From, msg.To, msg.CallID, err)
			return nil
		}
	}

	ackMsg, err := h.applyProtocolRules(&msg)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if msg.Type != TypeClientPing {
		h.enqueue(ctx, msg.To, encoded)
		h.dispatchLocal(msg.To, encoded)
	}

	if ackMsg != nil {
		if ackBytes, err := json.Marshal(ackMsg); err == nil {
			h.enqueue(ctx, msg.From, ackBytes)
			h.dispatchLocal(msg.From, ackBytes)
		} else {
			h.logger.Warn().Err(err).Msg("failed to marshal ack message")
		}
	}

	if msg.Type == TypeCallInvite {
		h.registerCallInvite(ctx, msg.CallID, msg.From, msg.To)
		h.sendCallNotification(ctx, msg.To, msg.From, msg.CallID)
		if h.metrics != nil {
			h.metrics.Inc("call_invite_total")
		}
	}
	h.recordCallLifecycle(ctx, msg)

	if msg.Type == TypeClientPing {
		return nil
	}

	envBytes, err := json.Marshal(redisEnvelope{
		NodeID: h.nodeID,
		Data:   encoded,
	})
	if err != nil {
		return err
	}

	return h.redis.Publish(ctx, h.channelName(msg.To), envBytes).Err()
}

func (h *Hub) applyProtocolRules(msg *SignalMessage) (*SignalMessage, error) {
	switch msg.Type {
	case TypeClientPing:
		return &SignalMessage{
			Type:    TypeServerPong,
			To:      msg.From,
			From:    msg.From,
			Payload: json.RawMessage("null"),
		}, nil
	case TypeCallInvite:
		if msg.CallID == "" {
			msg.CallID = uuid.NewString()
		}
		return &SignalMessage{
			Type:    TypeCallInviteAck,
			CallID:  msg.CallID,
			To:      msg.From,
			From:    msg.From,
			Payload: msg.Payload,
		}, nil
	case TypeCallAccept, TypeCallReject, TypeCallEnd:
		if msg.CallID == "" {
			return nil, fmt.Errorf("call_id required for message type %s", msg.Type)
		}
	case TypeIceCandidate:
		if msg.CallID == "" {
			return nil, fmt.Errorf("call_id required for ice candidate message")
		}
		if len(msg.Payload) == 0 {
			return nil, fmt.Errorf("payload required for ice candidate message")
		}
	default:
		// Legacy types (offer/answer/etc.) are still allowed without additional validation.
	}
	return nil, nil
}
