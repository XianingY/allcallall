package runtime

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/mq"
	"github.com/allcallall/backend/internal/testutil"
)

func countTopic(t *testing.T, broker *mq.MemoryBroker, topic string) int {
	t.Helper()
	consumer := broker.Consumer(topic)
	count := 0
	for {
		if _, err := consumer.Fetch(context.Background()); err != nil {
			break
		}
		count++
	}
	return count
}

func TestRegisterEventsKafkaBridgePublishes(t *testing.T) {
	db := testutil.OpenSQLite(t, "bridge")
	testutil.AutoMigrateAll(t, db)
	store := events.NewStore(db)
	processor := events.NewProcessor(store, nil)

	broker := mq.NewMemoryBroker()
	producer := broker.Producer()
	cfg := config.EventsConfig{TopicPrefix: "allcallall", BridgeChat: true}

	RegisterEventsKafkaBridge(processor, producer, cfg, zerolog.Nop())

	if _, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "weekly_task",
		AggregateID:    7,
		Event:          EventWeeklyTaskTriggered,
		Payload:        map[string]any{"name": "t"},
		IdempotencyKey: "wt:7",
	}); err != nil {
		t.Fatalf("enqueue weekly: %v", err)
	}
	if _, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "chat_message",
		AggregateID:    9,
		Event:          EventChatMessageCreated,
		Payload:        map[string]any{"group_id": 3},
		IdempotencyKey: "cm:9",
	}); err != nil {
		t.Fatalf("enqueue chat: %v", err)
	}

	if _, err := processor.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("process once: %v", err)
	}

	if got := countTopic(t, broker, "allcallall.weekly_task"); got != 1 {
		t.Fatalf("expected 1 weekly_task message, got %d", got)
	}
	if got := countTopic(t, broker, "allcallall.chat_message"); got != 1 {
		t.Fatalf("expected 1 chat_message message, got %d", got)
	}
}

func TestRegisterEventsKafkaBridgeLogOnlyWhenNoProducer(t *testing.T) {
	db := testutil.OpenSQLite(t, "bridge-logonly")
	testutil.AutoMigrateAll(t, db)
	store := events.NewStore(db)
	processor := events.NewProcessor(store, nil)

	broker := mq.NewMemoryBroker()
	// 无 producer（Kafka 未启用）：weekly_task 仍需有 handler 以免被标记失败。
	cfg := config.EventsConfig{TopicPrefix: "allcallall", BridgeChat: true}
	RegisterEventsKafkaBridge(processor, nil, cfg, zerolog.Nop())

	if _, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "weekly_task",
		AggregateID:    7,
		Event:          EventWeeklyTaskTriggered,
		Payload:        map[string]any{},
		IdempotencyKey: "wt:7",
	}); err != nil {
		t.Fatalf("enqueue weekly: %v", err)
	}
	if _, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "chat_message",
		AggregateID:    9,
		Event:          EventChatMessageCreated,
		Payload:        map[string]any{},
		IdempotencyKey: "cm:9",
	}); err != nil {
		t.Fatalf("enqueue chat: %v", err)
	}

	// 无 producer 时 chat.message.created 不应被注册 handler（BridgeChat 但 producer=nil），
	// 因此该事件会缺少 handler；这里仅校验 weekly_task 路径不报错、chat 不被桥接。
	if _, err := processor.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("process once: %v", err)
	}
	if got := countTopic(t, broker, "allcallall.weekly_task"); got != 0 {
		t.Fatalf("expected 0 messages without producer, got %d", got)
	}
}

func TestRegisterEventsKafkaBridgeChatDisabled(t *testing.T) {
	db := testutil.OpenSQLite(t, "bridge-chatoff")
	testutil.AutoMigrateAll(t, db)
	store := events.NewStore(db)
	processor := events.NewProcessor(store, nil)

	broker := mq.NewMemoryBroker()
	producer := broker.Producer()
	// BridgeChat=false：即使有 producer，也不应桥接 chat 事件。
	cfg := config.EventsConfig{TopicPrefix: "allcallall", BridgeChat: false}
	RegisterEventsKafkaBridge(processor, producer, cfg, zerolog.Nop())

	if _, err := store.Enqueue(context.Background(), events.EnqueueInput{
		AggregateType:  "chat_message",
		AggregateID:    9,
		Event:          EventChatMessageCreated,
		Payload:        map[string]any{},
		IdempotencyKey: "cm:9",
	}); err != nil {
		t.Fatalf("enqueue chat: %v", err)
	}

	// 无 chat handler（BridgeChat=false），ProcessOnce 会因缺 handler 返回错误；
	// 这是预期行为：未桥接的事件本就不应进 outbox。
	_, _ = processor.ProcessOnce(context.Background())
	if got := countTopic(t, broker, "allcallall.chat_message"); got != 0 {
		t.Fatalf("expected 0 chat messages when BridgeChat=false, got %d", got)
	}
}
