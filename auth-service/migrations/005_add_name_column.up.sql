-- Добавление колонки name для интеграционных тестов (если её нет)
ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR(255);

-- Заполняем name из first_name + last_name для существующих записей
UPDATE users SET name = CONCAT(first_name, ' ', last_name) WHERE first_name IS NOT NULL OR last_name IS NOT NULL;