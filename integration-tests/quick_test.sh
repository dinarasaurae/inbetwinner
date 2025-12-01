#!/bin/bash

echo "Быстрая проверка интеграций inBeTwin"
echo "===================================="

echo "Запуск быстрых in-memory тестов..."
go test -v -run="TestInMemoryEnvironment" -timeout=30s

if [ $? -eq 0 ]; then
    echo "Быстрые тесты прошли успешно!"

    echo ""
    echo "Проверка доступности реальных сервисов..."

    if curl -s http://localhost:8080/health > /dev/null; then
        echo "API Gateway доступен"

        if curl -s http://localhost:8081/health > /dev/null; then
            echo "Auth Service доступен"
            echo ""
            echo "Система готова к полному тестированию!"
            echo "Запустите: ./run_integration_tests.sh"
        else
            echo "Auth Service недоступен (запустите docker-compose up -d)"
        fi
    else
        echo "Сервисы не запущены (запустите docker-compose up -d)"
    fi
else
    echo "Быстрые тесты не прошли - проверьте код"
    exit 1
fi