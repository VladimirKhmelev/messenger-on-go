import { state } from '../state.js';
import { getTheme, toggleTheme } from '../theme.js';
import { renderAvatar, groupAvatarUrl, avatarUrl, snapshotAvatarImages, restoreAvatarImages } from '../avatar.js';

export function renderSidebar(root, handlers) {
  const isDark = getTheme() === 'dark';
  const query = state.searchQuery.trim().toLowerCase();
  const filtered = state.chats.filter((chat) => {
    if (!query) return true;
    if (chat.type === 'group') return chat.name.toLowerCase().includes(query);
    return (
      chat.peer.tag.toLowerCase().includes(query) ||
      (chat.peer.displayName || '').toLowerCase().includes(query) ||
      (chat.peer.email || '').toLowerCase().includes(query)
    );
  });

  const prevInput = root.querySelector('[data-input="search"]');
  const hadFocus = document.activeElement === prevInput;
  const selectionStart = prevInput?.selectionStart;
  const selectionEnd = prevInput?.selectionEnd;
  const avatarSnapshot = snapshotAvatarImages(root);

  root.innerHTML = `
    <div class="sidebar">
      <div class="sidebar-top">
        <div class="sidebar-top-row">
          <div class="brand">
            <div class="brand-mark brand-mark--sm"></div>
            <div class="brand-name brand-name--sm">Wisply</div>
          </div>
          <div class="sidebar-top-actions">
            <button class="theme-toggle" data-on="${isDark}" title="Тёмная тема" data-action="toggle-theme">
              <span class="knob"></span>
            </button>
            <button class="settings-btn" title="Новая группа" data-action="open-group-creator">☰</button>
            <button class="settings-btn" title="Настройки" data-action="open-settings">⚙</button>
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
        ${state.foundUsers.map((user) => renderCreateChatRow(user)).join('')}
        ${filtered.map((chat) => renderChatRow(chat)).join('')}
      </div>
    </div>
  `;
  restoreAvatarImages(root, avatarSnapshot);

  root.querySelector('[data-action="toggle-theme"]').addEventListener('click', () => {
    toggleTheme();
    handlers.onRerenderAll();
  });

  root.querySelector('[data-action="logout"]').addEventListener('click', () => {
    handlers.onLogout();
  });

  root.querySelector('[data-action="open-settings"]').addEventListener('click', () => {
    handlers.onOpenSettings();
  });

  root.querySelector('[data-action="open-group-creator"]').addEventListener('click', () => {
    handlers.onOpenGroupCreator();
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

  root.querySelectorAll('[data-action="create-chat"]').forEach((el) => {
    el.addEventListener('click', () => {
      const userId = el.getAttribute('data-user-id');
      const user = state.foundUsers.find((u) => u.id === userId);
      handlers.onCreateChat(user);
    });
  });
}

function renderCreateChatRow(user) {
  const name = user.displayName || user.tag;
  return `
    <button class="chat-row" data-action="create-chat" data-user-id="${escapeHtml(user.id)}">
      ${renderAvatar(user.id, user.tag, name)}
      <div class="chat-row-body">
        <div class="chat-row-name">Создать чат с ${escapeHtml(name)}</div>
        <div class="chat-row-tag">@${escapeHtml(user.tag)}</div>
      </div>
    </button>
  `;
}

function renderChatRow(chat) {
  const isGroup = chat.type === 'group';
  const name = isGroup ? chat.name : chat.peer.displayName || chat.peer.tag;
  const avatarId = isGroup ? chat.id : chat.peer.id;
  const avatarTag = isGroup ? chat.name : chat.peer.tag;
  const lastMessage = chat.messages[chat.messages.length - 1];
  const lastTime = lastMessage ? formatTime(lastMessage.createdAtUnix) : '';
  const lastPreview = lastMessage ? escapeHtml(lastMessage.text) : '';
  const isSelected = chat.id === state.selectedChatId;
  const dotColor = chat.online ? 'var(--dot-online)' : 'var(--dot-offline)';
  const unreadBadge =
    chat.unreadCount > 0 ? `<div class="chat-row-unread-badge">${chat.unreadCount}</div>` : '';

  return `
    <button class="chat-row" data-chat-id="${chat.id}" data-selected="${isSelected}">
      ${renderAvatar(avatarId, avatarTag, name, {
        extraHtml: isGroup ? '' : `<div class="avatar-dot" style="background:${dotColor}"></div>`,
        src: isGroup ? groupAvatarUrl(chat.id) : avatarUrl(chat.peer.id),
      })}
      <div class="chat-row-body">
        <div class="chat-row-line1">
          <div class="chat-row-name">${escapeHtml(name)}</div>
          <div class="chat-row-time">${lastTime}</div>
        </div>
        <div class="chat-row-preview">${lastPreview}</div>
      </div>
      ${unreadBadge}
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

function isSameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function formatDateLabel(unixSeconds) {
  if (!unixSeconds) return '';
  const date = new Date(Number(unixSeconds) * 1000);
  const now = new Date();

  if (isSameDay(date, now)) return 'Сегодня';

  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (isSameDay(date, yesterday)) return 'Вчера';

  const sameYear = date.getFullYear() === now.getFullYear();
  return date.toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: sameYear ? undefined : 'numeric',
  });
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str ?? '';
  return div.innerHTML;
}

export { escapeHtml, formatTime, formatDateLabel };
