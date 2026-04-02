# inBeTwin — Smoke-test & Integration Runbook

Этот документ описывает как поднять стек через Docker Compose и проверить
production path end-to-end: входящее VK сообщение → draft в БД → approve → отправка в VK.

---

## 1. Обязательные переменные окружения

Создайте `.env` в корне репозитория:

```dotenv
# ─── LLM ─────────────────────────────────────────────────────────────────────
# Вариант A — OpenAI
LLM_PROVIDER=openai
OPENAI_API_KEY=sk-...

# Вариант B — Groq (бесплатный tier)
# LLM_PROVIDER=groq
# LLM_API_KEY=gsk_...

# Вариант C — OpenRouter (100+ моделей)
# LLM_PROVIDER=openrouter
# LLM_API_KEY=sk-or-...

# Вариант D — любой OpenAI-совместимый сервер
# LLM_PROVIDER=openai_compatible
# LLM_API_KEY=your-key
# LLM_BASE_URL=https://your.llm.host/v1
# LLM_MODEL=mistral-7b-instruct

# ─── Режим оркестрации ────────────────────────────────────────────────────────
# legacy      — встроенная логика vk_agent без llm-service
# llm_service — всё через llm-service
# hybrid      — llm-service с fallback на legacy при ошибке/таймауте
LLM_ORCHESTRATION_MODE=llm_service

# ─── VK ──────────────────────────────────────────────────────────────────────
VK_APP_ID_WEB=...
VK_APP_SECRET_WEB=...
VK_REDIRECT_URI_WEB=http://localhost:3002/social/vk/oauth/user/callback
VK_REDIRECT_URI_GROUP=http://localhost:3002/social/vk/oauth/callback

# ─── RAG / Pinecone (опционально) ────────────────────────────────────────────
PINECONE_API_KEY=...
PINECONE_HOST=https://inbetwin-xxx.svc.pinecone.io

# ─── MinIO (работает локально без доп. ключей) ───────────────────────────────
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
```

---

## 2. Поднять стек

```bash
docker compose up -d
```

Дождитесь, пока все сервисы станут healthy:

```bash
docker compose ps
# Все должны показывать "healthy" или "running"
```

Проверить логи rag-service (MinIO + Pinecone):

```bash
docker compose logs rag-service --tail=30
```

---

## 3. Создание тестового workspace

### 3.1 Зарегистрировать пользователя

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"test@example.com","password":"testpass123","name":"Test User"}' | jq .
```

Сохраните `access_token` из ответа:

```bash
export TOKEN="eyJ..."
```

### 3.2 Подключить VK-группу

Выполните OAuth flow через `/api/v1/social/vk/oauth/start` в браузере
(или используйте мобильное приложение). После подключения группы вернётся
`integration_id`.

```bash
export INTEGRATION_ID="uuid-of-your-integration"
```

### 3.3 Настроить агента

```bash
curl -s -X PUT http://localhost:8080/api/v1/social/vk/agent/settings \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "'"$INTEGRATION_ID"'",
    "draft_first": true,
    "auto_reply_enabled": false,
    "safe_intents": ["faq","hours","basic_prices","qualification"],
    "tone_of_voice": "Дружелюбный и профессиональный",
    "escalation_policy": "Передай менеджеру если клиент хочет созвониться",
    "orchestration_mode": "llm_service"
  }' | jq .
```

---

## 4. Проверить e2e: draft создался

### 4.1 Симулировать входящее VK-сообщение

Отправьте реальное сообщение в вашу VK-группу через аккаунт пользователя.
Long Poll воркер автоматически подхватит его и запустит обработку.

Альтернативно — вставьте сообщение напрямую в БД:

```bash
docker compose exec postgres psql -U social_user -d social_db -c "
  INSERT INTO vk_messages
    (integration_id, from_vk_user_id, message_id, text, is_incoming, is_processed)
  VALUES
    ('$INTEGRATION_ID'::uuid, 777, 99999, 'Сколько стоит ваш продукт?', TRUE, FALSE)
  RETURNING id;
"
```

Сохраните `message_id`:

```bash
export MSG_ID="uuid-returned-above"
```

### 4.2 Вручную запустить генерацию черновика

```bash
curl -s -X POST http://localhost:8080/api/v1/social/vk/drafts/generate \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "'"$INTEGRATION_ID"'",
    "message_id": "'"$MSG_ID"'",
    "force": false
  }' | jq .
```

### 4.3 Проверить черновик в БД

```bash
docker compose exec postgres psql -U social_user -d social_db -c "
  SELECT
    id,
    status,
    orchestration_source,
    llm_latency_ms,
    fallback_reason,
    left(draft_text, 80) AS draft_preview,
    safe_intent,
    intent,
    prompt_tokens,
    completion_tokens
  FROM vk_reply_drafts
  WHERE integration_id = '$INTEGRATION_ID'::uuid
  ORDER BY generated_at DESC
  LIMIT 5;
