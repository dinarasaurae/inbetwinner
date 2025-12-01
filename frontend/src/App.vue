<template>
  <div class="app">
    <header class="header">
      <h1>🚀 inBeTwin API Tester</h1>
      <p>Test your microservices APIs</p>
    </header>

    <div class="status-section">
      <h2>📊 Service Status</h2>
      <div class="status-grid">
        <div class="status-card" :class="{ online: apiGatewayStatus, offline: !apiGatewayStatus }">
          <h3>API Gateway</h3>
          <div class="status">{{ apiGatewayStatus ? '🟢 Online' : '🔴 Offline' }}</div>
          <button @click="checkApiGateway">Check Status</button>
        </div>

        <div class="status-card" :class="{ online: authServiceStatus, offline: !authServiceStatus }">
          <h3>Auth Service</h3>
          <div class="status">{{ authServiceStatus ? '🟢 Online' : '🔴 Offline' }}</div>
          <button @click="checkAuthService">Check Status</button>
        </div>
      </div>
    </div>

    <div class="auth-section">
      <h2>🔐 Authentication</h2>

      <div class="auth-tabs">
        <button @click="authTab = 'register'" :class="{ active: authTab === 'register' }">Register</button>
        <button @click="authTab = 'login'" :class="{ active: authTab === 'login' }">Login</button>
        <button @click="authTab = 'profile'" :class="{ active: authTab === 'profile' }">Profile</button>
      </div>

      <div v-if="authTab === 'register'" class="auth-form">
        <h3>Register New User</h3>
        <form @submit.prevent="register">
          <input v-model="registerForm.name" placeholder="Name" required />
          <input v-model="registerForm.email" placeholder="Email" type="email" required />
          <input v-model="registerForm.password" placeholder="Password" type="password" required />
          <button type="submit" :disabled="loading">{{ loading ? 'Registering...' : 'Register' }}</button>
        </form>
      </div>

      <div v-if="authTab === 'login'" class="auth-form">
        <h3>Login</h3>
        <form @submit.prevent="login">
          <input v-model="loginForm.email" placeholder="Email" type="email" required />
          <input v-model="loginForm.password" placeholder="Password" type="password" required />
          <button type="submit" :disabled="loading">{{ loading ? 'Logging in...' : 'Login' }}</button>
        </form>
      </div>

      <div v-if="authTab === 'profile'" class="auth-form">
        <h3>User Profile</h3>
        <div v-if="!token" class="no-token">
          <p>Please login first to view profile</p>
        </div>
        <div v-else>
          <button @click="getProfile" :disabled="loading">{{ loading ? 'Loading...' : 'Get Profile' }}</button>
          <button @click="logout" class="logout">Logout</button>
        </div>
      </div>
    </div>

    <div class="api-section" v-if="token">
      <h2>🔗 Protected APIs</h2>
      <div class="api-grid">
        <div class="api-card">
          <h3>Social Service</h3>
          <button @click="testApi('/api/v1/social/')">Test Social API</button>
        </div>
        <div class="api-card">
          <h3>Agent Service</h3>
          <button @click="testApi('/api/v1/agent/')">Test Agent API</button>
        </div>
        <div class="api-card">
          <h3>LLM Service</h3>
          <button @click="testApi('/api/v1/llm/')">Test LLM API</button>
        </div>
      </div>
    </div>

    <div class="response-section">
      <h2>📋 Response Log</h2>
      <div class="response-log">
        <div v-for="(response, index) in responseLog" :key="index" class="response-item">
          <div class="response-header">
            <span class="method" :class="response.method.toLowerCase()">{{ response.method }}</span>
            <span class="url">{{ response.url }}</span>
            <span class="status" :class="response.status >= 400 ? 'error' : 'success'">{{ response.status }}</span>
            <span class="time">{{ response.time }}</span>
          </div>
          <div class="response-body">
            <pre>{{ JSON.stringify(response.data, null, 2) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import axios from 'axios'

export default {
  name: 'App',
  data() {
    return {
      loading: false,
      authTab: 'register',
      apiGatewayStatus: false,
      authServiceStatus: false,
      token: localStorage.getItem('token'),
      registerForm: {
        name: '',
        email: '',
        password: ''
      },
      loginForm: {
        email: '',
        password: ''
      },
      responseLog: []
    }
  },
  methods: {
    async checkApiGateway() {
      try {
        const response = await axios.get('/api/v1/../health')
        this.apiGatewayStatus = response.status === 200
        this.addToLog('GET', '/health (API Gateway)', response.status, response.data)
      } catch (error) {
        this.apiGatewayStatus = false
        this.addToLog('GET', '/health (API Gateway)', error.response?.status || 0, { error: error.message })
      }
    },

    async checkAuthService() {
      try {
        const response = await axios.get('http://localhost:3001/health')
        this.authServiceStatus = response.status === 200
        this.addToLog('GET', '/health (Auth Service)', response.status, response.data)
      } catch (error) {
        this.authServiceStatus = false
        this.addToLog('GET', '/health (Auth Service)', error.response?.status || 0, { error: error.message })
      }
    },

    async register() {
      this.loading = true
      try {
        const response = await axios.post('/api/v1/auth/register', this.registerForm)
        this.addToLog('POST', '/api/v1/auth/register', response.status, response.data)

        // Clear form
        this.registerForm = { name: '', email: '', password: '' }

        // Switch to login tab
        this.authTab = 'login'
      } catch (error) {
        this.addToLog('POST', '/api/v1/auth/register', error.response?.status || 0,
          error.response?.data || { error: error.message })
      }
      this.loading = false
    },

    async login() {
      this.loading = true
      try {
        const response = await axios.post('/api/v1/auth/login', this.loginForm)
        this.addToLog('POST', '/api/v1/auth/login', response.status, response.data)

        if (response.data.access_token) {
          this.token = response.data.access_token
          localStorage.setItem('token', this.token)
          axios.defaults.headers.common['Authorization'] = `Bearer ${this.token}`

          // Clear form
          this.loginForm = { email: '', password: '' }

          // Switch to profile tab
          this.authTab = 'profile'
        }
      } catch (error) {
        this.addToLog('POST', '/api/v1/auth/login', error.response?.status || 0,
          error.response?.data || { error: error.message })
      }
      this.loading = false
    },

    async getProfile() {
      this.loading = true
      try {
        const response = await axios.get('/api/v1/auth/profile', {
          headers: { Authorization: `Bearer ${this.token}` }
        })
        this.addToLog('GET', '/api/v1/auth/profile', response.status, response.data)
      } catch (error) {
        this.addToLog('GET', '/api/v1/auth/profile', error.response?.status || 0,
          error.response?.data || { error: error.message })
      }
      this.loading = false
    },

    async testApi(endpoint) {
      try {
        const response = await axios.get(endpoint, {
          headers: { Authorization: `Bearer ${this.token}` }
        })
        this.addToLog('GET', endpoint, response.status, response.data)
      } catch (error) {
        this.addToLog('GET', endpoint, error.response?.status || 0,
          error.response?.data || { error: error.message })
      }
    },

    logout() {
      this.token = null
      localStorage.removeItem('token')
      delete axios.defaults.headers.common['Authorization']
      this.authTab = 'login'
    },

    addToLog(method, url, status, data) {
      this.responseLog.unshift({
        method,
        url,
        status,
        data,
        time: new Date().toLocaleTimeString()
      })

      // Keep only last 20 responses
      if (this.responseLog.length > 20) {
        this.responseLog = this.responseLog.slice(0, 20)
      }
    }
  },

  mounted() {
    // Set token in axios headers if exists
    if (this.token) {
      axios.defaults.headers.common['Authorization'] = `Bearer ${this.token}`
    }

    // Check services status on load
    this.checkApiGateway()
    this.checkAuthService()
  }
}
</script>

<style scoped>
.app {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px;
}

.header {
  text-align: center;
  margin-bottom: 30px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  padding: 20px;
  border-radius: 8px;
}

.status-section, .auth-section, .api-section, .response-section {
  margin-bottom: 30px;
  background: white;
  padding: 20px;
  border-radius: 8px;
  box-shadow: 0 2px 5px rgba(0,0,0,0.1);
}

.status-grid, .api-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 15px;
  margin-top: 15px;
}

.status-card, .api-card {
  padding: 15px;
  border-radius: 8px;
  border: 2px solid #e1e5e9;
  text-align: center;
}

.status-card.online {
  border-color: #28a745;
  background-color: #f8fff9;
}

.status-card.offline {
  border-color: #dc3545;
  background-color: #fff8f8;
}

.auth-tabs {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
}

.auth-tabs button {
  padding: 10px 20px;
  border: 1px solid #ddd;
  background: #f8f9fa;
  cursor: pointer;
  border-radius: 4px;
}

.auth-tabs button.active {
  background: #007bff;
  color: white;
  border-color: #007bff;
}

.auth-form {
  max-width: 400px;
}

.auth-form input {
  width: 100%;
  padding: 10px;
  margin: 5px 0;
  border: 1px solid #ddd;
  border-radius: 4px;
  box-sizing: border-box;
}

.auth-form button {
  width: 100%;
  padding: 12px;
  background: #007bff;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  margin: 5px 0;
}

.auth-form button:hover:not(:disabled) {
  background: #0056b3;
}

.auth-form button:disabled {
  background: #6c757d;
  cursor: not-allowed;
}

.logout {
  background: #dc3545 !important;
}

.logout:hover {
  background: #c82333 !important;
}

.api-card button, .status-card button {
  padding: 8px 16px;
  background: #28a745;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.response-log {
  max-height: 500px;
  overflow-y: auto;
}

.response-item {
  margin-bottom: 15px;
  border: 1px solid #e1e5e9;
  border-radius: 4px;
}

.response-header {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 10px;
  background: #f8f9fa;
  border-bottom: 1px solid #e1e5e9;
}

.method {
  padding: 2px 8px;
  border-radius: 3px;
  font-weight: bold;
  color: white;
  font-size: 12px;
}

.method.get { background: #28a745; }
.method.post { background: #ffc107; color: #212529; }
.method.put { background: #17a2b8; }
.method.delete { background: #dc3545; }

.status.success { color: #28a745; font-weight: bold; }
.status.error { color: #dc3545; font-weight: bold; }

.url { flex: 1; font-family: monospace; }
.time { font-size: 12px; color: #6c757d; }

.response-body {
  padding: 10px;
}

.response-body pre {
  margin: 0;
  white-space: pre-wrap;
  font-size: 12px;
  background: #f8f9fa;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}

.no-token {
  text-align: center;
  color: #6c757d;
  font-style: italic;
}
</style>