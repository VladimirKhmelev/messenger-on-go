import { state, onZone, notify, notifyAll } from './state.js';
import { initTheme } from './theme.js';
import {
  authApi,
  chatApi,
  refreshAccessToken,
  getAccessToken,
  setAccessToken,
  currentUserIdFromToken,
  translateApiError,
} from './api.js';
import { WsClient } from './ws.js';
import { bumpAvatarCacheVersion } from './avatar.js';
import {
  generateAndWrapKeyPair,
  unwrapPrivateKey,
  rewrapPrivateKey,
  hasPrivateKey,
  clearPrivateKey,
  loadPrivateKeyFromThisDevice,
  generateChatKey,
  encryptChatKeyForPeer,
  decryptChatKey,
  encryptMessage,
  decryptMessage,
} from './crypto.js';
import { renderAuth, passwordMeetsRules } from './screens/auth.js';
import { renderSidebar } from './screens/sidebar.js';
import { renderConversation } from './screens/conversation.js';
import { renderToast } from './screens/toast.js';
import { renderSettings } from './screens/settings.js';

const TOAST_AUTO_DISMISS_MS = 5_000;
const HISTORY_PAGE_SIZE = 50;
const PRESENCE_REFRESH_INTERVAL_MS = 5_000;
const MAX_CACHED_MESSAGES_PER_CHAT = 200;
// Mirrors chat-service's own typing TTL (services/chat-service/internal/cache/presence.go) —
// the indicator self-clears if a further start_typing (sent every couple keystrokes while
// composing) never arrives, so a dropped "stop" signal can't leave it stuck.
const TYPING_INDICATOR_TTL_MS = 3_000;
const TYPING_SEND_THROTTLE_MS = 2_000;
const YANDEX_METRIKA_COUNTER_ID = 111937141;

const app = document.getElementById('app');
let ws = null;
let presenceRefreshTimer = null;

// Decrypted AES-GCM chat keys, kept in memory only — re-derived from the
// server's RSA-encrypted copy (via our own private key) on first use per
// chat, per page load. Never persisted: there's nothing sensitive to cache
// across reloads that IndexedDB (holding the RSA private key) doesn't
// already cover.
const chatKeyCache = new Map();
const chatKeyRequests = new Map();

async function getChatKey(chatId) {
  if (chatKeyCache.has(chatId)) return chatKeyCache.get(chatId);
  if (chatKeyRequests.has(chatId)) return chatKeyRequests.get(chatId);

  const promise = (async () => {
    const data = await chatApi.getChatKey(chatId);
    const key = await decryptChatKey(data.encryptedChatKey);
    chatKeyCache.set(chatId, key);
    return key;
  })();

  chatKeyRequests.set(chatId, promise);
  try {
    return await promise;
  } finally {
    chatKeyRequests.delete(chatId);
  }
}

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
        onUnlock: handleUnlock,
        onLogout: handleLogout,
        onGitHubPassphrase: handleGitHubPassphrase,
        onCancelGitHub: handleCancelGitHub,
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
        onTyping: handleTyping,
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

  if (isRegister && !passwordMeetsRules(password)) {
    state.authError = 'Пароль не соответствует требованиям ниже';
    state.authBusy = false;
    notify('auth');
    return;
  }

  try {
    if (isRegister) {
      const { publicKeySpkiBase64, wrappedPrivateKeyBase64, keyWrapSaltBase64 } =
        await generateAndWrapKeyPair(password);
      await authApi.register(
        email,
        password,
        tag,
        displayName,
        publicKeySpkiBase64,
        wrappedPrivateKeyBase64,
        keyWrapSaltBase64
      );
      state.pendingVerifyEmail = email;
      state.authMode = 'verify';
      state.authBusy = false;
      notify('auth');
      return;
    }

    const data = await authApi.login(email, password);
    setAccessToken(data.accessToken);

    // Recover this account's RSA private key on whatever device is logging
    // in right now — same password, same key, no matter how many other
    // devices are already using this account.
    const userId = currentUserIdFromToken();
    const wrappedKeyData = await authApi.getWrappedPrivateKey();
    await unwrapPrivateKey(userId, password, wrappedKeyData.wrappedPrivateKey, wrappedKeyData.keyWrapSalt);

    await enterApp();
  } catch (err) {
    console.error('login failed:', err);
    state.authError = translateApiError(err) ?? `Что-то пошло не так: ${err?.message || err}`;
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
    state.authError = translateApiError(err) ?? 'Не удалось подтвердить код';
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
    state.authError = translateApiError(err) ?? 'Не удалось отправить код';
    state.authBusy = false;
    notify('auth');
  }
}

