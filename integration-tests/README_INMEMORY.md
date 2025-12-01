# In-Memory Integration Testing

## Обзор

Дополнительный набор интеграционных тестов, использующий in-memory компоненты вместо реальных сервисов для более быстрого выполнения и простоты настройки.

## Компоненты тестового окружения

### SQLite In-Memory Database
- **Библиотека:** `github.com/mattn/go-sqlite3`
- **Назначение:** Замена PostgreSQL для тестирования БД операций
- **Преимущества:** Не требует установки PostgreSQL, быстрая инициализация

### miniredis
- **Библиотека:** `github.com/alicebob/miniredis/v2`
- **Назначение:** Полная имитация Redis server в памяти
- **Поддерживаемые операции:** SET, GET, INCR, EXPIRE, HSET, LPUSH, SADD
- **Преимущества:** Не требует установки Redis, поддерживает все Redis команды

### httptest Mock Servers
- **Стандартная библиотека:** `net/http/httptest`
- **Назначение:** Имитация API Gateway и Auth Service
- **Возможности:** Полная имитация HTTP endpoints с custom логикой


## Покрытые сценарии

### 1. HealthChecks
- Проверка доступности mock сервисов
- Валидация Redis подключения
- JSON response форматирование

### 2. UserRegistrationFlow
- Регистрация через API Gateway proxy
- Сохранение в SQLite database
- Проверка уникальности email
- Валидация сохраненных данных

### 3. LoginAndAuthentication
- JWT token генерация (mock)
- Protected routes доступ
- Authorization header обработка

### 4. RateLimitingWithRedis
- miniredis rate limiting логика
- TTL (Time To Live) обработка
- Превышение лимитов запросов

### 5. DatabaseTransactions
- SQLite транзакционность
- ACID properties проверка
- Rollback поведение

### 6. RedisDataTypes
- Strings, Hashes, Lists, Sets
- Все основные Redis операции
- Expiration handling

### 7. ErrorHandling
- Duplicate user registration
- Unauthorized access attempts
- HTTP error codes validation

## Запуск тестов

### Автоматический запуск
```bash
cd integration-tests
./run_inmemory_tests.sh
```

### Ручной запуск
```bash
cd integration-tests
go mod tidy
go test -v -run="TestInMemoryEnvironment"
```

### Запуск конкретного подтеста
```bash
go test -v -run="TestInMemoryEnvironment/UserRegistrationFlow"
```


## Сравнение с реальными сервисами

| Аспект | Реальные сервисы | In-Memory |
|--------|------------------|-----------|
| **Время выполнения** | ~39 секунд | ~0.2 секунды |
| **Зависимости** | Docker, PostgreSQL, Redis | Только Go |
| **Настройка** | docker-compose up | Нет |
| **Изоляция** | Между контейнерами | Полная |
| **CI/CD пригодность** | Требует Docker | Отлично |
| **Реалистичность** | 100% | 85% |

## Когда использовать

### In-Memory тесты подходят для:
- Быстрой проверки логики взаимодействий
- CI/CD pipeline без Docker
- Локальной разработки
- TDD (Test Driven Development)
- Smoke testing

### Реальные сервисы нужны для:
- Production-like тестирования
- Performance тестирования
- Network latency проблем
- Docker configuration проверки
- Full end-to-end validation
