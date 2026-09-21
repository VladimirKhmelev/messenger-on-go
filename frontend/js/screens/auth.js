import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';
import { t, getLocale, setLocale } from '../i18n.js';

// Mirrors auth-service's own rule (services/auth-service/internal/service/password.go):
// 8+ chars, ASCII-only, at least one letter, at least one digit. ASCII-only
// isn't just style — the password wraps the user's RSA private key, and
// non-ASCII input is subject to Unicode normalization that can differ across
// devices/keyboards, silently locking a user out of their own key elsewhere.
export const PASSWORD_RULES = [
  { key: 'length', label: () => t('auth.passwordRules.length'), test: (pw) => pw.length >= 8 },
  { key: 'letter', label: () => t('auth.passwordRules.letter'), test: (pw) => /[a-zA-Z]/.test(pw) },
  { key: 'digit', label: () => t('auth.passwordRules.digit'), test: (pw) => /\d/.test(pw) },
  { key: 'ascii', label: () => t('auth.passwordRules.ascii'), test: (pw) => /^[\x20-\x7e]*$/.test(pw) },
];

export function passwordMeetsRules(password) {
  return PASSWORD_RULES.every((rule) => rule.test(password));
}

// The auth screens are the only place a user can pick a language before
// logging in — Settings (the other switcher) is behind the login wall.
function renderAuthHeader() {
  const isDark = getTheme() === 'dark';
  return `
    <div class="auth-header">
      <div class="brand">
        <div class="brand-mark"></div>
        <div class="brand-name">${t('brand.name')}</div>
      </div>
      <div class="auth-header-actions">
        <select class="locale-select" data-input="locale" title="${t('language.label')}">
          <option value="ru" ${getLocale() === 'ru' ? 'selected' : ''}>RU</option>
          <option value="en" ${getLocale() === 'en' ? 'selected' : ''}>EN</option>
        </select>
        <button class="theme-toggle" data-on="${isDark}" title="${t('theme.toggleTitle')}" data-action="toggle-theme">
          <span class="knob"></span>
        </button>
      </div>
    </div>
  `;
}

function wireAuthHeader(root, handlers) {
  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerender();
  });

  root.querySelector('[data-input="locale"]').addEventListener('change', (event) => {
    setLocale(event.target.value);
    handlers.onRerender();
  });
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
        ${renderAuthHeader()}

        <div class="auth-title">${isRegister ? t('auth.register.title') : t('auth.login.title')}</div>
        <div class="auth-subtitle">${isRegister ? t('auth.register.subtitle') : t('auth.login.subtitle')}</div>

        <form class="field-list" data-form="auth">
          ${
            isRegister
              ? `<div class="field">
                   <label>${t('auth.register.nameLabel')}</label>
                   <input type="text" name="displayName" placeholder="${t('auth.register.namePlaceholder')}" required autocomplete="name" value="${escapeHtml(
                     displayNameValue
                   )}" />
                 </div>`
              : ''
          }
          <div class="field">
            <label>${t('auth.fields.emailLabel')}</label>
            <input type="email" name="email" placeholder="${t('auth.fields.emailPlaceholder')}" required autocomplete="email" value="${escapeHtml(emailValue)}" />
          </div>
          <div class="field field--password">
            <label>${t('auth.fields.passwordLabel')}</label>
            <input type="password" name="password" placeholder="${t('auth.fields.passwordPlaceholder')}" required autocomplete="${
              isRegister ? 'new-password' : 'current-password'
            }" value="${escapeHtml(passwordValue)}" data-input="password" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="${t('auth.fields.showPassword')}">${eyeIcon(
              false
            )}</button>
          </div>
          ${isRegister ? `<div class="password-rules" data-password-rules>${renderPasswordRules(passwordValue)}</div>` : ''}
          ${
            !isRegister
              ? `<div class="auth-forgot-password">
                   <span class="action" data-action="forgot-password">${t('auth.forgotPassword')}</span>
                 </div>`
              : ''
          }
          ${
            isRegister
              ? `<div class="field field--tag">
                   <label>${t('auth.register.tagLabel')}</label>
                   <span class="at-prefix">@</span>
                   <input type="text" name="tag" placeholder="${t('auth.register.tagPlaceholder')}" required data-input="tag" value="${escapeHtml(tagValue)}" />
                 </div>
                 <div class="field-hint">${t('auth.register.tagHint')}</div>
                 <div class="tag-availability" data-tag-availability>${renderTagAvailability()}</div>`
              : ''
          }
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>
            ${isRegister ? t('auth.register.submit') : t('auth.login.submit')}
          </button>
        </form>

        <div class="divider">
          <div class="line"></div><span>${t('auth.or')}</span><div class="line"></div>
        </div>
        <button class="btn-secondary" data-action="github-login">${t('auth.githubLogin')}</button>

        <div class="auth-toggle">
          ${isRegister ? t('auth.register.toggle') : t('auth.login.toggle')}
          <span class="action" data-action="toggle-auth-mode">${isRegister ? t('auth.register.toggleAction') : t('auth.login.toggleAction')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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
    passwordToggle.title = showing ? t('auth.fields.showPassword') : t('auth.fields.hidePassword');
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
    return `<span class="password-rule password-rule--${cls}"><span class="password-rule-icon">${icon}</span>${escapeHtml(rule.label())}</span>`;
  }).join('');
}

