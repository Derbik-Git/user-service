package service // unit тесты

import (
	"context"
	"errors"
	"testing"
	"time"

	"log/slog"

	"github.com/Derbik-Git/user-service/internal/domain"
	errorsx "github.com/Derbik-Git/user-service/internal/errors"
	"github.com/Derbik-Git/user-service/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// В этом, саом первом тесте даны грамотные объяснения как работает тестирование
func TestService_CreateUser(t *testing.T) {
	t.Parallel() // объявляем что тесты могут выполнятся параллельно

	ctx := context.Background() // создаём контекст, просто потому что этого требует логика сервиса

	var capturedTopic string
	var capturedEvent string
	var capturedUser *domain.User // !!! эта перменная нужня для проверки правильные ли данные сервис пытался отправить в брокер kafka

	tests := []struct {
		nameTest      string
		emailArgument string
		nameArgument  string
		repo          *mocks.UserRepositoryMock
		cache         *mocks.CacheMock
		broker        *mocks.EventProducerMock
		wantCafkaCall bool
		wantErr       error // ожидание что вернёт тест, тут идёт сверка, то ли врнул тест или нет(аргумент, который мы ожидаем)
	}{
		{
			nameTest:      "success",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			repo: &mocks.UserRepositoryMock{
				CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) { // говорим что от этого метода ожидаем вот такой вот return(строчка ниже)
					return &domain.User{ID: 1, Email: email, Name: name}, nil // мы лишь описываем что должно вернутся в случае если сервис вызовет CreateFunc, то есть если при запуске теста именно сервис вызовет CreateFunc в моке, так как настоящий репозиторий ничего не может вернуть, так как это Unit тесты и у нас стоит мок, мы говорим что при вызове такой то функции из мока &mocks.UserRepositoryMock{CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error), хотим увидеть вот такой результат return &domain.User{ID: 1, Email: email, Name: name}, nil
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					// перехватываем событие в структуру, что бы потом проверить правильное ли сообщение хотел отправить сервис в брокер kafka
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event
					return nil // символезируем об успешном выполнении без ошибок
				},
			},
			wantCafkaCall: true,
			wantErr:       nil,
		},
		{
			nameTest:      "invalid input",
			emailArgument: "",
			nameArgument:  "",
			repo:          &mocks.UserRepositoryMock{}, // тут программа даже не доходит типо до репозитория, она падает уже в сервисе, потому что ввод не верный, эта логика указана в сервисе, тио если email == "" || name == "", то возвращаем и errorsx.ErrInvalidArgument, поэтому и тут всё так, ожидаем именно эту ошибку и не вызываем как бы вот такой метод мока CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) как в сосседних тестах, потому что программа даже до него не доходит, а падает на проверке if == "" уже в сервисе не доходя до репозитория
			broker:        &mocks.EventProducerMock{},
			wantErr:       errorsx.ErrInvalidInput,
		},
		{
			nameTest:      "repository error",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			repo: &mocks.UserRepositoryMock{
				CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
					return nil, errors.New("db error") // а тут мы ожидаем ошибку именно от репозитория, поэтому и вызываем мок метод репозитория(ну то есть говорим что хотим от него получить)
				},
			},
			cache: nil,
			// Кафка не должна быть вызвана сервисом в случае ошибки бд, поэтому мы роняем тест, если сервис после ошибки бд, попытался вызвать метод kafka
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					t.Fatalf("Сервис при ошибке repository, попытался вызвать метод kafka, этого не должно было случиться!")
					return nil
				},
			},
			wantCafkaCall: false, // Сервис должен прервать работу до вызова Кафки
			wantErr:       errors.New("db error"),
		},
		{
			nameTest:      "broker error: user created but kafka failed",
			emailArgument: "test@mail.com",
			nameArgument:  "Bob Proctor",
			repo: &mocks.UserRepositoryMock{
				CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
					return &domain.User{ID: 1, Email: email, Name: name}, nil
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event

					return errors.New("kafka connection refused")
				},
			},
			wantCafkaCall: true,
			wantErr:       nil,
		},
	}

	for _, tt := range tests {
		tt := tt                              // дословно мы туту говорим Скопируй содержимое листа tt и положи его на новый отдельный лист, который будет жить только в этой итерации, это нужно для того что бы у каждой горутины/теста был свой собственный лист, как я понял, в кратце мы создаём дял каждого теста свою копию, что бы тесты не трогали один и тот же лист, без этого все тетсы читали бы последний тест кейс, а с этой конструкцией, каждй тест читает свой тест кейс
		t.Run(tt.nameTest, func(*testing.T) { // говорим чтот запускаем тесты с таким именем tt.nameTest

			capturedEvent = "" // обнуляем перменную, что бы данные от одного теста не перетикали в другой тест
			capturedTopic = "" // обнуляем для правильности, хотя в этом сервисе для всего используется 1 и тот же топик, потому что в этом сервисе логика связана только с юзерами
			capturedUser = nil

			svc := NewUserService(tt.repo, tt.cache, tt.broker, slog.Default(), time.Minute) // даём сервису данные в конструктор для вызова/теста определённого метода
			u, err := svc.CreateUser(ctx, tt.emailArgument, tt.nameArgument)                 // вызываем сам метод

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}
			// !!! ДОБАВЛЯЕМ ПРОВЕРКУ KAFKA !!!
			if tt.wantCafkaCall {
				require.NotNil(t, capturedEvent, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", capturedTopic)
				assert.Equal(t, domain.UserCreated, capturedEvent)
				assert.Equal(t, tt.emailArgument, capturedUser.Email)
			} else {
				// Убеждаемся, что переменная осталась пустой (брокер не вызывался)
				require.Nil(t, capturedUser, "сервис не должен был вызывать Kafka, но вызвал")
			}
		})
	}
}