async function handleResetPassword({ token, newPassword }) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    // Resetting via email means the user doesn't know the password that
    // wrapped their old private key, so there is no way to recover it — the
    // old key material is gone no matter what happens here. The only choice
    // left is generating a fresh identity so the account stays usable,
    // instead of silently stranding it with a wrapped blob nothing can ever
    // open again. The reset-password screen warns about this before
    // submitting.
    const { publicKeySpkiBase64, wrappedPrivateKeyBase64, keyWrapSaltBase64 } =
      await generateAndWrapKeyPair(newPassword);
    await authApi.resetPassword(
      token,
      newPassword,
      publicKeySpkiBase64,
      wrappedPrivateKeyBase64,
      keyWrapSaltBase64
    );
    state.authMode = 'login';
    state.authError = 'Пароль изменён. Теперь войдите.';
    state.authBusy = false;
    notify('auth');
  } catch (err) {
    state.authError = translateApiError(err) ?? 'Не удалось изменить пароль';
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

// Called after any successful session restore (fresh login, or a refresh
// token restored via cookie on page reload) — makes sure this device has the
// private key unwrapped before touching chats, since every chat list/history
// fetch immediately tries to decrypt message previews. A fresh login always
// already has it (handleAuthSubmit unwraps before calling this). A restored
// session might not — loadPrivateKeyFromThisDevice recovers it from this
// device's own IndexedDB cache if a previous unlock left one there;
// otherwise falls back to prompting for the password once.
async function enterAppOrPromptUnlock() {
  const userId = currentUserIdFromToken();

  if (!hasPrivateKey()) {
    const loaded = await loadPrivateKeyFromThisDevice(userId);
    if (!loaded) {
      state.view = 'login';
      state.authMode = 'unlock';
      state.authError = '';
      state.authBusy = false;
      renderRoot();
      return;
    }
  }

  await enterApp();
}

async function handleUnlock(password) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  try {
    const userId = currentUserIdFromToken();
    const wrappedKeyData = await authApi.getWrappedPrivateKey();
    await unwrapPrivateKey(userId, password, wrappedKeyData.wrappedPrivateKey, wrappedKeyData.keyWrapSalt);
    await enterApp();
  } catch (err) {
    state.authError = translateApiError(err) ?? 'Неверный пароль';
    state.authBusy = false;
    notify('auth');
  }
}

// Reached when the page loads on /auth/github/callback?code=... — GitHub's
// code is single-use, so we can't first ask "does this account exist yet" and
// then separately register: we only get one shot at exchanging it. Instead we
// stash the code and ask for the encryption passphrase up front; the actual
// exchange (and the decision of whether this is a new account) happens in
// handleGitHubPassphrase once we have it.
function handleGitHubCallback(code) {
  window.history.replaceState(null, '', '/');
  state.pendingGitHubCode = code;
  state.authMode = 'github-passphrase';
  state.authError = '';
  state.view = 'login';
  renderRoot();
}

function handleCancelGitHub() {
  state.pendingGitHubCode = '';
  state.authMode = 'login';
  state.authError = '';
  notify('auth');
}

