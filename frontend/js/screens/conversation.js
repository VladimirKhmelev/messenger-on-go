import { state } from '../state.js';
import { avatarPalette, initialsFrom } from '../avatar.js';
import { formatTime, escapeHtml } from './sidebar.js';

document.addEventListener('click', () => {
  document.querySelectorAll('[data-menu]:not([hidden])').forEach((m) => (m.hidden = true));
  document.querySelectorAll('.message-row[data-menu-open]').forEach((r) => r.removeAttribute('data-menu-open'));
});

export function renderConversation(root, handlers) {
  const chat = state.chats.find((c) => c.id === state.selectedChatId);

  if (!chat) {
    root.innerHTML = `
      <div class="empty-state">
        <div class="empty-graphic">
          <div class="rect-a"></div>
          <div class="rect-b"></div>
        </div>
        <div class="empty-title">Выберите чат</div>
        <div class="empty-subtitle">Выберите диалог слева, чтобы начать переписку</div>
      </div>
    `;
    return;
  }

  const palette = avatarPalette(chat.peer.tag);
  const statusText = presenceText(chat);
  const sendDisabled = !state.draft.trim();

  const prevInput = root.querySelector('[data-input="draft"]');
  const hadFocus = document.activeElement === prevInput;
  const selectionStart = prevInput?.selectionStart;
  const selectionEnd = prevInput?.selectionEnd;

  root.innerHTML = `
    <div class="conversation">
      <div class="conversation-header">
        <div class="conversation-header-inner">
          <div class="avatar avatar--md" style="background:${palette.bg};color:${palette.text}">
            ${initialsFrom(chat.peer.tag)}
          </div>
          <div>
            <div class="conversation-header-name">${escapeHtml(chat.peer.tag)}</div>
            <div class="conversation-header-status">${statusText}</div>
          </div>
        </div>
      </div>
      <div class="message-list" data-list="messages">
        ${
          chat.historyLoaded && chat.messages.length === 0
            ? renderNoMessagesYet()
            : `<div class="message-list-inner">${chat.messages
                .map((msg) => renderMessage(msg, state.editingMessageId === msg.messageId))
                .join('')}</div>`
        }
      </div>
      <div class="composer">
        <div class="composer-inner">
          <input
            type="text"
            class="composer-input"
            placeholder="Написать сообщение..."
            value="${escapeHtml(state.draft)}"
            data-input="draft"
          />
          <button class="send-btn" data-enabled="${!sendDisabled}" data-action="send">→</button>
        </div>
      </div>
    </div>
  `;

  const list = root.querySelector('[data-list="messages"]');
  list.scrollTop = list.scrollHeight;

  const draftInput = root.querySelector('[data-input="draft"]');
  draftInput.addEventListener('input', (event) => {
    state.draft = event.target.value;
    handlers.onDraftChange();
  });
  draftInput.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      handlers.onSend();
    }
  });

  root.querySelector('[data-action="send"]').addEventListener('click', () => {
    handlers.onSend();
  });

  wireMessageActions(root, handlers);

  if (hadFocus) {
    draftInput.focus();
    draftInput.setSelectionRange(selectionStart, selectionEnd);
  }
}

function presenceText(chat) {
  if (chat.online) return 'В сети';
  if (!chat.lastSeenUnix) return 'Не в сети';
  return `Был(а) в сети в ${formatTime(chat.lastSeenUnix)}`;
}

function renderMessage(msg, isEditing) {
  if (msg.deleted) {
    return `
      <div class="message-row" data-mine="${msg.mine}">
        <div class="bubble bubble--deleted">Сообщение удалено</div>
      </div>
    `;
  }

  if (isEditing) {
    return `
      <div class="message-row" data-mine="${msg.mine}" data-message-id="${msg.messageId}">
        <div class="bubble bubble--editing">
          <input type="text" class="edit-input" data-input="edit" value="${escapeHtml(msg.text)}" />
          <div class="edit-actions">
            <span class="action" data-action="save-edit">Сохранить</span>
            <span class="action" data-action="cancel-edit">Отмена</span>
          </div>
        </div>
      </div>
    `;
  }

  const editedTag = msg.editedAtUnix ? '<span class="message-edited-tag">изменено</span>' : '';

  return `
    <div class="message-row" data-mine="${msg.mine}" data-message-id="${msg.messageId}">
      <div class="message-row-inner">
        <button class="message-menu-btn" data-action="open-menu" title="Действия">⋯</button>
        <div class="bubble">
          <span class="bubble-text">${escapeHtml(msg.text)}</span>
          <div class="message-menu" data-menu hidden>
            <div class="message-menu-item" data-action="copy">Копировать текст</div>${
              msg.mine
                ? `<div class="message-menu-item" data-action="edit">Редактировать</div><div class="message-menu-item message-menu-item--danger" data-action="delete-for-all">Удалить у всех</div>`
                : ''
            }<div class="message-menu-item" data-action="delete-for-me">Удалить у меня</div>
          </div>
        </div>
      </div>
      <div class="message-time">${formatTime(msg.createdAtUnix)}${editedTag}</div>
    </div>
  `;
}

