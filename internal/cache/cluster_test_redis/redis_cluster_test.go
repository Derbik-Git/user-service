package clusterTest

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

/*
✔ slot hashing
✔ node distribution
✔ MOVED redirect
✔ cluster routing
✔ multi-key ограничения
✔ hash-tags
✔ concurrency
✔ slot migration (реальный)
✔ topology
✔ hot key
✔ network issues
*/

// вспомогательная функция контекст
func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// проверяем что кластер находится в состоянии OK
func TestCluster_StateOK(t *testing.T) {
	ctx := testCtx(t)

	info, err := env.Client.ClusterInfo(ctx).Result() //достаём инфо о клстере
	require.NoError(t, err)                           // проверяем что всё хорошо и клстер не вернул ошибку

	require.Contains(t, info, "cluster_state:ok")
}

// Проверяем на большое ли количество слотов из 16384 распределяются ключи | если 1000 ключей распределятся всего лишь на 5 слотов, это может отрицательно сказатся на работоспособности redis cluster, но если к примеру 1000 ключей распределились более чем по 1000 слотам, то всё хорошо(НАГРУЗКА РАСПРЕДЕЛЯЕТСЯ КОРРЕКТНО) | 1 ключь != 1 слот
func TestCluster_RealSlotDistribution(t *testing.T) {
	ctx := testCtx(t)

	slots := make(map[int]bool) // !!! здесь [int] - это будет именно номер слота redis, потому что ClusterKeySlot возвращает номер слота, мы будем в эту мапу класть все номера слотов и потом будем смотреть в какую длинну вышла мапа что бы посчитать достаточно ли redis задействовал слотов для распределения нагрузки по слотам

	for i := 1; i <= 200; i++ {
		key := fmt.Sprintf("slot:%d", i)

		slot, err := env.Client.ClusterKeySlot(ctx, key).Result() // Функция ClusterKeySlot возвращает номер слота, в который попадает данный ключ.
		require.NoError(t, err)

		slots[int(slot)] = true // Здесь в карту slots добавляется элемент с ключом, равным номеру слота, и значением true.
	}

	require.Greater(t, len(slots), 50) // смотрим превышвет ли у нас количество задействованных слотов, число 50, таким образом смотрим, по достаточному ли количеству слотов redis распределил ключи, проверяем именно на больше 50, потмоу что чем меньше, тем больше нагрузка для redis
}

//!!!!
// Проверяем как redis перенаправляет запросы между нодами
// Если драйвер отправит запрос GET user:1 на ноду, которая не владеет слотом для этого ключа, Redis ответит ошибкой вида:
// -MOVED 3999 127.0.0.1:6379
/*Эта ошибка говорит драйверу:
* MOVED: Твой ключ переехал.
* 3999: Номер слота, который ты искал.
* 127.0.0.1:6379: Адрес ноды, где этот слот лежит сейчас. */
// Умный драйвер в таком случае должен Обновить свою внутреннюю таблицу соответствий (слот -> адрес). 2. Повторно отправить тот же самый запрос уже на правильный адрес.
// Получается что мы перепроверяем нормально ли redis достаёт ключи с других нод !!!! - сама суть, того, чего мы проверяем

/*
В интеграционных тестах для кластера env.Client — это не просто подключение к одной ноде. Это Cluster Client.

Когда ты вызываешь env.Client.Set(...), происходит следующее:

1. Драйвер вычисляет хэш ключа user:1.
2. Он смотрит в свою таблицу: «Куда мне отправить этот запрос?».
3. Сценарий А (Таблица пуста): Если это первый запрос или таблица устарела, драйвер отправляет запрос на любую доступную ноду (например, на первую в списке).
4. Нода отвечает: -MOVED ....
5. Драйвер ловит эту ошибку, перестраивает карту слотов и автоматически повторяет запрос на правильную ноду.
6. Данные записываются.

Когда ты вызываешь env.Client.Get(...):

1. Драйвер снова вычисляет хэш ключа user:1.
2. Теперь в его таблице уже есть актуальная информация (благодаря шагу выше).
3. Он сразу отправляет запрос на правильную ноду.
4. Данные читаются.
*/
// В самом кратце, если redis не перераспределяет нормально ключи между нодами, то просто тест завершится с ошибкой так как он вызвав метод get не увидит ключа в просматриваемой ноде, не сможет достать из другой и выдаст ошибку(вот это проблему связи между нодами мы и проверяем)
func TestCluster_MOVEDRedirect(t *testing.T) {
	ctx := testCtx(t)

	for i := 1; i <= 50; i++ {

		key := fmt.Sprintf("%s:user:1", t.Name())

		_, err := env.Client.Set(ctx, key, "Bob"+t.Name(), 0).Result()
		require.NoError(t, err)

		result, err := env.Client.Get(ctx, key).Result()
		require.NoError(t, err)
		require.Equal(t, "Bob"+t.Name(), result)
	}
}

