-include .env
export

# Автоматическое определение ОС (Windows или Linux/Mac)
ifeq ($(OS),Windows_NT)
    SLEEP_CMD = powershell -Command "Start-Sleep -Seconds 12"
else
    SLEEP_CMD = sleep 12
endif

.PHONY: infra-up infra-down test-integration test-all

# 1. Поднимаем только инфраструктуру
infra-up:
	docker compose up -d postgres kafka-1 kafka-2 kafka-3 redis-1 redis-2 redis-3 redis-cluster-init init-kafka
	@echo Waiting 12 seconds for cluster initialization...
	@$(SLEEP_CMD)
	@echo Infrastructure is ready!

# 2. Очищаем инфраструктуру и вольюмы
infra-down:
	docker compose down -v

# 3. Запускаем интеграционные тесты
test-integration:
	@echo Running tests...
	go test ./... -v

# 4. Команда "Всё под ключ"
test-all: infra-up test-integration infra-down