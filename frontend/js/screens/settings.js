import { state } from '../state.js';
import { renderAvatar } from '../avatar.js';

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
          <div class="modal-title">Настройки</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="settings-avatar-row">
          ${renderAvatar(state.currentUser?.id ?? '', state.currentUser?.tag ?? '', nameValue, {
            sizeClass: 'avatar--md',
          })}
          <div class="settings-avatar-actions">
            <label class="btn-secondary settings-avatar-upload">
              Изменить фото
              <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" data-input="avatar-file" hidden />
            </label>
            ${state.settingsAvatarBusy ? '<span class="settings-avatar-status">Загрузка...</span>' : ''}
          </div>
        </div>
        <div class="form-error">${state.settingsAvatarError || ''}</div>

        <div class="field">
          <label>Имя</label>
          <input type="text" value="${escapeHtml(nameValue)}" data-input="settings-name" />
        </div>
        <div class="form-error">${state.settingsNameError || ''}</div>
        <button class="btn-primary" data-action="save-name" ${state.settingsNameBusy ? 'disabled' : ''}>
          Сохранить имя
        </button>

        <div class="field field--tag">
          <label>Тег</label>
          <span class="at-prefix">@</span>
          <input type="text" value="${escapeHtml(tagValue)}" data-input="settings-tag" />
        </div>
        <div class="tag-availability" data-tag-availability>${renderTagAvailability()}</div>
        <div class="form-error">${state.settingsError || ''}</div>
        <button class="btn-primary" data-action="save-tag" ${state.settingsBusy ? 'disabled' : ''}>
          Сохранить тег
        </button>

        <div class="field">
          <label>Текущий пароль</label>
          <input type="password" data-input="settings-old-password" autocomplete="current-password" />
        </div>
        <div class="field">
          <label>Новый пароль</label>
          <input type="password" data-input="settings-new-password" autocomplete="new-password" />
        </div>
        <div class="field">
          <label>Подтвердите новый пароль</label>
          <input type="password" data-input="settings-new-password-confirm" autocomplete="new-password" />
        </div>
        <div class="form-error">${state.settingsPasswordError || ''}</div>
        <div class="form-success">${state.settingsPasswordSuccess || ''}</div>
        <button class="btn-primary" data-action="save-password" ${state.settingsPasswordBusy ? 'disabled' : ''}>
          Сменить пароль
        </button>
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
