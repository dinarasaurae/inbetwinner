import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  BarChart3,
  Bell,
  BookOpen,
  Bot,
  Check,
  CheckCircle2,
  ChevronRight,
  Database,
  ExternalLink,
  FileText,
  Globe2,
  KeyRound,
  Link2,
  Loader2,
  Lock,
  LogIn,
  LogOut,
  MessageSquare,
  PlugZap,
  RefreshCw,
  Search,
  Send,
  Settings,
  Shield,
  SlidersHorizontal,
  Sparkles,
  Table2,
  User,
  UserPlus,
  Users,
  Workflow,
} from 'lucide-react';

type UserProfile = {
  id: string;
  email: string;
  name?: string | null;
  first_name?: string | null;
  last_name?: string | null;
  avatar_url?: string | null;
  email_verified?: boolean;
  subscription_plan?: string;
  created_at?: string;
};

type AuthResponse = {
  access_token: string;
  refresh_token: string;
  user: UserProfile;
};

type RefreshResponse = {
  access_token: string;
  expires_in?: number;
};

type AuthMode = 'login' | 'register';
type AppTab = 'workspace' | 'integrations' | 'agent' | 'knowledge' | 'profile';
type ProfileSection = 'main' | 'personal' | 'security' | 'notifications' | 'statistics' | 'accounts';

type ListResponse<T> = {
  items?: T[];
  total?: number;
};

type VKConnectedGroup = {
  id: string;
  group_id: number;
  group_name: string;
  group_screen_name: string;
  group_photo?: string | null;
  is_active?: boolean;
  community_access_enabled?: boolean;
  messaging_enabled?: boolean;
  posts_count?: number;
  message_count?: number;
  lead_count?: number;
  context_ready?: boolean;
};

type VKConnectedGroupsData = {
  count: number;
  groups: VKConnectedGroup[];
};

type VKBusinessSnapshot = {
  summary?: string;
  twin_stage?: string;
  positioning?: string;
  audience_summary?: string;
  offer_signals?: string[];
  content_signals?: string[];
  knowledge_signals?: string[];
  missing_signals?: string[];
  recommended_actions?: string[];
  community_access_enabled?: boolean;
  long_poll_enabled?: boolean;
  ready_for_conversations?: boolean;
};

type VKWorkspaceAnalytics = {
  incoming_messages?: number;
  outgoing_messages?: number;
  pending_drafts?: number;
  leads?: number;
  first_response_time_minutes?: number;
  handoff_rate?: number;
  lead_conversion_rate?: number;
  confidence_by_intent?: Array<{
    intent: string;
    avg_confidence?: number;
    count?: number;
  }>;
};

type VKWorkspaceMessage = {
  id: string;
  from_vk_user_id: number;
  text?: string;
  is_incoming?: boolean;
  is_processed?: boolean;
  received_at?: string;
  draft?: VKReplyDraft | null;
};

type VKWorkspacePost = {
  id: string;
  vk_post_id?: number;
  text?: string;
  has_media?: boolean;
  media_type?: string;
  likes_count?: number;
  comments_count?: number;
  posted_at?: string;
};

type VKWorkspaceLead = {
  id: string;
  vk_user_id: number;
  first_name?: string;
  last_name?: string;
  city?: string;
  country?: string;
  about?: string;
  status?: string;
  followers_count?: number;
};

type VKReplyDraft = {
  id: string;
  inbound_message_id?: string;
  from_vk_user_id?: number;
  intent?: string;
  confidence?: number;
  safe_intent?: boolean;
  status?: string;
  source?: string;
  draft_text?: string;
  rationale?: string;
  knowledge_snippets?: string[];
};

type VKAgentSettings = {
  id?: string;
  integration_id?: string;
  draft_first: boolean;
  auto_reply_enabled: boolean;
  safe_intents: string[];
  tone_of_voice: string;
  forbidden_promises: string[];
  escalation_policy: string;
  rag_enabled: boolean;
};

type VKWorkspaceData = {
  integration?: VKConnectedGroup | null;
  business_snapshot?: VKBusinessSnapshot | null;
  recent_posts?: VKWorkspacePost[];
  recent_messages?: VKWorkspaceMessage[];
  leads?: VKWorkspaceLead[];
  drafts?: VKReplyDraft[];
  recommendations?: string[];
  analytics?: VKWorkspaceAnalytics;
  agent_settings?: VKAgentSettings | null;
};

type TelegramStatus = {
  connected?: boolean;
  channel_username?: string | null;
  channel_title?: string | null;
  integration_status?: string | null;
  last_synced_at?: string | null;
};

type CRMStatus = {
  connected?: boolean;
  account_name?: string;
  portal?: string;
  integration?: unknown;
};

type KnowledgeCounts = {
  documents: number;
  qa: number;
  tables: number;
  web: number;
};

type NoticeTone = 'info' | 'success' | 'error';

const defaultAgentSettings: VKAgentSettings = {
  draft_first: true,
  auto_reply_enabled: false,
  safe_intents: ['faq', 'hours', 'basic_prices', 'qualification'],
  tone_of_voice: 'Дружелюбный, уверенный, по делу',
  forbidden_promises: ['Не обещать цену, сроки или результат, если этого нет в контексте.'],
  escalation_policy: 'Эскалировать спорные случаи, жалобы, срочные проблемы и запросы вне safe-intents.',
  rag_enabled: false,
};

const navItems: Array<{ id: AppTab; label: string; icon: React.ComponentType<{ size?: number; className?: string }> }> = [
  { id: 'workspace', label: 'VK Workspace', icon: MessageSquare },
  { id: 'integrations', label: 'Подключения', icon: PlugZap },
  { id: 'agent', label: 'Инструкции', icon: Bot },
  { id: 'knowledge', label: 'База знаний', icon: BookOpen },
  { id: 'profile', label: 'Профиль', icon: User },
];

const fallbackDialogs = [
  {
    name: 'Мария Соколова',
    source: 'VK',
    text: 'Нужна автоматизация ответов для 15 менеджеров.',
    score: 94,
    status: 'Hot',
  },
  {
    name: 'Алексей Романов',
    source: 'Telegram',
    text: 'Уточняет интеграцию с amoCRM и календарем.',
    score: 78,
    status: 'Warm',
  },
  {
    name: 'SaaS Ops',
    source: 'Web',
    text: 'Запросили пилот на один отдел поддержки.',
    score: 63,
    status: 'New',
  },
];

const agentEvents = [
  {
    title: 'VK lead qualified',
    meta: '12 секунд назад',
    detail: 'Агент поднял score до 94 и подготовил следующий шаг.',
    tone: 'green',
  },
  {
    title: 'CRM task created',
    meta: '3 минуты назад',
    detail: 'Задача на демо добавлена для отдела продаж.',
    tone: 'blue',
  },
  {
    title: 'Knowledge matched',
    meta: '8 минут назад',
    detail: 'Найдено 4 релевантных фрагмента из базы знаний.',
    tone: 'amber',
  },
];

function displayName(user: UserProfile | null) {
  if (!user) {
    return 'Demo workspace';
  }

  const fullName = [user.first_name, user.last_name].filter(Boolean).join(' ');
  return fullName || user.name || user.email;
}

function initials(user: UserProfile | null) {
  const name = displayName(user);
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join('') || 'AI';
}

function saveAuthSession(accessToken: string, refreshToken?: string, user?: UserProfile) {
  localStorage.setItem('inbetwin_access_token', accessToken);

  if (refreshToken) {
    localStorage.setItem('inbetwin_refresh_token', refreshToken);
  }

  if (user) {
    localStorage.setItem('inbetwin_user', JSON.stringify(user));
  }
}

function clearAuthSession() {
  localStorage.removeItem('inbetwin_access_token');
  localStorage.removeItem('inbetwin_refresh_token');
  localStorage.removeItem('inbetwin_user');
}

function unwrapPayload<T>(payload: unknown): T {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as { data: T }).data;
  }
  return payload as T;
}

class ApiRequestError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = status;
  }
}

async function readJson<T>(response: Response): Promise<T> {
  const payload = await response.json().catch(() => null);

  if (!response.ok) {
    const details = payload && typeof payload === 'object' && 'details' in payload
      ? (payload as { details?: unknown }).details
      : null;
    const error = payload && typeof payload === 'object' && 'error' in payload
      ? (payload as { error?: string }).error
      : null;
    const message = payload && typeof payload === 'object' && 'message' in payload
      ? (payload as { message?: string }).message
      : null;

    throw new ApiRequestError(String(error || message || details || 'Request failed'), response.status);
  }

  return unwrapPayload<T>(payload);
}

function buildRequestHeaders(init: RequestInit, token: string | null) {
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  return headers;
}

