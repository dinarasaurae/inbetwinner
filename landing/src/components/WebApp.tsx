import React, { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  BarChart3,
  Bell,
  BookOpen,
  Bot,
  CheckCircle2,
  ChevronRight,
  LayoutDashboard,
  Loader2,
  Lock,
  LogIn,
  MessageSquare,
  Search,
  Send,
  Settings,
  Shield,
  Sparkles,
  User,
  UserPlus,
  Workflow,
} from 'lucide-react';
import { Button } from './ui/button';

type UserProfile = {
  id: string;
  email: string;
  name?: string | null;
  first_name?: string | null;
  last_name?: string | null;
  avatar_url?: string | null;
  subscription_plan?: string;
};

type AuthResponse = {
  access_token: string;
  refresh_token: string;
  user: UserProfile;
};

type AuthMode = 'login' | 'register';

const navItems = [
  { id: 'overview', label: 'Главная', icon: LayoutDashboard },
  { id: 'dialogs', label: 'Диалоги', icon: MessageSquare },
  { id: 'agents', label: 'Агенты', icon: Bot },
  { id: 'knowledge', label: 'База знаний', icon: BookOpen },
  { id: 'profile', label: 'Профиль', icon: User },
];

const agentEvents = [
  {
    title: 'VK lead qualified',
    meta: '12 секунд назад',
    detail: 'AI Supervisor поднял score до 94 и подготовил следующий шаг.',
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

const dialogs = [
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

const knowledgeItems = [
  { name: 'Тарифы Enterprise', count: '18 Q&A' },
  { name: 'Интеграции CRM', count: '11 документов' },
  { name: 'Сценарии демо', count: '7 плейбуков' },
];

function displayName(user: UserProfile | null) {
  if (!user) {
    return 'Demo workspace';
  }

  const fullName = [user.first_name, user.last_name].filter(Boolean).join(' ');
  return fullName || user.name || user.email;
}

async function readJson<T>(response: Response): Promise<T> {
  const payload = await response.json().catch(() => null);

  if (!response.ok) {
    const error = payload?.error || payload?.message || 'Request failed';
    throw new Error(error);
  }

  return payload as T;
}

const WebApp: React.FC = () => {
  const [activeTab, setActiveTab] = useState('overview');
  const [authMode, setAuthMode] = useState<AuthMode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('inbetwin_access_token'));
  const [user, setUser] = useState<UserProfile | null>(() => {
    const saved = localStorage.getItem('inbetwin_user');
    return saved ? JSON.parse(saved) as UserProfile : null;
  });
  const [authStatus, setAuthStatus] = useState<string>('');
  const [isSubmitting, setSubmitting] = useState(false);

  useEffect(() => {
    document.title = 'inBeTwin Web App';
  }, []);

  useEffect(() => {
    if (!token) {
      return;
    }

    fetch('/api/v1/auth/profile', {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((response) => readJson<{ data: UserProfile }>(response))
      .then((payload) => {
        setUser(payload.data);
        localStorage.setItem('inbetwin_user', JSON.stringify(payload.data));
      })
      .catch(() => {
        localStorage.removeItem('inbetwin_access_token');
        localStorage.removeItem('inbetwin_refresh_token');
        localStorage.removeItem('inbetwin_user');
        setToken(null);
        setUser(null);
      });
  }, [token]);

  const workspaceName = useMemo(() => displayName(user), [user]);

  const handleAuth = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setAuthStatus('');

    const endpoint = authMode === 'login' ? '/api/v1/auth/login' : '/api/v1/auth/register';
    const body = authMode === 'login'
      ? { email, password }
      : { email, password, name };

    try {
      const payload = await readJson<AuthResponse | { data: UserProfile }>(await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }));

      if ('access_token' in payload) {
        localStorage.setItem('inbetwin_access_token', payload.access_token);
        localStorage.setItem('inbetwin_refresh_token', payload.refresh_token);
        localStorage.setItem('inbetwin_user', JSON.stringify(payload.user));
        setToken(payload.access_token);
        setUser(payload.user);
        setAuthStatus('Готово: сессия открыта.');
      } else {
        setAuthMode('login');
        setAuthStatus('Аккаунт создан. Теперь можно войти.');
      }
    } catch (error) {
      setAuthStatus(error instanceof Error ? error.message : 'Не удалось выполнить запрос.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem('inbetwin_access_token');
    localStorage.removeItem('inbetwin_refresh_token');
    localStorage.removeItem('inbetwin_user');
    setToken(null);
    setUser(null);
  };

  return (
    <main className="min-h-screen bg-[#0c0d10] text-[#f7f7f5]">
      <div className="flex min-h-screen">
        <aside className="hidden lg:flex w-[280px] shrink-0 flex-col border-r border-white/10 bg-[#101216] px-5 py-6">
          <a href="/" className="mb-8 flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-[#eef2ff] text-[#111318]">
              <Sparkles size={19} />
            </div>
            <div>
              <div className="font-display text-2xl italic leading-none">inBeTwin</div>
              <div className="mt-1 text-xs text-white/45">AI sales cockpit</div>
            </div>
          </a>

          <nav className="flex flex-col gap-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = activeTab === item.id;
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => setActiveTab(item.id)}
                  className={`flex h-11 items-center gap-3 rounded-lg px-3 text-left text-sm transition ${
                    isActive ? 'bg-white text-[#101216]' : 'text-white/66 hover:bg-white/7 hover:text-white'
                  }`}
                >
                  <Icon size={18} />
                  {item.label}
                </button>
              );
            })}
          </nav>

          <div className="mt-auto rounded-lg border border-white/10 bg-white/[0.03] p-4">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Shield size={16} className="text-emerald-300" />
              Агент активен
            </div>
            <p className="mt-2 text-xs leading-5 text-white/48">
              24 диалога под наблюдением, 3 требуют ручного подтверждения.
            </p>
          </div>
        </aside>

        <section className="flex min-w-0 flex-1 flex-col pb-20 lg:pb-0">
          <header className="sticky top-0 z-40 border-b border-white/10 bg-[#0c0d10]/85 px-4 py-4 backdrop-blur-xl sm:px-6 lg:px-8">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-xs uppercase tracking-[0.18em] text-white/42">Workspace</div>
                <h1 className="truncate text-xl font-semibold sm:text-2xl">{workspaceName}</h1>
              </div>
              <div className="flex items-center gap-2">
                <div className="hidden h-10 items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 text-sm text-white/52 md:flex">
                  <Search size={16} />
                  Поиск лидов
                </div>
                <button
                  type="button"
                  className="grid h-10 w-10 place-items-center rounded-lg border border-white/10 bg-white/[0.03] text-white/72"
                  aria-label="Уведомления"
                  title="Уведомления"
                >
                  <Bell size={18} />
                </button>
              </div>
            </div>
          </header>

          <div className="grid flex-1 gap-5 px-4 py-5 sm:px-6 lg:grid-cols-[minmax(0,1fr)_360px] lg:px-8">
            <div className="min-w-0 space-y-5">
              <section className="grid gap-3 sm:grid-cols-3">
                {[
                  { label: 'Lead score avg.', value: '82', suffix: '+9%', icon: BarChart3, color: 'text-sky-300' },
                  { label: 'Автоответы', value: '1 284', suffix: 'сегодня', icon: Send, color: 'text-emerald-300' },
                  { label: 'Tools executed', value: '318', suffix: '12 ошибок', icon: Workflow, color: 'text-amber-300' },
                ].map((metric) => {
                  const Icon = metric.icon;
                  return (
                    <article key={metric.label} className="rounded-lg border border-white/10 bg-[#14171d] p-4">
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-xs text-white/45">{metric.label}</span>
                        <Icon size={17} className={metric.color} />
                      </div>
                      <div className="mt-4 flex items-end gap-2">
                        <strong className="text-3xl font-semibold tracking-normal">{metric.value}</strong>
                        <span className="pb-1 text-xs text-white/42">{metric.suffix}</span>
                      </div>
                    </article>
                  );
                })}
              </section>

              <section className="rounded-lg border border-white/10 bg-[#14171d]">
                <div className="flex items-center justify-between border-b border-white/10 px-4 py-4">
                  <div>
                    <h2 className="text-base font-semibold">Диалоги</h2>
                    <p className="mt-1 text-xs text-white/45">Новые обращения, score и следующий шаг агента.</p>
                  </div>
                  <Button variant="heroSecondary" size="sm" onClick={() => setActiveTab('dialogs')}>
                    Открыть
                    <ChevronRight size={15} />
                  </Button>
                </div>

                <div className="divide-y divide-white/10">
                  {dialogs.map((dialog) => (
                    <button
                      key={dialog.name}
                      type="button"
                      className="grid w-full gap-3 px-4 py-4 text-left transition hover:bg-white/[0.03] md:grid-cols-[minmax(0,1fr)_130px]"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{dialog.name}</span>
                          <span className="rounded-md bg-white/7 px-2 py-1 text-xs text-white/52">{dialog.source}</span>
                        </div>
                        <p className="mt-2 text-sm leading-6 text-white/56">{dialog.text}</p>
                      </div>
                      <div className="flex items-center gap-3 md:justify-end">
                        <div className="text-right">
                          <div className="text-2xl font-semibold">{dialog.score}</div>
                          <div className="text-xs text-emerald-300">{dialog.status}</div>
                        </div>
                        <CheckCircle2 size={20} className="text-emerald-300" />
                      </div>
                    </button>
                  ))}
                </div>
              </section>

              <section className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
                <article className="rounded-lg border border-white/10 bg-[#14171d] p-4">
                  <div className="flex items-center justify-between">
                    <h2 className="text-base font-semibold">Agent stream</h2>
                    <Activity size={18} className="text-emerald-300" />
                  </div>
                  <div className="mt-4 space-y-3">
                    {agentEvents.map((event) => (
                      <div key={event.title} className="flex gap-3 rounded-lg bg-white/[0.025] p-3">
                        <span className={`mt-1 h-2.5 w-2.5 shrink-0 rounded-full ${
                          event.tone === 'green' ? 'bg-emerald-300' : event.tone === 'blue' ? 'bg-sky-300' : 'bg-amber-300'
                        }`} />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-medium">{event.title}</span>
                            <span className="text-xs text-white/36">{event.meta}</span>
                          </div>
                          <p className="mt-1 text-sm leading-5 text-white/52">{event.detail}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </article>

                <article className="rounded-lg border border-white/10 bg-[#14171d] p-4">
                  <h2 className="text-base font-semibold">База знаний</h2>
                  <div className="mt-4 space-y-3">
                    {knowledgeItems.map((item) => (
                      <button
                        key={item.name}
                        type="button"
                        className="flex w-full items-center justify-between rounded-lg bg-white/[0.025] px-3 py-3 text-left hover:bg-white/[0.05]"
                      >
                        <span className="text-sm">{item.name}</span>
                        <span className="text-xs text-white/42">{item.count}</span>
                      </button>
                    ))}
                  </div>
                </article>
              </section>
            </div>

            <aside className="space-y-5">
              <section className="rounded-lg border border-white/10 bg-[#14171d] p-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h2 className="text-base font-semibold">{token ? 'Профиль' : 'Вход'}</h2>
                    <p className="mt-1 text-xs text-white/45">{token ? user?.email : 'Нет активной сессии'}</p>
                  </div>
                  {token ? <User size={18} className="text-sky-300" /> : <Lock size={18} className="text-sky-300" />}
                </div>

                {token ? (
                  <div className="mt-5 space-y-3">
                    <div className="rounded-lg bg-white/[0.03] p-3">
                      <div className="text-xs text-white/42">Тариф</div>
                      <div className="mt-1 font-medium">{user?.subscription_plan || 'free'}</div>
                    </div>
                    <Button variant="heroSecondary" className="w-full" onClick={handleLogout}>
                      Выйти
                    </Button>
                  </div>
                ) : (
                  <form className="mt-5 space-y-3" onSubmit={handleAuth}>
                    <div className="grid grid-cols-2 rounded-lg bg-white/[0.04] p-1">
                      <button
                        type="button"
                        onClick={() => setAuthMode('login')}
                        className={`rounded-md px-3 py-2 text-sm ${authMode === 'login' ? 'bg-white text-[#111318]' : 'text-white/56'}`}
                      >
                        Вход
                      </button>
                      <button
                        type="button"
                        onClick={() => setAuthMode('register')}
                        className={`rounded-md px-3 py-2 text-sm ${authMode === 'register' ? 'bg-white text-[#111318]' : 'text-white/56'}`}
                      >
                        Регистрация
                      </button>
                    </div>

                    {authMode === 'register' && (
                      <label className="block">
                        <span className="mb-1 block text-xs text-white/44">Имя</span>
                        <input
                          value={name}
                          onChange={(event) => setName(event.target.value)}
                          className="h-11 w-full rounded-lg border border-white/10 bg-[#0c0d10] px-3 text-sm outline-none focus:border-sky-300"
                          autoComplete="name"
                        />
                      </label>
                    )}

                    <label className="block">
                      <span className="mb-1 block text-xs text-white/44">Email</span>
                      <input
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                        className="h-11 w-full rounded-lg border border-white/10 bg-[#0c0d10] px-3 text-sm outline-none focus:border-sky-300"
                        type="email"
                        autoComplete="email"
                        required
                      />
                    </label>

                    <label className="block">
                      <span className="mb-1 block text-xs text-white/44">Пароль</span>
                      <input
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        className="h-11 w-full rounded-lg border border-white/10 bg-[#0c0d10] px-3 text-sm outline-none focus:border-sky-300"
                        type="password"
                        autoComplete={authMode === 'login' ? 'current-password' : 'new-password'}
                        minLength={6}
                        required
                      />
                    </label>

                    <Button variant="heroSecondary" className="w-full gap-2" disabled={isSubmitting}>
                      {isSubmitting ? <Loader2 size={16} className="animate-spin" /> : authMode === 'login' ? <LogIn size={16} /> : <UserPlus size={16} />}
                      {authMode === 'login' ? 'Войти' : 'Создать аккаунт'}
                    </Button>

                    {authStatus && <p className="text-xs leading-5 text-white/54">{authStatus}</p>}
                  </form>
                )}
              </section>

              <section className="rounded-lg border border-white/10 bg-[#14171d] p-4">
                <div className="flex items-center gap-2">
                  <Settings size={18} className="text-amber-300" />
                  <h2 className="text-base font-semibold">Agent settings</h2>
                </div>
                <div className="mt-4 space-y-3">
                  {['Auto-send only high confidence', 'Use CRM tools', 'Enrich digital twins'].map((setting, index) => (
                    <label key={setting} className="flex items-center justify-between rounded-lg bg-white/[0.025] px-3 py-3">
                      <span className="text-sm text-white/72">{setting}</span>
                      <input className="h-4 w-4 accent-sky-300" type="checkbox" defaultChecked={index !== 0} />
                    </label>
                  ))}
                </div>
              </section>
            </aside>
          </div>
        </section>
      </div>

      <nav className="fixed inset-x-0 bottom-0 z-50 grid grid-cols-5 border-t border-white/10 bg-[#101216]/95 px-2 py-2 backdrop-blur-xl lg:hidden">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => setActiveTab(item.id)}
              className={`flex min-h-14 flex-col items-center justify-center gap-1 rounded-lg text-[11px] ${
                isActive ? 'bg-white text-[#101216]' : 'text-white/58'
              }`}
              title={item.label}
            >
              <Icon size={18} />
              <span className="max-w-full truncate">{item.label}</span>
            </button>
          );
        })}
      </nav>
    </main>
  );
};

export default WebApp;
