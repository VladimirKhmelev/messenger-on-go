import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';

export function renderAuth(root, handlers) {
  if (state.authMode === 'verify') {
    renderVerify(root, handlers);
    return;
  }

  const isRegister = state.authMode === 'register';
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisp</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">${isRegister ? 'Создать аккаунт' : 'С возвращением'}</div>
        <div class="auth-subtitle">${
          isRegister ? 'Заполните данные, чтобы начать переписку' : 'Войдите, чтобы продолжить переписку'
        }</div>

        <form class="field-list" data-form="auth">
          <div class="field">
            <label>Email</label>
            <input type="email" name="email" placeholder="you@example.com" required autocomplete="email" />
          </div>
          <div class="field">
            <label>Пароль</label>
            <input type="password" name="password" placeholder="••••••••" required autocomplete="${
              isRegister ? 'new-password' : 'current-password'
            }" />
          </div>
          ${
            isRegister
              ? `<div class="field field--tag">
                   <label>Тег</label>
                   <span class="at-prefix">@</span>
                   <input type="text" name="tag" placeholder="username" required />
                 </div>`
              : ''
          }
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>
            ${isRegister ? 'Создать аккаунт' : 'Войти'}
          </button>
        </form>

        <div class="divider">
          <div class="line"></div><span>или</span><div class="line"></div>
        </div>
        <button class="btn-secondary" data-action="github-login">Продолжить с GitHub</button>

        <div class="auth-toggle">
          ${isRegister ? 'Уже есть аккаунт?' : 'Нет аккаунта?'}
          <span class="action" data-action="toggle-auth-mode">${isRegister ? 'Войти' : 'Зарегистрироваться'}</span>
        </div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerender();
  });

  root.querySelector('[data-action="toggle-auth-mode"]').addEventListener('click', () => {
    state.authMode = isRegister ? 'login' : 'register';
    state.authError = '';
    handlers.onRerender();
  });

  root.querySelector('[data-action="github-login"]').addEventListener('click', () => {
    handlers.onGitHubLogin();
  });

  root.querySelector('[data-form="auth"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const email = formData.get('email');
    const password = formData.get('password');
    const tag = formData.get('tag');
    handlers.onSubmit({ email, password, tag, isRegister });
  });
}

function renderVerify(root, handlers) {
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisp</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">Подтвердите email</div>
        <div class="auth-subtitle">Мы отправили код на ${state.pendingVerifyEmail}</div>

        <form class="field-list" data-form="verify">
          <div class="field">
            <label>Код подтверждения</label>
            <input
              type="text"
              name="code"
              placeholder="123456"
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength="6"
              required
            />
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>Подтвердить</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="back-to-login">Назад ко входу</span>
        </div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerender();
  });

  root.querySelector('[data-action="back-to-login"]').addEventListener('click', () => {
    state.authMode = 'login';
    state.authError = '';
    handlers.onRerender();
  });

  root.querySelector('[data-form="verify"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const code = formData.get('code');
    handlers.onVerify({ email: state.pendingVerifyEmail, code });
  });
}
