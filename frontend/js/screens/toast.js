import { state } from '../state.js';
import { avatarPalette, initialsFrom } from '../avatar.js';
import { escapeHtml } from './sidebar.js';

export function renderToast(root, handlers) {
  if (!state.toast) {
    root.innerHTML = '';
    return;
  }

  const palette = avatarPalette(state.toast.name);

  root.innerHTML = `
    <div class="toast" data-action="open">
      <div class="avatar avatar--sm" style="background:${palette.bg};color:${palette.text}">
        ${initialsFrom(state.toast.name)}
      </div>
      <div class="toast-body">
        <div class="toast-line1">
          <div class="toast-name">${escapeHtml(state.toast.name)}</div>
          <div class="toast-dismiss" data-action="dismiss">×</div>
        </div>
        <div class="toast-text">${escapeHtml(state.toast.text)}</div>
      </div>
    </div>
  `;

  root.querySelector('[data-action="dismiss"]').addEventListener('click', (event) => {
    event.stopPropagation();
    handlers.onDismiss();
  });

  root.querySelector('[data-action="open"]').addEventListener('click', () => {
    handlers.onOpen(state.toast.chatId);
  });
}
