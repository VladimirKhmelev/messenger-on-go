import { state } from '../state.js';
import { avatarPalette, initialsFrom } from '../avatar.js';
import { formatTime, escapeHtml } from './sidebar.js';

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
  const statusText = chat.online ? 'В сети' : 'Не в сети';
  const sendDisabled = !state.draft.trim();

  const prevInput = root.querySelector('[data-input="draft"]');
  const hadFocus = document.activeElement === prevInput;
  const selectionStart = prevInput?.selectionStart;
  const selectionEnd = prevInput?.selectionEnd;

  root.innerHTML = `
    <div class="conversation">
      <div class="conversation-header">
        <div class="avatar avatar--md" style="background:${palette.bg};color:${palette.text}">
          ${initialsFrom(chat.peer.tag)}
        </div>
        <div>
          <div class="conversation-header-name">${escapeHtml(chat.peer.tag)}</div>
          <div class="conversation-header-status">${statusText}</div>
        </div>
      </div>
      <div class="message-list" data-list="messages">
        ${
          chat.historyLoaded && chat.messages.length === 0
            ? renderNoMessagesYet()
            : chat.messages.map((msg) => renderMessage(msg)).join('')
        }
      </div>
      <div class="composer">
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

  if (hadFocus) {
    draftInput.focus();
    draftInput.setSelectionRange(selectionStart, selectionEnd);
  }
}

function renderMessage(msg) {
  return `
    <div class="message-row" data-mine="${msg.mine}">
      <div class="bubble">${escapeHtml(msg.text)}</div>
      <div class="message-time">${formatTime(msg.createdAtUnix)}</div>
    </div>
  `;
}

function renderNoMessagesYet() {
  return `
    <div class="empty-state" style="height:100%;padding:0">
      <div class="empty-title">Сообщений пока нет</div>
      <div class="empty-subtitle">Напишите первое сообщение, чтобы начать переписку</div>
    </div>
  `;
}
