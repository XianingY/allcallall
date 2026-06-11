package mq

import (
	"context"
	"errors"
	"sync"
)

var ErrNoMessages = errors.New("no messages available")

type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Partition int
	Offset    int64
}

type Producer interface {
	Publish(ctx context.Context, topic string, message Message) error
	Close() error
}

type Consumer interface {
	Fetch(ctx context.Context) (Message, error)
	Commit(ctx context.Context, message Message) error
	Close() error
}

type MemoryBroker struct {
	mu     sync.Mutex
	topics map[string][]Message
}

func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{topics: map[string][]Message{}}
}

func (b *MemoryBroker) Producer() Producer {
	return &memoryProducer{broker: b}
}

func (b *MemoryBroker) Consumer(topic string) Consumer {
	return &memoryConsumer{broker: b, topic: topic}
}

type memoryProducer struct {
	broker *MemoryBroker
}

func (p *memoryProducer) Publish(_ context.Context, topic string, message Message) error {
	p.broker.mu.Lock()
	defer p.broker.mu.Unlock()
	message.Topic = topic
	p.broker.topics[topic] = append(p.broker.topics[topic], message)
	return nil
}

func (p *memoryProducer) Close() error { return nil }

type memoryConsumer struct {
	broker *MemoryBroker
	topic  string
	offset int
}

func (c *memoryConsumer) Fetch(_ context.Context) (Message, error) {
	c.broker.mu.Lock()
	defer c.broker.mu.Unlock()
	messages := c.broker.topics[c.topic]
	if c.offset >= len(messages) {
		return Message{}, ErrNoMessages
	}
	message := messages[c.offset]
	c.offset++
	return message, nil
}

func (c *memoryConsumer) Commit(context.Context, Message) error { return nil }

func (c *memoryConsumer) Close() error { return nil }