async function refreshStoredAccessToken(): Promise<string | null> {
  const refreshToken = localStorage.getItem('inbetwin_refresh_token');
  if (!refreshToken) {
    return null;
  }

  const response = await fetch('/api/v1/auth/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  const payload = await readJson<RefreshResponse>(response);
  if (!payload.access_token) {
    return null;
  }

  saveAuthSession(payload.access_token);
  window.dispatchEvent(new CustomEvent('inbetwin:auth-token', { detail: { accessToken: payload.access_token } }));
  return payload.access_token;
}

async function apiRequest<T>(path: string, token: string | null, init: RequestInit = {}) {
  const storedToken = token ? localStorage.getItem('inbetwin_access_token') : null;
  const effectiveToken = storedToken || token;
  const doFetch = (nextToken: string | null) => (
    fetch(path, { ...init, headers: buildRequestHeaders(init, nextToken) })
  );

  const response = await doFetch(effectiveToken);

  try {
    return await readJson<T>(response);
  } catch (error) {
    if (
      token &&
      error instanceof ApiRequestError &&
      error.status === 401 &&
      !path.includes('/api/v1/auth/refresh')
    ) {
      const nextToken = await refreshStoredAccessToken().catch(() => null);
      if (nextToken) {
        const retry = await doFetch(nextToken);
        return readJson<T>(retry);
      }
    }

    if (token && error instanceof ApiRequestError && error.status === 401) {
      throw new Error('Сессия истекла. Войдите снова и повторите действие.');
    }

    throw error;
  }
}

async function apiMaybe<T>(path: string, token: string | null, init: RequestInit = {}): Promise<T | null> {
  try {
    return await apiRequest<T>(path, token, init);
  } catch {
    return null;
  }
}

function formatPercent(value?: number) {
  if (!value) {
    return '0%';
  }
  return `${Math.round(value * 100)}%`;
}

function groupHandle(group?: VKConnectedGroup | null) {
  if (!group) {
    return '@community';
  }
  return group.group_screen_name ? `@${group.group_screen_name}` : `club${group.group_id}`;
}

function sectionTitle(tab: AppTab, profileSection: ProfileSection) {
  if (tab === 'profile' && profileSection === 'statistics') {
    return 'Statistics';
  }
  if (tab === 'profile') {
    return 'Profile';
  }
  return navItems.find((item) => item.id === tab)?.label || 'Workspace';
}

const WebApp: React.FC = () => {
  const [activeTab, setActiveTab] = useState<AppTab>('workspace');
  const [profileSection, setProfileSection] = useState<ProfileSection>('main');
  const [authMode, setAuthMode] = useState<AuthMode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [fullName, setFullName] = useState('');
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('inbetwin_access_token'));
  const [user, setUser] = useState<UserProfile | null>(() => {
    const saved = localStorage.getItem('inbetwin_user');
    if (!saved) {
      return null;
    }
    try {
      return JSON.parse(saved) as UserProfile;
    } catch {
      return null;
    }
  });
  const [authStatus, setAuthStatus] = useState('');
  const [notice, setNotice] = useState<{ tone: NoticeTone; text: string } | null>(null);
  const [isSubmitting, setSubmitting] = useState(false);
  const [isAppLoading, setAppLoading] = useState(false);
  const [vkGroups, setVkGroups] = useState<VKConnectedGroup[]>([]);
  const [activeIntegrationId, setActiveIntegrationId] = useState('');
  const [vkWorkspace, setVkWorkspace] = useState<VKWorkspaceData | null>(null);
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [amoStatus, setAmoStatus] = useState<CRMStatus | null>(null);
  const [zohoStatus, setZohoStatus] = useState<CRMStatus | null>(null);
  const [knowledgeCounts, setKnowledgeCounts] = useState<KnowledgeCounts>({ documents: 0, qa: 0, tables: 0, web: 0 });
  const [agentSettings, setAgentSettings] = useState<VKAgentSettings>(defaultAgentSettings);
  const [manualGroupRef, setManualGroupRef] = useState('');
  const [manualCommunityToken, setManualCommunityToken] = useState('');
  const [profileForm, setProfileForm] = useState({ first_name: '', last_name: '' });
  const [passwordForm, setPasswordForm] = useState({ current_password: '', new_password: '' });

  const workspaceName = useMemo(() => displayName(user), [user]);
  const selectedGroup = useMemo(() => {
    const fromWorkspace = vkWorkspace?.integration;
    return fromWorkspace || vkGroups.find((group) => group.id === activeIntegrationId) || vkGroups[0] || null;
  }, [activeIntegrationId, vkGroups, vkWorkspace]);

  const loadWorkspace = useCallback(async (integrationId: string, currentToken = token) => {
    if (!currentToken || !integrationId) {
      return;
    }

    const workspace = await apiMaybe<VKWorkspaceData>(
      `/api/v1/social/vk/workspace?integration_id=${encodeURIComponent(integrationId)}`,
      currentToken,
    );

    if (workspace) {
      setVkWorkspace(workspace);
      if (workspace.agent_settings) {
        setAgentSettings({ ...defaultAgentSettings, ...workspace.agent_settings });
      }
    }
  }, [token]);

  const loadAppData = useCallback(async (currentToken = token) => {
    if (!currentToken) {
      return;
    }

    setAppLoading(true);

    const [
      profile,
      groupsPayload,
      telegram,
      amo,
      zoho,
      documents,
      qa,
      tables,
      webSources,
    ] = await Promise.all([
      apiMaybe<UserProfile>('/api/v1/auth/profile', currentToken),
      apiMaybe<VKConnectedGroupsData>('/api/v1/social/vk/groups', currentToken),
      apiMaybe<TelegramStatus>('/api/v1/social/telegram/status', currentToken),
      apiMaybe<CRMStatus>('/api/v1/llm/amocrm/status', currentToken),
      apiMaybe<CRMStatus>('/api/v1/llm/zoho/status', currentToken),
      apiMaybe<ListResponse<unknown>>('/api/v1/rag/documents', currentToken),
      apiMaybe<ListResponse<unknown>>('/api/v1/rag/qa', currentToken),
      apiMaybe<ListResponse<unknown>>('/api/v1/rag/sheets', currentToken),
      apiMaybe<ListResponse<unknown>>('/api/v1/rag/web', currentToken),
    ]);

    if (profile) {
      setUser(profile);
      localStorage.setItem('inbetwin_user', JSON.stringify(profile));
      setProfileForm({
        first_name: profile.first_name || '',
        last_name: profile.last_name || '',
      });
    }

    const groups = groupsPayload?.groups || [];
    setVkGroups(groups);
    setTelegramStatus(telegram);
    setAmoStatus(amo);
    setZohoStatus(zoho);
    setKnowledgeCounts({
      documents: documents?.total ?? documents?.items?.length ?? 0,
      qa: qa?.total ?? qa?.items?.length ?? 0,
      tables: tables?.total ?? tables?.items?.length ?? 0,
      web: webSources?.total ?? webSources?.items?.length ?? 0,
    });

    const nextIntegrationId = activeIntegrationId || groups[0]?.id || '';
    if (nextIntegrationId) {
      setActiveIntegrationId(nextIntegrationId);
      await loadWorkspace(nextIntegrationId, currentToken);
    }

    setAppLoading(false);
  }, [activeIntegrationId, loadWorkspace, token]);

  useEffect(() => {
    document.title = 'inBeTwin AI Agent';
  }, []);

  useEffect(() => {
    const handleTokenRefresh = (event: Event) => {
      const accessToken = (event as CustomEvent<{ accessToken?: string }>).detail?.accessToken;
      if (accessToken) {
        setToken(accessToken);
      }
    };

    window.addEventListener('inbetwin:auth-token', handleTokenRefresh);
    return () => window.removeEventListener('inbetwin:auth-token', handleTokenRefresh);
  }, []);

  useEffect(() => {
    const isVKCallback =
      window.location.pathname === '/app/auth/vk/callback' ||
      window.location.pathname === '/auth/vk/callback';
    const query = new URLSearchParams(window.location.search);
    const groupConnected = query.get('group_connected');

    if (window.location.pathname === '/app/vk-error') {
      const reason = query.get('reason') || 'VK auth failed';
      window.setTimeout(() => setAuthStatus(`VK: ${reason}`), 0);
      window.history.replaceState(null, '', '/app');
      return;
    }

    if (window.location.pathname.startsWith('/app/dashboard/vk/groups') || groupConnected) {
      window.setTimeout(() => {
        setActiveTab('integrations');
        setNotice({
          tone: 'success',
          text: groupConnected ? `VK-сообщество ${groupConnected} подключено.` : 'VK подключен. Обновляем список сообществ.',
        });
        const storedToken = localStorage.getItem('inbetwin_access_token');
        if (storedToken) {
          void loadAppData(storedToken);
        }
      }, 0);
      window.history.replaceState(null, '', '/app');
      return;
    }

    if (!isVKCallback) {
      return;
    }

    const fragment = new URLSearchParams(window.location.hash.replace(/^#/, ''));
    const error = fragment.get('error') || query.get('error');

    if (error) {
      const reason = fragment.get('reason') || query.get('reason') || 'VK не вернул сессию.';
      window.setTimeout(() => setAuthStatus(`VK: ${reason}`), 0);
      window.history.replaceState(null, '', '/app');
      return;
    }

    const accessToken = fragment.get('access_token');
    const refreshToken = fragment.get('refresh_token');

    if (!accessToken) {
      window.setTimeout(() => setAuthStatus('VK не вернул access token. Попробуйте ещё раз.'), 0);
      window.history.replaceState(null, '', '/app');
      return;
    }

    saveAuthSession(accessToken, refreshToken || undefined);
    window.setTimeout(() => {
      setToken(accessToken);
      setAuthStatus('');
      setNotice({ tone: 'success', text: 'Вход через VK выполнен.' });
    }, 0);
    window.history.replaceState(null, '', '/app');
  }, [loadAppData]);

  useEffect(() => {
    if (token) {
      const timer = window.setTimeout(() => {
        void loadAppData(token);
      }, 0);
      return () => window.clearTimeout(timer);
    }
  }, [loadAppData, token]);

  const setStatus = (tone: NoticeTone, text: string) => {
    setNotice({ tone, text });
  };

  const handleAuth = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setAuthStatus('');

    const [firstName, ...lastNameParts] = fullName.trim().split(/\s+/).filter(Boolean);
    const registerBody = {
      email,
      password,
      name: fullName.trim() || undefined,
      first_name: firstName || undefined,
      last_name: lastNameParts.join(' ') || undefined,
    };

    try {
      if (authMode === 'register') {
        await apiRequest<UserProfile>('/api/v1/auth/register', null, {
          method: 'POST',
          body: JSON.stringify(registerBody),
        });
      }

      const payload = await apiRequest<AuthResponse>('/api/v1/auth/login', null, {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });

      saveAuthSession(payload.access_token, payload.refresh_token, payload.user);
      setToken(payload.access_token);
      setUser(payload.user);
      setAuthStatus('');
      setNotice({ tone: 'success', text: authMode === 'register' ? 'Аккаунт создан.' : 'Сессия открыта.' });
    } catch (error) {
      setAuthStatus(error instanceof Error ? error.message : 'Не удалось выполнить запрос.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleVKLogin = () => {
    setAuthStatus('Перенаправляем в VK ID...');
    const returnURL = `${window.location.origin}/app/auth/vk/callback`;
    window.location.href = `/api/v1/auth/vk/start?return_url=${encodeURIComponent(returnURL)}`;
  };

  const handleLogout = () => {
    clearAuthSession();
    setToken(null);
    setUser(null);
    setVkWorkspace(null);
    setVkGroups([]);
    setNotice(null);
    setActiveTab('workspace');
    setProfileSection('main');
  };

  const startSocialVKOAuth = async () => {
    if (!token) {
      return;
    }
    const groupRef = manualGroupRef.trim();
    const oauthPath = groupRef
      ? `/api/v1/social/vk/oauth/start?platform=web&group_ref=${encodeURIComponent(groupRef)}`
      : activeIntegrationId
      ? `/api/v1/social/vk/community-access/start?platform=web&integration_id=${encodeURIComponent(activeIntegrationId)}`
      : '';

    if (!oauthPath) {
      setStatus('error', 'Укажите ID или ссылку VK-сообщества, чтобы подключить его через OAuth.');
      return;
    }

    setStatus('info', 'Открываем VK для доступа к сообществу...');
    try {
      const result = await apiRequest<{ auth_url: string }>(oauthPath, token);
      window.location.href = result.auth_url;
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось открыть VK OAuth для сообщества.');
    }
  };

  const saveGroupTokenByRef = async () => {
    if (!token || !manualGroupRef.trim() || !manualCommunityToken.trim()) {
      return;
    }
    setStatus('info', 'Сохраняем ключ сообщества...');
    try {
      const group = await apiRequest<VKConnectedGroup>('/api/v1/social/vk/groups/token/by-ref', token, {
        method: 'POST',
        body: JSON.stringify({
          group_ref: manualGroupRef.trim(),
          token: manualCommunityToken.trim(),
        }),
      });
      setManualCommunityToken('');
      setManualGroupRef('');
      setActiveIntegrationId(group.id);
      setStatus('success', 'Сообщество подключено.');
      await loadAppData(token);
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось подключить сообщество.');
    }
  };

  const buildContext = async () => {
    if (!token || !activeIntegrationId) {
      return;
    }
    setStatus('info', 'Собираем контекст VK...');
    try {
      await apiRequest('/api/v1/social/vk/context/bootstrap', token, {
        method: 'POST',
        body: JSON.stringify({
          integration_id: activeIntegrationId,
          count: 30,
          include_subscriptions: false,
        }),
      });
      setStatus('success', 'Контекст обновлен.');
      await loadWorkspace(activeIntegrationId, token);
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось собрать контекст.');
    }
  };

  const saveAgentSettings = async () => {
    if (!token || !activeIntegrationId) {
      setStatus('error', 'Сначала подключите VK-сообщество.');
      return;
    }
    try {
      const saved = await apiRequest<VKAgentSettings>('/api/v1/social/vk/agent/settings', token, {
        method: 'PUT',
        body: JSON.stringify({
          integration_id: activeIntegrationId,
          draft_first: agentSettings.draft_first,
          auto_reply_enabled: agentSettings.auto_reply_enabled,
          safe_intents: agentSettings.safe_intents,
          tone_of_voice: agentSettings.tone_of_voice,
          forbidden_promises: agentSettings.forbidden_promises,
          escalation_policy: agentSettings.escalation_policy,
          rag_enabled: agentSettings.rag_enabled,
        }),
      });
      setAgentSettings({ ...defaultAgentSettings, ...saved });
      setStatus('success', 'Инструкции агента сохранены.');
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось сохранить инструкции.');
    }
  };

  const generateDraft = async (messageId: string) => {
    if (!token || !activeIntegrationId) {
      return;
    }
    try {
      await apiRequest('/api/v1/social/vk/drafts/generate', token, {
        method: 'POST',
        body: JSON.stringify({
          integration_id: activeIntegrationId,
          message_id: messageId,
          force: true,
        }),
      });
      setStatus('success', 'Черновик создан.');
      await loadWorkspace(activeIntegrationId, token);
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось создать черновик.');
    }
  };

  const approveDraft = async (draftId: string) => {
    if (!token || !activeIntegrationId) {
      return;
    }
    try {
      await apiRequest(`/api/v1/social/vk/drafts/${draftId}/approve`, token, {
        method: 'POST',
        body: JSON.stringify({ text_override: '' }),
      });
      setStatus('success', 'Черновик отправлен.');
      await loadWorkspace(activeIntegrationId, token);
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось отправить черновик.');
    }
  };

  const openCrmOAuth = async (kind: 'amocrm' | 'zoho') => {
    if (!token) {
      return;
    }
    try {
      const result = await apiRequest<{ auth_url?: string; url?: string }>(`/api/v1/llm/${kind}/auth-url`, token);
      const url = result.auth_url || result.url;
      if (url) {
        window.open(url, '_blank', 'noopener,noreferrer');
        setStatus('info', 'После OAuth обновите статус интеграций.');
      }
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'OAuth CRM недоступен.');
    }
  };

  const updateProfile = async () => {
    if (!token) {
      return;
    }
    try {
      const updated = await apiRequest<UserProfile>('/api/v1/auth/profile', token, {
        method: 'PUT',
        body: JSON.stringify(profileForm),
      });
      setUser(updated);
      localStorage.setItem('inbetwin_user', JSON.stringify(updated));
      setStatus('success', 'Профиль сохранен.');
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось сохранить профиль.');
    }
  };

  const changePassword = async () => {
    if (!token || !passwordForm.current_password || !passwordForm.new_password) {
      return;
    }
    try {
      await apiRequest('/api/v1/auth/profile/password', token, {
        method: 'PUT',
        body: JSON.stringify(passwordForm),
      });
      setPasswordForm({ current_password: '', new_password: '' });
      setStatus('success', 'Пароль обновлен.');
    } catch (error) {
      setStatus('error', error instanceof Error ? error.message : 'Не удалось обновить пароль.');
    }
  };

  if (!token) {
    return (
      <AuthScreen
        authMode={authMode}
        authStatus={authStatus}
        email={email}
        fullName={fullName}
        isSubmitting={isSubmitting}
        onAuth={handleAuth}
        onFullName={setFullName}
        onMode={setAuthMode}
        onPassword={setPassword}
        onVKLogin={handleVKLogin}
        onYandexLogin={() => setAuthStatus('Yandex ID OAuth еще не подключен в backend.')}
        password={password}
        setEmail={setEmail}
      />
    );
  }

  return (
    <main className="min-h-screen bg-[#040b1b] text-[#e7edff]">
      <div className="flex min-h-screen">
        <aside className="hidden w-[304px] shrink-0 flex-col border-r border-[#25314f] bg-[#071022] px-5 py-6 lg:flex">
          <a href="/" className="mb-8 flex items-center">
            <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-16 w-auto brightness-0 invert" />
          </a>

          <nav className="flex flex-col gap-1.5">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = activeTab === item.id;
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => {
                    setActiveTab(item.id);
                    if (item.id !== 'profile') {
                      setProfileSection('main');
                    }
                  }}
                  className={`flex h-12 items-center gap-3 rounded-lg px-3 text-left text-sm transition ${
                    isActive ? 'bg-[#e7edff] text-[#071022]' : 'text-[#c4cbe0] hover:bg-[#101c36] hover:text-white'
                  }`}
                >
                  <Icon size={18} />
                  {item.label}
                </button>
              );
            })}
          </nav>

          <div className="mt-auto rounded-lg border border-[#25314f] bg-[#101c36] p-4">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Shield size={16} className="text-[#3ac58a]" />
              Агент активен
            </div>
            <p className="mt-2 text-xs leading-5 text-[#8e9ab8]">
              Draft-first, CRM tools и RAG управляются в разделе инструкций.
            </p>
          </div>
        </aside>

        <section className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-40 border-b border-[#25314f] bg-[#040b1b]/88 px-4 py-4 backdrop-blur-xl sm:px-6 lg:px-8">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-xs uppercase tracking-[0.24em] text-[#8e9ab8]">Workspace</div>
                <h1 className="truncate text-xl font-semibold sm:text-2xl">{sectionTitle(activeTab, profileSection)}</h1>
                <div className="mt-1 truncate text-sm text-[#8e9ab8]">{workspaceName}</div>
              </div>
              <div className="flex items-center gap-2">
                <div className="hidden h-10 items-center gap-2 rounded-lg border border-[#25314f] bg-[#0b1328] px-3 text-sm text-[#8e9ab8] md:flex">
                  <Search size={16} />
                  Поиск лидов
                </div>
                <button
                  type="button"
                  className="grid h-10 w-10 place-items-center rounded-lg border border-[#25314f] bg-[#0b1328] text-[#c4cbe0]"
                  aria-label="Обновить"
                  title="Обновить"
                  onClick={() => void loadAppData(token)}
                >
                  {isAppLoading ? <Loader2 size={18} className="animate-spin" /> : <RefreshCw size={18} />}
                </button>
                <button
                  type="button"
                  className="grid h-10 w-10 place-items-center rounded-lg border border-[#25314f] bg-[#0b1328] text-[#c4cbe0]"
                  aria-label="Уведомления"
                  title="Уведомления"
                >
                  <Bell size={18} />
                </button>
                <button
                  type="button"
                  onClick={handleLogout}
                  className="grid h-10 w-10 place-items-center rounded-lg border border-[#25314f] bg-[#0b1328] text-[#c4cbe0] hover:text-white"
                  aria-label="Выйти"
                  title="Выйти"
                >
                  <LogOut size={18} />
                </button>
              </div>
            </div>

            <nav className="mt-4 flex gap-2 overflow-x-auto pb-1 lg:hidden">
              {navItems.map((item) => {
                const Icon = item.icon;
                const isActive = activeTab === item.id;
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setActiveTab(item.id)}
                    className={`flex h-10 shrink-0 items-center gap-2 rounded-lg px-3 text-sm ${
                      isActive ? 'bg-[#e7edff] text-[#071022]' : 'bg-[#0b1328] text-[#c4cbe0]'
                    }`}
                  >
                    <Icon size={16} />
                    {item.label}
                  </button>
                );
              })}
            </nav>
          </header>

          {notice && (
            <div className="border-b border-[#25314f] px-4 py-3 sm:px-6 lg:px-8">
              <div className={`rounded-lg border px-4 py-3 text-sm ${
                notice.tone === 'success'
                  ? 'border-[#3ac58a]/35 bg-[#3ac58a]/10 text-[#bdf3d9]'
                  : notice.tone === 'error'
                    ? 'border-[#ff5e73]/35 bg-[#ff5e73]/10 text-[#ffc4cb]'
                    : 'border-[#4c88ff]/35 bg-[#4c88ff]/10 text-[#c9dcff]'
              }`}>
                {notice.text}
              </div>
            </div>
          )}

          <div className="flex-1 px-4 py-5 sm:px-6 lg:px-8">
            {activeTab === 'workspace' && (
              <VKWorkspaceView
                activeIntegrationId={activeIntegrationId}
                group={selectedGroup}
                groups={vkGroups}
                onBuildContext={buildContext}
                onGenerateDraft={generateDraft}
                onOpenAgent={() => setActiveTab('agent')}
                onOpenIntegrations={() => setActiveTab('integrations')}
                onRefresh={() => activeIntegrationId ? loadWorkspace(activeIntegrationId, token) : loadAppData(token)}
                onSelectIntegration={(id) => {
                  setActiveIntegrationId(id);
                  void loadWorkspace(id, token);
                }}
                onApproveDraft={approveDraft}
                workspace={vkWorkspace}
              />
            )}

            {activeTab === 'integrations' && (
              <IntegrationsView
                amoStatus={amoStatus}
                groupRef={manualGroupRef}
                groups={vkGroups}
                onGroupRef={setManualGroupRef}
                onOpenCrm={openCrmOAuth}
                onRefresh={() => void loadAppData(token)}
                onSaveVKToken={saveGroupTokenByRef}
                onStartVKOAuth={startSocialVKOAuth}
                onToken={setManualCommunityToken}
                telegramStatus={telegramStatus}
                tokenValue={manualCommunityToken}
                zohoStatus={zohoStatus}
              />
            )}

            {activeTab === 'agent' && (
              <AgentInstructionsView
                settings={agentSettings}
                setSettings={setAgentSettings}
                onSave={saveAgentSettings}
              />
            )}

            {activeTab === 'knowledge' && (
              <KnowledgeBaseView counts={knowledgeCounts} token={token} onNotice={setStatus} onRefresh={() => void loadAppData(token)} />
            )}

            {activeTab === 'profile' && (
              <ProfileView
                counts={knowledgeCounts}
                onChangePassword={changePassword}
                onLogout={handleLogout}
                onOpenAgent={() => setActiveTab('agent')}
                onPasswordForm={setPasswordForm}
                onProfileForm={setProfileForm}
                onSaveProfile={updateProfile}
                passwordForm={passwordForm}
                profileForm={profileForm}
                section={profileSection}
                setSection={setProfileSection}
                user={user}
                workspace={vkWorkspace}
              />
            )}
          </div>
        </section>
      </div>
    </main>
  );
};

