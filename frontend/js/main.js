import { state, onZone, notify, notifyAll } from './state.js';
import { initTheme } from './theme.js';
import {
  authApi,
  chatApi,
  refreshAccessToken,
  getAccessToken,
  setAccessToken,
  currentUserIdFromToken,
  ApiError,
} from './api.js';
import { WsClient } from './ws.js';
import { bumpAvatarCacheVersion } from './avatar.js';
import { renderAuth } from './screens/auth.js';
import { renderSidebar } from './screens/sidebar.js';
import { renderConversation } from './screens/conversation.js';
import { renderToast } from './screens/toast.js';
import { renderSettings } from './screens/settings.js';

const TOAST_AUTO_DISMISS_MS = 5_000;
const HISTORY_PAGE_SIZE = 50;
const PRESENCE_REFRESH_INTERVAL_MS = 15_000;

const app = document.getElementById('app');
let ws = null;
let presenceRefreshTimer = null;

// Mobile CSS reads this to decide whether to show the chat list or the
// conversation as the single full-screen view — see the max-width media
// query in app.css. No-op layout-wise on desktop, where both stay visible.
function syncMobileViewAttr() {
  app.setAttribute('data-mobile-view', state.selectedChatId ? 'conversation' : 'sidebar');
}

// 100dvh doesn't reliably track the on-screen keyboard on some Android
// Chrome versions — the layout box stays taller than the actual visible
// area, so the browser scrolls the page to keep the focused input visible,
// dragging the header off-screen with it. visualViewport.height is the one
// source of truth for "how much space is actually visible right now"; pin
// #app to that directly instead of trusting dvh alone.
if (window.visualViewport) {
  const syncViewportHeight = () => {
    app.style.height = `${window.visualViewport.height}px`;
    window.scrollTo(0, 0);
  };
  window.visualViewport.addEventListener('resize', syncViewportHeight);
  syncViewportHeight();
}

// overflow:hidden on html/body stops wheel/programmatic scroll but not a raw
// touch drag — with the keyboard open, dragging inside a focused input can
// still get the OS/browser to scroll the document itself, revealing empty
// space below the composer. Block touchmove outside our real scroll
// containers so there's nothing left for a stray drag to scroll.
document.addEventListener(
  'touchmove',
  (event) => {
    if (!event.target.closest('[data-list="messages"], .chat-list')) {
      event.preventDefault();
    }
  },
  { passive: false }
);

initTheme();

function renderRoot() {
  if (state.view === 'login') {
    app.innerHTML = '<div data-zone="auth"></div>';
  } else {
    app.innerHTML = `
      <div class="app-shell">
        <div data-zone="sidebar"></div>
        <div class="conversation-pane" data-zone="conversation"></div>
      </div>
      <div data-zone="toast"></div>
      <div data-zone="settings"></div>
    `;
  }
  wireZones();
  notifyAll();
}