async function handleGitHubPassphrase(password) {
  state.authError = '';
  state.authBusy = true;
  notify('auth');

  const code = state.pendingGitHubCode;

  try {
    // Generated speculatively — only kept if the server confirms this is a
    // brand-new account (see below). Wasted work for a returning user, but
    // the code is one-shot, so there's no way to check first without
    // spending it.
    const { publicKeySpkiBase64, wrappedPrivateKeyBase64, keyWrapSaltBase64 } =
      await generateAndWrapKeyPair(password);

    const data = await authApi.loginWithGitHub(code, publicKeySpkiBase64, wrappedPrivateKeyBase64, keyWrapSaltBase64);
    setAccessToken(data.accessToken);
    state.pendingGitHubCode = '';

    const userId = currentUserIdFromToken();
    if (data.isNewUser) {
      await unwrapPrivateKey(userId, password, wrappedPrivateKeyBase64, keyWrapSaltBase64);
    } else {
      // Existing account — the keys we just generated were never stored server-side
      // (the backend ignores them for a login). Fetch the real ones and unwrap those.
      const wrappedKeyData = await authApi.getWrappedPrivateKey();
      await unwrapPrivateKey(userId, password, wrappedKeyData.wrappedPrivateKey, wrappedKeyData.keyWrapSalt);
    }

    await enterApp();
  } catch (err) {
    console.error('GitHub login failed:', err);
    state.authError = translateApiError(err) ?? 'Не удалось войти через GitHub';
    state.authBusy = false;
    notify('auth');
  }
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
        if (lastMessage) await decryptMessageInPlace(summary.chatId, lastMessage);

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
          unreadCount: 0,
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
    const targetKeyData = await authApi.getPublicKey(user.id);
    if (!targetKeyData?.publicKey) {
      console.error('failed to create chat: peer has no encryption key uploaded yet');
      return;
    }

    const myKeyData = await authApi.getPublicKey(state.currentUser.id);
    const { key: chatKey, raw: rawChatKey } = await generateChatKey();
    const encryptedChatKey = {
      [state.currentUser.id]: await encryptChatKeyForPeer(rawChatKey, myKeyData.publicKey),
      [user.id]: await encryptChatKeyForPeer(rawChatKey, targetKeyData.publicKey),
    };
    const wrappedForPublicKey = {
      [state.currentUser.id]: myKeyData.publicKey,
      [user.id]: targetKeyData.publicKey,
    };

    const data = await chatApi.createChat(user.id, encryptedChatKey, wrappedForPublicKey);
    chatKeyCache.set(data.chatId, chatKey);

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
      unreadCount: 0,
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
  trimInactiveChatHistory(state.selectedChatId);
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
  if (chat) chat.unreadCount = 0;
  if (chat && !chat.historyLoaded) {
    ws?.getHistory(chatId);
  }
  if (chat) {
    ws?.getReadStatus(chatId, chat.peer.id);
  }

  reKeyChatForStalePeers(chatId).catch((err) => {
    console.error('failed to re-key chat for stale peer public keys:', err);
  });
}

// A peer who reset their password (see handleResetPassword) gets a brand
// new RSA keypair — their old encrypted_chat_key entry, sealed under their
// previous public key, becomes undecryptable to them. Only someone who
// still holds the plaintext chat key (i.e. whose own private key never
// changed) can fix this: on opening the chat, check every other member's
// wrapped_for_public_key against their current public key, and if they no
// longer match, re-encrypt the chat key under the new one and push it. This
// runs on every open (cheap — a handful of RSA ops at most) rather than
// needing a signal from the server about who changed keys.
async function reKeyChatForStalePeers(chatId) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat) return;

  // getChatKey can legitimately fail here — if THIS account is the one that
  // reset its password, its own copy is exactly what's stale, and only a
  // peer can fix that (see below). That must not stop this account from
  // still fixing a stale copy for a *different* peer whose key it can
  // decrypt — so the two are independent, not a single Promise.all.
  let chatKey;
  try {
    chatKey = await getChatKey(chatId);
  } catch {
    return;
  }
  const rawChatKey = await crypto.subtle.exportKey('raw', chatKey);

  const keysData = await chatApi.listChatKeys(chatId);
  for (const memberKey of keysData.memberKeys || []) {
    if (memberKey.userId === state.currentUser?.id) continue;

    const peerKeyData = await authApi.getPublicKey(memberKey.userId);
    const currentPublicKey = peerKeyData?.publicKey;
    if (!currentPublicKey || currentPublicKey === memberKey.wrappedForPublicKey) continue;

    const encryptedChatKey = await encryptChatKeyForPeer(rawChatKey, currentPublicKey);
    await chatApi.updateChatKey(chatId, memberKey.userId, encryptedChatKey, currentPublicKey);
  }
}

