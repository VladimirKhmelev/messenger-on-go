import { state } from '../state.js';
import { renderAvatar, avatarUrl } from '../avatar.js';
import { escapeHtml } from './sidebar.js';

// Private-chat-only: block/unblock the peer. Opened from the conversation
// header for a non-group, non-self chat — see main.js's handleOpenUserProfile.
export function renderUserProfile(root, handlers) {
  if (!state.userProfileOpen) {
    root.innerHTML = '';
    return;
  }

  const chat = state.chats.find((c) => c.id === state.selectedChatId);
  if (!chat || chat.type === 'group' || chat.isSelfChat) {
    root.innerHTML = '';
    return;
  }

  const peer = chat.peer;
  const name = peer.displayName || peer.tag;

  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-backdrop">
      <div class="modal" data-action="stop-propagation">
        <div class="modal-header">
          <div class="modal-title">${escapeHtml(name)}</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="settings-avatar-row">
          ${renderAvatar(peer.id, peer.tag, name, { sizeClass: 'avatar--md', src: avatarUrl(peer.id), deleted: !!peer.deleted })}
        </div>

        <div class="form-error">${state.userProfileError || ''}</div>

        ${
          state.userProfileBlocked
            ? `<button class="btn-secondary" data-action="unblock-user" ${state.userProfileBusy ? 'disabled' : ''}>Разблокировать</button>`
            : `<button class="btn-danger" data-action="block-user" ${state.userProfileBusy ? 'disabled' : ''}>Заблокировать</button>`
        }
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

  root.querySelector('[data-action="block-user"]')?.addEventListener('click', () => {
    handlers.onBlock(peer.id);
  });
  root.querySelector('[data-action="unblock-user"]')?.addEventListener('click', () => {
    handlers.onUnblock(peer.id);
  });
}