function wireZones() {
  const authRoot = app.querySelector('[data-zone="auth"]');
  if (authRoot) {
    onZoneOnce('auth', () =>
      renderAuth(authRoot, {
        onRerender: () => notify('auth'),
        onSubmit: handleAuthSubmit,
        onVerify: handleVerifyEmail,
        onGitHubLogin: handleGitHubLogin,
        onTagInput: handleAuthTagInput,
        onRequestPasswordReset: handleRequestPasswordReset,
        onResetPassword: handleResetPassword,
      })
    );
  }

  const sidebarRoot = app.querySelector('[data-zone="sidebar"]');
  if (sidebarRoot) {
    onZoneOnce('sidebar', () =>
      renderSidebar(sidebarRoot, {
        onRerenderAll: notifyAll,
        onSearchChange: handleSearchChange,
        onSelectChat: handleSelectChat,
        onCreateChat: handleCreateChat,
        onLogout: handleLogout,
        onOpenSettings: handleOpenSettings,
      })
    );
  }

  const settingsRoot = app.querySelector('[data-zone="settings"]');
  if (settingsRoot) {
    onZoneOnce('settings', () =>
      renderSettings(settingsRoot, {
        onClose: handleCloseSettings,
        onTagInput: handleSettingsTagInput,
        onSaveTag: handleSaveTag,
        onSaveDisplayName: handleSaveDisplayName,
        onChangePassword: handleChangePassword,
        onUploadAvatar: handleUploadAvatar,
      })
    );
  }

  const conversationRoot = app.querySelector('[data-zone="conversation"]');
  if (conversationRoot) {
    onZoneOnce('conversation', () =>
      renderConversation(conversationRoot, {
        onDraftChange: () => notify('conversation'),
        onSend: handleSend,
        onEditMessage: handleEditMessage,
        onDeleteMessageForAll: handleDeleteMessageForAll,
        onDeleteMessageForMe: handleDeleteMessageForMe,
        onLoadMoreHistory: handleLoadMoreHistory,
        onMessageVisible: handleMessageVisible,
        onBack: handleBackToChats,
      })
    );
  }

  const toastRoot = app.querySelector('[data-zone="toast"]');
  if (toastRoot) {
    onZoneOnce('toast', () =>
      renderToast(toastRoot, {
        onDismiss: handleToastDismiss,
        onOpen: (chatId) => {
          handleSelectChat(chatId);
          handleToastDismiss();
        },
      })
    );
  }
}

const registered = {};
function onZoneOnce(zone, fn) {
  if (registered[zone]) return;
  registered[zone] = true;
  onZone(zone, fn);
  fn();
}

async function handleAuthSubmit({ email, password, tag, displayName, isRegister }) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    if (isRegister) {
      await authApi.register(email, password, tag, displayName);
      state.pendingVerifyEmail = email;
      state.authMode = 'verify';
      state.authBusy = false;
      notify('auth');
      return;
    }

    const data = await authApi.login(email, password);
    setAccessToken(data.accessToken);
    await enterApp();
  } catch (err) {
    state.authError = err instanceof ApiError ? err.message : 'Что-то пошло не так';
    state.authBusy = false;
    notify('auth');
  }
}

async function handleVerifyEmail({ email, code }) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    await authApi.verifyEmail(email, code);
    state.authMode = 'login';
    state.authError = 'Email подтверждён. Теперь войдите.';
    state.pendingVerifyEmail = '';
    state.authBusy = false;
    notify('auth');
  } catch (err) {
    state.authError = err instanceof ApiError ? err.message : 'Не удалось подтвердить код';
    state.authBusy = false;
    notify('auth');
  }
}

async function handleRequestPasswordReset(email) {
  state.authError = '';
  state.authSuccess = '';
  state.authBusy = true;
  notify('auth');

  try {
    await authApi.requestPasswordReset(email);
    state.authMode = 'reset-password';
    state.authBusy = false;
    notify('auth');
  } catch (err) {
    state.authError = err instanceof ApiError ? err.message : 'Не удалось отправить код';
    state.authBusy = false;
    notify('auth');
  }
}

async function handleResetPassword({ token, newPassword }) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    await authApi.resetPassword(token, newPassword);
    state.authMode = 'login';
    state.authError = 'Пароль изменён. Теперь войдите.';
    state.authBusy = false;
    notify('auth');
  } catch (err) {
    state.authError = err instanceof ApiError ? err.message : 'Не удалось изменить пароль';
    state.authBusy = false;
    notify('auth');
  }
}

function handleGitHubLogin() {
  const clientId = window.WISP_GITHUB_CLIENT_ID || '';
  const redirectUri = `${window.location.origin}/auth/github/callback`;
  window.location.href = `https://github.com/login/oauth/authorize?client_id=${encodeURIComponent(
    clientId
  )}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=user:email`;
}

const TAG_CHECK_DEBOUNCE_MS = 350;
const TAG_MIN_LENGTH = 3;
let tagCheckDebounceTimer = null;
let tagCheckSeq = 0;