// On mobile the sidebar and conversation are two full-screen views (CSS
// switches between them based on whether a chat is selected) — this un-
// selects the chat to go back to the list. Harmless no-op on desktop, where
// both panes are always visible side by side.
function handleBackToChats() {
  trimInactiveChatHistory(state.selectedChatId);
  state.selectedChatId = null;
  syncMobileViewAttr();
  notify('sidebar');
  notify('conversation');
}

// Chats accumulate messages forever as you scroll up through history — fine
// while a chat stays open (nothing should vanish from under the cursor),
// but with no cap at all a long-lived tab that's visited a lot of chats
// keeps every page it ever loaded in memory. Trim once a chat is no longer
// the one on screen, keeping the newest page (oldest-loaded ones are the
// least likely to be revisited).
function trimInactiveChatHistory(chatId) {
  if (!chatId) return;
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat || chat.messages.length <= MAX_CACHED_MESSAGES_PER_CHAT) return;

  const trimmedCount = chat.messages.length - MAX_CACHED_MESSAGES_PER_CHAT;
  chat.messages = chat.messages.slice(-MAX_CACHED_MESSAGES_PER_CHAT);
  // Server-side history offset is measured against the chat's real message
  // count, which trimming doesn't change — track what we cut so the next
  // "load more" request still asks for the messages actually above what's
  // cached, instead of re-fetching (and duplicating) what's already here.
  chat.trimmedOffset = (chat.trimmedOffset || 0) + trimmedCount;
  chat.hasMoreHistory = true;
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

const lastTypingSentAt = new Map();

function handleTyping(chatId) {
  const now = Date.now();
  const last = lastTypingSentAt.get(chatId) || 0;
  if (now - last < TYPING_SEND_THROTTLE_MS) return;
  lastTypingSentAt.set(chatId, now);
  ws?.startTyping(chatId);
}

function handleLoadMoreHistory(chatId) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat || !chat.hasMoreHistory || chat.loadingMoreHistory) return;

  chat.loadingMoreHistory = true;
  ws?.getHistory(chatId, HISTORY_PAGE_SIZE, chat.messages.length + (chat.trimmedOffset || 0));
  notify('conversation');
}

async function handleSend() {
  const text = state.draft.trim();
  const chatId = state.selectedChatId;
  if (!text || !chatId) return;

  state.draft = '';
  state.scrollToBottomOnRender = true;
  notify('conversation');

  const chatKey = await getChatKey(chatId);
  const ciphertext = await encryptMessage(chatKey, text);
  ws?.sendMessage(chatId, ciphertext);
}

async function handleEditMessage(messageId, newText) {
  const text = newText.trim();
  const chatId = state.selectedChatId;
  if (!text || !chatId) return;

  const chatKey = await getChatKey(chatId);
  const ciphertext = await encryptMessage(chatKey, text);
  ws?.editMessage(chatId, messageId, ciphertext);
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
  clearPrivateKey();
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
  trackLoginPageView();
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
    state.settingsError = translateApiError(err) ?? 'Не удалось изменить тег';
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
    state.settingsNameError = translateApiError(err) ?? 'Не удалось изменить имя';
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
    // The wrapping key is derived from the password, so the wrapped private
    // key has to be re-wrapped for the new password before the server ever
    // sees it — the RSA keypair itself is untouched, only its packaging.
    const { wrappedPrivateKeyBase64, keyWrapSaltBase64 } = await rewrapPrivateKey(newPassword);
    await authApi.changePassword(oldPassword, newPassword, wrappedPrivateKeyBase64, keyWrapSaltBase64);

    state.settingsPasswordBusy = false;
    state.settingsPasswordSuccess = 'Пароль изменён';
    notify('settings');
  } catch (err) {
    state.settingsPasswordError = translateApiError(err) ?? 'Не удалось изменить пароль';
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
    state.settingsAvatarError = translateApiError(err) ?? 'Не удалось загрузить фото';
    state.settingsAvatarBusy = false;
    notify('settings');
  }
}

