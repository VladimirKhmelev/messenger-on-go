import { state } from '../state.js';
import { renderAvatar } from '../avatar.js';
import { escapeHtml } from './sidebar.js';

export function renderGroupCreator(root, handlers) {
  if (!state.groupCreatorOpen) {
    root.innerHTML = '';
    return;
  }

  const prevNameInput = root.querySelector('[data-input="group-name"]');
  const nameHadFocus = document.activeElement === prevNameInput;
  const nameSelectionStart = prevNameInput?.selectionStart;
  const nameSelectionEnd = prevNameInput?.selectionEnd;

  const prevSearchInput = root.querySelector('[data-input="group-search"]');
  const searchHadFocus = document.activeElement === prevSearchInput;
  const searchSelectionStart = prevSearchInput?.selectionStart;
  const searchSelectionEnd = prevSearchInput?.selectionEnd;

  const canSubmit = state.groupCreatorName.trim() && state.groupCreatorSelected.length >= 2;

  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-backdrop">
      <div class="modal" data-action="stop-propagation">
        <div class="modal-header">
          <div class="modal-title">Новая группа</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="field">
          <label>Название группы</label>
          <input type="text" value="${escapeHtml(state.groupCreatorName)}" data-input="group-name" placeholder="Например, Прайм 2026" />
        </div>

        ${
          state.groupCreatorSelected.length > 0
            ? `<div class="group-selected-chips">
                ${state.groupCreatorSelected
                  .map(
                    (u) => `
                  <span class="group-chip">
                    ${escapeHtml(u.displayName || u.tag)}
                    <span class="group-chip-remove" data-action="remove-selected" data-user-id="${escapeHtml(u.id)}">×</span>
                  </span>
                `
                  )
                  .join('')}
              </div>`
            : ''
        }

        <div class="field">
          <label>Добавить участников</label>
          <input type="text" value="${escapeHtml(state.groupCreatorQuery)}" data-input="group-search" placeholder="Поиск по тегу" />
        </div>

        <div class="chat-list group-search-results">
          ${state.groupCreatorFoundUsers.map((user) => renderCandidateRow(user)).join('')}
        </div>

        <div class="form-error">${state.groupCreatorError || ''}</div>
        <button class="btn-primary" data-action="submit" ${canSubmit ? '' : 'disabled'} ${state.groupCreatorBusy ? 'disabled' : ''}>
          ${state.groupCreatorBusy ? 'Создание...' : 'Создать группу'}
        </button>
      </div>
    </div>
  `;

  root.querySelector('[data-action="close-backdrop"]').addEventListener('click', () => handlers.onClose());
  root.querySelector('[data-action="close"]').addEventListener('click', () => handlers.onClose());
  root.querySelector('.modal').addEventListener('click', (event) => event.stopPropagation());

  const nameInput = root.querySelector('[data-input="group-name"]');
  nameInput.addEventListener('input', (event) => {
    state.groupCreatorName = event.target.value;
    handlers.onRerender();
  });
  if (nameHadFocus) {
    nameInput.focus();
    nameInput.setSelectionRange(nameSelectionStart, nameSelectionEnd);
  }

  const searchInput = root.querySelector('[data-input="group-search"]');
  searchInput.addEventListener('input', (event) => {
    handlers.onSearchChange(event.target.value);
  });
  if (searchHadFocus) {
    searchInput.focus();
    searchInput.setSelectionRange(searchSelectionStart, searchSelectionEnd);
  }

  root.querySelectorAll('[data-action="add-candidate"]').forEach((el) => {
    el.addEventListener('click', () => {
      handlers.onToggleMember(el.getAttribute('data-user-id'));
    });
  });

  root.querySelectorAll('[data-action="remove-selected"]').forEach((el) => {
    el.addEventListener('click', () => {
      handlers.onToggleMember(el.getAttribute('data-user-id'));
    });
  });

  root.querySelector('[data-action="submit"]').addEventListener('click', () => {
    handlers.onSubmit();
  });
}

function renderCandidateRow(user) {
  const name = user.displayName || user.tag;
  return `
    <button class="chat-row" data-action="add-candidate" data-user-id="${escapeHtml(user.id)}">
      ${renderAvatar(user.id, user.tag, name)}
      <div class="chat-row-body">
        <div class="chat-row-name">${escapeHtml(name)}</div>
        <div class="chat-row-tag">@${escapeHtml(user.tag)}</div>
      </div>
    </button>
  `;
}
