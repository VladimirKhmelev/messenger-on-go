import { state } from '../state.js';
import { renderAvatar, avatarUrl, groupAvatarUrl, snapshotAvatarImages, restoreAvatarImages } from '../avatar.js';
import { formatTime, formatDateLabel, escapeHtml } from './sidebar.js';

document.addEventListener('click', () => {
  document.querySelectorAll('[data-menu]:not([hidden])').forEach((m) => (m.hidden = true));
  document.querySelectorAll('.message-row[data-menu-open]').forEach((r) => r.removeAttribute('data-menu-open'));
});

// renderConversation() re-mounts the whole message list on every change, so
// without an explicit disconnect the previous observer would just keep
// existing (with no targets left to watch) until GC eventually collects it.
let activeReadObserver = null;

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

  const isGroup = chat.type === 'group';
  const isSelfChat = !isGroup && !!chat.isSelfChat;
  const name = isGroup ? chat.name : isSelfChat ? 'Избранное' : chat.peer.displayName || chat.peer.tag;
  const avatarId = isGroup ? chat.id : chat.peer.id;
  const avatarTag = isGroup ? chat.name : chat.peer.tag;
  const statusText = isGroup ? `${chat.members.length} участников` : isSelfChat ? '' : presenceText(chat);
  const sendDisabled = !state.draft.trim();

  const prevInput = root.querySelector('[data-input="draft"]');
  const hadFocus = document.activeElement === prevInput;
  const selectionStart = prevInput?.selectionStart;
  const selectionEnd = prevInput?.selectionEnd;

  const prevList = root.querySelector('[data-list="messages"]');
  const prevScrollHeight = prevList?.scrollHeight ?? 0;
  const prevScrollTop = prevList?.scrollTop ?? 0;
  const wasNearBottom = prevList ? prevScrollHeight - prevScrollTop - prevList.clientHeight < 80 : true;
  const isSameChat = prevList?.getAttribute('data-chat-id') === chat.id;
  const avatarSnapshot = snapshotAvatarImages(root);

  root.innerHTML = `
    <div class="conversation">
      <div class="conversation-header">
        <div class="conversation-header-inner" ${isGroup ? 'data-action="open-group-members"' : ''}>
          <button class="conversation-back-btn" data-action="back-to-chats" title="К списку чатов" aria-label="Назад">‹</button>
          <div class="${isSelfChat ? '' : 'avatar--clickable'}" data-action="${isGroup || isSelfChat ? '' : 'open-avatar'}" data-user-id="${escapeHtml(avatarId)}">
            ${
              isSelfChat
                ? '<div class="avatar avatar--md avatar--saved-messages">🔖</div>'
                : renderAvatar(avatarId, avatarTag, name, {
                    sizeClass: 'avatar--md',
                    src: isGroup ? groupAvatarUrl(chat.id) : avatarUrl(chat.peer.id),
                    deleted: !isGroup && !!chat.peer.deleted,
                  })
            }
          </div>
          <div>
            <div class="conversation-header-name">
              ${escapeHtml(name)}
              ${isGroup || isSelfChat ? '' : `<span class="conversation-header-tag">@${escapeHtml(chat.peer.tag)}</span>`}
            </div>
            ${statusText ? `<div class="conversation-header-status" data-typing="${!!chat.peerTyping}">${statusText}</div>` : ''}
          </div>
        </div>
      </div>
      <div class="message-list" data-list="messages" data-chat-id="${chat.id}">
        <div class="date-sticky" data-sticky-date hidden><span></span></div>
        ${
          (() => {
            // A "Сообщение удалено" tombstone is for the other person's
            // benefit (so a message doesn't just vanish from their view of
            // the conversation) — in Saved Messages there's no one else to
            // inform, so deleted notes are dropped outright instead of left
            // as placeholders.
            const visibleMessages = isSelfChat ? chat.messages.filter((m) => !m.deleted) : chat.messages;
            return chat.historyLoaded && visibleMessages.length === 0
              ? renderNoMessagesYet()
              : `${chat.loadingMoreHistory ? renderLoadingMoreHistory() : ''}<div class="message-list-inner">${renderMessagesWithDateDividers(
                  visibleMessages,
                  isGroup ? null : chat.peerLastReadMessageId,
                  isGroup ? chat.members : null,
                  isGroup ? chat.createdBy : null,
                  isSelfChat
                )}</div>`;
          })()
        }
      </div>
      <button class="scroll-to-bottom-btn" data-action="scroll-to-bottom" hidden title="К последним сообщениям">↓</button>
      <div class="composer">
        ${state.mediaUploadError ? `<div class="composer-error">${escapeHtml(state.mediaUploadError)}</div>` : ''}
        <div class="composer-inner">
          <button
            class="attach-btn"
            data-action="attach-file"
            data-busy="${state.mediaUploadBusy}"
            title="Прикрепить файл"
            ${state.mediaUploadBusy ? 'disabled' : ''}
          >📎</button>
          <input type="file" data-input="attach-file" hidden />
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
    ${renderAvatarPreview()}
    ${renderMediaPreview()}
  `;
  restoreAvatarImages(root, avatarSnapshot);

  const list = root.querySelector('[data-list="messages"]');
  if (list) {
    const forceScrollToBottom = state.scrollToBottomOnRender;
    state.scrollToBottomOnRender = false;

    if (isSameChat && !wasNearBottom && !forceScrollToBottom) {
      // Prepended older messages (or another background update) while the
      // user was scrolled up — keep their view anchored instead of jumping.
      list.scrollTop = list.scrollHeight - prevScrollHeight + prevScrollTop;
    } else {
      list.scrollTop = list.scrollHeight;
    }

    const scrollToBottomBtn = root.querySelector('[data-action="scroll-to-bottom"]');
    const updateScrollToBottomBtn = () => {
      const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 80;
      scrollToBottomBtn.hidden = nearBottom;
    };

    updateStickyDate(list);
    updateScrollToBottomBtn();
    list.addEventListener('scroll', () => {
      if (list.scrollTop < 200) {
        handlers.onLoadMoreHistory(chat.id);
      }
      updateStickyDate(list);
      updateScrollToBottomBtn();
    });

    scrollToBottomBtn.addEventListener('click', () => {
      list.scrollTo({ top: list.scrollHeight, behavior: 'smooth' });
    });

    wireReadObserver(list, chat.id, handlers);
  }

  const draftInput = root.querySelector('[data-input="draft"]');
  const sendBtn = root.querySelector('[data-action="send"]');
  draftInput.addEventListener('input', (event) => {
    // A full renderConversation() on every keystroke re-mounts the whole
    // message list (including a fresh IntersectionObserver per message) —
    // on mobile, with the keyboard up, that was visibly janky. The only
    // thing typing actually needs to update is the send button's enabled
    // state, so patch that directly instead of re-rendering.
    state.draft = event.target.value;
    sendBtn.setAttribute('data-enabled', String(!!state.draft.trim()));
    if (state.draft.trim()) handlers.onTyping(chat.id);
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

  const attachInput = root.querySelector('[data-input="attach-file"]');
  root.querySelector('[data-action="attach-file"]')?.addEventListener('click', () => {
    attachInput.click();
  });
  attachInput.addEventListener('change', () => {
    const file = attachInput.files?.[0];
    attachInput.value = '';
    if (file) handlers.onSendFile(file);
  });

  wireMessageActions(root, handlers, chat.id);

  root.querySelector('[data-action="open-avatar"]')?.addEventListener('click', () => {
    state.avatarPreview = { userId: chat.peer.id, name };
    handlers.onDraftChange();
  });

  root.querySelectorAll('[data-action="open-sender-profile"]').forEach((el) => {
    el.addEventListener('click', (event) => {
      event.stopPropagation();
      state.avatarPreview = { userId: el.getAttribute('data-user-id'), name: el.getAttribute('data-user-name') };
      handlers.onDraftChange();
    });
  });

  root.querySelector('[data-action="open-group-members"]')?.addEventListener('click', (event) => {
    if (event.target.closest('[data-action="back-to-chats"]')) return;
    handlers.onOpenGroupMembers();
  });

  root.querySelector('[data-action="back-to-chats"]')?.addEventListener('click', () => {
    handlers.onBack();
  });

  root.querySelectorAll('[data-action="close-avatar-preview"]').forEach((el) => {
    el.addEventListener('click', () => {
      state.avatarPreview = null;
      handlers.onDraftChange();
    });
  });

  root.querySelectorAll('[data-action="close-media-preview"]').forEach((el) => {
    el.addEventListener('click', () => {
      // Not revoking the object URL here — it's the same one still shown
      // inline in the message bubble (loadMediaAttachment only creates one
      // per attachment), so revoking it on close would break that thumbnail
      // too. It's cleaned up on page unload like any other object URL.
      state.mediaPreview = null;
      handlers.onDraftChange();
    });
  });

  root.querySelectorAll('.avatar-preview').forEach((el) => {
    el.addEventListener('click', (event) => {
      event.stopPropagation();
    });
  });

  if (hadFocus) {
    draftInput.focus();
    draftInput.setSelectionRange(selectionStart, selectionEnd);
  } else if (state.focusDraftOnRender) {
    state.focusDraftOnRender = false;
    draftInput.focus();
  }
}

function renderAvatarPreview() {
  if (!state.avatarPreview) return '';
  const { userId, name } = state.avatarPreview;
  return `
    <div class="modal-backdrop" data-action="close-avatar-preview">
      <div class="avatar-preview" data-action="stop-propagation">
        <img class="avatar-preview-img" src="${avatarUrl(userId)}" alt="${escapeHtml(name)}" />
        <button class="modal-close avatar-preview-close" data-action="close-avatar-preview">×</button>
      </div>
    </div>
  `;
}

function renderMediaPreview() {
  if (!state.mediaPreview) return '';
  const { objectUrl, fileName } = state.mediaPreview;
  return `
    <div class="modal-backdrop" data-action="close-media-preview">
      <div class="avatar-preview media-preview" data-action="stop-propagation">
        <img class="avatar-preview-img" src="${objectUrl}" alt="${escapeHtml(fileName || '')}" />
        <button class="modal-close avatar-preview-close" data-action="close-media-preview">×</button>
      </div>
    </div>
  `;
}

function renderMessagesWithDateDividers(messages, peerLastReadMessageId, groupMembers, groupCreatedBy, isSelfChat = false) {
  const lastReadIndex = peerLastReadMessageId
    ? messages.findIndex((m) => m.messageId === peerLastReadMessageId)
    : -1;
  const memberById = groupMembers ? new Map(groupMembers.map((m) => [m.id, m])) : null;
  const myId = state.currentUser?.id;
  const myMember = memberById ? memberById.get(myId) : null;
  // Mirrors the backend's DeleteMessageForAll rule: any admin (creator
  // included, since the creator is always also an admin) can delete
  // someone else's message in a group, not just their own.
  const canModerate = !!memberById && (myMember?.role === 'admin' || myId === groupCreatedBy);

  let lastDateLabel = null;
  return messages
    .map((msg, index) => {
      const dateLabel = formatDateLabel(msg.createdAtUnix);
      const divider = dateLabel && dateLabel !== lastDateLabel ? renderDateDivider(dateLabel) : '';
      lastDateLabel = dateLabel || lastDateLabel;
      // Nobody but you ever reads a Saved Messages note, so
      // peerLastReadMessageId-based tracking is meaningless here — show it
      // as always-read instead of always-unread.
      const isRead = isSelfChat || (lastReadIndex !== -1 && index <= lastReadIndex);
      const sender = !msg.mine && memberById ? memberById.get(msg.senderUserId) : null;
      const isSenderCreator = !!sender && sender.id === groupCreatedBy;
      return (
        divider +
        renderMessage(msg, state.editingMessageId === msg.messageId, isRead, sender, isSenderCreator, canModerate, isSelfChat)
      );
    })
    .join('');
}

function wireReadObserver(list, chatId, handlers) {
  activeReadObserver?.disconnect();
  activeReadObserver = null;

  const targets = list.querySelectorAll('[data-observe-read]');
  if (targets.length === 0) return;

  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const messageId = entry.target.getAttribute('data-message-id');
        if (messageId) handlers.onMessageVisible(chatId, messageId);
        observer.unobserve(entry.target);
      }
    },
    { root: list, threshold: 0.6 }
  );

  targets.forEach((el) => observer.observe(el));
  activeReadObserver = observer;
}