function makeTagInputHandler(zone) {
  return (tag) => {
    clearTimeout(tagCheckDebounceTimer);

    if (tag.length < TAG_MIN_LENGTH) {
      state.tagCheck = null;
      notify(zone);
      return;
    }

    const seq = ++tagCheckSeq;
    tagCheckDebounceTimer = setTimeout(async () => {
      try {
        const data = await authApi.checkTagAvailable(tag);
        if (seq !== tagCheckSeq) return;
        state.tagCheck = { tag, available: data.available, suggestedTag: data.suggestedTag || '' };
        notify(zone);
      } catch {
        if (seq !== tagCheckSeq) return;
        state.tagCheck = null;
        notify(zone);
      }
    }, TAG_CHECK_DEBOUNCE_MS);
  };
}

const handleAuthTagInput = makeTagInputHandler('auth');
const handleSettingsTagInput = makeTagInputHandler('settings');

async function enterApp() {
  const me = await resolveCurrentUser();
  state.currentUser = me;
  state.chats = await loadChats(me.id);
  state.view = 'app';
  renderRoot();
  syncMobileViewAttr();
  connectWs();
  requestNotificationPermission();
}

async function resolvePeer(userId) {
  try {
    const user = await authApi.getUserByID(userId);
    return {
      id: userId,
      email: user?.email ?? null,
      tag: user?.tag ?? userId,
      displayName: user?.displayName || user?.tag || userId,
    };
  } catch {
    return { id: userId, email: null, tag: userId, displayName: userId };
  }
}

async function loadChats(myUserId) {
  try {
    const data = await chatApi.listChats();
    const summaries = data?.chats || [];

    const chats = await Promise.all(
      summaries.map(async (summary) => {
        const peerId = (summary.memberUserIds || []).find((id) => id !== myUserId);
        const peer = await resolvePeer(peerId);

        const lastMessage = summary.lastMessage
          ? { ...summary.lastMessage, mine: summary.lastMessage.senderUserId === myUserId }
          : null;

        return {
          id: summary.chatId,
          peer,
          messages: lastMessage ? [lastMessage] : [],
          historyLoaded: false,
          hasMoreHistory: true,
          loadingMoreHistory: false,
          online: false,
          lastSeenUnix: 0,
          peerLastReadMessageId: null,
        };
      })
    );

    return chats;
  } catch (err) {
    console.error('failed to load chats:', err);
    return [];
  }
}

async function resolveCurrentUser() {
  const id = currentUserIdFromToken();
  if (!id) return { id: null, email: null, tag: null };

  try {
    const data = await authApi.getUserByID(id);
    return {
      id,
      email: data?.email ?? null,
      tag: data?.tag ?? null,
      displayName: data?.displayName || data?.tag || null,
    };
  } catch {
    return { id, email: null, tag: null, displayName: null };
  }
}

const SEARCH_DEBOUNCE_MS = 350;
let searchDebounceTimer = null;

const SEARCH_MIN_QUERY_LEN = 3;

function handleSearchChange() {
  state.foundUsers = [];
  notify('sidebar');

  clearTimeout(searchDebounceTimer);
  const query = state.searchQuery.trim();
  if (query.length < SEARCH_MIN_QUERY_LEN) return;

  searchDebounceTimer = setTimeout(async () => {
    try {
      const data = await authApi.searchUsers(query);
      const existingChatUserIds = new Set(state.chats.map((c) => c.peer.id));

      state.foundUsers = (data?.users || [])
        .filter((u) => u.userId !== state.currentUser?.id && !existingChatUserIds.has(u.userId))
        .map((u) => ({ id: u.userId, tag: u.tag, displayName: u.displayName || u.tag }));

      notify('sidebar');
    } catch {
      // search failed or query too short server-side — leave foundUsers cleared
    }
  }, SEARCH_DEBOUNCE_MS);
}