// В Redis есть команды, которые работают сразу с несколькими ключами, и нам нужно следить что бы redis не мог работать сразу с двумя разными ключами из разных нод и возвращал ошибку CROSSSLOT, потому что если ты например решил удалить 2 ключа, указываешь ключь user:1 и ключь user:2(из за сильно отличающегося окончания очень вероятно что они будут в разных слотах), redis захватит ноду с первым ключом, начнёт выполнять операцию, а доступа ко второму ключу нету(функция не выполняет поставленную задачу удалить 2 ключа), так как он находится на другой ноде и redis вернёт ошибку CROSSSLOT, !!! логикой redis подрузомевается выдавать такую ошибку, потому что если бы было возможно выполнять операции с мульти ключами из разных нод, это значительно бы замедляла работу redis, поэтому мы наоборот следим что бы redis возвращал данную ошибку | опять же говорю что можно сделать так что бы redis работал с мульти ключами на разных нодах, но это будет серьёзно влиять на производительность, как это вообще осуществить, можно спросить у нейросети, но такая практика не пользуется спросом, во первых из за производительности, во вторых в настоящих проектах нету задач, где такая практика нужна была бы, иначе можно было удалять 2 ключа одновременно с помощью postgres
// с MultiSet это работает так, что захватывается нода куда будет создаваться первый ключь, а так как ококначания сильно отличаются, второй ключь должен попасть в другой слот, а к нему доступа быть не должно, поэтому ожидаем ошибку
func TestCluster_MultiKeyRestruction(t *testing.T) {
	ctx := testCtx(t)

	err := env.Client.MSet(ctx,
		fmt.Sprintf("%s:user:1", t.Name()), "Ivan",
		fmt.Sprintf("%s:user:2", t.Name()), "Maksim",
	).Err()

	require.Error(t, err) // ещё раз повторю, ожидаем ошибку потому что redis не может выполнять запросы с мульти ключами из разных нод так как нету доступа к другой ноде, если действия совершаются с одной нодой. Redis может выполнять опрации одновременно только с одной нодой, если бы redis мог работать с двумя нодами одновременно(что можно сделать), то от этого сильно бы упала производительность, что теряет смысл основоного приемущества redis(скорости)
}

// {} - вот такие ковычки можно использовать что бы ключи гарантированно были на одной ноде
// Тут мы будем проверять что мульти операции проходят успешно, если 2 разных ключи находятся на одной ноде(это мы гарантируем за счёт этих ковычек {})
func TestCluster_HashTagMultiKey(t *testing.T) {
	ctx := testCtx(t)

	err := env.Client.MSet(ctx,
		"user:{1}:name", "Bob",
		"user:{1}:email", "Kop",
	).Err()

	require.NoError(t, err)
}

// Проверяем парралельные записи(нагрузка)
// Тестируем concurrency и Thread safety клиента
func TestCluster_ParallelWrites(t *testing.T) {
	ctx := testCtx(t)

	var wg sync.WaitGroup

	for i := 1; i <= 200; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			key := fmt.Sprintf("parallel:%d", i)

			err := env.Client.Set(ctx, key, "value", 0).Err()
			if err != nil {
				t.Errorf("write %d failed: %v", i, err)
			}
		}(i)
	}

	wg.Wait()
}

// !! проверяет, что ключи распределяются по разным(нодам) узлам кластера равномерно, а не попадают все в один «узкий» слот.
func TestCluster_NodeDistribution(t *testing.T) {
	ctx := testCtx(t)

	nodeHits := make(map[string]int) // это словарь, где ключом будет адрес узла (например, 127.0.0.1:7000), а значением — количество ключей, которые попали на этот узел.

	slotsInfo, err := env.Client.ClusterSlots(ctx).Result() // Получает карту слотов. запрашивает у кластера информацию о том, какие диапазоны слотов (например, 0–5460, 5461–10922) обслуживаются какими узлами. Результат — это список структур, где для каждого диапазона указан адрес мастера.
	require.NoError(t, err)

	for i := 0; i < 200; i++ {

		key := fmt.Sprintf("dist:%d", i)

		require.NoError(t, env.Client.Set(ctx, key, "value", 0).Err())

		slot, err := env.Client.ClusterKeySlot(ctx, key).Result() // Это специальная команда, которая не записывая данные, вычисляет, какому слоту принадлежит ключ по его имени.
		require.NoError(t, err)
		slotInt := int(slot)

		for _, s := range slotsInfo { // Зная номер слота (например, 1234), тест пробегает по списку slotsInfo и находит тот диапазон(ноду | диапозон слотов это нода, потому что нода хранит диапопзон слотов), в который попадает этот слот.
			if slotInt >= s.Start && slotInt <= s.End {
				nodeHits[s.Nodes[0].Addr]++ // Как только диапазон найден, он берёт адрес мастера из этого диапазона (s.Nodes[0].Addr) и увеличивает счётчик для этого адреса в словаре nodeHits.
				break
			}
		}
	}

	require.Greater(t, len(nodeHits), 1) // Проверяет, что ключи попали более чем на один узел(ноду). Если бы все 200 ключей попали на один узел, длина словаря была бы равна 1, и тест бы упал. Это защищает от ситуации «горячего узла».

	for _, hits := range nodeHits {
		require.Greater(t, hits, 10) // Проверяет, что на каждый из задействованных узлов попало более 10 ключей.
	}
}

