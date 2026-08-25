import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';

// Mirrors auth-service's own rule (services/auth-service/internal/service/password.go):
// 8+ chars, at least one letter, at least one digit.
export const PASSWORD_RULES = [
  { key: 'length', label: 'Минимум 8 символов', test: (pw) => pw.length >= 8 },
  { key: 'letter', label: 'Хотя бы одна буква', test: (pw) => /\p{L}/u.test(pw) },
  { key: 'digit', label: 'Хотя бы одна цифра', test: (pw) => /\d/.test(pw) },
];

export function passwordMeetsRules(password) {
  return PASSWORD_RULES.every((rule) => rule.test(password));
}

export function renderAuth(root, handlers) {
  if (state.authMode === 'unlock') {
    renderUnlock(root, handlers);
    return;
  }
  if (state.authMode === 'verify') {
    renderVerify(root, handlers);
    return;
  }
  if (state.authMode === 'forgot-password') {
    renderForgotPassword(root, handlers);
    return;
  }
  if (state.authMode === 'reset-password') {
    renderResetPassword(root, handlers);
    return;
  }
  if (state.authMode === 'github-passphrase') {
    renderGitHubPassphrase(root, handlers);
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
            <div class="brand-name">Wisply</div>
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
          ${isRegister ? `<div class="password-rules" data-password-rules>${renderPasswordRules(passwordValue)}</div>` : ''}
          ${
            !isRegister
              ? `<div class="auth-forgot-password">
                   <span class="action" data-action="forgot-password">Забыли пароль?</span>
                 </div>`
              : ''
          }
          ${
            isRegister
              ? `<div class="field field--tag">
                   <label>Тег</label>
                   <span class="at-prefix">@</span>
                   <input type="text" name="tag" placeholder="username" required data-input="tag" value="${escapeHtml(tagValue)}" />
                 </div>
                 <div class="field-hint">Только строчные латинские буквы, цифры и _, от 3 до 20 символов</div>
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

  root.querySelector('[data-action="forgot-password"]')?.addEventListener('click', () => {
    state.authMode = 'forgot-password';
    state.authError = '';
    state.authSuccess = '';
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
      const lower = event.target.value.toLowerCase();
      if (lower !== event.target.value) {
        const pos = event.target.selectionStart;
        event.target.value = lower;
        event.target.setSelectionRange(pos, pos);
      }
      handlers.onTagInput(lower);
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

  if (isRegister) {
    const rulesEl = root.querySelector('[data-password-rules]');
    passwordInput.addEventListener('input', (event) => {
      rulesEl.innerHTML = renderPasswordRules(event.target.value);
    });
  }
}

function eyeIcon(open) {
  return open
    ? `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.94 10.94 0 0 1 12 20c-7 0-11-8-11-8a18.5 18.5 0 0 1 5.06-5.94M9.9 4.24A10.94 10.94 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`
    : `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`;
}

function renderPasswordRules(password) {
  return PASSWORD_RULES.map((rule) => {
    const ok = password.length > 0 && rule.test(password);
    const cls = password.length === 0 ? 'pending' : ok ? 'ok' : 'fail';
    const icon = password.length === 0 ? '•' : ok ? '✓' : '✕';
    return `<span class="password-rule password-rule--${cls}"><span class="password-rule-icon">${icon}</span>${escapeHtml(rule.label)}</span>`;
  }).join('');
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

// Shown when a session is restored via the refresh-token cookie (F5, reopened
// tab) but this device has no locally-cached private key yet — the key only
// ever lives unwrapped in memory/IndexedDB, never in the refresh token, so
// the password has to be typed once more to unlock chats on this device.
function renderUnlock(root, handlers) {
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisply</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">Разблокировать чаты</div>
        <div class="auth-subtitle">Введите пароль, чтобы расшифровать сообщения на этом устройстве</div>

        <form class="field-list" data-form="unlock">
          <div class="field field--password">
            <label>Пароль</label>
            <input type="password" name="password" placeholder="••••••••" required autocomplete="current-password" data-input="password" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="Показать пароль">${eyeIcon(
              false
            )}</button>
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>Разблокировать</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="logout-instead">Выйти из аккаунта</span>
        </div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerender();
  });

  root.querySelector('[data-action="logout-instead"]').addEventListener('click', () => {
    handlers.onLogout();
  });

  root.querySelector('[data-form="unlock"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const password = formData.get('password');
    handlers.onUnlock(password);
  });

  const passwordInput = root.querySelector('[data-input="password"]');
  passwordInput.focus();
  const passwordToggle = root.querySelector('[data-action="toggle-password"]');
  passwordToggle.addEventListener('click', () => {
    const showing = passwordInput.type === 'text';
    passwordInput.type = showing ? 'password' : 'text';
    passwordToggle.innerHTML = eyeIcon(!showing);
    passwordToggle.title = showing ? 'Показать пароль' : 'Скрыть пароль';
  });
}

// GitHub OAuth confirms identity but carries no secret we can derive an
// encryption key from — so a first-time GitHub login still needs a password
// from the user, used only to wrap/unwrap this device's E2E private key
// (never sent to GitHub or checked against anything server-side beyond the
// wrapped blob). Returning GitHub users are prompted for the same password
// they set the first time — get it wrong and messages just won't decrypt.
function renderGitHubPassphrase(root, handlers) {
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisply</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">Пароль для шифрования</div>
        <div class="auth-subtitle">GitHub подтвердил вашу личность, но для сквозного шифрования сообщений нужен отдельный пароль — придумайте его сейчас, если входите впервые, или введите тот же, что и раньше</div>

        <form class="field-list" data-form="github-passphrase">
          <div class="field field--password">
            <label>Пароль</label>
            <input type="password" name="password" placeholder="••••••••" required autocomplete="new-password" data-input="password" minlength="8" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="Показать пароль">${eyeIcon(
              false
            )}</button>
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>Продолжить</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="cancel-github">Отменить вход</span>
        </div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerender();
  });

  root.querySelector('[data-action="cancel-github"]').addEventListener('click', () => {
    handlers.onCancelGitHub();
  });

  root.querySelector('[data-form="github-passphrase"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const password = formData.get('password');
    handlers.onGitHubPassphrase(password);
  });

  const passwordInput = root.querySelector('[data-input="password"]');
  passwordInput.focus();
  const passwordToggle = root.querySelector('[data-action="toggle-password"]');
  passwordToggle.addEventListener('click', () => {
    const showing = passwordInput.type === 'text';
    passwordInput.type = showing ? 'password' : 'text';
    passwordToggle.innerHTML = eyeIcon(!showing);
    passwordToggle.title = showing ? 'Показать пароль' : 'Скрыть пароль';
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
            <div class="brand-name">Wisply</div>
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

function renderForgotPassword(root, handlers) {
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisply</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">Восстановление пароля</div>
        <div class="auth-subtitle">Введите email — мы отправим токен для сброса пароля</div>
        <div class="form-warning">
          Сброс пароля создаст новый ключ шифрования — история переписки в старых чатах станет
          недоступна для чтения. Новые сообщения будут отправляться и читаться как обычно.
        </div>

        <form class="field-list" data-form="forgot-password">
          <div class="field">
            <label>Email</label>
            <input type="email" name="email" placeholder="you@example.com" required autocomplete="email" />
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <div class="form-success">${state.authSuccess || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>Отправить токен</button>
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
    state.authSuccess = '';
    handlers.onRerender();
  });

  root.querySelector('[data-form="forgot-password"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const email = formData.get('email');
    handlers.onRequestPasswordReset(email);
  });
}

function renderResetPassword(root, handlers) {
  const isDark = getTheme() === 'dark';

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        <div class="auth-header">
          <div class="brand">
            <div class="brand-mark"></div>
            <div class="brand-name">Wisply</div>
          </div>
          <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
            <span class="knob"></span>
          </button>
        </div>

        <div class="auth-title">Новый пароль</div>
        <div class="auth-subtitle">Введите токен из письма и новый пароль</div>
        <div class="form-warning">
          Сообщения зашифрованы ключом, который восстанавливается только вашим текущим паролем.
          Сброс пароля создаст новый ключ шифрования — история переписки в старых чатах станет
          недоступна для чтения. Новые сообщения будут отправляться и читаться как обычно.
        </div>

        <form class="field-list" data-form="reset-password">
          <div class="field">
            <label>Токен из письма</label>
            <input type="text" name="token" placeholder="Токен сброса пароля" required />
          </div>
          <div class="field">
            <label>Новый пароль</label>
            <input type="password" name="newPassword" placeholder="••••••••" required autocomplete="new-password" />
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>Сохранить пароль</button>
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
    state.authSuccess = '';
    handlers.onRerender();
  });

  root.querySelector('[data-form="reset-password"]').addEventListener('submit', (event) => {
    event.preventDefault();
    const formData = new FormData(event.target);
    const token = formData.get('token');
    const newPassword = formData.get('newPassword');
    handlers.onResetPassword({ token, newPassword });
  });
}