async function handleCreateChat(user) {
  if (!user) return;
  try {
    const data = await chatApi.createChat(user.id);
    const chat = {
      id: data.chatId,
      peer: { id: user.id, email: null, tag: user.tag, displayName: user.displayName || user.tag },
      messages: [],
      historyLoaded: true, // brand new chat, nothing to fetch
      hasMoreHistory: false,
      loadingMoreHistory: false,
      online: false,
      lastSeenUnix: 0,
      peerLastReadMessageId: null,
    };
    state.chats.unshift(chat);
    state.foundUsers = [];
    state.searchQuery = '';
    handleSelectChat(chat.id);
    notify('sidebar');
    ws?.getPresence(chat.peer.id);
  } catch (err) {
    console.error('failed to create chat:', err);
  }
}

function handleSelectChat(chatId) {
  state.selectedChatId = chatId;
  syncMobileViewAttr();
  state.focusDraftOnRender = true;
  if (state.toast?.chatId === chatId) {
    state.toast = null;
    notify('toast');
  }
  notify('sidebar');
  notify('conversation');

  const chat = state.chats.find((c) => c.id === chatId);
  if (chat && !chat.historyLoaded) {
    ws?.getHistory(chatId);
  }
  if (chat) {
    ws?.getReadStatus(chatId, chat.peer.id);
  }
}

// On mobile the sidebar and conversation are two full-screen views (CSS
// switches between them based on whether a chat is selected) — this un-
// selects the chat to go back to the list. Harmless no-op on desktop, where
// both panes are always visible side by side.
function handleBackToChats() {
  state.selectedChatId = null;
  syncMobileViewAttr();
  notify('sidebar');
  notify('conversation');
}

// Tracks the highest-index message we've already told the server we read,
// per chat — IntersectionObserver fires repeatedly as messages scroll past,
// and we only want to notify the server when we've read further than before.
const readProgress = new Map();

function handleMessageVisible(chatId, messageId) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat) return;

  const index = chat.messages.findIndex((m) => m.messageId === messageId);
  if (index === -1) return;

  const message = chat.messages[index];
  if (message.mine) return; // no point marking our own messages as read

  const lastIndex = readProgress.get(chatId) ?? -1;
  if (index <= lastIndex) return;

  readProgress.set(chatId, index);
  ws?.markRead(chatId, messageId);
}

function handleLoadMoreHistory(chatId) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat || !chat.hasMoreHistory || chat.loadingMoreHistory) return;

  chat.loadingMoreHistory = true;
  ws?.getHistory(chatId, HISTORY_PAGE_SIZE, chat.messages.length);
  notify('conversation');
}

function handleSend() {
  const text = state.draft.trim();
  if (!text || !state.selectedChatId) return;

  ws?.sendMessage(state.selectedChatId, text);
  state.draft = '';
  state.scrollToBottomOnRender = true;
  notify('conversation');
}

function handleEditMessage(messageId, newText) {
  const text = newText.trim();
  if (!text || !state.selectedChatId) return;
  ws?.editMessage(state.selectedChatId, messageId, text);
}

function handleDeleteMessageForAll(messageId) {
  if (!state.selectedChatId) return;
  ws?.deleteMessageForAll(state.selectedChatId, messageId);
}

function handleDeleteMessageForMe(messageId) {
  if (!state.selectedChatId) return;
  ws?.deleteMessageForMe(state.selectedChatId, messageId);
}

async function handleLogout() {
  ws?.close();
  ws = null;
  clearInterval(presenceRefreshTimer);
  readProgress.clear();
  try {
    await authApi.logout();
  } catch {
    // best-effort — even if the request fails, drop local session state
  }
  setAccessToken(null);
  state.view = 'login';
  state.authMode = 'login';
  state.authError = '';
  state.currentUser = null;
  state.chats = [];
  state.selectedChatId = null;
  state.searchQuery = '';
  state.foundUsers = [];
  state.draft = '';
  state.toast = null;
  registered.auth = false;
  registered.sidebar = false;
  registered.conversation = false;
  registered.toast = false;
  registered.settings = false;
  renderRoot();
}

