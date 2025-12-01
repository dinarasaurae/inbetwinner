# Frontend

## Features

**Service Status Monitoring**
- Check API Gateway health
- Check Auth Service health

**Authentication Testing**
- User registration
- User login
- Profile management
- JWT token handling

**Protected APIs Testing**
- Social Service
- Agent Service
- LLM Service

**Response Logging**
- Real-time API response monitoring
- Request/response history

## Quick Start

```bash
npm install

npm run dev

# Server will run on http://localhost:3000
```

## Setup Requirements

1. **Backend Services**: 
   ```bash
   docker-compose up -d
   ```

2. **API Gateway**: Should be available at `http://localhost:8080`
3. **Auth Service**: Should be available at `http://localhost:3001`

## Usage

1. **Check Service Status**: Click buttons to verify services are online
2. **Register User**: Create a test user account
3. **Login**: Get JWT token for protected endpoints
4. **Test APIs**: Use authenticated endpoints
5. **View Responses**: Check the response log for API results

## API Endpoints Tested

- `GET /health` - Service health checks
- `POST /api/v1/auth/register` - User registration
- `POST /api/v1/auth/login` - User authentication
- `GET /api/v1/auth/profile` - User profile (protected)
- `GET /api/v1/social/` - Social service (protected)
- `GET /api/v1/agent/` - Agent service (protected)
- `GET /api/v1/llm/` - LLM service (protected)

## Development

Built with:
- Vue 3
- Vite
- Axios
