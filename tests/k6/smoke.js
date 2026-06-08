import http from 'k6/http';
import { check, fail, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || 'http://localhost').replace(/\/+$/, '');
const VUS = Number.parseInt(__ENV.K6_VUS || '2', 10);
const RAMP_UP = __ENV.K6_RAMP_UP || '30s';
const STEADY_DURATION = __ENV.K6_DURATION || '3m';
const RAMP_DOWN = __ENV.K6_RAMP_DOWN || '30s';
const AUTO_REGISTER = String(__ENV.K6_AUTO_REGISTER || 'false').toLowerCase() === 'true';
const TEST_EMAIL = __ENV.TEST_EMAIL || 'nfr-smoke@example.com';
const TEST_PASSWORD = __ENV.TEST_PASSWORD || 'nfr-smoke-password';
const P95_MS = Number.parseInt(__ENV.K6_P95_MS || '300', 10);
const ERROR_RATE = __ENV.K6_ERROR_RATE || '0.01';
const SUCCESS_RATE = __ENV.K6_SUCCESS_RATE || '0.99';

const endpointSuccess = new Rate('inbetwin_endpoint_success_rate');
const endpoint5xx = new Counter('inbetwin_endpoint_5xx_total');

http.setResponseCallback(http.expectedStatuses({ min: 100, max: 499 }));

export const options = {
  scenarios: {
    smoke: {
      executor: 'ramping-vus',
      stages: [
        { duration: RAMP_UP, target: VUS },
        { duration: STEADY_DURATION, target: VUS },
        { duration: RAMP_DOWN, target: 0 },
      ],
      gracefulRampDown: '15s',
    },
  },
  thresholds: {
    http_req_failed: [`rate<${ERROR_RATE}`],
    http_req_duration: [`p(95)<${P95_MS}`],
    inbetwin_endpoint_success_rate: [`rate>${SUCCESS_RATE}`],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  userAgent: 'inbetwin-k6-smoke/1.0',
};

const publicRoutes = [
  { path: '/health', weight: 5 },
];

const privateRoutes = [
  { path: '/api/v1/auth/profile', weight: 5 },
  { path: '/api/v1/agent/builtin-tools', weight: 4 },
  { path: '/api/v1/agent/agents', weight: 4 },
  { path: '/api/v1/rag/namespaces', weight: 3 },
  { path: '/api/v1/rag/documents', weight: 3 },
  { path: '/api/v1/rag/qa', weight: 3 },
  { path: '/api/v1/rag/web', weight: 3 },
  { path: '/api/v1/rag/sheets', weight: 2 },
  { path: '/api/v1/llm/tools', weight: 3 },
  { path: '/api/v1/llm/google/status', weight: 2 },
  { path: '/api/v1/llm/amocrm/status', weight: 2 },
  { path: '/api/v1/llm/zoho/status', weight: 2 },
  { path: '/api/v1/social/telegram/status', weight: 2 },
];

function jsonParams(tags = {}) {
  return {
    headers: {
      'Content-Type': 'application/json',
    },
    tags,
    timeout: '5s',
  };
}

function authParams(token, tags = {}) {
  return {
    headers: {
      Authorization: `Bearer ${token}`,
    },
    tags,
    timeout: '5s',
  };
}

function chooseWeighted(routes) {
  const total = routes.reduce((sum, route) => sum + route.weight, 0);
  let cursor = Math.random() * total;

  for (const route of routes) {
    cursor -= route.weight;
    if (cursor <= 0) {
      return route.path;
    }
  }

  return routes[routes.length - 1].path;
}

function record(route, res) {
  const ok = res.status > 0 && res.status < 500;
  endpointSuccess.add(ok, { endpoint: route });

  if (res.status >= 500 || res.status === 0) {
    endpoint5xx.add(1, { endpoint: route });
  }

  check(res, {
    'gateway returned a response below 500': (r) => r.status > 0 && r.status < 500,
  }, { endpoint: route });
}

export function setup() {
  if (!TEST_EMAIL || !TEST_PASSWORD) {
    fail('TEST_EMAIL and TEST_PASSWORD are required');
  }

  if (AUTO_REGISTER) {
    const registerBody = JSON.stringify({
      email: TEST_EMAIL,
      password: TEST_PASSWORD,
      name: 'NFR Smoke Test',
    });

    const register = http.post(
      `${BASE_URL}/api/v1/auth/register`,
      registerBody,
      jsonParams({ endpoint: '/api/v1/auth/register', name: 'POST /api/v1/auth/register' }),
    );

    check(register, {
      'test user exists or was created': (r) => r.status === 201 || r.status === 409,
    });
  }

  const login = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ email: TEST_EMAIL, password: TEST_PASSWORD }),
    jsonParams({ endpoint: '/api/v1/auth/login', name: 'POST /api/v1/auth/login' }),
  );

  const loginOk = check(login, {
    'login succeeded': (r) => r.status === 200 && Boolean(r.json('access_token')),
  });

  if (!loginOk) {
    fail(`login failed with status ${login.status}: ${login.body}`);
  }

  return {
    token: login.json('access_token'),
  };
}

export default function (data) {
  const privateTraffic = Math.random() < 0.8;
  const route = privateTraffic ? chooseWeighted(privateRoutes) : chooseWeighted(publicRoutes);

  group(privateTraffic ? 'authenticated read-only API' : 'public API', () => {
    const params = privateTraffic
      ? authParams(data.token, { endpoint: route, name: `GET ${route}` })
      : { tags: { endpoint: route, name: `GET ${route}` }, timeout: '5s' };

    const res = http.get(`${BASE_URL}${route}`, params);
    record(route, res);
  });

  sleep(0.5 + Math.random());
}
