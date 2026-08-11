import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';
import { avatarPalette, initialsFrom } from '../avatar.js';

export function renderSidebar(root, handlers) {
  const isDark = getTheme() === 'dark';
  const query = state.searchQuery.trim().toLowerCase();
  const filtered = state.chats.filter((chat) => {
    if (!query) return true;
    return (
      chat.peer.tag.toLowerCase().includes(query) ||
      (chat.peer.email || '').toLowerCase().includes(query)
    );
  });

  const prevInput = root.querySelector('[data-input="search"]');
  const hadFocus = document.activeElement === prevInput;
  const selectionStart = prevInput?.selectionStart;
  const selectionEnd = prevInput?.selectionEnd;

  root.innerHTML = `
    <div class="sidebar">
      <div class="sidebar-top">
        <div class="sidebar-top-row">
          <div class="brand">
            <div class="brand-mark brand-mark--sm"></div>
            <div class="brand-name brand-name--sm">Wisp</div>
          </div>
          <div class="sidebar-top-actions">
            <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
              <span class="knob"></span>
            </button>
            <button class="logout-btn" title="Выйти" data-action="logout">Выход</button>
          </div>
        </div>
        <div class="search-wrap">
          <span class="search-icon-circle"></span>
          <span class="search-icon-line"></span>
          <input
            type="text"
            class="search-input"
            placeholder="Найти по тегу"
            value="${escapeHtml(state.searchQuery)}"
            data-input="search"
          />
        </div>
      </div>
      <div class="chat-list">
        ${filtered.length === 0 && state.foundUser ? renderCreateChatRow(state.foundUser) : ''}
        ${filtered.map((chat) => renderChatRow(chat)).join('')}
      </div>
    </div>
  `;

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerenderAll();
  });

  root.querySelector('[data-action="logout"]').addEventListener('click', () => {
    handlers.onLogout();
  });

  const searchInput = root.querySelector('[data-input="search"]');
  searchInput.addEventListener('input', (event) => {
    state.searchQuery = event.target.value;
    handlers.onSearchChange();
  });
  if (hadFocus) {
    searchInput.focus();
    searchInput.setSelectionRange(selectionStart, selectionEnd);
  }

  root.querySelectorAll('[data-chat-id]').forEach((el) => {
    el.addEventListener('click', () => {
      handlers.onSelectChat(el.getAttribute('data-chat-id'));
    });
  });

  const createRow = root.querySelector('[data-action="create-chat"]');
  createRow?.addEventListener('click', () => {
    handlers.onCreateChat(state.foundUser);
  });
}

function renderCreateChatRow(user) {
  const palette = avatarPalette(user.tag);
  return `
    <button class="chat-row" data-action="create-chat">
      <div class="avatar" style="background:${palette.bg};color:${palette.text}">
        ${initialsFrom(user.tag)}
      </div>
      <div class="chat-row-body">
        <div class="chat-row-name">Создать чат с @${escapeHtml(user.tag)}</div>
        <div class="chat-row-tag">@${escapeHtml(user.tag)}</div>
      </div>
    </button>
  `;
}

function renderChatRow(chat) {
  const palette = avatarPalette(chat.peer.tag);
  const lastMessage = chat.messages[chat.messages.length - 1];
  const lastTime = lastMessage ? formatTime(lastMessage.createdAtUnix) : '';
  const lastPreview = lastMessage ? escapeHtml(lastMessage.text) : '';
  const isSelected = chat.id === state.selectedChatId;
  const dotColor = chat.online ? 'var(--dot-online)' : 'var(--dot-offline)';

  return `
    <button class="chat-row" data-chat-id="${chat.id}" data-selected="${isSelected}">
      <div class="avatar" style="background:${palette.bg};color:${palette.text}">
        ${initialsFrom(chat.peer.tag)}
        <div class="avatar-dot" style="background:${dotColor}"></div>
      </div>
      <div class="chat-row-body">
        <div class="chat-row-line1">
          <div class="chat-row-name">${escapeHtml(chat.peer.tag)}</div>
          <div class="chat-row-time">${lastTime}</div>
        </div>
        <div class="chat-row-preview">${lastPreview}</div>
      </div>
    </button>
  `;
}

function formatTime(unixSeconds) {
  if (!unixSeconds) return '';
  const date = new Date(Number(unixSeconds) * 1000);
  const h = date.getHours();
  const m = date.getMinutes();
  return `${h}:${m < 10 ? '0' : ''}${m}`;
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str ?? '';
  return div.innerHTML;
}

export { escapeHtml, formatTime };