// В гет kafka не учавствует, что бы не терять скорость
func TestService_GetUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		nameTest string
		id       int64
		repo     *mocks.UserRepositoryMock
		cache    *mocks.CacheMock
		broker   *mocks.EventProducerMock
		wantErr  error
		wantNil  bool // нужно для того что бы, отличать случаи, когда мы ожидаем пользователь не найден, то есть когда мы ожидаем return nil, nil, wantNil = true, так же он всегда будет true, в любых тестах, где мы ожидаем ошибку, где мы ожидаем результат, то есть пользователь есть, wantNil всегда будет = false
	}{
		{
			nameTest: "from cache",
			id:       1,
			repo:     &mocks.UserRepositoryMock{},
			cache: &mocks.CacheMock{
				GetUserFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return &domain.User{ID: id, Email: "1@email.com", Name: "Test"}, nil
				},
			},
			wantErr: nil,
			wantNil: false,
		},
		{
			nameTest: "repository return error",
			id:       2,
			repo: &mocks.UserRepositoryMock{
				GetUserByIDFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, errors.New("db error")
				},
			},
			cache:   &mocks.CacheMock{},
			broker:  &mocks.EventProducerMock{},
			wantErr: errors.New("db error"),
			wantNil: true,
		},
		{
			nameTest: "repository returns nil", // отсутствие пользователя по указанному id
			id:       3,
			repo: &mocks.UserRepositoryMock{
				GetUserByIDFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, nil
				},
			},
			cache:   &mocks.CacheMock{},
			broker:  &mocks.EventProducerMock{},
			wantErr: nil,
			wantNil: true,
		},
		{
			nameTest: "invalid id",
			id:       0,
			repo:     &mocks.UserRepositoryMock{},
			cache:    &mocks.CacheMock{},
			broker:   &mocks.EventProducerMock{},
			wantErr:  errorsx.ErrInvalidInput,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(*testing.T) {
			t.Parallel()
			svc := NewUserService(tt.repo, tt.cache, tt.broker, slog.Default(), time.Minute)
			u, err := svc.GetUser(ctx, tt.id)

			if tt.wantErr == nil { //если в этом тесте мы не ждём ошибку
				require.NoError(t, err) // если ошибка есть, тест сразу падает

				if tt.wantNil { // ожидаем что результат будет nil
					assert.Nil(t, u) // проверяем что пользователь не существует
				} else {
					assert.NotNil(t, u) // иначе проверяем что пользователь существует
				}
			} else {
				require.Error(t, err)                            // если ожидаем ошибку, проверяем что она есть
				assert.Equal(t, tt.wantErr.Error(), err.Error()) // проверяем что ошибка совпадает с ожидаемой
			}
		})

	}
}