function handleOpenSettings() {
  state.settingsOpen = true;
  state.settingsError = '';
  state.settingsPasswordError = '';
  state.settingsPasswordSuccess = '';
  state.tagCheck = null;
  notify('settings');
}

function handleCloseSettings() {
  state.settingsOpen = false;
  state.settingsError = '';
  state.settingsPasswordError = '';
  state.settingsPasswordSuccess = '';
  state.tagCheck = null;
  notify('settings');
}

async function handleSaveTag(tag) {
  state.settingsError = '';
  state.settingsBusy = true;
  notify('settings');

  try {
    const data = await authApi.updateTag(tag);
    if (state.currentUser) state.currentUser.tag = data.tag;
    state.settingsOpen = false;
    state.tagCheck = null;
    state.settingsBusy = false;
    notify('settings');
    notify('sidebar');
  } catch (err) {
    state.settingsError = err instanceof ApiError ? err.message : 'Не удалось изменить тег';
    state.settingsBusy = false;
    notify('settings');
  }
}

async function handleSaveDisplayName(displayName) {
  state.settingsNameError = '';
  state.settingsNameBusy = true;
  notify('settings');

  try {
    const data = await authApi.updateDisplayName(displayName);
    if (state.currentUser) state.currentUser.displayName = data.displayName;
    state.settingsNameBusy = false;
    notify('settings');
    notify('sidebar');
    if (state.selectedChatId) notify('conversation');
  } catch (err) {
    state.settingsNameError = err instanceof ApiError ? err.message : 'Не удалось изменить имя';
    state.settingsNameBusy = false;
    notify('settings');
  }
}

async function handleChangePassword(oldPassword, newPassword, confirmPassword) {
  state.settingsPasswordError = '';
  state.settingsPasswordSuccess = '';

  if (!oldPassword || !newPassword || !confirmPassword) {
    state.settingsPasswordError = 'Заполните все поля';
    notify('settings');
    return;
  }
  if (newPassword !== confirmPassword) {
    state.settingsPasswordError = 'Пароли не совпадают';
    notify('settings');
    return;
  }

  state.settingsPasswordBusy = true;
  notify('settings');

  try {
    await authApi.changePassword(oldPassword, newPassword);
    state.settingsPasswordBusy = false;
    state.settingsPasswordSuccess = 'Пароль изменён';
    notify('settings');
  } catch (err) {
    state.settingsPasswordError = err instanceof ApiError ? err.message : 'Не удалось изменить пароль';
    state.settingsPasswordBusy = false;
    notify('settings');
  }
}

const MAX_AVATAR_SIZE_BYTES = 2 * 1024 * 1024;

async function handleUploadAvatar(file) {
  state.settingsAvatarError = '';

  if (file.size > MAX_AVATAR_SIZE_BYTES) {
    state.settingsAvatarError = 'Файл слишком большой (максимум 2МБ)';
    notify('settings');
    return;
  }

  state.settingsAvatarBusy = true;
  notify('settings');

  try {
    await authApi.uploadAvatar(file);
    bumpAvatarCacheVersion();
    state.settingsAvatarBusy = false;
    notify('settings');
    notify('sidebar');
    if (state.selectedChatId) notify('conversation');
  } catch (err) {
    state.settingsAvatarError = err instanceof ApiError ? err.message : 'Не удалось загрузить фото';
    state.settingsAvatarBusy = false;
    notify('settings');
  }
}

function handleToastDismiss() {
  state.toast = null;
  notify('toast');
}