function updateStickyDate(list) {
  const sticky = list.querySelector('[data-sticky-date]');
  if (!sticky) return;

  const dividers = list.querySelectorAll('[data-date-divider]');
  const listTop = list.getBoundingClientRect().top;

  let current = null;
  for (const divider of dividers) {
    if (divider.getBoundingClientRect().top - listTop <= 0) {
      current = divider;
    } else {
      break;
    }
  }

  if (!current) {
    sticky.hidden = true;
    return;
  }

  sticky.hidden = false;
  sticky.querySelector('span').textContent = current.getAttribute('data-date-divider');
}

function renderDateDivider(label) {
  return `<div class="date-divider" data-date-divider="${escapeHtml(label)}"><span>${escapeHtml(label)}</span></div>`;
}

export function presenceText(chat) {
  if (chat.peerTyping) return 'печатает...';
  if (chat.online) return 'В сети';
  if (!chat.lastSeenUnix) return 'Не в сети';

  const dateLabel = formatDateLabel(chat.lastSeenUnix);
  const time = formatTime(chat.lastSeenUnix);
  // Same-day is common enough (recent activity) that spelling out "Сегодня"
  // every time would just be noise — only prefix the date once it's no
  // longer obvious from context, same threshold as the message dividers.
  if (dateLabel === 'Сегодня') return `Был(а) в сети в ${time}`;
  return `Был(а) в сети ${dateLabel.toLowerCase()} в ${time}`;
}

