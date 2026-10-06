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

// Структура для изоляции перехваченных данных каждого отдельного подтеста
type capturedData struct {
	topic       string
	event       string
	user        *domain.User
	cacheCalled bool
}

// В этом, самом первом тесте даны грамотные объяснения как работает тестирование
func TestService_CreateUser(t *testing.T) {
	t.Parallel() // объявляем что тесты могут выполнятся параллельно

	ctx := context.Background() // создаём контекст, просто потому что этого требует логика сервиса

	tests := []struct {
		nameTest      string
		emailArgument string
		nameArgument  string
		setup         func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData)
		wantKafkaCall bool
		wantCacheCall bool
		wantErr       error // ожидание что вернёт тест, тут идёт сверка, то ли врнул тест или нет(аргумент, который мы ожидаем)
	}{
		{
			nameTest:      "success",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) { // говорим что от этого метода ожидаем вот такой вот return(строчка ниже)
						return &domain.User{ID: 1, Email: email, Name: name}, nil // мы лишь описываем что должно вернутся в случае если сервис вызовет CreateFunc, то есть если при запуске теста именно сервис вызовет CreateFunc в моке, так как настоящий репозиторий ничего не может вернуть, так как это Unit тесты и у нас стоит мок, мы говорим что при вызове такой то функции из мока &mocks.UserRepositoryMock{CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error), хотим увидеть вот такой результат return &domain.User{ID: 1, Email: email, Name: name}, nil
					},
				}
				cache := &mocks.CacheMock{}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						// перехватываем событие в структуру, что бы потом проверить правильное ли сообщение хотел отправить сервис в брокер kafka
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return nil // символезируем об успешном выполнении без ошибок
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: true,
			wantCacheCall: false,
			wantErr:       nil,
		},
		{
			nameTest:      "invalid input",
			emailArgument: "",
			nameArgument:  "",
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{} // тут программа даже не доходит типо до репозитория, она падает уже в сервисе, потому что ввод не верный, эта логика указана в сервисе, тио если email == "" || name == "", то возвращаем и errorsx.ErrInvalidArgument, поэтому и тут всё так, ожидаем именно эту ошибку и не вызываем как бы вот такой метод мока CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) как в сосседних тестах, потому что программа даже до него не доходит, а падает на проверке if == "" уже в сервисе не доходя до репозитория
				broker := &mocks.EventProducerMock{}
				cache := &mocks.CacheMock{}
				return repo, cache, broker, cap
			},
			wantKafkaCall: false,
			wantCacheCall: false,
			wantErr:       errorsx.ErrInvalidInput,
		},
		{
			nameTest:      "repository error",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
						return nil, errors.New("db error") // а тут мы ожидаем ошибку именно от репозитория, поэтому и вызываем мок метод репозитория(ну то есть говорим что хотим от него получить)
					},
				}
				cache := &mocks.CacheMock{}
				// Кафка не должна быть вызвана сервисом в случае ошибки бд, поэтому мы роняем тест, если сервис после ошибки бд, попытался вызвать метод kafka
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						t.Fatalf("Сервис при ошибке repository, попытался вызвать метод kafka, этого не должно было случиться!")
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: false, // Сервис должен прервать работу до вызова Кафки
			wantCacheCall: false,
			wantErr:       errors.New("db error"),
		},
		{
			nameTest:      "broker error: user created but kafka failed",
			emailArgument: "test@mail.com",
			nameArgument:  "Bob Proctor",
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
						return &domain.User{ID: 1, Email: email, Name: name}, nil
					},
				}
				cache := &mocks.CacheMock{}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event

						return errors.New("kafka connection refused")
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: true,
			wantCacheCall: false,
			wantErr:       nil,
		},
		{
			nameTest:      "cache error: user created but cache failed",
			emailArgument: "test@email.com",
			nameArgument:  "Bob Proctor",
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					CreateFunc: func(ctx context.Context, email, name string) (*domain.User, error) {
						return &domain.User{ID: 1, Email: email, Name: name}, nil
					},
				}
				cache := &mocks.CacheMock{}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = "user-events"
						cap.event = domain.UserCreated
						cap.user = event
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: true,
			wantCacheCall: false, // сервис при создании не вызвает кеш, нечего проверять, не ожидаем вызов кеша
			wantErr:       nil,
		},
	}

	for _, tt := range tests {
		tt := tt                                // дословно мы туту говорим Скопируй содержимое листа tt и положи его на новый отдельный лист, который будет жить только в этой итерации, это нужно для того что бы у каждой горутины/теста был свой собственный лист, как я понял, в кратце мы создаём дял каждого теста свою копию, что бы тесты не трогали один и тот же лист, без этого все тетсы читали бы последний тест кейс, а с этой конструкцией, каждй тест читает свой тест кейс
		t.Run(tt.nameTest, func(t *testing.T) { // говорим чтот запускаем тесты с таким именем tt.nameTest
			t.Parallel() // Запускаем подтесты параллельно и без гонок данных!

			repo, cache, broker, cap := tt.setup(t)

			svc := NewUserService(repo, cache, broker, slog.Default(), time.Minute) // даём сервису данные в конструктор для вызова/теста определённого метода
			u, err := svc.CreateUser(ctx, tt.emailArgument, tt.nameArgument)        // вызываем сам метод

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			}
			// !!! ДОБАВЛЯЕМ ПРОВЕРКУ KAFKA !!!
			if tt.wantKafkaCall {
				require.NotNil(t, cap.user, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", cap.topic)
				assert.Equal(t, domain.UserCreated, cap.event)
				assert.Equal(t, tt.emailArgument, cap.user.Email)
			} else {
				// Убеждаемся, что переменная осталась пустой (брокер не вызывался)
				require.Nil(t, cap.user, "сервис не должен был вызывать Kafka, но вызвал")
			}
			// !!! ДОБАВЛЯЕМ ПРОВЕРКУ КЕША !!!
			if tt.wantCacheCall {
				require.True(t, cap.cacheCalled, "ожидалось, что сервис вызовет метод кеша для сохранения данных, но он этого не сделал")
			} else {
				require.False(t, cap.cacheCalled, "сервис не должен был вызывать метод кеша, но он это сделал")
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
			cache: &mocks.CacheMock{
				GetUserFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, nil
				},
			},
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
		{
			nameTest: "cache miss, fetch from db and set to cache ",
			id:       4,
			repo: &mocks.UserRepositoryMock{
				GetUserByIDFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return &domain.User{ID: id, Email: "4@email.com", Name: "From DB"}, nil
				},
			},
			cache: &mocks.CacheMock{
				GetUserFunc: func(ctx context.Context, id int64) (*domain.User, error) {
					return nil, nil
				},
			},
			broker:  &mocks.EventProducerMock{},
			wantErr: nil,
			wantNil: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(t *testing.T) {
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
				require.Error(t, err)                         // если ожидаем ошибку, проверяем что она есть
				assert.EqualError(t, err, tt.wantErr.Error()) // проверяем что ошибка совпадает с ожидаемой
			}
		})

	}
}

