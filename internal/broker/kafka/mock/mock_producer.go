package mockKafka

import (
	"context"

	"github.com/segmentio/kafka-go"
)

// что бы это являлось моком, эта структура должна реализовывать методы WriteMessages и Close, реализуя их, она автоматически становится реализацией интерфейса в настоящем продюсере
// таким образом мы дёргаем мок струткуру, вызываем через неё наши мок методы
type MockKafkaWriter struct {
	CapturedMessage kafka.Message //m.capturedMessage = msgs[0] вот таким образом в мок методе WriteMessages кладёт сюда данные, что бы потом мы могли их достать и проверить в тестах, то есть брать данные именно из ЭКЗЕМПЛЯРА структуры
}

func (m *MockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if len(msgs) > 0 {
		m.CapturedMessage = msgs[0]
	}
	return nil
}

func (m *MockKafkaWriter) Close() error {
	return nil
}
