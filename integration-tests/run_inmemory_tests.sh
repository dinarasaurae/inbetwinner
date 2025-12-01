#!/bin/bash

echo "Запуск интеграционных тестов в тестовом окружении (In-Memory)"
echo "============================================================="

echo "Проверка зависимостей Go..."
if ! command -v go &> /dev/null; then
    echo "Go не установлен. Установите Go 1.21+ и повторите попытку."
    exit 1
fi

echo "Установка зависимостей..."
go mod tidy

if [ $? -ne 0 ]; then
    echo "Ошибка при установке зависимостей"
    exit 1
fi

echo "Зависимости установлены успешно"
echo ""

echo "Запуск in-memory интеграционных тестов..."
echo "Используемые компоненты:"
echo "  - SQLite (in-memory база данных)"
echo "  - miniredis (in-memory Redis)"
echo "  - httptest (mock HTTP серверы)"
echo ""

go test -v -run="TestInMemoryEnvironment" -count=1

if [ $? -eq 0 ]; then
    echo ""
    echo "Все in-memory интеграционные тесты прошли успешно!"
    echo ""
    echo "Статистика:"
    echo "  - Использована SQLite база данных в памяти"
    echo "  - Протестирован miniredis для rate limiting"
    echo "  - Проверены mock HTTP серверы"
    echo "  - Время выполнения: ~1-2 секунды (быстрее реальных сервисов)"
    echo ""
    echo "Преимущества in-memory тестирования:"
    echo "  - Быстрое выполнение без Docker"
    echo "  - Не требует внешних зависимостей"
    echo "  - Изолированная среда для каждого теста"
    echo "  - Подходит для CI/CD pipeline"
else
    echo ""
    echo "Некоторые тесты завершились с ошибкой"
    echo "Проверьте вывод выше для деталей"
    exit 1
fi