"
```

Ожидаемый результат при `orchestration_mode=llm_service`:

| Поле | Ожидаемое значение |
|------|--------------------|
| `status` | `pending` |
| `orchestration_source` | `llm_service` |
| `llm_latency_ms` | > 0 |
| `fallback_reason` | NULL |
| `draft_preview` | Текст от LLM |

---

## 5. Проверить fallback

Временно поднять llm-service с неправильным API-ключом чтобы симулировать сбой:

```bash
docker compose stop llm-service
```

Или изменить `LLM_ORCHESTRATION_MODE=hybrid` и убить llm-service.
Запустите генерацию черновика снова (с новым сообщением).

Ожидаемый результат в БД:

| Поле | Ожидаемое значение |
|------|--------------------|
| `orchestration_source` | `fallback_legacy` |
| `fallback_reason` | описание ошибки (например, "connection refused") |
| `draft_preview` | Текст от legacy-агента |

Вернуть llm-service:

```bash
docker compose start llm-service
```

---

## 6. Проверить approve → send в VK

```bash
# Получить draft_id
DRAFT_ID=$(docker compose exec postgres psql -U social_user -d social_db -tAc \
  "SELECT id FROM vk_reply_drafts WHERE integration_id='$INTEGRATION_ID'::uuid AND status='pending' LIMIT 1")

# Approve и отправить
curl -s -X POST "http://localhost:8080/api/v1/social/vk/drafts/$DRAFT_ID/approve" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{}' | jq .

# Опционально: override текста перед отправкой
curl -s -X POST "http://localhost:8080/api/v1/social/vk/drafts/$DRAFT_ID/approve" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"text_override": "Исправленный ответ клиенту"}' | jq .
```

Ожидаемый ответ:

```json
{
  "status": "sent",
  "sent_message_id": 12345,
  "approved_at": "2026-04-02T...",
  "sent_at": "2026-04-02T..."
}
```

Проверить статус в БД:

```bash
docker compose exec postgres psql -U social_user -d social_db -c "
  SELECT status, sent_message_id, approved_at, sent_at
  FROM vk_reply_drafts WHERE id = '$DRAFT_ID'::uuid;
"
```

---

## 7. Проверить auto_reply (автоматическая отправка)

Изменить настройки агента:

```bash
curl -s -X PUT http://localhost:8080/api/v1/social/vk/agent/settings \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "integration_id": "'"$INTEGRATION_ID"'",
    "auto_reply_enabled": true,
    "safe_intents": ["faq","hours","basic_prices"]
  }' | jq .
```

Отправьте сообщение с safe intent (например, вопрос о часах работы).
Черновик должен появиться сразу со статусом `auto_sent`:

```bash
docker compose exec postgres psql -U social_user -d social_db -c "
  SELECT status, orchestration_source, sent_message_id, sent_at
  FROM vk_reply_drafts
  WHERE integration_id = '$INTEGRATION_ID'::uuid
  ORDER BY generated_at DESC LIMIT 1;
"
```

---

## 8. Переключение провайдера (provider-neutral)

Смена LLM-провайдера не требует изменения кода. Только `.env`:

```bash
# Переключиться на Groq (llama3-8b-8192)
echo "LLM_PROVIDER=groq
LLM_API_KEY=gsk_your_groq_key
LLM_MODEL=llama3-8b-8192" >> .env

docker compose up -d llm-service social-service  # перезапустить только нужные сервисы
```

Проверить логи:

```bash
docker compose logs llm-service --tail=20 | grep "provider"
# Ожидается: "groq provider" в сообщениях об ошибках или статусе
```

---

## 9. Запуск интеграционных тестов

Требуется доступная Postgres со схемой:

```bash
export INTEGRATION_TEST_DB="postgres://social_user:social_password@localhost:5432/social_db?sslmode=disable"

cd social-service
go test -tags integration -v ./internal/services/ -run TestE2E -timeout 120s
```

Ожидаемый вывод:

```
=== RUN   TestE2E_LLMService_Success_DraftPersisted
--- PASS: TestE2E_LLMService_Success_DraftPersisted (2.3s)
=== RUN   TestE2E_Hybrid_Error_FallbackLegacy
--- PASS: TestE2E_Hybrid_Error_FallbackLegacy (1.8s)
=== RUN   TestE2E_ApproveDraft_OutboundSend
--- PASS: TestE2E_ApproveDraft_OutboundSend (2.1s)
=== RUN   TestE2E_ApproveDraft_WithTextOverride
--- PASS: TestE2E_ApproveDraft_WithTextOverride (2.0s)
=== RUN   TestE2E_AutoReply_SafeIntent_AutoSent
--- PASS: TestE2E_AutoReply_SafeIntent_AutoSent (3.2s)
=== RUN   TestE2E_LLMNilClient_FallsBackToLegacy
--- PASS: TestE2E_LLMNilClient_FallsBackToLegacy (1.5s)
```

---

## 10. MinIO Console (загрузка файлов в базу знаний)

```
http://localhost:9001
Login: minioadmin / minioadmin
```

Bucket `inbetwin-rag` создаётся автоматически при старте rag-service.
Файлы загружаются через API:

```bash
curl -X POST http://localhost:3004/rag/documents/upload \
  -H "X-User-ID: $WORKSPACE_UUID" \
  -F "file=@/path/to/product-catalog.pdf" \
  -F "namespace_id=$NAMESPACE_UUID" \
  -F "chunk_size=512"
```

---

## Диагностика

| Симптом | Проверить |
|---------|-----------|
| Draft не создаётся | `docker compose logs social-service` — ошибки llm-service? |
| `orchestration_source=fallback_legacy` везде | Доступен ли llm-service? `curl http://localhost:3005/health` |
| `status=failed` | Ошибка VK API (токен протух?). `docker compose logs social-service \| grep "messages.send"` |
| Пустой `draft_text` | Legacy path сработал без настроенного LLM_API_KEY в social-service |
| MinIO 503 | Healthcheck: `docker compose ps minio` |
