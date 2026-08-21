function bufToBase64(buf) {
  let binary = '';
  const bytes = new Uint8Array(buf);
  for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

function base64ToBuf(base64) {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

const PBKDF2_ITERATIONS = 600_000;
const WRAP_SALT_LENGTH = 16;
const GCM_IV_LENGTH = 12;

// crypto.subtle only exists in a secure context (HTTPS or localhost) — on
// plain HTTP over LAN (e.g. a phone hitting the dev box by IP) it's silently
// undefined instead of throwing something diagnosable, which otherwise shows
// up as a confusing "Cannot read properties of undefined" deep inside a
// WebCrypto call.
function requireSubtleCrypto() {
  if (!window.isSecureContext || !crypto.subtle) {
    throw new Error(
      'Шифрование недоступно: откройте сайт по HTTPS (crypto.subtle работает только в защищённом контексте)'
    );
  }
}

// Kept in memory for the lifetime of the tab, populated by unwrapping the
// server's wrapped blob with the password (see unwrapPrivateKey). Also
// mirrored into IndexedDB as a device-local convenience cache — see below —
// so a page reload on a device that already unlocked the account once
// doesn't force the password to be re-typed every time.
let cachedPrivateKey = null;
let cachedPublicKeySpkiBase64 = null;

const DB_NAME = 'wisp-crypto';
const DB_VERSION = 1;
const STORE_NAME = 'keys';

function openDb() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      req.result.createObjectStore(STORE_NAME);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function idbGet(key) {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly');
    const req = tx.objectStore(STORE_NAME).get(key);
    req.onsuccess = () => resolve(req.result ?? null);
    req.onerror = () => reject(req.error);
  });
}