function renderMessage(msg, isEditing, isRead, sender, isSenderCreator, canModerate, isSelfChat = false) {
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

  if (msg.media) {
    return renderMediaMessage(msg, isRead, sender, isSenderCreator, canModerate, isSelfChat);
  }

  const editedTag = msg.editedAtUnix ? '<span class="message-edited-tag">изменено</span>' : '';
  const readTicks = msg.mine ? renderReadTicks(isRead) : '';
  const observeAttr = !msg.mine ? 'data-observe-read' : '';
  const senderName = sender ? sender.displayName || sender.tag : null;
  const senderRoleLabel = isSenderCreator
    ? '<span class="message-sender-role">Создатель</span>'
    : sender?.role === 'admin'
      ? '<span class="message-sender-role">Админ</span>'
      : '';
  const senderAvatar = sender
    ? `<div class="message-sender-avatar avatar--clickable" data-action="open-sender-profile" data-user-id="${escapeHtml(sender.id)}" data-user-name="${escapeHtml(senderName)}">${renderAvatar(sender.id, sender.tag, senderName, { sizeClass: 'avatar--sm', deleted: !!sender.deleted })}</div>`
    : '';
  const senderLabel = senderName
    ? `<div class="message-sender-name">${escapeHtml(senderName)}${senderRoleLabel}</div>`
    : '';

  return `
    <div class="message-row" data-mine="${msg.mine}" data-message-id="${msg.messageId}" ${observeAttr}>
      <div class="message-row-inner">
        ${senderAvatar}
        <button class="message-menu-btn" data-action="open-menu" title="Действия">⋯</button>
        <div class="bubble">
          ${senderLabel}
          <span class="bubble-text">${escapeHtml(msg.text)}</span>
          <div class="message-menu" data-menu hidden>
            <div class="message-menu-item" data-action="copy">Копировать текст</div>${
              msg.mine ? '<div class="message-menu-item" data-action="edit">Редактировать</div>' : ''
            }${
              isSelfChat
                ? // "for all" vs "for me" is a distinction between two people —
                  // meaningless when you're the only participant, so collapse
                  // to a single delete that removes the note outright.
                  '<div class="message-menu-item message-menu-item--danger" data-action="delete-for-all">Удалить</div>'
                : `${
                    msg.mine || canModerate
                      ? `<div class="message-menu-item message-menu-item--danger" data-action="delete-for-all">Удалить у всех</div>`
                      : ''
                  }<div class="message-menu-item" data-action="delete-for-me">Удалить у меня</div>`
            }
          </div>
        </div>
      </div>
      <div class="message-time">${formatTime(msg.createdAtUnix)}${editedTag}${readTicks}</div>
    </div>
  `;
}

