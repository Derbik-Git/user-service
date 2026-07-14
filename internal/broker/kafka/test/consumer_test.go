package kafkaTest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Derbik-Git/user-service/internal/broker/kafka"
	mockKafka "github.com/Derbik-Git/user-service/internal/broker/kafka/mock"
	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumer_ProccessRawMassage_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	expectedEvent := domain.UserEvent{
		ID:   "test-uuid-123",
		Type: domain.UserCreated,
		Payload: domain.User{
			ID:        1,
			Email:     "test@gamil.com",
			Name:      "TestUser",
			CreatedAt: time.Now().Truncate(time.Second),
		},
		CreatedAt: time.Now().Truncate(time.Second),
	}

	massageBytes, err := json.Marshal(expectedEvent)
	require.NoError(t, err)

	mockHandler := &mockKafka.MockConsumerHandler{}

	consumer := &kafka.Consumer{
		Log:     logger,
		Handler: mockHandler.HendlerAddEvent,
	}

	err = consumer.ProcessRawMessage(ctx, massageBytes)
	require.NoError(t, err)

	assert.True(t, mockHandler.IsCalled)
	assert.Equal(t, expectedEvent, mockHandler.ReceivedEvent)
}

// проверяем, вызоветься ли логика со сломанным сообщением
func TestConsumer_ProcessRowMassage_InvalidJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	badJSON := []byte(`{"id": "123", broken_json_here}`)

	mockHandler := &mockKafka.MockConsumerHandler{}
	consumer := &kafka.Consumer{
		Log:     logger,
		Handler: mockHandler.HendlerAddEvent,
	}

	err := consumer.ProcessRawMessage(ctx, badJSON)
	require.NoError(t, err)

	assert.False(t, mockHandler.IsCalled)
}

func TestConsumer_ProccessRawMessage_HandlerError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	validJSON := []byte(`{"id": "123", "type": "user.created"}`)
	expectedError := errors.New("database down")

	mockHandler := &mockKafka.MockConsumerHandler{
		ErrToReturn: expectedError,
	}

	consumer := kafka.Consumer{
		Log:     logger,
		Handler: mockHandler.HendlerAddEvent,
	}

	err := consumer.ProcessRawMessage(ctx, validJSON)
	require.NoError(t, err)

	require.ErrorIs(t, err, expectedError)
}