async function idbSet(key, value) {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite');
    tx.objectStore(STORE_NAME).put(value, key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function idbDelete(key) {
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite');
    tx.objectStore(STORE_NAME).delete(key);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

// Keyed by user ID so switching accounts in the same browser can't pick up
// a stale/wrong cached key.
function deviceCacheKey(userId) {
  return `unlocked-private-key:${userId}`;
}

// Tries the device-local cache first — the CryptoKey stashed in IndexedDB
// the last time this browser unwrapped the key with the password. Only ever
// populated by rememberPrivateKeyOnThisDevice below, and only readable by
// this origin (IndexedDB is already origin-scoped), so it doesn't weaken the
// "server can't decrypt" property; it just saves re-typing the password on
// every reload of a device that's already unlocked the account once.
export async function loadPrivateKeyFromThisDevice(userId) {
  const stored = await idbGet(deviceCacheKey(userId));
  if (!stored) return false;

  cachedPrivateKey = stored.privateKey;
  cachedPublicKeySpkiBase64 = stored.publicKeySpkiBase64;
  return true;
}

async function rememberPrivateKeyOnThisDevice(userId) {
  await idbSet(deviceCacheKey(userId), {
    privateKey: cachedPrivateKey,
    publicKeySpkiBase64: cachedPublicKeySpkiBase64,
  });
}

export async function forgetPrivateKeyOnThisDevice(userId) {
  await idbDelete(deviceCacheKey(userId));
}

async function deriveWrappingKey(password, keyWrapSaltBase64) {
  const passwordKey = await crypto.subtle.importKey('raw', new TextEncoder().encode(password), 'PBKDF2', false, [
    'deriveKey',
  ]);
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: base64ToBuf(keyWrapSaltBase64), iterations: PBKDF2_ITERATIONS, hash: 'SHA-256' },
    passwordKey,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  );
}

// Generates a fresh RSA-OAEP-2048 keypair and wraps the private key with a
// password-derived key. Called once, at registration. Returns everything the
// register request needs to send, plus caches the private key for immediate
// use in this tab. Doesn't need a userId (registration happens before one
// exists) — call rememberPrivateKeyOnThisDevice-equivalent separately if
// needed; in practice the post-verify login flow re-derives via
// unwrapPrivateKey anyway, which does cache.
export async function generateAndWrapKeyPair(password) {
  requireSubtleCrypto();
  const keyPair = await crypto.subtle.generateKey(
    { name: 'RSA-OAEP', modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' },
    true,
    ['encrypt', 'decrypt']
  );

  const spki = await crypto.subtle.exportKey('spki', keyPair.publicKey);
  const pkcs8 = await crypto.subtle.exportKey('pkcs8', keyPair.privateKey);

  const keyWrapSalt = crypto.getRandomValues(new Uint8Array(WRAP_SALT_LENGTH));
  const keyWrapSaltBase64 = bufToBase64(keyWrapSalt.buffer);
  const wrappingKey = await deriveWrappingKey(password, keyWrapSaltBase64);

  const iv = crypto.getRandomValues(new Uint8Array(GCM_IV_LENGTH));
  const wrappedBuf = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, wrappingKey, pkcs8);
  const combined = new Uint8Array(iv.byteLength + wrappedBuf.byteLength);
  combined.set(iv, 0);
  combined.set(new Uint8Array(wrappedBuf), iv.byteLength);

  cachedPrivateKey = keyPair.privateKey;
  cachedPublicKeySpkiBase64 = bufToBase64(spki);

  return {
    publicKeySpkiBase64: cachedPublicKeySpkiBase64,
    wrappedPrivateKeyBase64: bufToBase64(combined.buffer),
    keyWrapSaltBase64,
  };
}

// Reverses generateAndWrapKeyPair's wrapping step: re-derives the wrapping
// key from the password + salt fetched from the server, then unwraps and
// imports the private key. Called on every login (any device) so this tab
// has a usable private key without ever having generated one itself. Also
// mirrors the unlocked key into this device's IndexedDB cache, so a later
// page reload on the same device can skip straight to
// loadPrivateKeyFromThisDevice instead of asking for the password again.
export async function unwrapPrivateKey(userId, password, wrappedPrivateKeyBase64, keyWrapSaltBase64) {
  requireSubtleCrypto();
  const wrappingKey = await deriveWrappingKey(password, keyWrapSaltBase64);

  const combined = new Uint8Array(base64ToBuf(wrappedPrivateKeyBase64));
  const iv = combined.slice(0, GCM_IV_LENGTH);
  const ciphertext = combined.slice(GCM_IV_LENGTH);
  const pkcs8 = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, wrappingKey, ciphertext);

  cachedPrivateKey = await crypto.subtle.importKey(
    'pkcs8',
    pkcs8,
    { name: 'RSA-OAEP', hash: 'SHA-256' },
    // Extractable, not "false" — rewrapPrivateKey (used on password change)
    // needs to re-export this key to wrap it under a new password-derived
    // key. It never leaves this module in raw form either way: exportKey is
    // only ever called from within crypto.js itself, immediately re-wrapped
    // before the result crosses back out to the rest of the app.
    true,
    ['decrypt']
  );

  await rememberPrivateKeyOnThisDevice(userId);
}

// Re-wraps the already-cached private key with a new password — used right
// before ChangePassword, so the ciphertext on the server stays in sync with
// whatever password the user will type in on their next login from any
// device. The RSA keypair itself doesn't change, only its wrapping.
export async function rewrapPrivateKey(newPassword) {
  if (!cachedPrivateKey) throw new Error('no private key loaded to rewrap');

  const pkcs8 = await crypto.subtle.exportKey('pkcs8', cachedPrivateKey);

  const keyWrapSalt = crypto.getRandomValues(new Uint8Array(WRAP_SALT_LENGTH));
  const keyWrapSaltBase64 = bufToBase64(keyWrapSalt.buffer);
  const wrappingKey = await deriveWrappingKey(newPassword, keyWrapSaltBase64);

  const iv = crypto.getRandomValues(new Uint8Array(GCM_IV_LENGTH));
  const wrappedBuf = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, wrappingKey, pkcs8);
  const combined = new Uint8Array(iv.byteLength + wrappedBuf.byteLength);
  combined.set(iv, 0);
  combined.set(new Uint8Array(wrappedBuf), iv.byteLength);

  return { wrappedPrivateKeyBase64: bufToBase64(combined.buffer), keyWrapSaltBase64 };
}