function connectWs() {
  ws = new WsClient({
    refreshAccessToken: async () => {
      const ok = await refreshAccessToken();
      return ok ? getAccessToken() : null;
    },
    onMessageReceived: async ({ chatId, message }) => {
      await appendMessage(chatId, message);

      if (chatId !== state.selectedChatId || document.hidden) {
        showToast(chatId, message.text);
      }
      notify('sidebar');
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onNotifyPush: ({ chatId }) => {
      if (chatId === state.selectedChatId && !document.hidden) return;
      const chat = state.chats.find((c) => c.id === chatId);
      showToast(chatId, chat?.messages.at(-1)?.text ?? 'Новое сообщение');
    },
    onHistory: ({ chatId, messages, offset }) => {
      const chat = state.chats.find((c) => c.id === chatId);
      if (!chat) return;

      const history = messages.map((m) => ({ ...m, mine: m.senderUserId === state.currentUser?.id }));

      if (offset > 0) {
        // Older page loaded while scrolling up — prepend, keep everything else untouched.
        const existingIds = new Set(chat.messages.map((m) => m.messageId));
        const older = history.filter((m) => !existingIds.has(m.messageId));
        chat.messages = [...older, ...chat.messages];
        chat.hasMoreHistory = messages.length >= HISTORY_PAGE_SIZE;
        chat.loadingMoreHistory = false;
      } else {
        // Re-sync of the latest page (initial load, or a reconnect refresh).
        // chat.messages may already hold more than this page — older ones
        // loaded via scroll-up pagination, or newer ones appended live after
        // this fetch was sent — so merge by id and re-sort by time instead of
        // assuming `history` is a prefix/suffix of what's already there.
        const merged = new Map(chat.messages.map((m) => [m.messageId, m]));
        for (const m of history) merged.set(m.messageId, m);
        chat.messages = [...merged.values()].sort((a, b) => a.createdAtUnix - b.createdAtUnix);
        chat.historyLoaded = true;
        chat.hasMoreHistory = messages.length >= HISTORY_PAGE_SIZE;
      }

      notify('sidebar');
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onMessageSent: () => {
      /* ack only; the actual message is appended via onMessageReceived fanout */
    },
    onPresenceChanged: ({ peerUserId, online, lastSeenUnix }) => {
      const chat = state.chats.find((c) => c.peer.id === peerUserId);
      if (!chat) return;
      chat.online = online;
      chat.lastSeenUnix = lastSeenUnix;
      notify('sidebar');
      if (chat.id === state.selectedChatId) notify('conversation');
    },
    onProfileUpdated: ({ peerUserId, tag, displayName }) => {
      const chat = state.chats.find((c) => c.peer.id === peerUserId);
      if (!chat) return;
      chat.peer.tag = tag;
      chat.peer.displayName = displayName;
      bumpAvatarCacheVersion();
      notify('sidebar');
      if (chat.id === state.selectedChatId) notify('conversation');
    },
    onReadStatus: ({ chatId, peerUserId, lastReadMessageId }) => {
      const chat = state.chats.find((c) => c.id === chatId && c.peer.id === peerUserId);
      if (!chat) return;
      chat.peerLastReadMessageId = lastReadMessageId || null;
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onMessageUpdated: ({ chatId, messageId, newText, deleted }) => {
      const chat = state.chats.find((c) => c.id === chatId);
      if (!chat) return;

      const message = chat.messages.find((m) => m.messageId === messageId);
      if (message) {
        if (deleted) {
          message.deleted = true;
        } else if (newText !== null) {
          message.text = newText;
          message.editedAtUnix = Math.floor(Date.now() / 1000);
        }
      }

      notify('sidebar');
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onError: ({ error }) => {
      console.error('ws-gateway error:', error);
    },
    onReconnected: () => {
      refreshAfterReconnect();
    },
  });
  ws.connect(getAccessToken());
  refreshAfterReconnect();
  startPresenceRefresh();
}

function refreshAfterReconnect() {
  for (const chat of state.chats) {
    ws.getPresence(chat.peer.id);
    if (chat.historyLoaded) {
      ws.getHistory(chat.id);
    }
    if (chat.id === state.selectedChatId) {
      ws.getReadStatus(chat.id, chat.peer.id);
    }
  }
}

// Presence is pushed over core NATS (no delivery guarantee) only when a peer
// connects/disconnects — if that single event is lost (e.g. during our own
// reconnect window), the peer's online status goes stale until someone
// re-asks. Poll periodically and on tab-focus so it self-heals either way.
function refreshAllPresence() {
  for (const chat of state.chats) {
    ws?.getPresence(chat.peer.id);
  }
}

function startPresenceRefresh() {
  clearInterval(presenceRefreshTimer);
  presenceRefreshTimer = setInterval(refreshAllPresence, PRESENCE_REFRESH_INTERVAL_MS);
}

document.addEventListener('visibilitychange', () => {
  if (!document.hidden) refreshAllPresence();
});

async function appendMessage(chatId, message) {
  const mine = message.senderUserId === state.currentUser?.id;
  let index = state.chats.findIndex((c) => c.id === chatId);

  if (index === -1) {
    if (mine) return; // nothing sensible to backfill from our own fanout copy
    const peer = await resolvePeer(message.senderUserId);
    state.chats.unshift({
      id: chatId,
      peer,
      messages: [],
      historyLoaded: true,
      hasMoreHistory: false,
      loadingMoreHistory: false,
      online: false,
      lastSeenUnix: 0,
      peerLastReadMessageId: null,
    });
    index = 0;
  }

  const chat = state.chats[index];
  chat.messages.push({ ...message, mine });

  // Bump the chat to the top of the list, same as every other messenger —
  // sidebar renders state.chats in array order with no separate sort.
  if (index > 0) {
    state.chats.splice(index, 1);
    state.chats.unshift(chat);
  }
}

let toastTimer = null;
function showToast(chatId, text) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat) return;

  const name = chat.peer.displayName || chat.peer.tag;
  playNotificationSound();

  if (document.hidden) {
    showBrowserNotification(chatId, name, text);
    return;
  }

  state.toast = { chatId, peerUserId: chat.peer.id, name, avatarSeed: chat.peer.tag, text };
  notify('toast');

  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    state.toast = null;
    notify('toast');
  }, TOAST_AUTO_DISMISS_MS);
}