// Записываем переменные на разные узлы кластера(ноды), за счёт отличных окончаний ключей друг от друга, переменные записываются в разные слоты.
// Тем самым, за счёт создания и получения пользователей мы проверяем работоспособность каждой ноды и нормально ли вообще мы достаём ключи с разных нод
func TestCluster_CrossNodeAccess(t *testing.T) {
	ctx := testCtx(t)

	keys := []string{
		fmt.Sprintf("%s:user:1", t.Name()), fmt.Sprintf("%s:user:2", t.Name()), fmt.Sprintf("%s:user:3", t.Name()), fmt.Sprintf("%s:user:1000", t.Name()),
	}

	for _, k := range keys {
		require.NoError(t, env.Client.Set(ctx, k, k, 0).Err())
	}

	for _, k := range keys {
		val, err := env.Client.Get(ctx, k).Result()
		require.NoError(t, err)
		require.Equal(t, k, val)
	}
}

func TestCluster_SlotMigration(t *testing.T) {
	ctx := testCtx(t)

	key := "migration:test"

	require.NoError(t, env.Client.Set(ctx, key, "value", 0).Err())

	slot, _ := env.Client.ClusterKeySlot(ctx, key).Result()
	slots, _ := env.Client.ClusterSlots(ctx).Result()
	slotInt := int(slot)

	var srcAddr, dstAddr string

	for _, s := range slots {
		if slotInt >= s.Start && slotInt <= s.End {
			srcAddr = s.Nodes[0].Addr
		} else if dstAddr == "" {
			dstAddr = s.Nodes[0].Addr
		}
	}

	src := redis.NewClient(&redis.Options{Addr: srcAddr})
	dst := redis.NewClient(&redis.Options{Addr: dstAddr})

	defer src.Close()
	defer dst.Close()

	srcID, _ := src.Do(ctx, "CLUSTER", "MYID").Text()
	dstID, _ := dst.Do(ctx, "CLUSTER", "MYID").Text()

	require.NoError(t, src.Do(ctx, "CLUSTER", "SETSLOT", slot, "MIGRATING", dstID).Err())
	require.NoError(t, dst.Do(ctx, "CLUSTER", "SETSLOT", slot, "IMPORTING", srcID).Err())

	require.NoError(t, src.Do(ctx, "MIGRATE",
		dstAddr[:strings.Index(dstAddr, ":")],
		dstAddr[strings.Index(dstAddr, ":")+1:],
		key, 0, 5000,
	).Err())

	require.NoError(t, src.Do(ctx, "CLUSTER", "SETSLOT", slot, "NODE", dstID).Err())
	require.NoError(t, dst.Do(ctx, "CLUSTER", "SETSLOT", slot, "NODE", dstID).Err())

	val, err := env.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	require.Equal(t, "value", val)
}

/*
В кластере Redis при большом количестве операций записи может запускаться процесс перераспределения слотов (ребалансировка).
  - Тест проверяет, что кластер продолжает корректно работать и принимать запросы во время этого процесса.
*/
func TestCluster_BulkRecordBack(t *testing.T) {
	ctx := testCtx(t)

	for i := 1; i <= 200; i++ {
		key := fmt.Sprintf("rebalanc:%d", i)

		require.NoError(t, env.Client.Set(ctx, key, "value", 0).Err())
	}
}

// Этот тест предназначен для проверки корректной работы кластера Redis с так называемым «горячим ключом» — то есть ключом, к которому одновременно обращается большое количество запросов.
func TestCluster_HotKey(t *testing.T) {
	ctx := testCtx(t)

	hotKey := "hot:Key"

	require.NoError(t, env.Client.Set(ctx, hotKey, "testHotKey", 0).Err())

	var wg sync.WaitGroup
	for i := 1; i <= 200; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := env.Client.Get(ctx, hotKey).Result()
			if err != nil {
				t.Errorf("hot key failed: %v, gorutine:%d", err, i)
			}
		}()
	}

	wg.Wait()
}