type AuthScreenProps = {
  authMode: AuthMode;
  authStatus: string;
  email: string;
  fullName: string;
  isSubmitting: boolean;
  onAuth: (event: React.FormEvent) => void;
  onFullName: (value: string) => void;
  onMode: (mode: AuthMode) => void;
  onPassword: (value: string) => void;
  onVKLogin: () => void;
  onYandexLogin: () => void;
  password: string;
  setEmail: (value: string) => void;
};

function AuthScreen({
  authMode,
  authStatus,
  email,
  fullName,
  isSubmitting,
  onAuth,
  onFullName,
  onMode,
  onPassword,
  onVKLogin,
  onYandexLogin,
  password,
  setEmail,
}: AuthScreenProps) {
  return (
    <main className="min-h-screen overflow-hidden bg-[#040b1b] text-[#e7edff]">
      <div className="mx-auto grid min-h-screen w-full max-w-6xl items-center gap-8 px-5 py-8 lg:grid-cols-[minmax(0,0.9fr)_430px]">
        <section className="hidden lg:block">
          <a href="/" className="mb-10 flex items-center">
            <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-20 w-auto brightness-0 invert" />
          </a>

          <div className="max-w-xl rounded-[22px] border border-[#25314f] bg-[#101c36]/78 p-6 shadow-2xl shadow-black/25">
            <div className="flex items-center gap-3">
              <div className="grid h-12 w-12 place-items-center rounded-[14px] bg-[#131f3a] text-[#86a2ff]">
                <Bot size={24} />
              </div>
              <div>
                <h1 className="text-3xl font-semibold">AI Agent</h1>
                <p className="mt-1 text-sm text-[#8e9ab8]">VK, CRM, knowledge base, draft-first replies.</p>
              </div>
            </div>

            <div className="mt-8 grid gap-3 sm:grid-cols-3">
              {[
                ['VK Workspace', 'History'],
                ['Agent Instructions', 'Guardrails'],
                ['Profile', 'Statistics'],
              ].map(([title, meta]) => (
                <div key={title} className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
                  <div className="text-sm font-semibold">{title}</div>
                  <div className="mt-2 text-xs text-[#8e9ab8]">{meta}</div>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="mx-auto w-full max-w-[430px]">
          <div className="mb-8 flex items-center lg:hidden">
            <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-16 w-auto brightness-0 invert" />
          </div>

          <form onSubmit={onAuth} className="rounded-[22px] border border-[#25314f] bg-[#101c36]/86 p-5 shadow-2xl shadow-black/30 sm:p-6">
            <div className="flex items-start justify-between gap-4">
              <div>
                <h1 className="text-3xl font-semibold">{authMode === 'login' ? 'Welcome Back' : 'Create account'}</h1>
                <p className="mt-2 text-sm text-[#8e9ab8]">Sign in to your AI Agent</p>
              </div>
              <Lock size={22} className="mt-1 text-[#86a2ff]" />
            </div>

            <div className="mt-6 grid grid-cols-2 rounded-lg bg-[#0b1328] p-1">
              <button
                type="button"
                onClick={() => onMode('login')}
                className={`h-11 rounded-md text-sm transition ${authMode === 'login' ? 'bg-[#e7edff] text-[#071022]' : 'text-[#8e9ab8] hover:text-white'}`}
              >
                Вход
              </button>
              <button
                type="button"
                onClick={() => onMode('register')}
                className={`h-11 rounded-md text-sm transition ${authMode === 'register' ? 'bg-[#e7edff] text-[#071022]' : 'text-[#8e9ab8] hover:text-white'}`}
              >
                Регистрация
              </button>
            </div>

            <div className="mt-5 space-y-4">
              {authMode === 'register' && (
                <Field label="Name">
                  <input
                    value={fullName}
                    onChange={(event) => onFullName(event.target.value)}
                    className="h-12 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none transition focus:border-[#86a2ff]"
                    autoComplete="name"
                    required
                  />
                </Field>
              )}

              <Field label="Email">
                <input
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  className="h-12 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none transition focus:border-[#86a2ff]"
                  type="email"
                  autoComplete="email"
                  required
                />
              </Field>

              <Field label="Password">
                <input
                  value={password}
                  onChange={(event) => onPassword(event.target.value)}
                  className="h-12 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none transition focus:border-[#86a2ff]"
                  type="password"
                  autoComplete={authMode === 'login' ? 'current-password' : 'new-password'}
                  minLength={6}
                  required
                />
              </Field>
            </div>

            <button
              type="submit"
              disabled={isSubmitting}
              className="mt-5 flex h-14 w-full items-center justify-center gap-3 rounded-full border border-[#e7edff]/45 bg-[#e7edff]/5 text-lg font-semibold text-white transition hover:bg-[#e7edff]/10 disabled:opacity-60"
            >
              {isSubmitting ? <Loader2 size={20} className="animate-spin" /> : authMode === 'login' ? <LogIn size={20} /> : <UserPlus size={20} />}
              {authMode === 'login' ? 'Sign In' : 'Create account'}
            </button>

            <div className="mt-4 grid gap-3">
              <button
                type="button"
                className="flex h-12 items-center justify-center gap-3 rounded-lg bg-black text-sm font-semibold text-white transition hover:bg-black/85"
                onClick={onYandexLogin}
              >
                <span className="font-bold">Я</span>
                Войти с Яндекс ID
              </button>

              <button
                type="button"
                onClick={onVKLogin}
                className="flex h-12 items-center justify-center gap-3 rounded-lg border border-[#72d1ff]/35 bg-[#72d1ff]/10 text-sm font-semibold text-[#dff6ff] transition hover:bg-[#72d1ff]/15"
              >
                <span className="grid h-7 w-7 place-items-center rounded-md bg-[#4c75a3] text-[11px] font-bold text-white">
                  VK
                </span>
                Войти через VK ID
              </button>
            </div>

            {authStatus && <p className="mt-4 text-sm leading-5 text-[#ff9aa8]">{authStatus}</p>}

            <button
              type="button"
              className="mx-auto mt-5 block text-sm font-semibold text-[#90a8ff]"
              onClick={() => onMode(authMode === 'login' ? 'register' : 'login')}
            >
              {authMode === 'login' ? 'Create account' : 'I already have an account'}
            </button>
            <button type="button" className="mx-auto mt-3 block text-sm text-[#8e9ab8]">
              Forgot your password?
            </button>
          </form>
        </section>
      </div>
    </main>
  );
}

function Field({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-[#8e9ab8]">{label}</span>
      {children}
    </label>
  );
}

function Panel({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <section className={`rounded-lg border border-[#25314f] bg-[#101c36] ${className}`}>
      {children}
    </section>
  );
}

function PanelHeader({
  action,
  children,
  icon: Icon,
  subtitle,
  title,
}: {
  action?: React.ReactNode;
  children?: React.ReactNode;
  icon?: React.ComponentType<{ size?: number; className?: string }>;
  subtitle?: string;
  title: string;
}) {
  return (
    <div className="border-b border-[#25314f] px-4 py-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            {Icon && <Icon size={18} className="text-[#86a2ff]" />}
            <h2 className="text-base font-semibold">{title}</h2>
          </div>
          {subtitle && <p className="mt-1 text-sm leading-5 text-[#8e9ab8]">{subtitle}</p>}
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}

function StatusChip({ tone = 'info', children }: { tone?: 'info' | 'success' | 'warning' | 'muted'; children: React.ReactNode }) {
  const classes = {
    info: 'bg-[#4c88ff]/12 text-[#c9dcff]',
    success: 'bg-[#3ac58a]/12 text-[#bdf3d9]',
    warning: 'bg-[#ffb44d]/12 text-[#ffe1aa]',
    muted: 'bg-[#131f3a] text-[#8e9ab8]',
  };
  return (
    <span className={`inline-flex items-center rounded-md px-2.5 py-1 text-xs font-medium ${classes[tone]}`}>
      {children}
    </span>
  );
}

function PrimaryAction({
  children,
  disabled,
  onClick,
  type = 'button',
}: {
  children: React.ReactNode;
  disabled?: boolean;
  onClick?: () => void;
  type?: 'button' | 'submit';
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg bg-[#5a6fff] px-4 py-2 text-sm font-semibold text-white transition hover:bg-[#4a5fe8] disabled:cursor-not-allowed disabled:opacity-55"
    >
      {children}
    </button>
  );
}

function SecondaryAction({
  children,
  disabled,
  onClick,
  type = 'button',
}: {
  children: React.ReactNode;
  disabled?: boolean;
  onClick?: () => void;
  type?: 'button' | 'submit';
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-[#8e9ab8]/35 bg-[#0b1328] px-4 py-2 text-sm font-semibold text-[#e7edff] transition hover:border-[#e7edff]/60 disabled:cursor-not-allowed disabled:opacity-55"
    >
      {children}
    </button>
  );
}

function VKWorkspaceView({
  activeIntegrationId,
  group,
  groups,
  onApproveDraft,
  onBuildContext,
  onGenerateDraft,
  onOpenAgent,
  onOpenIntegrations,
  onRefresh,
  onSelectIntegration,
  workspace,
}: {
  activeIntegrationId: string;
  group: VKConnectedGroup | null;
  groups: VKConnectedGroup[];
  onApproveDraft: (draftId: string) => void;
  onBuildContext: () => void;
  onGenerateDraft: (messageId: string) => void;
  onOpenAgent: () => void;
  onOpenIntegrations: () => void;
  onRefresh: () => void;
  onSelectIntegration: (id: string) => void;
  workspace: VKWorkspaceData | null;
}) {
  const analytics = workspace?.analytics || {};
  const messages = workspace?.recent_messages || [];
  const posts = workspace?.recent_posts || [];
  const leads = workspace?.leads || [];
  const drafts = workspace?.drafts || [];
  const settings = workspace?.agent_settings || defaultAgentSettings;

  if (!group) {
    return (
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
        <Panel className="p-6">
          <div className="flex items-start gap-4">
            <div className="grid h-12 w-12 shrink-0 place-items-center rounded-[14px] bg-[#131f3a] text-[#86a2ff]">
              <MessageSquare size={24} />
            </div>
            <div className="min-w-0">
              <h2 className="text-2xl font-semibold">VK Workspace</h2>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-[#8e9ab8]">
                После подключения сообщества сюда попадут summary бизнеса, сообщения, черновики, лиды и рекомендации агента.
              </p>
              <PrimaryAction onClick={onOpenIntegrations}>
                <PlugZap size={16} />
                Подключить площадки
              </PrimaryAction>
            </div>
          </div>
        </Panel>
        <Panel className="p-5">
          <h3 className="font-semibold">Agent settings</h3>
          <div className="mt-4 space-y-3">
            {['Draft-first', 'Safe-intents', 'RAG'].map((item) => (
              <div key={item} className="flex items-center justify-between rounded-lg bg-[#0b1328] px-3 py-3 text-sm text-[#c4cbe0]">
                {item}
                <StatusChip tone="muted">pending</StatusChip>
              </div>
            ))}
          </div>
        </Panel>
      </div>
    );
  }

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
      <div className="min-w-0 space-y-5">
        <Panel>
          <PanelHeader
            icon={MessageSquare}
            title="VK Workspace"
            subtitle="Цифровой двойник, очередь ответов и рабочие сигналы сообщества."
            action={(
              <div className="flex flex-wrap justify-end gap-2">
                <SecondaryAction onClick={onRefresh}>
                  <RefreshCw size={16} />
                  Обновить
                </SecondaryAction>
                <PrimaryAction onClick={onBuildContext}>
                  <Workflow size={16} />
                  {group.context_ready ? 'Обновить контекст' : 'Запустить контекст'}
                </PrimaryAction>
              </div>
            )}
          >
            {groups.length > 1 && (
              <div className="mt-4 flex flex-wrap gap-2">
                {groups.map((candidate) => (
                  <button
                    key={candidate.id}
                    type="button"
                    onClick={() => onSelectIntegration(candidate.id)}
                    className={`rounded-lg border px-3 py-2 text-sm ${
                      activeIntegrationId === candidate.id
                        ? 'border-[#86a2ff] bg-[#86a2ff]/12 text-[#dbe6ff]'
                        : 'border-[#25314f] bg-[#0b1328] text-[#8e9ab8]'
                    }`}
                  >
                    {candidate.group_name}
                  </button>
                ))}
              </div>
            )}
          </PanelHeader>

          <div className="p-4">
            <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
              <div className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="min-w-0">
                    <h3 className="truncate text-xl font-semibold">{group.group_name}</h3>
                    <p className="mt-1 text-sm text-[#8e9ab8]">{groupHandle(group)}</p>
                  </div>
                  <StatusChip tone={group.context_ready ? 'success' : 'warning'}>
                    {group.context_ready ? 'Контекст готов' : 'Нужен контекст'}
                  </StatusChip>
                </div>
                <div className="mt-5 grid gap-3 sm:grid-cols-3">
                  <MetricCard label="Посты" value={group.posts_count || posts.length || 0} icon={FileText} />
                  <MetricCard label="Сообщения" value={analytics.incoming_messages || group.message_count || messages.length || 0} icon={MessageSquare} />
                  <MetricCard label="Лиды" value={analytics.leads || group.lead_count || leads.length || 0} icon={Users} />
                </div>
              </div>

              <div className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
                <h3 className="font-semibold">Режим ответов</h3>
                <p className="mt-2 text-sm leading-6 text-[#8e9ab8]">
                  {settings.draft_first ? 'Draft-first включен.' : 'Draft-first выключен.'} {settings.auto_reply_enabled ? 'Автоответ разрешен для safe-intents.' : 'Автоответ пока не включен.'}
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  {(settings.safe_intents || []).map((intent) => (
                    <StatusChip key={intent} tone="info">{intent}</StatusChip>
                  ))}
                </div>
                <SecondaryAction onClick={onOpenAgent}>
                  <SlidersHorizontal size={16} />
                  Настроить
                </SecondaryAction>
              </div>
            </div>

            {(workspace?.business_snapshot?.summary || workspace?.recommendations?.length) && (
              <div className="mt-4 rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
                <div className="flex items-center gap-2">
                  <Sparkles size={18} className="text-[#86a2ff]" />
                  <h3 className="font-semibold">Digital twin</h3>
                </div>
                {workspace.business_snapshot?.summary && (
                  <p className="mt-3 text-sm leading-6 text-[#c4cbe0]">{workspace.business_snapshot.summary}</p>
                )}
                {!!workspace.recommendations?.length && (
                  <div className="mt-4 grid gap-2 md:grid-cols-2">
                    {workspace.recommendations.slice(0, 4).map((item) => (
                      <div key={item} className="rounded-lg bg-[#131f3a] px-3 py-3 text-sm text-[#c4cbe0]">
                        {item}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </Panel>

        <Panel>
          <PanelHeader icon={MessageSquare} title="Сообщения сообщества" subtitle="Входящие, draft-first и ответы от лица группы." />
          <div className="divide-y divide-[#25314f]">
            {(messages.length ? messages : fallbackDialogs.map((dialog, index) => ({
              id: `fallback-${index}`,
              from_vk_user_id: 0,
              text: dialog.text,
              is_incoming: true,
              is_processed: index === 0,
            }))).slice(0, 6).map((message) => (
              <div key={message.id} className="grid gap-3 px-4 py-4 md:grid-cols-[minmax(0,1fr)_auto]">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">VK user {message.from_vk_user_id || ''}</span>
                    <StatusChip tone={message.is_processed ? 'success' : 'warning'}>
                      {message.is_processed ? 'processed' : 'incoming'}
                    </StatusChip>
                  </div>
                  <p className="mt-2 text-sm leading-6 text-[#c4cbe0]">{message.text || 'Сообщение без текста'}</p>
                </div>
                <SecondaryAction onClick={() => onGenerateDraft(message.id)}>
                  <Send size={16} />
                  Черновик
                </SecondaryAction>
              </div>
            ))}
          </div>
        </Panel>

        <div className="grid gap-5 xl:grid-cols-2">
          <Panel>
            <PanelHeader icon={Bot} title="Черновики" subtitle="Pending drafts перед отправкой." />
            <div className="space-y-3 p-4">
              {(drafts.length ? drafts : []).map((draft) => (
                <div key={draft.id} className="rounded-lg bg-[#0b1328] p-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusChip tone={draft.safe_intent ? 'success' : 'warning'}>{draft.intent || 'intent'}</StatusChip>
                    <span className="text-xs text-[#8e9ab8]">{Math.round((draft.confidence || 0) * 100)}% confidence</span>
                  </div>
                  <p className="mt-3 text-sm leading-6 text-[#c4cbe0]">{draft.draft_text}</p>
                  <div className="mt-3">
                    <PrimaryAction onClick={() => onApproveDraft(draft.id)}>
                      <Check size={16} />
                      Approve
                    </PrimaryAction>
                  </div>
                </div>
              ))}
              {!drafts.length && <EmptyState text="Черновики появятся после первых входящих сообщений." />}
            </div>
          </Panel>

          <Panel>
            <PanelHeader icon={Users} title="Лиды" subtitle="Обогащенные профили VK." />
            <div className="space-y-3 p-4">
              {leads.slice(0, 5).map((lead) => (
                <div key={lead.id} className="rounded-lg bg-[#0b1328] p-4">
                  <div className="font-semibold">{[lead.first_name, lead.last_name].filter(Boolean).join(' ') || `VK ${lead.vk_user_id}`}</div>
                  <div className="mt-1 text-sm text-[#8e9ab8]">{[lead.city, lead.country].filter(Boolean).join(', ') || 'Гео не определено'}</div>
                  {!!lead.followers_count && <div className="mt-2 text-xs text-[#8e9ab8]">{lead.followers_count} followers</div>}
                </div>
              ))}
              {!leads.length && <EmptyState text="Лиды появятся после квалификации диалогов." />}
            </div>
          </Panel>
        </div>
      </div>

      <aside className="space-y-5">
        <Panel className="p-4">
          <h3 className="font-semibold">Analytics</h3>
          <div className="mt-4 grid gap-3">
            <MetricRow label="Incoming" value={analytics.incoming_messages || 0} />
            <MetricRow label="Outgoing" value={analytics.outgoing_messages || 0} />
            <MetricRow label="Pending drafts" value={analytics.pending_drafts || drafts.length || 0} />
            <MetricRow label="Handoff rate" value={formatPercent(analytics.handoff_rate)} />
          </div>
        </Panel>

        <Panel>
          <PanelHeader icon={FileText} title="Посты" subtitle="Последние материалы сообщества." />
          <div className="space-y-3 p-4">
            {posts.slice(0, 4).map((post) => (
              <div key={post.id} className="rounded-lg bg-[#0b1328] p-3">
                <p className="line-clamp-4 text-sm leading-5 text-[#c4cbe0]">{post.text || 'Пост без текста'}</p>
                <div className="mt-2 text-xs text-[#8e9ab8]">{post.likes_count || 0} likes · {post.comments_count || 0} comments</div>
              </div>
            ))}
            {!posts.length && <EmptyState text="Посты подтянутся после сборки контекста." />}
          </div>
        </Panel>
      </aside>
    </div>
  );
}

function IntegrationsView({
  amoStatus,
  groupRef,
  groups,
  onGroupRef,
  onOpenCrm,
  onRefresh,
  onSaveVKToken,
  onStartVKOAuth,
  onToken,
  telegramStatus,
  tokenValue,
  zohoStatus,
}: {
  amoStatus: CRMStatus | null;
  groupRef: string;
  groups: VKConnectedGroup[];
  onGroupRef: (value: string) => void;
  onOpenCrm: (kind: 'amocrm' | 'zoho') => void;
  onRefresh: () => void;
  onSaveVKToken: () => void;
  onStartVKOAuth: () => void;
  onToken: (value: string) => void;
  telegramStatus: TelegramStatus | null;
  tokenValue: string;
  zohoStatus: CRMStatus | null;
}) {
  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
      <div className="space-y-5">
        <Panel>
          <PanelHeader
            icon={PlugZap}
            title="Соцсети"
            subtitle="Площадки, из которых агент собирает контекст бизнеса."
            action={<SecondaryAction onClick={onRefresh}><RefreshCw size={16} />Обновить</SecondaryAction>}
          />
          <div className="space-y-4 p-4">
            <div className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h3 className="text-xl font-semibold">ВКонтакте</h3>
                  <p className="mt-1 text-sm text-[#8e9ab8]">
                    {groups.length ? `${groups.length} сообществ подключено` : 'Аккаунт и сообщество еще не подключены'}
                  </p>
                </div>
                <PrimaryAction onClick={onStartVKOAuth}>
                  <ExternalLink size={16} />
                  Подключить через VK
                </PrimaryAction>
              </div>

              {!!groups.length && (
                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  {groups.map((group) => (
                    <div key={group.id} className="rounded-lg bg-[#101c36] p-3">
                      <div className="font-semibold">{group.group_name}</div>
                      <div className="mt-1 text-sm text-[#8e9ab8]">{groupHandle(group)}</div>
                      <div className="mt-3 flex flex-wrap gap-2">
                        <StatusChip tone={group.context_ready ? 'success' : 'warning'}>
                          {group.context_ready ? 'context' : 'no context'}
                        </StatusChip>
                        <StatusChip tone={group.community_access_enabled ? 'success' : 'muted'}>
                          {group.community_access_enabled ? 'messages' : 'token needed'}
                        </StatusChip>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              <div className="mt-5 grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                <input
                  value={groupRef}
                  onChange={(event) => onGroupRef(event.target.value)}
                  placeholder="123456, club123456 или vk.com/club123456"
                  className="h-11 rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
                />
                <input
                  value={tokenValue}
                  onChange={(event) => onToken(event.target.value)}
                  placeholder="Ключ сообщества"
                  className="h-11 rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
                />
                <SecondaryAction disabled={!groupRef || !tokenValue} onClick={onSaveVKToken}>
                  <KeyRound size={16} />
                  Сохранить
                </SecondaryAction>
              </div>
            </div>

            <IntegrationCard
              title="Telegram"
              meta={telegramStatus?.connected ? telegramStatus.channel_title || telegramStatus.channel_username || 'Connected' : 'Disconnected'}
              connected={Boolean(telegramStatus?.connected)}
              icon={Send}
            />
          </div>
        </Panel>
      </div>

      <aside className="space-y-5">
        <Panel>
          <PanelHeader icon={Database} title="CRM" subtitle="Передача квалифицированных лидов в воронку." />
          <div className="space-y-3 p-4">
            <IntegrationCard
              title="Kommo (AmoCRM)"
              meta={amoStatus?.connected ? 'Connected' : 'Disconnected'}
              connected={Boolean(amoStatus?.connected)}
              icon={Workflow}
              action={<SecondaryAction onClick={() => onOpenCrm('amocrm')}><ExternalLink size={16} />OAuth</SecondaryAction>}
            />
            <IntegrationCard
              title="Zoho CRM"
              meta={zohoStatus?.connected ? 'Connected' : 'Disconnected'}
              connected={Boolean(zohoStatus?.connected)}
              icon={Workflow}
              action={<SecondaryAction onClick={() => onOpenCrm('zoho')}><ExternalLink size={16} />OAuth</SecondaryAction>}
            />
          </div>
        </Panel>

        <Panel className="p-4">
          <h3 className="font-semibold">Instagram / Facebook</h3>
          <p className="mt-2 text-sm leading-6 text-[#8e9ab8]">В мобильном приложении эти карточки находятся в списке соцсетей как soon-state.</p>
        </Panel>
      </aside>
    </div>
  );
}

function IntegrationCard({
  action,
  connected,
  icon: Icon,
  meta,
  title,
}: {
  action?: React.ReactNode;
  connected: boolean;
  icon: React.ComponentType<{ size?: number; className?: string }>;
  meta: string;
  title: string;
}) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
      <div className="flex min-w-0 items-center gap-3">
        <div className="grid h-11 w-11 shrink-0 place-items-center rounded-[14px] bg-[#131f3a] text-[#86a2ff]">
          <Icon size={20} />
        </div>
        <div className="min-w-0">
          <div className="font-semibold">{title}</div>
          <div className="mt-1 truncate text-sm text-[#8e9ab8]">{meta}</div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusChip tone={connected ? 'success' : 'muted'}>{connected ? 'Подключено' : 'Не подключено'}</StatusChip>
        {action}
      </div>
    </div>
  );
}

function AgentInstructionsView({
  onSave,
  setSettings,
  settings,
}: {
  onSave: () => void;
  setSettings: React.Dispatch<React.SetStateAction<VKAgentSettings>>;
  settings: VKAgentSettings;
}) {
  const toggleIntent = (intent: string) => {
    setSettings((current) => {
      const exists = current.safe_intents.includes(intent);
      return {
        ...current,
        safe_intents: exists
          ? current.safe_intents.filter((item) => item !== intent)
          : [...current.safe_intents, intent],
      };
    });
  };

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Panel>
        <PanelHeader icon={Bot} title="Agent Instructions" subtitle="Те же guardrails, tone of voice и safe-intents, что в мобильной версии." />
        <div className="space-y-5 p-4">
          <div className="grid gap-4 md:grid-cols-3">
            <ToggleCard
              checked={settings.draft_first}
              label="Draft-first"
              onChange={(checked) => setSettings((current) => ({ ...current, draft_first: checked }))}
            />
            <ToggleCard
              checked={settings.auto_reply_enabled}
              label="Auto-reply safe intents"
              onChange={(checked) => setSettings((current) => ({ ...current, auto_reply_enabled: checked }))}
            />
            <ToggleCard
              checked={settings.rag_enabled}
              label="RAG enabled"
              onChange={(checked) => setSettings((current) => ({ ...current, rag_enabled: checked }))}
            />
          </div>

          <Field label="Tone of voice">
            <textarea
              value={settings.tone_of_voice}
              onChange={(event) => setSettings((current) => ({ ...current, tone_of_voice: event.target.value }))}
              className="min-h-24 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 py-3 text-sm outline-none focus:border-[#86a2ff]"
            />
          </Field>

          <Field label="Forbidden promises">
            <textarea
              value={settings.forbidden_promises.join('\n')}
              onChange={(event) => setSettings((current) => ({
                ...current,
                forbidden_promises: event.target.value.split('\n').map((item) => item.trim()).filter(Boolean),
              }))}
              className="min-h-28 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 py-3 text-sm outline-none focus:border-[#86a2ff]"
            />
          </Field>

          <Field label="Escalation policy">
            <textarea
              value={settings.escalation_policy}
              onChange={(event) => setSettings((current) => ({ ...current, escalation_policy: event.target.value }))}
              className="min-h-24 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 py-3 text-sm outline-none focus:border-[#86a2ff]"
            />
          </Field>

          <PrimaryAction onClick={onSave}>
            <Check size={16} />
            Сохранить инструкции
          </PrimaryAction>
        </div>
      </Panel>

      <aside className="space-y-5">
        <Panel>
          <PanelHeader icon={Shield} title="Safe-intents" />
          <div className="space-y-3 p-4">
            {[
              ['faq', 'FAQ'],
              ['hours', 'Часы работы'],
              ['basic_prices', 'Базовые цены'],
              ['qualification', 'Первичная квалификация'],
            ].map(([id, label]) => (
              <button
                key={id}
                type="button"
                onClick={() => toggleIntent(id)}
                className={`flex w-full items-center justify-between rounded-lg border px-3 py-3 text-left text-sm ${
                  settings.safe_intents.includes(id)
                    ? 'border-[#3ac58a]/45 bg-[#3ac58a]/10 text-[#bdf3d9]'
                    : 'border-[#25314f] bg-[#0b1328] text-[#8e9ab8]'
                }`}
              >
                {label}
                {settings.safe_intents.includes(id) && <Check size={16} />}
              </button>
            ))}
          </div>
        </Panel>
      </aside>
    </div>
  );
}

function ToggleCard({
  checked,
  label,
  onChange,
}: {
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex min-h-20 cursor-pointer items-center justify-between rounded-lg border border-[#25314f] bg-[#0b1328] px-4 py-3">
      <span className="text-sm font-semibold">{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-5 w-5 accent-[#86a2ff]"
      />
    </label>
  );
}

function KnowledgeBaseView({
  counts,
  onNotice,
  onRefresh,
  token,
}: {
  counts: KnowledgeCounts;
  onNotice: (tone: NoticeTone, text: string) => void;
  onRefresh: () => void;
  token: string | null;
}) {
  const [section, setSection] = useState<'hub' | 'qa' | 'docs' | 'sheets' | 'web'>('hub');
  const [quickText, setQuickText] = useState({ question: '', answer: '', url: '' });

  const createQuickQA = async () => {
    if (!token || !quickText.question || !quickText.answer) {
      return;
    }
    const namespaces = await apiMaybe<ListResponse<{ id: string; type: string }>>('/api/v1/rag/namespaces', token);
    let namespaceId = namespaces?.items?.find((item) => item.type === 'qa')?.id;
    if (!namespaceId) {
      const ns = await apiRequest<{ id: string }>('/api/v1/rag/namespaces', token, {
        method: 'POST',
        body: JSON.stringify({ name: 'QA', type: 'qa', scope: 'workspace', description: 'QA' }),
      });
      namespaceId = ns.id;
    }
    await apiRequest('/api/v1/rag/qa', token, {
      method: 'POST',
      body: JSON.stringify({
        namespace_id: namespaceId,
        question: quickText.question,
        answer: quickText.answer,
        tags: [],
        is_strict: false,
      }),
    });
    setQuickText({ question: '', answer: '', url: '' });
    onNotice('success', 'QA-пара добавлена.');
    onRefresh();
  };

  const tiles = [
    { key: 'sheets' as const, label: 'Google Таблицы', count: counts.tables, icon: Table2 },
    { key: 'qa' as const, label: 'QA-пары', count: counts.qa, icon: MessageSquare },
    { key: 'docs' as const, label: 'Документы и тексты', count: counts.documents, icon: FileText },
    { key: 'web' as const, label: 'Сайты и ссылки', count: counts.web, icon: Globe2 },
  ];

  if (section !== 'hub') {
    return (
      <Panel>
        <PanelHeader
          icon={tiles.find((tile) => tile.key === section)?.icon}
          title={tiles.find((tile) => tile.key === section)?.label || 'База знаний'}
          action={<SecondaryAction onClick={() => setSection('hub')}>Назад</SecondaryAction>}
        />
        <div className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
            {section === 'qa' ? (
              <div className="space-y-3">
                <input
                  value={quickText.question}
                  onChange={(event) => setQuickText((current) => ({ ...current, question: event.target.value }))}
                  placeholder="Вопрос"
                  className="h-11 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
                />
                <textarea
                  value={quickText.answer}
                  onChange={(event) => setQuickText((current) => ({ ...current, answer: event.target.value }))}
                  placeholder="Ответ"
                  className="min-h-32 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 py-3 text-sm outline-none focus:border-[#86a2ff]"
                />
                <PrimaryAction disabled={!quickText.question || !quickText.answer} onClick={() => void createQuickQA()}>
                  <Check size={16} />
                  Добавить QA-пару
                </PrimaryAction>
              </div>
            ) : (
              <EmptyState text="Формы этого раздела будут использовать те же RAG endpoints, что мобильное приложение." />
            )}
          </div>
          <div className="rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
            <h3 className="font-semibold">Статус</h3>
            <p className="mt-2 text-sm leading-6 text-[#8e9ab8]">
              Сейчас в разделе {tiles.find((tile) => tile.key === section)?.count || 0} элементов.
            </p>
          </div>
        </div>
      </Panel>
    );
  }

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Panel>
        <PanelHeader icon={BookOpen} title="База знаний" subtitle="Источники знаний для AI-агента." />
        <div className="p-4">
          <div className="grid gap-3 sm:grid-cols-4">
            <MetricCard label="Документы" value={counts.documents} icon={FileText} />
            <MetricCard label="QA-пары" value={counts.qa} icon={MessageSquare} />
            <MetricCard label="Таблицы" value={counts.tables} icon={Table2} />
            <MetricCard label="Сайты" value={counts.web} icon={Globe2} />
          </div>

          <div className="mt-5 grid gap-3 md:grid-cols-2">
            {tiles.map((tile) => {
              const Icon = tile.icon;
              return (
                <button
                  key={tile.key}
                  type="button"
                  onClick={() => setSection(tile.key)}
                  className="flex items-center justify-between rounded-lg border border-[#25314f] bg-[#0b1328] p-4 text-left transition hover:border-[#86a2ff]/60"
                >
                  <div className="flex items-center gap-3">
                    <div className="grid h-11 w-11 place-items-center rounded-[14px] bg-[#131f3a] text-[#86a2ff]">
                      <Icon size={20} />
                    </div>
                    <div>
                      <div className="font-semibold">{tile.label}</div>
                      <div className="mt-1 text-sm text-[#8e9ab8]">{tile.count} элементов</div>
                    </div>
                  </div>
                  <ChevronRight size={18} className="text-[#8e9ab8]" />
                </button>
              );
            })}
          </div>
        </div>
      </Panel>

      <Panel className="p-4">
        <h3 className="font-semibold">Google аккаунт</h3>
        <p className="mt-2 text-sm leading-6 text-[#8e9ab8]">OAuth для Google Sheets доступен через RAG-сервис.</p>
        <SecondaryAction onClick={() => {
          if (!token) return;
          apiRequest<{ auth_url?: string; url?: string }>('/api/v1/rag/google/oauth/start', token)
            .then((result) => {
              const url = result.auth_url || result.url;
              if (url) window.open(url, '_blank', 'noopener,noreferrer');
            })
            .catch((error) => onNotice('error', error instanceof Error ? error.message : 'Google OAuth недоступен.'));
        }}>
          <ExternalLink size={16} />
          Подключить
        </SecondaryAction>
      </Panel>
    </div>
  );
}

function ProfileView({
  counts,
  onChangePassword,
  onLogout,
  onOpenAgent,
  onPasswordForm,
  onProfileForm,
  onSaveProfile,
  passwordForm,
  profileForm,
  section,
  setSection,
  user,
  workspace,
}: {
  counts: KnowledgeCounts;
  onChangePassword: () => void;
  onLogout: () => void;
  onOpenAgent: () => void;
  onPasswordForm: React.Dispatch<React.SetStateAction<{ current_password: string; new_password: string }>>;
  onProfileForm: React.Dispatch<React.SetStateAction<{ first_name: string; last_name: string }>>;
  onSaveProfile: () => void;
  passwordForm: { current_password: string; new_password: string };
  profileForm: { first_name: string; last_name: string };
  section: ProfileSection;
  setSection: (section: ProfileSection) => void;
  user: UserProfile | null;
  workspace: VKWorkspaceData | null;
}) {
  if (section === 'statistics') {
    return <StatisticsDashboard counts={counts} onBack={() => setSection('main')} workspace={workspace} />;
  }

  if (section === 'personal') {
    return (
      <Panel>
        <PanelHeader title="Personal Information" icon={User} action={<SecondaryAction onClick={() => setSection('main')}>Назад</SecondaryAction>} />
        <div className="max-w-xl space-y-4 p-4">
          <Field label="First name">
            <input
              value={profileForm.first_name}
              onChange={(event) => onProfileForm((current) => ({ ...current, first_name: event.target.value }))}
              className="h-11 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
            />
          </Field>
          <Field label="Last name">
            <input
              value={profileForm.last_name}
              onChange={(event) => onProfileForm((current) => ({ ...current, last_name: event.target.value }))}
              className="h-11 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
            />
          </Field>
          <PrimaryAction onClick={onSaveProfile}>Сохранить профиль</PrimaryAction>
        </div>
      </Panel>
    );
  }

  if (section === 'security') {
    return (
      <Panel>
        <PanelHeader title="Security & Privacy" icon={Lock} action={<SecondaryAction onClick={() => setSection('main')}>Назад</SecondaryAction>} />
        <div className="max-w-xl space-y-4 p-4">
          <Field label="Current password">
            <input
              value={passwordForm.current_password}
              onChange={(event) => onPasswordForm((current) => ({ ...current, current_password: event.target.value }))}
              className="h-11 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
              type="password"
            />
          </Field>
          <Field label="New password">
            <input
              value={passwordForm.new_password}
              onChange={(event) => onPasswordForm((current) => ({ ...current, new_password: event.target.value }))}
              className="h-11 w-full rounded-lg border border-[#25314f] bg-[#040b1b] px-3 text-sm outline-none focus:border-[#86a2ff]"
              type="password"
              minLength={6}
            />
          </Field>
          <PrimaryAction disabled={!passwordForm.current_password || !passwordForm.new_password} onClick={onChangePassword}>
            Обновить пароль
          </PrimaryAction>
        </div>
      </Panel>
    );
  }

  if (section === 'notifications') {
    return (
      <Panel>
        <PanelHeader title="Notifications" icon={Bell} action={<SecondaryAction onClick={() => setSection('main')}>Назад</SecondaryAction>} />
        <div className="grid gap-3 p-4 md:grid-cols-3">
          <ToggleCard checked label="Новые лиды" onChange={() => undefined} />
          <ToggleCard checked label="Черновики" onChange={() => undefined} />
          <ToggleCard checked={false} label="Отчеты" onChange={() => undefined} />
        </div>
      </Panel>
    );
  }

  if (section === 'accounts') {
    return (
      <Panel>
        <PanelHeader title="Connected Accounts" icon={Link2} action={<SecondaryAction onClick={() => setSection('main')}>Назад</SecondaryAction>} />
        <div className="grid gap-3 p-4 md:grid-cols-2">
          <IntegrationCard title="VK" meta={workspace?.integration ? groupHandle(workspace.integration) : 'Disconnected'} connected={Boolean(workspace?.integration)} icon={MessageSquare} />
          <IntegrationCard title="CRM" meta="See Integrations" connected={false} icon={Workflow} />
        </div>
      </Panel>
    );
  }

  const settings = [
    { id: 'personal' as const, title: 'Personal Information', icon: User },
    { id: 'security' as const, title: 'Security & Privacy', icon: Lock },
    { id: 'notifications' as const, title: 'Notifications', icon: Bell },
    { id: 'statistics' as const, title: 'Statistics', icon: BarChart3 },
    { id: 'accounts' as const, title: 'Connected Accounts', icon: Link2 },
  ];

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Panel>
        <PanelHeader title="Profile" icon={User} action={<SecondaryAction onClick={onLogout}><LogOut size={16} />Log out</SecondaryAction>} />
        <div className="p-4">
          <div className="flex items-center gap-4 rounded-lg border border-[#25314f] bg-[#0b1328] p-4">
            <div className="grid h-16 w-16 place-items-center rounded-full bg-[#5a6fff] text-xl font-bold text-white">
              {initials(user)}
            </div>
            <div className="min-w-0">
              <div className="truncate text-lg font-semibold">{displayName(user)}</div>
              <div className="mt-1 truncate text-sm text-[#8e9ab8]">{user?.email}</div>
            </div>
          </div>

          <div className="mt-5 grid gap-3 md:grid-cols-2">
            {settings.map((item) => {
              const Icon = item.icon;
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => setSection(item.id)}
                  className="flex items-center justify-between rounded-lg border border-[#25314f] bg-[#0b1328] p-4 text-left transition hover:border-[#86a2ff]/60"
                >
                  <span className="flex items-center gap-3">
                    <Icon size={18} className="text-[#86a2ff]" />
                    <span className="font-semibold">{item.title}</span>
                  </span>
                  <ChevronRight size={18} className="text-[#8e9ab8]" />
                </button>
              );
            })}
            <button
              type="button"
              onClick={onOpenAgent}
              className="flex items-center justify-between rounded-lg border border-[#25314f] bg-[#0b1328] p-4 text-left transition hover:border-[#86a2ff]/60"
            >
              <span className="flex items-center gap-3">
                <Bot size={18} className="text-[#86a2ff]" />
                <span className="font-semibold">Agent Instructions</span>
              </span>
              <ChevronRight size={18} className="text-[#8e9ab8]" />
            </button>
          </div>
        </div>
      </Panel>

      <Panel className="p-4">
        <h3 className="font-semibold">Account</h3>
        <div className="mt-4 space-y-3">
          <MetricRow label="Plan" value={user?.subscription_plan || 'free'} />
          <MetricRow label="Email verified" value={user?.email_verified ? 'yes' : 'no'} />
          <MetricRow label="Knowledge items" value={counts.documents + counts.qa + counts.tables + counts.web} />
        </div>
      </Panel>
    </div>
  );
}

function StatisticsDashboard({
  counts,
  onBack,
  workspace,
}: {
  counts: KnowledgeCounts;
  onBack: () => void;
  workspace: VKWorkspaceData | null;
}) {
  const analytics = workspace?.analytics || {};
  const dialogs = workspace?.recent_messages?.length
    ? workspace.recent_messages.slice(0, 3).map((message, index) => ({
      name: `VK user ${message.from_vk_user_id}`,
      source: 'VK',
      text: message.text || 'Сообщение без текста',
      score: [94, 78, 63][index] || 70,
      status: message.is_processed ? 'Done' : 'New',
    }))
    : fallbackDialogs;

  const knowledgeItems = [
    { name: 'Документы', count: `${counts.documents}` },
    { name: 'QA-пары', count: `${counts.qa}` },
    { name: 'Таблицы', count: `${counts.tables}` },
  ];

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold">Statistics</h2>
          <p className="mt-1 text-sm text-[#8e9ab8]">Плейсхолдер-дашборд из текущей web-версии.</p>
        </div>
        <SecondaryAction onClick={onBack}>Назад</SecondaryAction>
      </div>

      <section className="grid gap-3 sm:grid-cols-3">
        <MetricCard label="Lead score avg." value={82} suffix="+9%" icon={BarChart3} />
        <MetricCard label="Автоответы" value={analytics.outgoing_messages || 1284} suffix="сегодня" icon={Send} />
        <MetricCard label="Tools executed" value={318} suffix="12 ошибок" icon={Workflow} />
      </section>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-5">
          <Panel>
            <PanelHeader
              title="Диалоги"
              subtitle="Новые обращения, score и следующий шаг агента."
              action={<SecondaryAction>Открыть <ChevronRight size={15} /></SecondaryAction>}
            />
            <div className="divide-y divide-[#25314f]">
              {dialogs.map((dialog) => (
                <button
                  key={`${dialog.name}-${dialog.text}`}
                  type="button"
                  className="grid w-full gap-3 px-4 py-4 text-left transition hover:bg-[#0b1328] md:grid-cols-[minmax(0,1fr)_130px]"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{dialog.name}</span>
                      <span className="rounded-md bg-[#131f3a] px-2 py-1 text-xs text-[#8e9ab8]">{dialog.source}</span>
                    </div>
                    <p className="mt-2 text-sm leading-6 text-[#8e9ab8]">{dialog.text}</p>
                  </div>
                  <div className="flex items-center gap-3 md:justify-end">
                    <div className="text-right">
                      <div className="text-2xl font-semibold">{dialog.score}</div>
                      <div className="text-xs text-[#3ac58a]">{dialog.status}</div>
                    </div>
                    <CheckCircle2 size={20} className="text-[#3ac58a]" />
                  </div>
                </button>
              ))}
            </div>
          </Panel>

          <Panel className="p-4">
            <div className="flex items-center justify-between">
              <h2 className="text-base font-semibold">Agent stream</h2>
              <Activity size={18} className="text-[#3ac58a]" />
            </div>
            <div className="mt-4 space-y-3">
              {agentEvents.map((event) => (
                <div key={event.title} className="flex gap-3 rounded-lg bg-[#0b1328] p-3">
                  <span className={`mt-1 h-2.5 w-2.5 shrink-0 rounded-full ${
                    event.tone === 'green' ? 'bg-[#3ac58a]' : event.tone === 'blue' ? 'bg-[#72d1ff]' : 'bg-[#ffb44d]'
                  }`} />
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{event.title}</span>
                      <span className="text-xs text-[#8e9ab8]">{event.meta}</span>
                    </div>
                    <p className="mt-1 text-sm leading-5 text-[#8e9ab8]">{event.detail}</p>
                  </div>
                </div>
              ))}
            </div>
          </Panel>
        </div>

        <aside className="space-y-5">
          <Panel className="p-4">
            <h2 className="text-base font-semibold">База знаний</h2>
            <div className="mt-4 space-y-3">
              {knowledgeItems.map((item) => (
                <div key={item.name} className="flex items-center justify-between rounded-lg bg-[#0b1328] px-3 py-3">
                  <span className="text-sm">{item.name}</span>
                  <span className="text-xs text-[#8e9ab8]">{item.count}</span>
                </div>
              ))}
            </div>
          </Panel>

          <Panel className="p-4">
            <div className="flex items-center gap-2">
              <Settings size={18} className="text-[#ffb44d]" />
              <h2 className="text-base font-semibold">Agent settings</h2>
            </div>
            <div className="mt-4 space-y-3">
              {['Auto-send only high confidence', 'Use CRM tools', 'Enrich digital twins'].map((setting, index) => (
                <label key={setting} className="flex items-center justify-between rounded-lg bg-[#0b1328] px-3 py-3">
                  <span className="text-sm text-[#c4cbe0]">{setting}</span>
                  <input className="h-4 w-4 accent-[#86a2ff]" type="checkbox" defaultChecked={index !== 0} />
                </label>
              ))}
            </div>
          </Panel>
        </aside>
      </div>
    </div>
  );
}

function MetricCard({
  icon: Icon,
  label,
  suffix,
  value,
}: {
  icon: React.ComponentType<{ size?: number; className?: string }>;
  label: string;
  suffix?: string;
  value: number | string;
}) {
  return (
    <article className="rounded-lg border border-[#25314f] bg-[#101c36] p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-[#8e9ab8]">{label}</span>
        <Icon size={17} className="text-[#72d1ff]" />
      </div>
      <div className="mt-4 flex items-end gap-2">
        <strong className="text-3xl font-semibold tracking-normal">{value}</strong>
        {suffix && <span className="pb-1 text-xs text-[#8e9ab8]">{suffix}</span>}
      </div>
    </article>
  );
}

function MetricRow({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="flex items-center justify-between rounded-lg bg-[#0b1328] px-3 py-3 text-sm">
      <span className="text-[#8e9ab8]">{label}</span>
      <span className="font-semibold text-[#e7edff]">{value}</span>
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-dashed border-[#25314f] bg-[#0b1328] px-4 py-8 text-center text-sm text-[#8e9ab8]">
      {text}
    </div>
  );
}

export default WebApp;