function requestNotificationPermission() {
  if (!('Notification' in window)) return;
  if (Notification.permission === 'default') {
    Notification.requestPermission().catch(() => {});
  }
}

function showBrowserNotification(chatId, name, text) {
  if (!('Notification' in window) || Notification.permission !== 'granted') return;

  const notification = new Notification(name, { body: text, tag: chatId });
  notification.addEventListener('click', () => {
    window.focus();
    handleSelectChat(chatId);
    notification.close();
  });
}

let audioCtx = null;
function unlockAudio() {
  audioCtx ||= new (window.AudioContext || window.webkitAudioContext)();
  if (audioCtx.state === 'suspended') audioCtx.resume();
}
document.addEventListener('click', unlockAudio, { once: true });
document.addEventListener('keydown', unlockAudio, { once: true });

function playNotificationSound() {
  try {
    audioCtx ||= new (window.AudioContext || window.webkitAudioContext)();
    if (audioCtx.state === 'suspended') {
      audioCtx.resume().then(() => playNotificationSound());
      return;
    }

    const now = audioCtx.currentTime;
    const gain = audioCtx.createGain();
    gain.gain.setValueAtTime(0.08, now);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.45);
    gain.connect(audioCtx.destination);

    const osc = audioCtx.createOscillator();
    osc.type = 'sine';
    osc.frequency.setValueAtTime(880, now);
    osc.frequency.setValueAtTime(1175, now + 0.16);
    osc.connect(gain);
    osc.start(now);
    osc.stop(now + 0.45);
  } catch (err) {
    console.warn('notification sound failed:', err);
  }
}

async function bootstrap() {
  const restored = await refreshAccessToken();
  if (restored) {
    await enterApp();
  } else {
    state.view = 'login';
    renderRoot();
  }
}

bootstrap();
