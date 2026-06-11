package settlement

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mq"
)

type Worker struct {
	consumer mq.Consumer
	service  *Service
	logger   zerolog.Logger
}

func NewWorker(consumer mq.Consumer, service *Service, logger zerolog.Logger) *Worker {
	return &Worker{
		consumer: consumer,
		service:  service,
		logger:   logger.With().Str("component", "settlement_worker").Logger(),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.service == nil {
		return errors.New("settlement worker is not initialized")
	}
	for {
		message, err := w.consumer.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if errors.Is(err, mq.ErrNoMessages) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}
			w.logger.Warn().Err(err).Msg("settlement fetch failed")
			continue
		}
		event, err := DecodeRoomEndedMessage(message)
		if err != nil {
			w.logger.Warn().Err(err).Msg("invalid settlement event")
			_ = w.consumer.Commit(ctx, message)
			continue
		}
		record, err := w.service.ApplyRoomEnded(ctx, event)
		if err != nil {
			w.logger.Error().Err(err).Str("event_id", event.EventID).Msg("settlement apply failed")
			continue
		}
		if err := w.consumer.Commit(ctx, message); err != nil {
			w.logger.Warn().Err(err).Str("event_id", event.EventID).Msg("settlement commit failed")
			continue
		}
		w.logger.Info().
			Str("event_id", event.EventID).
			Uint64("settlement_id", record.ID).
			Uint64("room_id", event.RoomID).
			Uint64("user_id", event.UserID).
			Msg("settlement event applied")
	}
}
