package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	kafkaWriter *kafka.Writer
}

func NewProducer() *Producer {
	return &Producer{
		kafkaWriter: &kafka.Writer{
			Addr:     kafka.TCP(brokers...), // принимает список адресов, по типу локалхоста, для установки начального соединения с кластером
			Balancer: &kafka.Hash{},         // балансировщик определяет в какую партицию отправлять сообщение. !!! Если у сообщения есть ключ (Key), Kafka вычисляет хэш от этого ключа и отправляет сообщение в партицию с номером hash % N, где N — общее число партиций в топике.
		},
	}
}

func (p *Producer) PublishUserEvent(ctx context.Context, topic string, user *domain.User) error {
	b, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshall user: %w", err)
	}

	return p.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(strconv.FormatInt(user.ID, 10)),
		Value: b,
	})
}