// Этот тест проверяет не только благополучное завершение по таймауту, но и корректную обработку сетевых таймаутов самим клиентом Redis.
func TestCluster_NetworkTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(1 * time.Millisecond) // гарант того, что таймаут наступит, до завершеия теста

	err := env.Client.Set(ctx, "net:test", "valNetTest", 0).Err()
	require.Error(t, err)
}

func TestCluster_NodesInfo(t *testing.T) {
	ctx := testCtx(t)

	nodes, err := env.Client.ClusterNodes(ctx).Result() // достаёт информацию о нодах нашего кластера
	require.NoError(t, err)

	require.Contains(t, nodes, "master")    // подтверждает что в кластрее есть узлы, выполняющие роль кластера
	require.Contains(t, nodes, "connected") // ищем слово connected в строке nodes для проверки успешного подключения
}

// Проверяем столько ли у нас мастеров, сколько мы запустили на самом деле.
func TestCluster_SlotsInfo(t *testing.T) {
	ctx := testCtx(t)

	slots, err := env.Client.ClusterSlots(ctx).Result() // этот метод возвращет массив, длинной в количество мастеров, реплики этому массиву длинны не прибаляют
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(slots), 3) // смотрим равна ли длинна массива, количеству мастеров, которое мы задали
}

// !!!! ВАЖНАЯ ПРОВЕРКА FAILOWER // что будет в случае падения контейнера, как отработает реплика
func TestCluster_Failover_Auto(t *testing.T) {
	ctx := testCtx(t)

	key := "failover:auto"

	// 1. Записываем данные
	require.NoError(t, env.Client.Set(ctx, key, "value", 0).Err())

	// 2. Получаем слот
	slot, err := env.Client.ClusterKeySlot(ctx, key).Result()
	require.NoError(t, err)

	slotInt := int(slot)

	// 3. Получаем информацию о слотах
	slots, err := env.Client.ClusterSlots(ctx).Result()
	require.NoError(t, err)

	var masterAddr string

	for _, s := range slots {
		if slotInt >= s.Start && slotInt <= s.End {
			masterAddr = s.Nodes[0].Addr
			break
		}
	}

	require.NotEmpty(t, masterAddr)

	t.Logf("Master node: %s", masterAddr)

	// 4. Маппинг addr → docker container
	container := mapAddrToContainer(masterAddr)
	require.NotEmpty(t, container)

	t.Logf("Killing container: %s", container)

	// 5. Убиваем контейнер
	require.NoError(t, dockerStop(container))

	// Это замена обычному time.Sleep, это нужно для того, что бы тест не зависал просто так на 15 секунд, потому чтот если нода поднялась за 4 секунды, то мы теряем 11 секунд просто так
	// этот метод внутри себя запускает переданную ему функцию и крутит её циулом (в нашем случае) 15 секунд и проводит опрос (	if err == nil && val == "value" ) каждые 500 миллисекунд
	// Даем кластеру до 15 секунд на перестроение, проверяя каждые 500 миллисекунд
	require.Eventually(t, func() bool {
		val, err := env.Client.Get(ctx, key).Result()
		return err == nil && val == "value"
	}, 15*time.Second, 500*time.Millisecond, "Кластер не смог восстановиться после падения мастера")

	require.NoError(t, dockerStart(container))

	// ожидаем и так же проверяем, когда новая нода войдёт в строй
	require.Eventually(t, func() bool {
		info, _ := env.Client.ClusterInfo(ctx).Result()
		return strings.Contains(info, "cluster_state:ok")
	}, 10*time.Second, 500*time.Millisecond, "Кластер не смог восстановиться после падения мастера")

}

// вспомогательные функции
func mapAddrToContainer(addr string) string {
	// Самый надежный способ - определять контейнер по порту,
	// так как порты уникально привязаны к контейнерам (7001 = redis-1 и т.д.)
	if strings.Contains(addr, "7001") {
		return "redis-1"
	}
	if strings.Contains(addr, "7002") {
		return "redis-2"
	}
	if strings.Contains(addr, "7003") {
		return "redis-3"
	}
	if strings.Contains(addr, "7004") {
		return "redis-4"
	}
	if strings.Contains(addr, "7005") {
		return "redis-5"
	}
	if strings.Contains(addr, "7006") {
		return "redis-6"
	}
	return ""
}

func dockerStop(container string) error {
	cmd := exec.Command("docker", "stop", container)
	return cmd.Run()
}

func dockerStart(container string) error {
	cmd := exec.Command("docker", "start", container)
	return cmd.Run()
}