function formatFileSize(bytes) {
  if (!bytes) return '';
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

function renderMediaMessage(msg, isRead, sender, isSenderCreator, canModerate, isSelfChat) {
  const readTicks = msg.mine ? renderReadTicks(isRead) : '';
  const observeAttr = !msg.mine ? 'data-observe-read' : '';
  const senderName = sender ? sender.displayName || sender.tag : null;
  const senderRoleLabel = isSenderCreator
    ? '<span class="message-sender-role">Создатель</span>'
    : sender?.role === 'admin'
      ? '<span class="message-sender-role">Админ</span>'
      : '';
  const senderAvatar = sender
    ? `<div class="message-sender-avatar avatar--clickable" data-action="open-sender-profile" data-user-id="${escapeHtml(sender.id)}" data-user-name="${escapeHtml(senderName)}">${renderAvatar(sender.id, sender.tag, senderName, { sizeClass: 'avatar--sm', deleted: !!sender.deleted })}</div>`
    : '';
  const senderLabel = senderName
    ? `<div class="message-sender-name">${escapeHtml(senderName)}${senderRoleLabel}</div>`
    : '';

  const { mediaId, fileName, contentType, sizeBytes } = msg.media;
  const isImage = contentType.startsWith('image/');
  const body = isImage
    ? `<div class="media-attachment media-attachment--image" data-action="load-media" data-autoload="true" data-media-id="${escapeHtml(mediaId)}" data-content-type="${escapeHtml(contentType)}" data-file-name="${escapeHtml(fileName)}">
        <div class="media-attachment-placeholder">Загрузка изображения...</div>
      </div>`
    : `<div class="media-attachment media-attachment--file" data-action="load-media" data-media-id="${escapeHtml(mediaId)}" data-content-type="${escapeHtml(contentType)}" data-file-name="${escapeHtml(fileName)}">
        <span class="media-attachment-icon">📎</span>
        <span class="media-attachment-name">${escapeHtml(fileName)}</span>
        <span class="media-attachment-size">${formatFileSize(sizeBytes)}</span>
      </div>`;

  return `
    <div class="message-row" data-mine="${msg.mine}" data-message-id="${msg.messageId}" ${observeAttr}>
      <div class="message-row-inner">
        ${senderAvatar}
        <button class="message-menu-btn" data-action="open-menu" title="Действия">⋯</button>
        <div class="bubble bubble--media">
          ${senderLabel}
          ${body}
          <div class="message-menu" data-menu hidden>${
            isSelfChat
              ? '<div class="message-menu-item message-menu-item--danger" data-action="delete-for-all">Удалить</div>'
              : `${
                  msg.mine || canModerate
                    ? `<div class="message-menu-item message-menu-item--danger" data-action="delete-for-all">Удалить у всех</div>`
                    : ''
                }<div class="message-menu-item" data-action="delete-for-me">Удалить у меня</div>`
          }</div>
        </div>
      </div>
      <div class="message-time">${formatTime(msg.createdAtUnix)}${readTicks}</div>
    </div>
  `;
}

function renderReadTicks(isRead) {
  return `<span class="read-ticks" data-read="${!!isRead}">${isRead ? '✓✓' : '✓'}</span>`;
}

function wireMessageActions(root, handlers, chatId) {
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

  root.querySelectorAll('[data-action="load-media"]').forEach((el) => {
    if (el.dataset.autoload === 'true') {
      loadMediaAttachment(el, handlers, chatId);
    } else {
      el.addEventListener('click', () => loadMediaAttachment(el, handlers, chatId));
    }
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

// Images auto-load (called directly on render, see wireMessageActions) so
// they appear inline like any other messenger; other files stay lazy and
// wait for a click — presigned download URLs are short-lived and most
// non-image attachments in a long history are never actually opened, so
// there's no reason to download+decrypt them eagerly.
async function loadMediaAttachment(el, handlers, chatId) {
  if (el.dataset.loading === 'true' || el.dataset.loaded === 'true') return;
  el.dataset.loading = 'true';

  const mediaId = el.getAttribute('data-media-id');
  const contentType = el.getAttribute('data-content-type');
  const fileName = el.getAttribute('data-file-name');
  const isImage = contentType.startsWith('image/');
  const placeholder = el.querySelector('.media-attachment-placeholder');

  try {
    const objectUrl = await handlers.onDownloadMedia(chatId, { mediaId, contentType });
    el.dataset.loaded = 'true';

    if (isImage) {
      el.innerHTML = `<img class="media-attachment-img" src="${objectUrl}" alt="${escapeHtml(fileName || '')}" />`;
      el.addEventListener('click', () => {
        state.mediaPreview = { objectUrl, fileName };
        handlers.onDraftChange();
      });
    } else {
      const link = document.createElement('a');
      link.href = objectUrl;
      link.download = fileName || 'file';
      link.click();
      if (placeholder) placeholder.textContent = '✓ Загружено';
      const icon = el.querySelector('.media-attachment-icon');
      if (icon) icon.textContent = '✓';
    }
  } catch (err) {
    console.error('failed to load media attachment:', err);
    if (placeholder) placeholder.textContent = 'Не удалось загрузить, нажмите ещё раз';
    el.dataset.loading = 'false';
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

function renderLoadingMoreHistory() {
  return `<div class="history-loading">Загрузка...</div>`;
}
