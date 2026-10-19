-include .env
export

# Автоматическое определение ОС (Windows или Linux/Mac)
ifeq ($(OS),Windows_NT)
    SLEEP_CMD = powershell -Command "Start-Sleep -Seconds 12"
else
    SLEEP_CMD = sleep 12
endif

.PHONY: up down infra-up infra-down run test-integration test-all

# 1. Запустить вообще ВСЁ (инфраструктура + сервис + миграции + Grafana/Prometheus)
up:
	docker compose up -d --build
	@echo Все сервисы собраны и запущены!

# 2. Полная остановка и очистка всех контейнеров и вольюмов
down:
	docker compose down -v

# 3. Поднять ТОЛЬКО инфраструктуру (БД, Kafka, Redis) — для написания и отладки кода
infra-up:
	docker compose up -d postgres kafka-1 kafka-2 kafka-3 redis-1 redis-2 redis-3 redis-cluster-init init-kafka
	@echo Waiting 12 seconds for cluster initialization...
	@$(SLEEP_CMD)
	@echo Infrastructure is ready!

# 4. Остановить только инфраструктуру
infra-down:
	docker compose down -v

# 5. Локальный запуск Go-сервиса
run:
	go run ./cmd/main.go

# 6. Запуск интеграционных тестов внутри изолированной Docker-сети
test-integration:
	@echo Running service-postgres-redis tests...
	docker compose run --rm test-runner go test -p 1 -v ./internal/service/service-postgres-redis_test

# 7. Запуск тестов "под ключ" (поднять инфра -> прогнать тесты -> всё потушить)
test-all: infra-up test-integration infra-down