func TestService_UpdateUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		nameTest      string
		user          *domain.User
		setup         func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData)
		wantErr       error
		wantKafkaCall bool // Флаг для определения, ожидаем ли мы отправку в Kafka
		wantCacheCall bool
	}{
		{
			// 1. УСПЕШНОЕ ДЕЙСТВИЕ
			nameTest: "success",
			user:     &domain.User{ID: 1, Email: "1@email.com", Name: "Test"},
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
						return user, nil
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantErr:       nil,
			wantKafkaCall: true, // Ожидаем вызов
			wantCacheCall: true,
		},
		{
			nameTest: "invalid input",
			user:     nil,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				return &mocks.UserRepositoryMock{}, &mocks.CacheMock{}, &mocks.EventProducerMock{}, cap
			},
			wantErr:       errorsx.ErrInvalidInput,
			wantKafkaCall: false, // Вызов не ожидаем
			wantCacheCall: false,
		},
		{
			// 2. ОШИБКА БАЗЫ ДАННЫХ
			nameTest: "repository error",
			user:     &domain.User{ID: 2, Email: "2@gmail.com", Name: "test"},
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
						return nil, errors.New("update failed")
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						t.Errorf("при ошибке repository, сервис не должен был вызвать метод кеша, но он это сделал!") // если сервис после ошибки бд, попытался вызвать метод кеша, тест упадёт с таким сообщением
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						t.Errorf("Сервис не должен отправлять событие, если БД вернула ошибку!")
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantErr:       errors.New("update failed"),
			wantKafkaCall: false, // Вызов не ожидаем, БД упала
			wantCacheCall: false,
		},
		{
			// 3. ОШИБКА БРОКЕРА
			nameTest: "broker error",
			user:     &domain.User{ID: 3, Email: "3@email.com", Name: "TestBroker"},
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
						return user, nil // БД обновляет успешно
					},
				}
				// при ошибке брокера, кеш всё равно должен отрабатывать успешно
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return errors.New("kafka is down") // Брокер возвращает ошибку
					},
				}
				return repo, cache, broker, cap
			},
			wantErr:       nil,  // Сервис не должен возвращать ошибку пользователю, если обновление в БД прошло успешно
			wantKafkaCall: true, // Вызов ожидаем, перехват нужно проверить
			wantCacheCall: true, // При ошибке брокера, кеш всё равно должен быть обновлён
		},
		{
			// 4. ОШИБКА КЕША
			nameTest: "cache error during update",
			user:     &domain.User{ID: 4, Email: "4@gmail.com", Name: "TestCacheErr"},
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					UpdateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
						return user, nil // БД обновляет успешно
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true // Флаг должен фиксировать вызов DeleteUser, то есть нашу инвалидацию
						return errors.New("cache error")
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return nil // Брокер работает успешно, событие отправляется
					},
				}
				return repo, cache, broker, cap
			},
			wantErr:       nil, // не смотря на падение кеша, пользователь всё равно должен получить успешный ответ
			wantKafkaCall: true,
			wantCacheCall: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(t *testing.T) {
			t.Parallel()

			repo, cache, broker, cap := tt.setup(t)

			svc := NewUserService(repo, cache, broker, slog.Default(), time.Minute)
			u, err := svc.UpdateUser(ctx, tt.user)

			// 1. Проверяем бизнес-логику сервиса (ошибки и возврат юзера)
			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.NotNil(t, u)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			}

			// 2. Проверяем логику отправки в Kafka
			if tt.wantKafkaCall {
				require.NotNil(t, cap.user, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", cap.topic)
				assert.Equal(t, domain.UserUpdated, cap.event)
				assert.Equal(t, tt.user.Email, cap.user.Email)
			} else {
				require.Nil(t, cap.user, "сервис не должен был вызывать Kafka, но вызвал")
			}

			// 3. Проверяем логику отправик в кеш
			if tt.wantCacheCall {
				require.True(t, cap.cacheCalled, "ожидалось, что сервис вызовет метод кеша для обновления данных, но он этого не сделал")
			} else {
				require.False(t, cap.cacheCalled, "сервис не должен был вызывать метод кеша, но он это сделал")
			}
		})
	}
}

