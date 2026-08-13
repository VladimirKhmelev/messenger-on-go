export const state = {
  view: 'login', // 'login' | 'app'
  authMode: 'login', // 'login' | 'register' | 'verify' | 'forgot-password' | 'reset-password'
  authError: '',
  authSuccess: '',
  authBusy: false,
  pendingVerifyEmail: '', // email awaiting a confirmation code, set when authMode === 'verify'
  tagCheck: null, // { tag, available, suggestedTag } — result of the last debounced tag-availability check

  currentUser: null, // { id, email, tag, displayName }

  chats: [], // { id, peer: {id, email, tag, displayName}, messages: [], online, lastSeenUnix }
  selectedChatId: null,
  searchQuery: '',
  foundUser: null, // { id, tag } — result of an exact-tag lookup when search matches no existing chat
  draft: '',
  editingMessageId: null, // set while a message's inline edit field is open
  focusDraftOnRender: false, // one-shot flag: focus the composer input on the next conversation render
  scrollToBottomOnRender: false, // one-shot flag: force-scroll to the newest message on the next conversation render

  toast: null, // { chatId, name, text }

  settingsOpen: false,
  settingsError: '',
  settingsBusy: false,
  settingsNameError: '',
  settingsNameBusy: false,
  settingsPasswordError: '',
  settingsPasswordSuccess: '',
  settingsPasswordBusy: false,
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
