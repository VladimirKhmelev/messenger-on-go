export const state = {
  view: 'login', // 'login' | 'app'
  authMode: 'login', // 'login' | 'register' | 'verify'
  authError: '',
  authBusy: false,
  pendingVerifyEmail: '', // email awaiting a confirmation code, set when authMode === 'verify'
  tagCheck: null, // { tag, available, suggestedTag } — result of the last debounced tag-availability check

  currentUser: null, // { id, email, tag }

  chats: [], // { id, peer: {id, email, tag}, messages: [], online, lastSeenUnix }
  selectedChatId: null,
  searchQuery: '',
  foundUser: null, // { id, tag } — result of an exact-tag lookup when search matches no existing chat
  draft: '',
  editingMessageId: null, // set while a message's inline edit field is open

  toast: null, // { chatId, name, text }

  settingsOpen: false,
  settingsError: '',
  settingsBusy: false,
};

const listeners = {
  auth: [],
  sidebar: [],
  conversation: [],
  toast: [],
  settings: [],
};

export function onZone(zone, fn) {
  listeners[zone].push(fn);
}

export function notify(zone) {
  for (const fn of listeners[zone]) fn();
}

export function notifyAll() {
  for (const zone of Object.keys(listeners)) notify(zone);
}