func TestService_DeleteUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		nameTest      string
		id            int64
		setup         func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData)
		wantKafkaCall bool
		wantCacheCall bool
		wantErr       error
	}{
		{
			nameTest: "success",
			id:       1,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					DeleteFunc: func(ctx context.Context, id int64) error {
						return nil
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic string, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: true,
			wantCacheCall: true,
			wantErr:       nil,
		},
		{
			nameTest: "invalid input",
			id:       0,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				return &mocks.UserRepositoryMock{}, &mocks.CacheMock{}, &mocks.EventProducerMock{}, cap
			},
			wantKafkaCall: false,
			wantCacheCall: false,
			wantErr:       errorsx.ErrInvalidInput,
		},
		{
			nameTest: "repository error",
			id:       1,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					DeleteFunc: func(ctx context.Context, id int64) error {
						return errors.New("delete failed")
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						t.Errorf("при ошибке repository, сервис не должен был вызвать метод кеша, но он это сделал!")
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic, eventType string, event *domain.User) error {
						t.Errorf("Сервис при ошибке repository, попытался вызвать метод kafka, этого не должно было случиться!")
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: false,
			wantCacheCall: false,
			wantErr:       errors.New("delete failed"),
		},
		{
			nameTest: "broker error",
			id:       1,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					DeleteFunc: func(ctx context.Context, id int64) error {
						return nil
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true
						return nil
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return errors.New("broker is down")
					},
				}
				return repo, cache, broker, cap
			},
			wantKafkaCall: true,
			wantCacheCall: true, // даже при ошибке брокера, кеш должен отрабатывать
			wantErr:       nil,
		},
		{
			nameTest: "cache error during delete",
			id:       1,
			setup: func(t *testing.T) (*mocks.UserRepositoryMock, *mocks.CacheMock, *mocks.EventProducerMock, *capturedData) {
				cap := &capturedData{}
				repo := &mocks.UserRepositoryMock{
					DeleteFunc: func(ctx context.Context, id int64) error {
						return nil
					},
				}
				cache := &mocks.CacheMock{
					DeleteUserFunc: func(ctx context.Context, id int64) error {
						cap.cacheCalled = true
						return errors.New("cache is down")
					},
				}
				broker := &mocks.EventProducerMock{
					PublishUserEventFunc: func(ctx context.Context, topic, eventType string, event *domain.User) error {
						cap.topic = topic
						cap.event = eventType
						cap.user = event
						return nil
					},
				}
				return repo, cache, broker, cap
			},
			wantErr:       nil,
			wantKafkaCall: true,
			wantCacheCall: true, // даже при ошибке кеша, событие в брокер должно отправляться
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.nameTest, func(t *testing.T) {
			t.Parallel() // Включаем параллельность: изоляция через setup предотвращает Data Race

			repo, cache, broker, cap := tt.setup(t)

			svc := NewUserService(repo, cache, broker, slog.Default(), time.Minute)
			err := svc.DeleteUser(ctx, tt.id)

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			}
			// !!! ДОБАВЛЯЕМ ПРОВЕРКУ KAFKA !!!
			if tt.wantKafkaCall {
				require.NotNil(t, cap.user, "ожидалось, что сервис отправит событие в Kafka, но он этого не сделал")
				assert.Equal(t, "user-events", cap.topic)
				assert.Equal(t, domain.UserDeleted, cap.event) // Предполагается, что константа называется UserDeleted
				assert.Equal(t, tt.id, cap.user.ID)            // При удалении обычно передается ID пользователя
			} else {
				require.Nil(t, cap.user, "сервис не должен был вызывать Kafka, но вызвал")
			}

			// !!! ДОБАВЛЯЕМ ПРОВЕРКУ КЕША !!!
			if tt.wantCacheCall {
				require.True(t, cap.cacheCalled, "ожидалось, что сервис вызовет метод кеша для удаления данных, но он этого не сделал")
			} else {
				require.False(t, cap.cacheCalled, "сервис не должен был вызывать метод кеша, но он это сделал")
			}
		})
	}
}