function handleToastDismiss() {
  state.toast = null;
  notify('toast');
}

// Every message body on the wire/in history is AES-GCM ciphertext under the
// chat's key — decrypt in place before it ever reaches render or the toast
// preview. A deleted message has no body to decrypt; a message from before
// this browser had access to the chat key (or plain decrypt failure) falls
// back to a placeholder rather than throwing and blanking the whole list.
async function decryptMessageInPlace(chatId, message) {
  if (!message || message.deleted || !message.text) return message;
  try {
    const chatKey = await getChatKey(chatId);
    message.text = await decryptMessage(chatKey, message.text);
  } catch (err) {
    console.error('failed to decrypt message:', err);
    message.text = UNDECRYPTABLE_MESSAGE_PLACEHOLDER;
  }
  return message;
}

// Shown whenever a message can't be decrypted with our current chat key.
// The single most common cause: this account reset its password (see
// handleResetPassword), which issued a brand new RSA keypair — every chat
// key sealed for the old one is unreadable until a peer who still has the
// old key re-seals it for the new one (see reKeyChatForStalePeers), which
// only happens once that peer opens the chat. Framed as "waiting", not a
// dead end, since it usually resolves itself without user action.
const UNDECRYPTABLE_MESSAGE_PLACEHOLDER =
  '[Не удалось расшифровать — если вы недавно сбросили пароль, дождитесь, пока собеседник откроет этот чат]';

