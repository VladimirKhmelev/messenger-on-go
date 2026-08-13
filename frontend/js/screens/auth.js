import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';

export function renderAuth(root, handlers) {
  if (state.authMode === 'verify') {
    renderVerify(root, handlers);
    return;
  }

  const isRegister = state.authMode === 'register';
  const isDark = getTheme() === 'dark';

  const prevTagInput = root.querySelector('[data-input="tag"]');
  const tagHadFocus = document.activeElement === prevTagInput;
  const tagSelectionStart = prevTagInput?.selectionStart;
  const tagSelectionEnd = prevTagInput?.selectionEnd;
  const tagValue = prevTagInput?.value ?? '';

  const prevEmailInput = root.querySelector('[name="email"]');
  const prevPasswordInput = root.querySelector('[name="password"]');
  const prevDisplayNameInput = root.querySelector('[name="displayName"]');
  const emailValue = prevEmailInput?.value ?? '';
  const passwordValue = prevPasswordInput?.value ?? '';
  const displayNameValue = prevDisplayNameInput?.value ?? '';
  const focusedField = document.activeElement?.getAttribute?.('name');
  const focusedSelectionStart = document.activeElement?.selectionStart;
  const focusedSelectionEnd = document.activeElement?.selectionEnd;

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
          ${
            isRegister
              ? `<div class="field">
                   <label>Имя</label>
                   <input type="text" name="displayName" placeholder="Как вас называть" required autocomplete="name" value="${escapeHtml(
                     displayNameValue
                   )}" />
                 </div>`
              : ''
          }
          <div class="field">
            <label>Email</label>
            <input type="email" name="email" placeholder="you@example.com" required autocomplete="email" value="${escapeHtml(emailValue)}" />
          </div>
          <div class="field field--password">
            <label>Пароль</label>
            <input type="password" name="password" placeholder="••••••••" required autocomplete="${
              isRegister ? 'new-password' : 'current-password'
            }" value="${escapeHtml(passwordValue)}" data-input="password" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="Показать пароль">${eyeIcon(
              false
            )}</button>
          </div>
          ${
            isRegister
              ? `<div class="field field--tag">
                   <label>Тег</label>
                   <span class="at-prefix">@</span>
                   <input type="text" name="tag" placeholder="username" required data-input="tag" value="${escapeHtml(tagValue)}" />
                 </div>
                 <div class="tag-availability" data-tag-availability>${renderTagAvailability()}</div>`
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
    const displayName = formData.get('displayName');
    handlers.onSubmit({ email, password, tag, displayName, isRegister });
  });

  if (isRegister) {
    const tagInput = root.querySelector('[data-input="tag"]');
    tagInput.addEventListener('input', (event) => {
      handlers.onTagInput(event.target.value);
    });

    root.querySelector('[data-tag-availability]')?.addEventListener('click', (event) => {
      const suggestion = event.target.closest('[data-action="use-suggested-tag"]');
      if (!suggestion) return;
      tagInput.value = state.tagCheck.suggestedTag;
      handlers.onTagInput(state.tagCheck.suggestedTag);
    });

    if (tagHadFocus) {
      tagInput.focus();
      tagInput.setSelectionRange(tagSelectionStart, tagSelectionEnd);
    }
  }

  if (focusedField === 'email' || focusedField === 'password' || focusedField === 'displayName') {
    const fieldInput = root.querySelector(`[name="${focusedField}"]`);
    fieldInput.focus();
    fieldInput.setSelectionRange(focusedSelectionStart, focusedSelectionEnd);
  }

  const passwordInput = root.querySelector('[data-input="password"]');
  const passwordToggle = root.querySelector('[data-action="toggle-password"]');
  passwordToggle.addEventListener('click', () => {
    const showing = passwordInput.type === 'text';
    passwordInput.type = showing ? 'password' : 'text';
    passwordToggle.innerHTML = eyeIcon(!showing);
    passwordToggle.title = showing ? 'Показать пароль' : 'Скрыть пароль';
  });
}

function eyeIcon(open) {
  return open
    ? `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.94 10.94 0 0 1 12 20c-7 0-11-8-11-8a18.5 18.5 0 0 1 5.06-5.94M9.9 4.24A10.94 10.94 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`
    : `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`;
}

function renderTagAvailability() {
  const check = state.tagCheck;
  if (!check) return '';

  if (check.available) {
    return '<span class="tag-availability--ok">Тег свободен</span>';
  }

  const suggestion = check.suggestedTag
    ? ` Попробуйте <span class="action" data-action="use-suggested-tag">@${escapeHtml(check.suggestedTag)}</span>`
    : '';
  return `<span class="tag-availability--taken">Тег уже занят.</span>${suggestion}`;
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str ?? '';
  return div.innerHTML;
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
