package mockKafka

import "github.com/Derbik-Git/user-service/internal/domain"

type MockConsumerHandler struct {
	ReceivedEvent domain.UserEvent // сам пользователь, проверять дынные = брать с этого поля
	IsCalled      bool             // было ли вызвано
	ErrToReturn   error
}

func (m *MockConsumerHandler) HendlerAddEvent(event domain.UserEvent) error {
	m.ReceivedEvent = event
	m.IsCalled = true
	return m.ErrToReturn
}
