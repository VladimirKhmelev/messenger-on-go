import { state } from '../state.js';
import { renderAvatar, groupAvatarUrl } from '../avatar.js';
import { escapeHtml } from './sidebar.js';
import { presenceText } from './conversation.js';
import { t } from '../i18n.js';

export function renderGroupMembers(root, handlers) {
  if (!state.groupMembersOpen) {
    root.innerHTML = '';
    return;
  }

  const chat = state.chats.find((c) => c.id === state.selectedChatId);
  if (!chat || chat.type !== 'group') {
    root.innerHTML = '';
    return;
  }

  const myId = state.currentUser?.id;
  const isCreator = chat.createdBy === myId;
  const myMember = chat.members.find((m) => m.id === myId);
  const isAdmin = myMember?.role === 'admin' || isCreator;

  const prevSearchInput = root.querySelector('[data-input="add-member-search"]');
  const searchHadFocus = document.activeElement === prevSearchInput;
  const searchSelectionStart = prevSearchInput?.selectionStart;
  const searchSelectionEnd = prevSearchInput?.selectionEnd;

  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-backdrop">
      <div class="modal" data-action="stop-propagation">
        <div class="modal-header">
          <div class="modal-title">${escapeHtml(chat.name)}</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="group-avatar-section">
          <div class="avatar--clickable ${isAdmin ? 'avatar--editable' : ''}" data-action="${isAdmin ? 'upload-group-avatar' : ''}" title="${isAdmin ? t('groupMembers.changeGroupPhoto') : ''}">
            ${renderAvatar(chat.id, chat.name, chat.name, { sizeClass: 'avatar--lg', src: groupAvatarUrl(chat.id) })}
            ${isAdmin ? `<div class="avatar-edit-overlay">${t('groupMembers.changePhoto')}</div>` : ''}
          </div>
          ${isAdmin ? '<input type="file" accept="image/jpeg,image/png,image/gif,image/webp" data-input="group-avatar-file" hidden />' : ''}
          ${state.groupMembersAvatarBusy ? `<div class="group-avatar-status">${t('groupMembers.uploading')}</div>` : ''}
          ${state.groupMembersAvatarError ? `<div class="form-error">${escapeHtml(state.groupMembersAvatarError)}</div>` : ''}
        </div>

        <div class="form-error">${state.groupMembersError || ''}</div>

        <div class="chat-list group-members-list">
          ${sortedMembers(chat.members, chat.createdBy)
            .map((m) => renderMemberRow(m, { isCreator, isAdmin, myId, chatCreatedBy: chat.createdBy }))
            .join('')}
        </div>

        ${
          isAdmin
            ? `<div class="field">
                <label>${t('groupMembers.addMemberLabel')}</label>
                <input type="text" value="${escapeHtml(state.groupMembersAddQuery || '')}" data-input="add-member-search" placeholder="${t('groupMembers.searchPlaceholder')}" />
              </div>
              <div class="chat-list group-search-results">
                ${(state.groupMembersAddFoundUsers || []).map((user) => renderCandidateRow(user)).join('')}
              </div>`
            : ''
        }

        ${
          !isCreator
            ? `<button class="btn-secondary" data-action="leave" ${state.groupMembersBusy ? 'disabled' : ''}>
                ${t('groupMembers.leaveGroup')}
              </button>`
            : ''
        }

        ${
          isCreator
            ? state.groupMembersDeleteConfirming
              ? `
                <div class="settings-danger-zone">
                  <div class="settings-danger-warning">
                    ${t('groupMembers.deleteWarning')}
                  </div>
                  <div class="settings-danger-actions">
                    <button class="btn-secondary" data-action="cancel-delete-chat" ${state.groupMembersBusy ? 'disabled' : ''}>
                      ${t('groupMembers.cancel')}
                    </button>
                    <button class="btn-danger" data-action="confirm-delete-chat" ${state.groupMembersBusy ? 'disabled' : ''}>
                      ${t('groupMembers.confirmDelete')}
                    </button>
                  </div>
                </div>
              `
              : `<div class="settings-danger-zone">
                  <button class="btn-danger" data-action="start-delete-chat">${t('groupMembers.startDelete')}</button>
                </div>`
            : ''
        }
      </div>
    </div>
  `;

  root.querySelector('[data-action="close-backdrop"]').addEventListener('click', () => handlers.onClose());
  root.querySelector('[data-action="close"]').addEventListener('click', () => handlers.onClose());
  root.querySelector('.modal').addEventListener('click', (event) => event.stopPropagation());

  const avatarFileInput = root.querySelector('[data-input="group-avatar-file"]');
  root.querySelector('[data-action="upload-group-avatar"]')?.addEventListener('click', () => {
    avatarFileInput?.click();
  });
  avatarFileInput?.addEventListener('change', (event) => {
    const file = event.target.files?.[0];
    if (file) handlers.onUploadGroupAvatar(file);
    event.target.value = '';
  });

  root.querySelectorAll('[data-action="promote"]').forEach((el) => {
    el.addEventListener('click', () => handlers.onSetRole(el.getAttribute('data-user-id'), 'admin'));
  });
  root.querySelectorAll('[data-action="demote"]').forEach((el) => {
    el.addEventListener('click', () => handlers.onSetRole(el.getAttribute('data-user-id'), 'member'));
  });
  root.querySelectorAll('[data-action="remove-member"]').forEach((el) => {
    el.addEventListener('click', () => handlers.onRemoveMember(el.getAttribute('data-user-id')));
  });
  root.querySelectorAll('[data-action="open-profile"]').forEach((el) => {
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      handlers.onOpenProfile(el.getAttribute('data-user-id'), el.getAttribute('data-user-name'));
    });
  });

  root.querySelector('[data-action="leave"]')?.addEventListener('click', () => {
    handlers.onLeave();
  });

  root.querySelector('[data-action="start-delete-chat"]')?.addEventListener('click', () => {
    handlers.onStartDeleteChat();
  });
  root.querySelector('[data-action="cancel-delete-chat"]')?.addEventListener('click', () => {
    handlers.onCancelDeleteChat();
  });
  root.querySelector('[data-action="confirm-delete-chat"]')?.addEventListener('click', () => {
    handlers.onConfirmDeleteChat();
  });

  const searchInput = root.querySelector('[data-input="add-member-search"]');
  searchInput?.addEventListener('input', (event) => {
    handlers.onAddMemberSearchChange(event.target.value);
  });
  if (searchHadFocus) {
    searchInput.focus();
    searchInput.setSelectionRange(searchSelectionStart, searchSelectionEnd);
  }

  root.querySelectorAll('[data-action="add-candidate"]').forEach((el) => {
    el.addEventListener('click', () => {
      handlers.onAddMember(el.getAttribute('data-user-id'));
    });
  });
}

function sortedMembers(members, chatCreatedBy) {
  const rank = (m) => (m.id === chatCreatedBy ? 0 : m.role === 'admin' ? 1 : 2);
  return [...members].sort((a, b) => rank(a) - rank(b));
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

function renderMemberRow(member, { isCreator, isAdmin, myId, chatCreatedBy }) {
  const name = member.displayName || member.tag;
  const isTargetCreator = member.id === chatCreatedBy;
  const isMe = member.id === myId;
  const roleLabel = isTargetCreator ? t('groupMembers.creator') : member.role === 'admin' ? t('groupMembers.admin') : '';
  const badge = roleLabel ? `<span class="group-member-role">${roleLabel}</span>` : '';
  const meBadge = isMe ? `<span class="group-member-you">${t('groupMembers.you')}</span>` : '';

  const canManage = !isTargetCreator && member.id !== myId && (isCreator || (isAdmin && member.role !== 'admin'));
  const canChangeRole = !isTargetCreator && member.id !== myId && isCreator;

  let actions = '';
  if (canChangeRole) {
    actions +=
      member.role === 'admin'
        ? `<span class="action" data-action="demote" data-user-id="${escapeHtml(member.id)}">${t('groupMembers.demote')}</span>`
        : `<span class="action" data-action="promote" data-user-id="${escapeHtml(member.id)}">${t('groupMembers.promote')}</span>`;
  }
  if (canManage) {
    actions += `<span class="action action--danger" data-action="remove-member" data-user-id="${escapeHtml(member.id)}">${t('groupMembers.remove')}</span>`;
  }

  const status = isMe ? '' : presenceText(member);

  return `
    <div class="chat-row group-member-row">
      ${renderAvatar(member.id, member.tag, name, { deleted: !!member.deleted })}
      <div class="chat-row-body">
        <div class="chat-row-name">${escapeHtml(name)} ${badge}${meBadge}</div>
        <div class="chat-row-tag">@${escapeHtml(member.tag)}</div>
        ${status ? `<div class="group-member-status">${status}</div>` : ''}
      </div>
      ${actions ? `<div class="group-member-actions">${actions}</div>` : ''}
    </div>
  `;
}
