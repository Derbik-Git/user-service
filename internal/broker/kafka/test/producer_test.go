package kafkaTest

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	mockKafka "github.com/Derbik-Git/user-service/internal/broker/kafka/mock"
	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProduser_PublishUserEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mockWriter := &mockKafka.MockKafkaWriter{} // подготвливаем переменную со структурой моком, реализующий интерфейс продюсера
	producer := &kafka.Producer{               // подставляем мок в структуру нашего кафка продюсера, что бы он работал не
		KafkaWriter: mockWriter,
	}

	user := &domain.User{
		ID:    99,
		Email: "test@example.com",
		Name:  "Test",
	}
	topic := "test-topic"

	err := producer.PublishUserEvent(ctx, domain.TopicUserEvents, topic, user) // мы за счёт поля KafkaWriter, структуры Producer, вызываем подставленный нами мок метод WriteMassage, который не имеет отношения к реальному выполнению задачи return p.kafkaWriter.WriteMessages(ctx, kafka.Message{
	require.NoError(t, err)

	// Достаем из Шпиона коробку и смотрим, правильный ли Топик написал Директор?
	assert.Equal(t, topic, mockWriter.CapturedMessage.Topic)

	expectedKey := []byte(strconv.FormatInt(user.ID, 10))
	assert.Equal(t, expectedKey, mockWriter.CapturedMessage.Key)

	var sentEvent domain.UserEvent
	err = json.Unmarshal(mockWriter.CapturedMessage.Value, &sentEvent) // распаковываем и кладём в структуру, что бы проверить точно ли правильный ID
	require.NoError(t, err)

	assert.NotEmpty(t, sentEvent.ID)
	assert.Equal(t, domain.UserCreated, sentEvent.Type)
	assert.Equal(t, user.ID, sentEvent.Payload.ID)
}
