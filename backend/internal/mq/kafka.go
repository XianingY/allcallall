package mq

import (
	"context"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaConfig struct {
	Brokers []string
	GroupID string
	Topic   string
}

func ParseBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

type KafkaProducer struct {
	writer *kafka.Writer
}

func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			RequiredAcks: kafka.RequireOne,
			Async:        false,
			Balancer:     &kafka.Hash{},
		},
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, topic string, message Message) error {
	headers := make([]kafka.Header, 0, len(message.Headers))
	for key, value := range message.Headers {
		headers = append(headers, kafka.Header{Key: key, Value: []byte(value)})
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic:   topic,
		Key:     message.Key,
		Value:   message.Value,
		Headers: headers,
		Time:    time.Now(),
	})
}

func (p *KafkaProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

type KafkaConsumer struct {
	reader *kafka.Reader
}

func NewKafkaConsumer(cfg KafkaConfig) *KafkaConsumer {
	return &KafkaConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  cfg.Brokers,
			GroupID:  cfg.GroupID,
			Topic:    cfg.Topic,
			MinBytes: 1,
			MaxBytes: 10e6,
		}),
	}
}

func (c *KafkaConsumer) Fetch(ctx context.Context) (Message, error) {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return Message{}, err
	}
	headers := make(map[string]string, len(msg.Headers))
	for _, header := range msg.Headers {
		headers[header.Key] = string(header.Value)
	}
	return Message{
		Topic:     msg.Topic,
		Key:       msg.Key,
		Value:     msg.Value,
		Headers:   headers,
		Partition: msg.Partition,
		Offset:    msg.Offset,
	}, nil
}

func (c *KafkaConsumer) Commit(ctx context.Context, message Message) error {
	return c.reader.CommitMessages(ctx, kafka.Message{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
	})
}

func (c *KafkaConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