function renderTagAvailability() {
  const check = state.tagCheck;
  if (!check) return '';

  if (check.available) {
    return `<span class="tag-availability--ok">${t('auth.register.tagAvailable')}</span>`;
  }

  const suggestion = check.suggestedTag
    ? ` ${t('auth.register.tagTrySuggestion')} <span class="action" data-action="use-suggested-tag">@${escapeHtml(check.suggestedTag)}</span>`
    : '';
  return `<span class="tag-availability--taken">${t('auth.register.tagTaken')}</span>${suggestion}`;
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

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        ${renderAuthHeader()}

        <div class="auth-title">${t('auth.unlock.title')}</div>
        <div class="auth-subtitle">${t('auth.unlock.subtitle')}</div>

        <form class="field-list" data-form="unlock">
          <div class="field field--password">
            <label>${t('auth.fields.passwordLabel')}</label>
            <input type="password" name="password" placeholder="${t('auth.fields.passwordPlaceholder')}" required autocomplete="current-password" data-input="password" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="${t('auth.fields.showPassword')}">${eyeIcon(
              false
            )}</button>
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>${t('auth.unlock.submit')}</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="logout-instead">${t('auth.unlock.logoutInstead')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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
    passwordToggle.title = showing ? t('auth.fields.showPassword') : t('auth.fields.hidePassword');
  });
}

// GitHub OAuth confirms identity but carries no secret we can derive an
// encryption key from — so a first-time GitHub login still needs a password
// from the user, used only to wrap/unwrap this device's E2E private key
// (never sent to GitHub or checked against anything server-side beyond the
// wrapped blob). Returning GitHub users are prompted for the same password
// they set the first time — get it wrong and messages just won't decrypt.
function renderGitHubPassphrase(root, handlers) {

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        ${renderAuthHeader()}

        <div class="auth-title">${t('auth.githubPassphrase.title')}</div>
        <div class="auth-subtitle">${t('auth.githubPassphrase.subtitle')}</div>

        <form class="field-list" data-form="github-passphrase">
          <div class="field field--password">
            <label>${t('auth.fields.passwordLabel')}</label>
            <input type="password" name="password" placeholder="${t('auth.fields.passwordPlaceholder')}" required autocomplete="new-password" data-input="password" minlength="8" />
            <button type="button" class="password-toggle" data-action="toggle-password" title="${t('auth.fields.showPassword')}">${eyeIcon(
              false
            )}</button>
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>${t('auth.githubPassphrase.submit')}</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="cancel-github">${t('auth.githubPassphrase.cancel')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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
    passwordToggle.title = showing ? t('auth.fields.showPassword') : t('auth.fields.hidePassword');
  });
}

function renderVerify(root, handlers) {

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        ${renderAuthHeader()}

        <div class="auth-title">${t('auth.verify.title')}</div>
        <div class="auth-subtitle">${t('auth.verify.subtitle', { email: state.pendingVerifyEmail })}</div>

        <form class="field-list" data-form="verify">
          <div class="field">
            <label>${t('auth.verify.codeLabel')}</label>
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
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>${t('auth.verify.submit')}</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="back-to-login">${t('auth.verify.backToLogin')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        ${renderAuthHeader()}

        <div class="auth-title">${t('auth.forgotPasswordScreen.title')}</div>
        <div class="auth-subtitle">${t('auth.forgotPasswordScreen.subtitle')}</div>
        <div class="form-warning">
          ${t('auth.forgotPasswordScreen.warning')}
        </div>

        <form class="field-list" data-form="forgot-password">
          <div class="field">
            <label>${t('auth.fields.emailLabel')}</label>
            <input type="email" name="email" placeholder="${t('auth.fields.emailPlaceholder')}" required autocomplete="email" />
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <div class="form-success">${state.authSuccess || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>${t('auth.forgotPasswordScreen.submit')}</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="back-to-login">${t('auth.forgotPasswordScreen.backToLogin')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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

  root.innerHTML = `
    <div class="auth-screen">
      <div class="auth-card">
        ${renderAuthHeader()}

        <div class="auth-title">${t('auth.resetPassword.title')}</div>
        <div class="auth-subtitle">${t('auth.resetPassword.subtitle')}</div>
        <div class="form-warning">
          ${t('auth.resetPassword.warning')}
        </div>

        <form class="field-list" data-form="reset-password">
          <div class="field">
            <label>${t('auth.resetPassword.tokenLabel')}</label>
            <input type="text" name="token" placeholder="${t('auth.resetPassword.tokenPlaceholder')}" required />
          </div>
          <div class="field">
            <label>${t('auth.resetPassword.newPasswordLabel')}</label>
            <input type="password" name="newPassword" placeholder="${t('auth.fields.passwordPlaceholder')}" required autocomplete="new-password" />
          </div>
          <div class="form-error">${state.authError || ''}</div>
          <button type="submit" class="btn-primary" ${state.authBusy ? 'disabled' : ''}>${t('auth.resetPassword.submit')}</button>
        </form>

        <div class="auth-toggle">
          <span class="action" data-action="back-to-login">${t('auth.resetPassword.backToLogin')}</span>
        </div>
      </div>
    </div>
  `;

  wireAuthHeader(root, handlers);

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
