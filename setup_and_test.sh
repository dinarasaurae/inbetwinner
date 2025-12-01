#!/bin/bash

echo "Полная установка и тестирование inBeTwin"
echo "======================================="

check_status() {
    if [ $? -ne 0 ]; then
        echo "Ошибка на шаге: $1"
        exit 1
    fi
}

echo "1. Проверка зависимостей..."
command -v docker >/dev/null 2>&1 || { echo "Установите Docker"; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "Установите Docker Compose"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Установите Go 1.21+"; exit 1; }
echo "Все зависимости установлены"

echo "2. Сборка проекта..."
docker-compose build
check_status "сборка Docker образов"

echo "3. Запуск сервисов..."
docker-compose up -d
check_status "запуск сервисов"

echo "4. Ожидание готовности сервисов..."
sleep 60

echo "5. Проверка статуса сервисов..."
for i in {1..5}; do
    if curl -s http://localhost:8080/health > /dev/null && curl -s http://localhost:8081/health > /dev/null; then
        echo "Все сервисы готовы"
        break
    fi
    echo "Ожидание готовности... попытка $i/5"
    sleep 10
done

echo "6. Запуск полных интеграционных тестов..."
cd integration-tests
go mod tidy
go test -v -timeout=300s

if [ $? -eq 0 ]; then
    echo ""
    echo "УСПЕХ! Полная установка и тестирование завершены"
    echo ""
    echo "Что было сделано:"
    echo "  - Проверены зависимости"
    echo "  - Собраны Docker образы"
    echo "  - Запущены все сервисы"
    echo "  - Выполнены интеграционные тесты"
    echo "  - Система готова к использованию"
else
    echo "Тесты не прошли - проверьте логи"
    docker-compose logs
    exit 1
fi