package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
)

// это логика нашего перехватчика(прометеуса), UnaryInterceptor - это перехватчик ивентов, в него поступает ивент, он увеличивает счётчик
// этот перехватчик мы регестрируем в app.go передаём в регистрацию наешего gRPC сервера
/*
gRPCServer := grpc.NewServer(
    grpc.UnaryInterceptor(metrics.UnaryInterceptor()),
)
*/

// ПРИМЕЧАНИЕ прометеус работает по страому протоколу http и не принимает сжатые proto данные(он не умеет их расшифровывать)
// рещаеться слудующим образом:
// gRPC-сервер (порт :50051) принимает запросы и обновляет метрики в оперативке.
// Вспомогательный HTTP-сервер (порт :2112), который мы открыли читает их из оперативки и отдает Прометеусу на /metrics.
// в app_main.go:
//mux := http.NewServeMux()
//mux.Handle("/metrics", promhttp.Handler())
/*
go func() {
    http.ListenAndServe(":2112", mux)
}()
*/

// создаём 3 перменные счётчика, значение которых
var (
	// Counter - только растёт вверх, считает общее количество запрососв и делит их на успешные и не успешные за счёт второго ярлыка
	requestsTotal = promauto.NewCounterVec( // NewCounterVec - New_новая Vec_таблица, для счётчика типа Counter Vec - таблица состоящая из заданных ярлыков и их количества, если нужно например узнать операция какого типа была совершена и удачно или с ошибкой, то как в этос случае, в конце указываем что каждый раз, когда я буду менять(добавлять в) эту переменную, обязательно должны передаваться 2 ярлыка []string{"method", "status"}
		prometheus.CounterOpts{
			Name: "grpc_requests_total",           // так этот график будет называться в графане
			Help: "Total number of gRPC requests", // просто подсказка или же описание
		},
		[]string{"method", "status"}, // указываем ярлыки, которы обязательно должен получить прометекс, что бы обновить счётчик
	)

	// Histogram - считает рвспределение времени
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "grpc_request_duration_seconds",
			Help: "Duration of gRPC requests",
		},
		[]string{"method"},
	)

	// Gauge - может расти и падать,
	// нужен нам для того, что бы считать сколько запрососв находяться внутри сервера,
	// если вдруг это число застыло, значит сервер завис
	inFlightRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "grpc_requests_in_flight",
			Help: "Current number of gRPC requests being processed",
		},
		[]string{"method"},
	)
)

/*
Функция UnaryInterceptor() — это просто фабрика. Мы вызываем её один раз
при старте сервера (metrics.UnaryInterceptor()), и она производит и отдает
серверу ту самую внутреннюю функцию-правило(return func). Сервер берет эту внутреннюю
функцию, сохраняет её у себя в памяти и вызывает её миллионы раз — для
каждого нового запроса(пришёл запрос, вызвался вот этот метод, который мы возвращаем (return func)).
*/
func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func( // "Я не знаю, как ты хочешь логировать запросы. Но если ты хочешь, чтобы я пропускал запросы через твой фильтр, дай мне правило (функцию). Я буду сам вызывать это правило каждый раз, когда ко мне придет новый клиент".
		ctx context.Context, // если пользователь отменяет запрос(закрывает приложение например), в ctx приходит сигнал об отмене
		req interface{}, // это данные которые прислал клиент своим действием(protobuf структура), interface - означает любой тип данных, потому что мы не знаем какой из четфрёх CRUD запросов именно нам придёт
		info *grpc.UnaryServerInfo, // Это сопроводительный лист, в нём написано куда клиент хочет попасть, именно от сюда мы берём info.FullMethod - то есть строку вида /user.v1.UserService/GetUser, что бы сказать прометеусу, какой метод сейчас вызываеться и передать его в качестве ярлыка
		handler grpc.UnaryHandler, // это ссылка на наш реальный хендлер, после обновления счётчиков, мы передаём запрос в хендлер, это буквально кнопка продолжить
	) (interface{}, error) {

		// запрос вошёл в систему, увеличиваем счётчик на 1 за счёт Inc() - increment, передаём тип ивента за счёт WithLabelValues(info.FullMethod)
		inFlightRequests.WithLabelValues(info.FullMethod).Inc()

		// по выходу запроса из системы, снимаем счётчик на 1, что бы понимамать что запрос больше не обрабатываеться, не висит в системе и всё хорошо и продолжает работать
		defer inFlightRequests.WithLabelValues(info.FullMethod).Dec()

		// запускаем таймер(сколько запрос обрабатывался в системе)
		timer := prometheus.NewTimer(requestDuration.WithLabelValues(info.FullMethod))
		defer timer.ObserveDuration()
		// !! таким образом результат таймера сразу записываеться в счётчик и нам не придётся в одной части кода запускать тайме, а потом останавливать другой, тут всё делаеться за счёт defer, и в конце не придёться переводить результат таймера в секунды и записывать в счётчик requestDuration. Всё делаеться в 2 строчки!!!

		resp, err := handler(ctx, req)

		status := "success"
		if err != nil {
			status = "error"
		}

		// сколько запросов пришло в систему всего, помечаем, какой запрос успешно прошёл, а какой с ошибкой
		requestsTotal.WithLabelValues(info.FullMethod, status).Inc()

		return resp, err
	}
}
