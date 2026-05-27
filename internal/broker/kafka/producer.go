package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	kafkaWriter *kafka.Writer
}

// Это функция настройки продюсера, она вызывается один раз при запуске го приложения, передаются адреса kafka черверов, функция устанавливает с ними постоянное сетевое соединение(трубу) и возвращает готовый producer, который мы используем для отправки сообщений p.kafkaWriter.WriteMessages(ctx, kafka.Message{ в функции PublishUserEvent
// передаётся: kafka.NewProducer([]string{"localhost:9091", "localhost:9092", ...}), это позволяетс продюсеру установить начальное соединение с кластером
func NewProducer(brokers []string) *Producer {
	return &Producer{
		kafkaWriter: &kafka.Writer{
			Addr:         kafka.TCP(brokers...), // принимает список адресов, по типу локалхоста, для установки начального соединения с кластером. Троеточие распаковывает элементы слайса на отдельные аргументы функции в данном случае это функция TCP
			Balancer:     &kafka.Hash{},         // балансировщик определяет в какую партицию отправлять сообщение. !!! Если у сообщения есть ключ (Key), Kafka вычисляет хэш от этого ключа и отправляет сообщение в партицию с номером hash % N, где N — общее число партиций в топике.
			Async:        false,                 // Этот параметр асинхронности отвечает за то, будет ли producer ждать подтверждения от брокера о том, что сообщение было сохранено и реплецированно на все оставшиеся брокеры, но в этот учёт не идёт ожидание получения сообщения об успешном получении данных со стороны другого микросервиса
			RequiredAcks: kafka.RequireAll,      // Ждем, пока ВСЕ 3 копии (реплики) запишутся на диски
			WriteTimeout: 10 * time.Second,      // Если за 10 сек брокер не ответил — выдаем ошибку
		},
	}
}

func (p *Producer) PublishUserEvent(ctx context.Context, topic string, eventType string, user *domain.User) error { // вызывается в сервисном слое
	event := domain.UserEvent{
		ID:        uuid.New().String(), // генерируем уникальный идентификатор для события, который может использоваться для отслеживания и обеспечения идемпотентности при обработке событий
		Type:      eventType,           // тип события ("user.created", "user.updated", "user.deleted")
		Payload:   *user,
		CreatedAt: time.Now(),
	}

	b, err := json.Marshal(event) // в kafka все данные передаются в байтах, поэтому на стороне producer мы серелизуем структуру в JSON, а на стороне consumer мы десерелизуем эти байты обратно в структуру что бы продолжать работать с ней в го коде
	if err != nil {
		return fmt.Errorf("marshall event: %w", err)
	}

	return p.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(strconv.FormatInt(user.ID, 10)), //(что бы операции над одним пользователем по user.ID попадали в одну партицию) таким образом переводим user.ID, который является ключом для kafka, в строку а затем в байты, потому что kafka принимает только байты
		Value: b,
	})
}