function connectWs() {
  ws = new WsClient({
    refreshAccessToken: async () => {
      const ok = await refreshAccessToken();
      return ok ? getAccessToken() : null;
    },
    onMessageReceived: async ({ chatId, message }) => {
      await decryptMessageInPlace(chatId, message);
      await appendMessage(chatId, message);

      if (chatId !== state.selectedChatId || document.hidden) {
        showToast(chatId);
      }
      notify('sidebar');
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onNotifyPush: ({ chatId }) => {
      if (chatId === state.selectedChatId && !document.hidden) return;
      showToast(chatId);
    },
    onHistory: async ({ chatId, messages, offset }) => {
      const chat = state.chats.find((c) => c.id === chatId);
      if (!chat) return;

      await Promise.all(messages.map((m) => decryptMessageInPlace(chatId, m)));
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
      // The 5s presence poll re-asks every open chat's status even when nothing
      // changed — a full sidebar re-render on every reply would recreate every
      // avatar <img>, triggering a fresh HTTP request each time. Skip the
      // re-render when the values are actually unchanged.
      if (chat.online === online && chat.lastSeenUnix === lastSeenUnix) return;
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
    onTypingChanged: ({ chatId, peerUserId }) => {
      const chat = state.chats.find((c) => c.id === chatId && c.peer.id === peerUserId);
      if (!chat) return;
      clearTimeout(chat.typingClearTimer);
      chat.peerTyping = true;
      chat.typingClearTimer = setTimeout(() => {
        chat.peerTyping = false;
        if (chatId === state.selectedChatId) notify('conversation');
      }, TYPING_INDICATOR_TTL_MS);
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onReadStatus: ({ chatId, peerUserId, lastReadMessageId }) => {
      const chat = state.chats.find((c) => c.id === chatId && c.peer.id === peerUserId);
      if (!chat) return;
      chat.peerLastReadMessageId = lastReadMessageId || null;
      if (chatId === state.selectedChatId) notify('conversation');
    },
    onMessageUpdated: async ({ chatId, messageId, newText, deleted }) => {
      const chat = state.chats.find((c) => c.id === chatId);
      if (!chat) return;

      const message = chat.messages.find((m) => m.messageId === messageId);
      if (message) {
        if (deleted) {
          message.deleted = true;
        } else if (newText !== null) {
          const chatKey = await getChatKey(chatId);
          try {
            message.text = await decryptMessage(chatKey, newText);
          } catch (err) {
            console.error('failed to decrypt edited message:', err);
            message.text = UNDECRYPTABLE_MESSAGE_PLACEHOLDER;
          }
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
  updateOwnActivity();
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
  presenceRefreshTimer = setInterval(() => {
    refreshAllPresence();
    if (isTabActive()) ws?.setActive();
  }, PRESENCE_REFRESH_INTERVAL_MS);
}

// Our own online status is server-side TTL-based (chat-service, ~30s) and only
// renewed by an explicit set_active — not by mere connectivity. Re-send on the
// same cadence as the presence poll so it doesn't expire while the tab is
// visible, and immediately on visibilitychange/blur/focus so switching away/back
// reflects right away rather than waiting for the next tick.
//
// visibilitychange alone isn't enough: it reliably fires for tab-switching, but
// whether it fires when the whole browser window loses focus (alt-tab, another
// window covering it, switching virtual desktops) is inconsistent across window
// managers/platforms — window.blur/focus catch that case regardless.
function isTabActive() {
  return !document.hidden && document.hasFocus();
}

function updateOwnActivity() {
  if (isTabActive()) {
    ws?.setActive();
  } else {
    ws?.setInactive();
  }
}

document.addEventListener('visibilitychange', () => {
  updateOwnActivity();
  if (!document.hidden) refreshAllPresence();
});
window.addEventListener('blur', updateOwnActivity);
window.addEventListener('focus', () => {
  updateOwnActivity();
  refreshAllPresence();
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
      unreadCount: 0,
    });
    index = 0;
  }

  const chat = state.chats[index];
  chat.messages.push({ ...message, mine });
  if (!mine && (chatId !== state.selectedChatId || document.hidden)) {
    chat.unreadCount = (chat.unreadCount || 0) + 1;
  }

  // Bump the chat to the top of the list, same as every other messenger —
  // sidebar renders state.chats in array order with no separate sort.
  if (index > 0) {
    state.chats.splice(index, 1);
    state.chats.unshift(chat);
  }
}

// Messages are end-to-end encrypted, so previews here are deliberately
// content-free — only "new message" plus how many are unread, never the
// decrypted text, even though we technically hold the key to show it.
function unreadPreviewText(chat) {
  const count = chat.unreadCount || 0;
  return count > 1 ? `Новые сообщения (${count})` : 'Новое сообщение';
}

let toastTimer = null;
function showToast(chatId) {
  const chat = state.chats.find((c) => c.id === chatId);
  if (!chat) return;

  const name = chat.peer.displayName || chat.peer.tag;
  const preview = unreadPreviewText(chat);
  playNotificationSound();

  if (document.hidden) {
    showBrowserNotification(chatId, name, preview);
    return;
  }

  state.toast = { chatId, peerUserId: chat.peer.id, name, avatarSeed: chat.peer.tag, text: preview };
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

function showBrowserNotification(chatId, name, preview) {
  if (!('Notification' in window) || Notification.permission !== 'granted') return;

  const notification = new Notification(name, { body: preview, tag: chatId });
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
  if (window.location.pathname === '/auth/github/callback') {
    const code = new URLSearchParams(window.location.search).get('code');
    if (code) {
      handleGitHubCallback(code);
      return;
    }
    // Missing code (denied consent, or a stray hit on this path) — fall through
    // to the normal boot flow instead of getting stuck on a dead-end URL.
    window.history.replaceState(null, '', '/');
  }

  const restored = await refreshAccessToken();
  if (restored) {
    await enterAppOrPromptUnlock();
  } else {
    state.view = 'login';
    renderRoot();
    trackLoginPageView();
  }
}

// Analytics only ever sees the logged-out door (login/register), never what
// happens inside a chat. GA/Metrika's automatic pageview is disabled in
// index.html — this fires it by hand, only from the two places a new,
// unauthenticated visit to that door actually happens: first load with no
// valid session, and returning here after logging out.
function trackLoginPageView() {
  window.gtag?.('event', 'page_view');
  window.ym?.(YANDEX_METRIKA_COUNTER_ID, 'hit', window.location.href);
}

bootstrap();