function wireMessageActions(root, handlers) {
  function closeAllMenus() {
    root.querySelectorAll('[data-menu]').forEach((m) => (m.hidden = true));
    root.querySelectorAll('.message-row[data-menu-open]').forEach((r) => r.removeAttribute('data-menu-open'));
  }

  function openMenuAt(menu, x, y, mine) {
    closeAllMenus();
    menu.hidden = false;
    menu.closest('.message-row').setAttribute('data-menu-open', 'true');

    const menuRect = menu.getBoundingClientRect();
    const margin = 6;

    let left = mine ? x - menuRect.width : x;
    left = Math.max(margin, Math.min(left, window.innerWidth - menuRect.width - margin));

    let top = y;
    top = Math.max(margin, Math.min(top, window.innerHeight - menuRect.height - margin));

    menu.style.left = `${left}px`;
    menu.style.top = `${top}px`;
  }

  root.querySelectorAll('[data-action="open-menu"]').forEach((btn) => {
    btn.addEventListener('click', (event) => {
      event.stopPropagation();
      const menu = btn.closest('.message-row-inner').querySelector('[data-menu]');
      const mine = btn.closest('.message-row').getAttribute('data-mine') === 'true';
      const rect = btn.getBoundingClientRect();
      openMenuAt(menu, mine ? rect.left : rect.right, rect.bottom + 4, mine);
    });
  });

  root.querySelectorAll('.message-row').forEach((row) => {
    const menu = row.querySelector('[data-menu]');
    if (!menu) return;
    row.addEventListener('contextmenu', (event) => {
      event.preventDefault();
      event.stopPropagation();
      const mine = row.getAttribute('data-mine') === 'true';
      openMenuAt(menu, event.clientX, event.clientY, mine);
    });
  });

  root.querySelectorAll('[data-action="copy"]').forEach((item) => {
    item.addEventListener('click', () => {
      const row = item.closest('[data-message-id]');
      const text = row.querySelector('.bubble-text')?.textContent ?? '';
      navigator.clipboard?.writeText(text).catch(() => {});
    });
  });

  root.querySelectorAll('[data-action="edit"]').forEach((item) => {
    item.addEventListener('click', () => {
      const row = item.closest('[data-message-id]');
      state.editingMessageId = row.getAttribute('data-message-id');
      handlers.onDraftChange();
    });
  });

  root.querySelectorAll('[data-action="delete-for-all"]').forEach((item) => {
    item.addEventListener('click', () => {
      const row = item.closest('[data-message-id]');
      handlers.onDeleteMessageForAll(row.getAttribute('data-message-id'));
    });
  });

  root.querySelectorAll('[data-action="delete-for-me"]').forEach((item) => {
    item.addEventListener('click', () => {
      const row = item.closest('[data-message-id]');
      handlers.onDeleteMessageForMe(row.getAttribute('data-message-id'));
    });
  });

  const editInput = root.querySelector('[data-input="edit"]');
  if (editInput) {
    editInput.focus();
    editInput.setSelectionRange(editInput.value.length, editInput.value.length);
    editInput.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        submitEdit(root, handlers);
      } else if (event.key === 'Escape') {
        state.editingMessageId = null;
        handlers.onDraftChange();
      }
    });

    root.querySelector('[data-action="save-edit"]').addEventListener('click', () => submitEdit(root, handlers));
    root.querySelector('[data-action="cancel-edit"]').addEventListener('click', () => {
      state.editingMessageId = null;
      handlers.onDraftChange();
    });
  }
}

function submitEdit(root, handlers) {
  const editInput = root.querySelector('[data-input="edit"]');
  const row = editInput.closest('[data-message-id]');
  const messageId = row.getAttribute('data-message-id');
  const newText = editInput.value.trim();
  if (newText) {
    handlers.onEditMessage(messageId, newText);
  }
  state.editingMessageId = null;
  handlers.onDraftChange();
}

function renderNoMessagesYet() {
  return `
    <div class="empty-state" style="height:100%;padding:0">
      <div class="empty-title">Сообщений пока нет</div>
      <div class="empty-subtitle">Напишите первое сообщение, чтобы начать переписку</div>
    </div>
  `;
}
