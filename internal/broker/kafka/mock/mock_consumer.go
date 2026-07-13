package mockKafka

import "github.com/Derbik-Git/user-service/internal/domain"

type MockConsumerHandler struct {
	receivedEvent domain.UserEvent
	isCalled      bool
	errToReturn   error
}

func (m *MockConsumerHandler) HendlerAddEvent(event domain.UserEvent) error {
	m.receivedEvent = event
	m.isCalled = true
	return m.errToReturn
}
