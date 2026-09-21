import { state } from '../state.js';
import { renderAvatar } from '../avatar.js';
import { t, getLocale, setLocale } from '../i18n.js';

export function renderSettings(root, handlers) {
  if (!state.settingsOpen) {
    root.innerHTML = '';
    return;
  }

  const prevTagInput = root.querySelector('[data-input="settings-tag"]');
  const tagHadFocus = document.activeElement === prevTagInput;
  const tagSelectionStart = prevTagInput?.selectionStart;
  const tagSelectionEnd = prevTagInput?.selectionEnd;
  const tagValue = prevTagInput ? prevTagInput.value : (state.currentUser?.tag ?? '');

  const prevNameInput = root.querySelector('[data-input="settings-name"]');
  const nameHadFocus = document.activeElement === prevNameInput;
  const nameSelectionStart = prevNameInput?.selectionStart;
  const nameSelectionEnd = prevNameInput?.selectionEnd;
  const nameValue = prevNameInput ? prevNameInput.value : (state.currentUser?.displayName ?? '');

  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-backdrop">
      <div class="modal" data-action="stop-propagation">
        <div class="modal-header">
          <div class="modal-title">${t('settings.title')}</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="settings-avatar-row">
          ${renderAvatar(state.currentUser?.id ?? '', state.currentUser?.tag ?? '', nameValue, {
            sizeClass: 'avatar--md',
          })}
          <div class="settings-avatar-actions">
            <label class="btn-secondary settings-avatar-upload">
              ${t('settings.changePhoto')}
              <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" data-input="avatar-file" hidden />
            </label>
            ${state.settingsAvatarBusy ? `<span class="settings-avatar-status">${t('settings.uploading')}</span>` : ''}
          </div>
        </div>
        <div class="form-error">${state.settingsAvatarError || ''}</div>

        <div class="field">
          <label>${t('language.label')}</label>
          <select data-input="settings-language">
            <option value="ru" ${getLocale() === 'ru' ? 'selected' : ''}>${t('language.ru')}</option>
            <option value="en" ${getLocale() === 'en' ? 'selected' : ''}>${t('language.en')}</option>
          </select>
        </div>

        ${
          state.settingsPushSupported
            ? `
              <div class="field field--toggle">
                <label>
                  <input type="checkbox" data-input="settings-push" ${state.settingsPushEnabled ? 'checked' : ''} ${state.settingsPushBusy ? 'disabled' : ''} />
                  ${t('settings.pushNotifications')}
                </label>
              </div>
              <div class="form-error">${state.settingsPushError || ''}</div>
            `
            : ''
        }

        <div class="field">
          <label>${t('settings.nameLabel')}</label>
          <input type="text" value="${escapeHtml(nameValue)}" data-input="settings-name" />
        </div>
        <div class="form-error">${state.settingsNameError || ''}</div>
        <button class="btn-primary" data-action="save-name" ${state.settingsNameBusy ? 'disabled' : ''}>
          ${t('settings.saveName')}
        </button>

        <div class="field field--tag">
          <label>${t('settings.tagLabel')}</label>
          <span class="at-prefix">@</span>
          <input type="text" value="${escapeHtml(tagValue)}" data-input="settings-tag" />
        </div>
        <div class="tag-availability" data-tag-availability>${renderTagAvailability()}</div>
        <div class="form-error">${state.settingsError || ''}</div>
        <button class="btn-primary" data-action="save-tag" ${state.settingsBusy ? 'disabled' : ''}>
          ${t('settings.saveTag')}
        </button>

        <div class="field">
          <label>${t('settings.oldPasswordLabel')}</label>
          <input type="password" data-input="settings-old-password" autocomplete="current-password" />
        </div>
        <div class="field">
          <label>${t('settings.newPasswordLabel')}</label>
          <input type="password" data-input="settings-new-password" autocomplete="new-password" />
        </div>
        <div class="field">
          <label>${t('settings.newPasswordConfirmLabel')}</label>
          <input type="password" data-input="settings-new-password-confirm" autocomplete="new-password" />
        </div>
        <div class="form-error">${state.settingsPasswordError || ''}</div>
        <div class="form-success">${state.settingsPasswordSuccess || ''}</div>
        <button class="btn-primary" data-action="save-password" ${state.settingsPasswordBusy ? 'disabled' : ''}>
          ${t('settings.changePassword')}
        </button>

        ${
          state.settingsBlockedUsers.length > 0
            ? `
              <div class="field">
                <label>${t('settings.blockedUsersLabel')}</label>
              </div>
              <div class="form-error">${state.settingsBlockedUsersError || ''}</div>
              <div class="blocked-users-list">
                ${state.settingsBlockedUsers
                  .map(
                    (u) => `
                      <div class="blocked-users-item">
                        <span>${escapeHtml(u.displayName || u.tag)}</span>
                        <button class="btn-secondary" data-action="unblock-user" data-user-id="${escapeHtml(u.id)}" ${state.settingsUnblockingUserId === u.id ? 'disabled' : ''}>
                          ${t('settings.unblock')}
                        </button>
                      </div>
                    `
                  )
                  .join('')}
              </div>
            `
            : ''
        }

        <div class="settings-danger-zone">
          <div class="settings-danger-title">${t('settings.dangerZone.title')}</div>
          ${
            state.settingsDeleteAccountConfirming
              ? `
                <div class="settings-danger-warning">
                  ${t('settings.dangerZone.warning')}
                </div>
                <div class="field">
                  <label>${t('settings.dangerZone.passwordLabel')}</label>
                  <input type="password" data-input="delete-account-password" autocomplete="current-password" placeholder="${t('settings.dangerZone.passwordPlaceholder')}" />
                </div>
                <div class="form-error">${state.settingsDeleteAccountError || ''}</div>
                <div class="settings-danger-actions">
                  <button class="btn-secondary" data-action="cancel-delete-account" ${state.settingsDeleteAccountBusy ? 'disabled' : ''}>
                    ${t('settings.dangerZone.cancel')}
                  </button>
                  <button class="btn-danger" data-action="confirm-delete-account" ${state.settingsDeleteAccountBusy ? 'disabled' : ''}>
                    ${t('settings.dangerZone.confirmDelete')}
                  </button>
                </div>
              `
              : `<button class="btn-danger" data-action="start-delete-account">${t('settings.dangerZone.startDelete')}</button>`
          }
        </div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="close-backdrop"]').addEventListener('click', () => {
    handlers.onClose();
  });
  root.querySelector('[data-action="close"]').addEventListener('click', () => {
    handlers.onClose();
  });
  root.querySelector('.modal').addEventListener('click', (event) => {
    event.stopPropagation();
  });

  root.querySelector('[data-input="settings-language"]').addEventListener('change', (event) => {
    setLocale(event.target.value);
    handlers.onRerenderAll();
  });

  const tagInput = root.querySelector('[data-input="settings-tag"]');
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

  root.querySelector('[data-action="save-tag"]').addEventListener('click', () => {
    handlers.onSaveTag(tagInput.value);
  });

  if (tagHadFocus) {
    tagInput.focus();
    tagInput.setSelectionRange(tagSelectionStart, tagSelectionEnd);
  }

  const nameInput = root.querySelector('[data-input="settings-name"]');
  root.querySelector('[data-action="save-name"]').addEventListener('click', () => {
    handlers.onSaveDisplayName(nameInput.value);
  });

  if (nameHadFocus) {
    nameInput.focus();
    nameInput.setSelectionRange(nameSelectionStart, nameSelectionEnd);
  }

  root.querySelector('[data-input="settings-push"]')?.addEventListener('change', (event) => {
    handlers.onTogglePush(event.target.checked);
  });

  root.querySelectorAll('[data-action="unblock-user"]').forEach((el) => {
    el.addEventListener('click', () => {
      handlers.onUnblockUser(el.getAttribute('data-user-id'));
    });
  });

  root.querySelector('[data-input="avatar-file"]').addEventListener('change', (event) => {
    const file = event.target.files?.[0];
    if (file) handlers.onUploadAvatar(file);
    event.target.value = '';
  });

  root.querySelector('[data-action="save-password"]').addEventListener('click', () => {
    const oldPassword = root.querySelector('[data-input="settings-old-password"]').value;
    const newPassword = root.querySelector('[data-input="settings-new-password"]').value;
    const confirmPassword = root.querySelector('[data-input="settings-new-password-confirm"]').value;
    handlers.onChangePassword(oldPassword, newPassword, confirmPassword);
  });

  root.querySelector('[data-action="start-delete-account"]')?.addEventListener('click', () => {
    handlers.onStartDeleteAccount();
  });
  root.querySelector('[data-action="cancel-delete-account"]')?.addEventListener('click', () => {
    handlers.onCancelDeleteAccount();
  });
  root.querySelector('[data-action="confirm-delete-account"]')?.addEventListener('click', () => {
    const password = root.querySelector('[data-input="delete-account-password"]')?.value ?? '';
    handlers.onConfirmDeleteAccount(password);
  });
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
