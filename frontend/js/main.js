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
import { renderAuth } from './screens/auth.js';
import { renderSidebar } from './screens/sidebar.js';
import { renderConversation } from './screens/conversation.js';
import { renderToast } from './screens/toast.js';
import { renderSettings } from './screens/settings.js';

const TOAST_AUTO_DISMISS_MS = 5_000;

const app = document.getElementById('app');
let ws = null;

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

async function handleAuthSubmit({ email, password, tag, isRegister }) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    if (isRegister) {
      await authApi.register(email, password, tag);
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
  connectWs();
  requestNotificationPermission();
}

async function loadChats(myUserId) {
  try {
    const data = await chatApi.listChats();
    const summaries = data?.chats || [];

    const chats = await Promise.all(
      summaries.map(async (summary) => {
        const peerId = (summary.memberUserIds || []).find((id) => id !== myUserId);
        let peer = { id: peerId, email: null, tag: peerId };
        try {
          const user = await authApi.getUserByID(peerId);
          peer = { id: peerId, email: user?.email ?? null, tag: user?.tag ?? peerId };
        } catch {
          // keep the fallback peer shape if the lookup fails
        }

        const lastMessage = summary.lastMessage
          ? { ...summary.lastMessage, mine: summary.lastMessage.senderUserId === myUserId }
          : null;

        return {
          id: summary.chatId,
          peer,
          messages: lastMessage ? [lastMessage] : [],
          historyLoaded: false,
          online: false,
          lastSeenUnix: 0,
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
    return { id, email: data?.email ?? null, tag: data?.tag ?? null };
  } catch {
    return { id, email: null, tag: null };
  }
}

const SEARCH_DEBOUNCE_MS = 350;
let searchDebounceTimer = null;

function handleSearchChange() {
  state.foundUser = null;
  notify('sidebar');

  clearTimeout(searchDebounceTimer);
  const query = state.searchQuery.trim();
  const hasExistingMatch = state.chats.some((c) => c.peer.tag.toLowerCase().includes(query.toLowerCase()));
  if (!query || hasExistingMatch) return;

  searchDebounceTimer = setTimeout(async () => {
    try {
      const user = await authApi.getUserByTag(query);
      if (user?.userId && user.userId !== state.currentUser?.id) {
        state.foundUser = { id: user.userId, tag: user.tag };
        notify('sidebar');
      }
    } catch {
      // no exact match — leave foundUser cleared
    }
  }, SEARCH_DEBOUNCE_MS);
}

async function handleCreateChat(user) {
  if (!user) return;
  try {
    const data = await chatApi.createChat(user.id);
    const chat = {
      id: data.chatId,
      peer: { id: user.id, email: null, tag: user.tag },
      messages: [],
      historyLoaded: true, // brand new chat, nothing to fetch
      online: false,
      lastSeenUnix: 0,
    };
    state.chats.unshift(chat);
    state.foundUser = null;
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
}

function handleSend() {
  const text = state.draft.trim();
  if (!text || !state.selectedChatId) return;

  ws?.sendMessage(state.selectedChatId, text);
  state.draft = '';
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
  state.foundUser = null;
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
  state.tagCheck = null;
  notify('settings');
}

function handleCloseSettings() {
  state.settingsOpen = false;
  state.settingsError = '';
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

function handleToastDismiss() {
  state.toast = null;
  notify('toast');
}

function connectWs() {
  ws = new WsClient({
    onMessageReceived: ({ chatId, message }) => {
      appendMessage(chatId, message);

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
    onHistory: ({ chatId, messages }) => {
      const chat = state.chats.find((c) => c.id === chatId);
      if (!chat) return;

      const history = messages.map((m) => ({ ...m, mine: m.senderUserId === state.currentUser?.id }));

      const historyIds = new Set(history.map((m) => m.messageId));
      const alreadyLive = chat.messages.filter((m) => !historyIds.has(m.messageId));
      chat.messages = [...history, ...alreadyLive];
      chat.historyLoaded = true;

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
  });
  ws.connect(getAccessToken());
  ws.socket.addEventListener('open', () => {
    for (const chat of state.chats) {
      ws.getPresence(chat.peer.id);
    }
  });
}

function appendMessage(chatId, message) {
  const index = state.chats.findIndex((c) => c.id === chatId);
  if (index === -1) return;

  const chat = state.chats[index];
  chat.messages.push({ ...message, mine: message.senderUserId === state.currentUser?.id });

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

  playNotificationSound();

  if (document.hidden) {
    showBrowserNotification(chatId, chat.peer.tag, text);
    return;
  }

  state.toast = { chatId, name: chat.peer.tag, text };
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
