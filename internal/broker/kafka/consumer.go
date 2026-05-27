package kafka

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Derbik-Git/user-service/internal/domain"
	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafka.Reader
	log    *slog.Logger
	//(Якобы это другой сервис) сюда можно вставить добавиь сервис, что бы потом добавить в StartKafkaConsumer функцию из сервиса для проверки идемпотентности
}

func NewConsumer(brokers []string, topic string, groupID string, log *slog.Logger) *Consumer {
	return &Consumer{
		log: log,
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			GroupID: groupID, // Невыносимо важно запомнить! !!!!! это как раз нужно для насущного вопроса. А что если вдруг мы отмасштабируем наш сервис и множество его копий будут пытаться читать из одного консьюмера сообщение, то будет онка данных, а при добавлении GroupID Если ты запустишь 3 экземпляра своего приложения (например, 3 контейнера в Docker) и дашь им одинаковый GroupID = "user-service-group", Kafka поймет, что это одна команда работников. Она отдаст первому приложению Партицию 1, второму — Партицию 2, третьему — Партицию 3. Они будут читать данные параллельно, разделяя нагрузку.
			Topic:   topic,   // Указываем название топика, из которого будет читать консьюмер, передаём константу из domain, которую мы указывали для продюсера

			MinBytes: 10e3, // минимальный размер сообщения, меньше которого консьюмер даже не будет брать
			MaxBytes: 10e6, // MinBytes наоборот, максимальный размер сообщения, который консьюмер может принять
		}),
	}
}

func (c *Consumer) StartKafkaConsumer(ctx context.Context) {

	// беконечный цикл, что бы консьюмер постоянно слушал топик, это базовая настройка для любого консьюмера
	for {
		m, err := c.reader.FetchMessage(ctx) // Метод читает, но не подтверждает выполнение сразу, что бы можно было за счёт continue в случае возникновениея ошибки можно было вернуться к повтрной поытке прочитать это сообщение 1. мы оставляем закладку, что работаем с этим сообщением
		if err != nil {
			if ctx.Err() != nil {
				c.log.Info("kafka consumer stopping due to context cancellation")
				return
			}
			c.log.Error("failed to fetch message", slog.Any("error", err))
			continue
		}

		var event domain.UserEvent
		if err := json.Unmarshal(m.Value, &event); err != nil {
			c.log.Error("failed to unmarshall event", slog.Any("error", err))
		} else {
			c.log.Info("processing event", slog.String("type", event.Type), slog.Int64("user_id", event.Payload.ID))
		}

		// на этом месте должен быть метод из сервиса, который обеспечивает идемпотентность проверяет это сообщение есть ли оно в таблице проверки идемпотентности и если от туда пришла ошибка -> читай ниже
		// !!!!! в этой логике, когда что то не получается и мы обрабатваем ошибку, например база данных не доступна, мы делаем continue(и продолжаем пытаться достать это сообщение, потому что FetchMessage извлекает, но не даёт сигнал о доставке сообщения, и при возникновении ошибки мы пишем continue и каждый раз продолжаем работать над этим сообщением), то есть не доходим до подтверждения выполнения операции, а возвращаемся заного к этой закладке_1._(сообщению), за счёт того что мы использовали FetchMessage, таким образом оставив закладку и сказав, что мы работаем над этим сообщением, и пока его не обработаем, от него не отойдём. Короче мы в начачале за счёт FetchMessage, говорим что работаем именно над этим сообщением и при ошибке пишем continue и возвращаемся его получить целостно заного, не следующее, а из за того, что мы сказали за счёт FetchMessage что начали работу над ним и работаем над ним и в случае возникновения ошибки будем дальше пробовать его извлечь из брокера.

		// подтверждение выполнения операции
		if err := c.reader.CommitMessages(ctx, m); err != nil {
			c.log.Error("failed to commit message", slog.Any("error", err))
		}
	}
}

// используется при выключении сервиса, что бы закрыть соединение с брокером kafka
func (c *Consumer) Close() error {
	return c.reader.Close()
}
