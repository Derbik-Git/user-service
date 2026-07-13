package mocks

import (
	"context"
	"errors"

	"github.com/Derbik-Git/user-service/internal/domain"
)

// EventProducerMock имитирует работу с Kafka для юнит-тестов
type EventProducerMock struct {
	PublishUserEventFunc func(ctx context.Context, topic string, eventType string, event *domain.User) error
}

func (m *EventProducerMock) PublishUserEvent(ctx context.Context, topic string, eventType string, event *domain.User) error {
	if m.PublishUserEventFunc == nil {
		return errors.New("PublishUserEvent method is not implemented in the unit tests of the service")
	}

	return m.PublishUserEventFunc(ctx, topic, eventType, event)
}