func TestService_UpdateUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var capturedTopic string
	var capturedEvent string
	var capturedUser *domain.User

	tests := []struct {
		nameTest      string
		user          *domain.User
		repository    *mocks.UserRepositoryMock
		cache         *mocks.CacheMock
		broker        *mocks.EventProducerMock
		wantErr       error
		wantKafkaCall bool // Флаг для определения, ожидаем ли мы отправку в Kafka
	}{
		{
			// 1. УСПЕШНОЕ ДЕЙСТВИЕ
			nameTest: "success",
			user:     &domain.User{ID: 1, Email: "1@email.com", Name: "Test"},
			repository: &mocks.UserRepositoryMock{
				UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
					return user, nil
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event
					return nil
				},
			},
			wantErr:       nil,
			wantKafkaCall: true, // Ожидаем вызов
		},
		{
			nameTest:      "invalid input",
			user:          nil,
			repository:    &mocks.UserRepositoryMock{},
			cache:         nil,
			broker:        &mocks.EventProducerMock{},
			wantErr:       errorsx.ErrInvalidInput,
			wantKafkaCall: false, // Вызов не ожидаем
		},
		{
			// 2. ОШИБКА БАЗЫ ДАННЫХ
			nameTest: "repository error",
			user:     &domain.User{ID: 2, Email: "2@gmail.com", Name: "test"},
			repository: &mocks.UserRepositoryMock{
				UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
					return nil, errors.New("update failed")
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					t.Fatalf("Сервис не должен отправлять событие, если БД вернула ошибку!")
					return nil
				},
			},
			wantErr:       errors.New("update failed"),
			wantKafkaCall: false, // Вызов не ожидаем, БД упала
		},
		{
			// 3. ОШИБКА БРОКЕРА
			nameTest: "broker error",
			user:     &domain.User{ID: 3, Email: "3@email.com", Name: "TestBroker"},
			repository: &mocks.UserRepositoryMock{
				UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
					return user, nil // БД обновляет успешно
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event
					return errors.New("kafka is down") // Брокер возвращает ошибку
				},
			},
			wantErr:       nil,  // Сервис не должен возвращать ошибку пользователю, если обновление в БД прошло успешно
			wantKafkaCall: true, // Вызов ожидаем, перехват нужно проверить
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(t *testing.T) { // исправлено на func(t *testing.T)
			// Обнуляем переменные перед каждым тест-кейсом
			capturedTopic = ""
			capturedEvent = ""
			capturedUser = nil

			svc := NewUserService(tt.repository, tt.cache, tt.broker, slog.Default(), time.Minute)
			u, err := svc.UpdateUser(ctx, tt.user)

			// 1. Проверяем бизнес-логику сервиса (ошибки и возврат юзера)
			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}

			// 2. Проверяем логику отправки в Kafka
			if tt.wantKafkaCall {
				// string не может быть nil, поэтому проверяем структуру
				require.NotNil(t, capturedUser, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", capturedTopic)
				assert.Equal(t, domain.UserUpdated, capturedEvent)
				assert.Equal(t, tt.user.Email, capturedUser.Email)
			} else {
				require.Nil(t, capturedUser, "сервис не должен был вызывать Kafka, но вызвал")
			}
		})
	}
}

func TestServic_DeleteUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var capturedTopic string
	var capturedEvent string
	var capturedUser *domain.User

	tests := []struct {
		nameTest      string
		id            int64
		repository    *mocks.UserRepositoryMock
		cache         *mocks.CacheMock
		broker        *mocks.EventProducerMock
		wantKafkaCall bool
		wantErr       error
	}{
		{
			nameTest: "success",
			id:       1,
			repository: &mocks.UserRepositoryMock{
				DeleteFunc: func(ctx context.Context, id int64) error {
					return nil
				},
			},
			cache: nil,
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event

					return nil
				},
			},
			wantKafkaCall: true,
			wantErr:       nil,
		},
		{
			nameTest:      "invalid input",
			id:            0,
			repository:    &mocks.UserRepositoryMock{},
			cache:         nil,
			broker:        &mocks.EventProducerMock{},
			wantKafkaCall: false,
			wantErr:       errorsx.ErrInvalidInput,
		},
		{
			nameTest: "repository error",
			id:       1,
			repository: &mocks.UserRepositoryMock{
				DeleteFunc: func(ctx context.Context, id int64) error {
					return errors.New("delete failed")
				},
			},
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic, eventType string, event *domain.User) error {
					t.Fatalf("Сервис при ошибке repository, попытался вызвать метод kafka, этого не должно было случиться!")
					return nil
				},
			},
			wantErr: errors.New("delete failed"),
		},
		{
			nameTest: "broker error",
			id:       1,
			repository: &mocks.UserRepositoryMock{
				DeleteFunc: func(ctx context.Context, id int64) error {
					return nil
				},
			},
			broker: &mocks.EventProducerMock{
				PublishUserEventFunc: func(ctx context.Context, topic, eventType string, event *domain.User) error {
					capturedTopic = topic
					capturedEvent = eventType
					capturedUser = event

					return errors.New("broker is down")
				},
			},
			wantKafkaCall: true,
			wantErr:       nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(*testing.T) {
			// без парралельности, иначе(с парралельностью) у нас например success тест будет заполнять capturedEvent, в ту же миллисекунду invalid input будет обнулять capturedIvent и произойдёт гонка данных
			capturedTopic = "" // сначала обнуляется, потом ниже выполняется метод, поэтому мы будем проверять уже заполненную, а не пустую переменнуую
			capturedEvent = ""
			capturedUser = nil

			svc := NewUserService(tt.repository, tt.cache, tt.broker, slog.Default(), time.Minute)
			err := svc.DeleteUser(ctx, tt.id)

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			}
			if tt.wantKafkaCall {
				require.NotNil(t, capturedEvent, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", capturedTopic)
				assert.Equal(t, domain.UserDeleted, capturedEvent)
				assert.Equal(t, tt.id, capturedUser.ID)
			} else {
				// Если была ошибка БД, переменная должна остаться пустой, соответственно проверяем пустая ли она, если нет то выводиться сообщение "Событие не должно было формироваться"
				require.Nil(t, capturedEvent, "Событие не должно было формироваться")
			}
		})
	}
}
