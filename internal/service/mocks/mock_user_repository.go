package mocks

import (
	"context"
	"errors"

	"github.com/Derbik-Git/user-service/internal/domain"
)

type UserRepositoryMock struct {
	GetUserByIDFunc func(ctx context.Context, id int64) (*domain.User, error)
	CreateFunc      func(ctx context.Context, email, name string) (*domain.User, error)
	UpdateFunc      func(ctx context.Context, user *domain.User) (*domain.User, error)
	DeleteFunc      func(ctx context.Context, id int64) error
}

func (m *UserRepositoryMock) GetUserByID(ctx context.Context, id int64) (*domain.User, error) {
	if m.GetUserByIDFunc == nil { // Если метод не реализован в тестах, то есть m.GetUserByIDFunc == nil, то мы ...
		return nil, errors.New("GetUserByID method is not implemented in the unit tests of the service") // Эта конструкция позволяет нам не вызывать этот метод в тестах и программа не будет падать
	}

	return m.GetUserByIDFunc(ctx, id) //  Метод GetUserByID обращается напрямую к полю GetUserByIDFunc. Если это поле не заполнено (оставлено равным nil), Go попытается вызвать метод, хотя фактически никакого метода не существует. Результатом станет паника с сообщением:
}

func (m *UserRepositoryMock) Create(ctx context.Context, email, name string) (*domain.User, error) {
	if m.CreateFunc == nil {
		return nil, errors.New("CreateUser method is not implemented in the unit tests of the service")
	}

	return m.CreateFunc(ctx, email, name)
}

func (m *UserRepositoryMock) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	if m.UpdateFunc == nil {
		return nil, errors.New("Update method is not implemented in the unit tests of the service")
	}

	return m.UpdateFunc(ctx, user)
}

func (m *UserRepositoryMock) Delete(ctx context.Context, id int64) error {
	if m.DeleteFunc == nil {
		return errors.New("Delete method is not implemented in the unit tests of the service")
	}

	return m.DeleteFunc(ctx, id)
}