export function hasPrivateKey() {
  return cachedPrivateKey !== null;
}

export function clearPrivateKey() {
  cachedPrivateKey = null;
  cachedPublicKeySpkiBase64 = null;
}

async function importPublicKey(spkiBase64) {
  return crypto.subtle.importKey('spki', base64ToBuf(spkiBase64), { name: 'RSA-OAEP', hash: 'SHA-256' }, false, [
    'encrypt',
  ]);
}

// Encrypts a raw AES key (as exported bytes) for a peer with their RSA
// public key. Returns base64 ciphertext suitable for chat_members storage.
export async function encryptChatKeyForPeer(rawAesKeyBuf, peerPublicKeySpkiBase64) {
  const peerKey = await importPublicKey(peerPublicKeySpkiBase64);
  const ciphertext = await crypto.subtle.encrypt({ name: 'RSA-OAEP' }, peerKey, rawAesKeyBuf);
  return bufToBase64(ciphertext);
}

// Decrypts this account's copy of a chat's AES key using the cached RSA
// private key (populated by unwrapPrivateKey/generateAndWrapKeyPair).
// Returns the imported AES-GCM CryptoKey, ready for message encrypt/decrypt.
export async function decryptChatKey(encryptedChatKeyBase64) {
  if (!cachedPrivateKey) throw new Error('no private key loaded — call unwrapPrivateKey after login first');
  const rawAesKeyBuf = await crypto.subtle.decrypt(
    { name: 'RSA-OAEP' },
    cachedPrivateKey,
    base64ToBuf(encryptedChatKeyBase64)
  );
  // Extractable, not "false" — reKeyChatForStalePeers needs to re-export
  // this key to re-seal it for a peer whose public key has changed. It
  // never leaves crypto.js/main.js in a way that reaches the server or
  // anywhere persisted — only ever re-wrapped under a peer's RSA public key
  // before being sent anywhere.
  return crypto.subtle.importKey('raw', rawAesKeyBuf, { name: 'AES-GCM' }, true, ['encrypt', 'decrypt']);
}

// Generates a brand-new AES-256 chat key and returns both the importable
// CryptoKey (for immediate use) and its raw bytes (to RSA-encrypt per member).
export async function generateChatKey() {
  const key = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, ['encrypt', 'decrypt']);
  const raw = await crypto.subtle.exportKey('raw', key);
  return { key, raw };
}

// Encrypts plaintext with a chat's AES-GCM key. Returns a single base64
// string: a random 12-byte IV followed by the ciphertext (with its GCM auth
// tag), so callers only need to store/transmit one opaque blob.
export async function encryptMessage(aesKey, plaintext) {
  const iv = crypto.getRandomValues(new Uint8Array(GCM_IV_LENGTH));
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, aesKey, new TextEncoder().encode(plaintext));
  const combined = new Uint8Array(iv.byteLength + ciphertext.byteLength);
  combined.set(iv, 0);
  combined.set(new Uint8Array(ciphertext), iv.byteLength);
  return bufToBase64(combined.buffer);
}

// Reverses encryptMessage. Throws if the blob is malformed or the auth tag
// doesn't verify (wrong key, tampered ciphertext).
export async function decryptMessage(aesKey, encodedBase64) {
  const combined = new Uint8Array(base64ToBuf(encodedBase64));
  const iv = combined.slice(0, GCM_IV_LENGTH);
  const ciphertext = combined.slice(GCM_IV_LENGTH);
  const plaintextBuf = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, aesKey, ciphertext);
  return new TextDecoder().decode(plaintextBuf);
}
