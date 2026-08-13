const RECONNECT_MIN_DELAY_MS = 1000;
const RECONNECT_MAX_DELAY_MS = 15000;

export class WsClient {
  constructor({
    onMessageReceived,
    onNotifyPush,
    onHistory,
    onMessageSent,
    onPresenceChanged,
    onMessageUpdated,
    onProfileUpdated,
    onError,
    onReconnected,
    refreshAccessToken,
  }) {
    this.socket = null;
    this.pendingMessages = [];
    this.onMessageReceived = onMessageReceived;
    this.onNotifyPush = onNotifyPush;
    this.onHistory = onHistory;
    this.onMessageSent = onMessageSent;
    this.onPresenceChanged = onPresenceChanged;
    this.onMessageUpdated = onMessageUpdated;
    this.onProfileUpdated = onProfileUpdated;
    this.onError = onError;
    this.onReconnected = onReconnected;
    this.refreshAccessToken = refreshAccessToken;

    this.accessToken = null;
    this.reconnectAttempts = 0;
    this.reconnectTimer = null;
    this.closedByUser = false;
    this.everConnected = false;
    this._refreshTokenNow = false;
  }

  connect(accessToken) {
    this.accessToken = accessToken;
    this.closedByUser = false;
    this._open();
  }

  _open() {
    clearTimeout(this.reconnectTimer);

    let openedSuccessfully = false;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(this.accessToken)}`;
    this.socket = new WebSocket(url);

    this.socket.addEventListener('open', () => {
      openedSuccessfully = true;
      const wasReconnect = this.everConnected;
      this.everConnected = true;
      this.reconnectAttempts = 0;

      const queued = this.pendingMessages;
      this.pendingMessages = [];
      for (const payload of queued) {
        this.socket.send(JSON.stringify(payload));
      }

      if (wasReconnect) {
        this.onReconnected?.();
      }
    });

    this.socket.addEventListener('message', (event) => {
      let msg;
      try {
        msg = JSON.parse(event.data);
      } catch {
        return;
      }
      this.dispatch(msg);
    });

    this.socket.addEventListener('close', () => {
      if (this.closedByUser) return;
      const refreshTokenFirst = !openedSuccessfully || this._refreshTokenNow;
      this._refreshTokenNow = false;
      this._scheduleReconnect({ refreshTokenFirst });
    });

    this.socket.addEventListener('error', () => {
      this.socket?.close();
    });
  }

  _scheduleReconnect({ refreshTokenFirst = false } = {}) {
    clearTimeout(this.reconnectTimer);
    const delay = Math.min(RECONNECT_MIN_DELAY_MS * 2 ** this.reconnectAttempts, RECONNECT_MAX_DELAY_MS);
    this.reconnectAttempts += 1;
    this.reconnectTimer = setTimeout(async () => {
      if (refreshTokenFirst && this.refreshAccessToken) {
        const freshToken = await this.refreshAccessToken();
        if (freshToken) this.accessToken = freshToken;
      }
      this._open();
    }, delay);
  }

  dispatch(msg) {
    switch (msg.type) {
      case 'message_received':
        this.onMessageReceived?.({ chatId: msg.chat_id, message: normalizeMessage(msg.message) });
        break;
      case 'notify_push':
        this.onNotifyPush?.({ chatId: msg.chat_id, messageId: msg.message_id });
        break;
      case 'history':
        this.onHistory?.({
          chatId: msg.chat_id,
          messages: (msg.messages || []).map(normalizeMessage),
          offset: msg.offset || 0,
        });
        break;
      case 'message_sent':
        this.onMessageSent?.({ messageId: msg.message_id });
        break;
      case 'presence_changed':
        this.onPresenceChanged?.({
          peerUserId: msg.peer_user_id,
          online: msg.online,
          lastSeenUnix: msg.last_seen_unix,
        });
        break;
      case 'message_updated':
        this.onMessageUpdated?.({
          chatId: msg.chat_id,
          messageId: msg.message_id,
          newText: msg.new_text ?? null,
          deleted: !!msg.deleted,
        });
        break;
      case 'profile_updated':
        this.onProfileUpdated?.({
          peerUserId: msg.peer_user_id,
          tag: msg.peer_tag,
          displayName: msg.peer_display_name,
        });
        break;
      case 'token_expired':
        this._refreshTokenNow = true;
        break;
      case 'error':
        this.onError?.({ error: msg.error });
        break;
    }
  }

  sendMessage(chatId, text) {
    this.send({ type: 'send_message', chat_id: chatId, text });
  }

  getHistory(chatId, limit = 50, offset = 0) {
    this.send({ type: 'get_history', chat_id: chatId, limit, offset });
  }

  getPresence(peerUserId) {
    this.send({ type: 'get_presence', peer_user_id: peerUserId });
  }

  editMessage(chatId, messageId, text) {
    this.send({ type: 'edit_message', chat_id: chatId, message_id: messageId, text });
  }

  deleteMessageForAll(chatId, messageId) {
    this.send({ type: 'delete_message_for_all', chat_id: chatId, message_id: messageId });
  }

  deleteMessageForMe(chatId, messageId) {
    this.send({ type: 'delete_message_for_me', chat_id: chatId, message_id: messageId });
  }

  send(payload) {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(payload));
    } else {
      // CONNECTING, CLOSING, CLOSED, or no socket yet — queue it. The next
      // successful open flushes pendingMessages, so nothing sent during a
      // reconnect gap is silently dropped.
      this.pendingMessages.push(payload);
    }
  }

  close() {
    this.closedByUser = true;
    clearTimeout(this.reconnectTimer);
    this.socket?.close();
    this.socket = null;
    this.pendingMessages = [];
  }
}

function normalizeMessage(raw) {
  if (!raw) return null;
  return {
    messageId: raw.MessageID ?? raw.messageId,
    senderUserId: raw.SenderUserID ?? raw.senderUserId,
    text: raw.Text ?? raw.text,
    createdAtUnix: raw.CreatedAtUnix ?? raw.createdAtUnix,
    editedAtUnix: raw.EditedAtUnix ?? raw.editedAtUnix ?? 0,
    deleted: raw.Deleted ?? raw.deleted ?? false,
  };
}
