const PALETTE_SIZE = 5;

export function avatarPalette(seed) {
  const index = (hashString(seed) % PALETTE_SIZE) + 1;
  return {
    bg: `var(--avatar-bg-${index})`,
    text: `var(--avatar-text-${index})`,
  };
}

// Bumped whenever any avatar might have changed (our own upload, or a
// profile_updated event from a peer) so <img src> cache-busts and re-fetches
// instead of showing the browser's cached copy of the old photo.
let cacheBustVersion = 0;
export function bumpAvatarCacheVersion() {
  cacheBustVersion += 1;
}

export function avatarUrl(userId) {
  return `/v1/users/${encodeURIComponent(userId)}/avatar?v=${cacheBustVersion}`;
}

export function groupAvatarUrl(chatId) {
  return `/v1/chats/${encodeURIComponent(chatId)}/avatar?v=${cacheBustVersion}`;
}

// Renders an avatar as a real photo when one exists, falling back to the
// initials circle on load failure (404 = no avatar uploaded, or a transient
// error) — avoids a round-trip existence check before every render. `extraHtml`
// lets callers inject sibling content (e.g. a presence dot) inside the circle.
// `avatarFor`/`src` default to the per-user avatar endpoint but can be
// overridden (e.g. groupAvatarUrl) to render a group chat's photo instead.
export function renderAvatar(userId, seed, name, { sizeClass = '', extraHtml = '', avatarFor = userId, src = avatarUrl(userId) } = {}) {
  const palette = avatarPalette(seed);
  const classes = `avatar ${sizeClass}`.trim();
  return `
    <div class="${classes}" style="background:${palette.bg};color:${palette.text}" data-avatar-for="${escapeAttr(
      avatarFor
    )}">
      <span class="avatar-fallback">${initialsFrom(name)}</span>
      <img
        class="avatar-img"
        src="${src}"
        alt=""
        loading="lazy"
        onerror="this.remove()"
      />${extraHtml}
    </div>
  `;
}

// A full `root.innerHTML = ...` re-render (the whole app's rendering model —
// no virtual DOM) always creates a fresh <img> for every avatar, even when
// nothing about that avatar changed. A brand-new <img> briefly shows the
// fallback initials before the browser's cache resolves the load, so on a
// chatty screen (presence/read-receipts/new messages all trigger a
// re-render) every avatar visibly flickers on each update.
//
// snapshotAvatarImages/restoreAvatarImages bracket the innerHTML swap: grab
// the already-loaded <img> elements beforehand by their data-avatar-for
// container, then splice them back in afterwards in place of the freshly
// created ones with the same src — same pixels stay on screen, no flicker.
export function snapshotAvatarImages(root) {
  const snapshot = new Map();
  root.querySelectorAll('[data-avatar-for] > img.avatar-img').forEach((img) => {
    const container = img.parentElement;
    const userId = container.getAttribute('data-avatar-for');
    if (userId) snapshot.set(userId, img);
  });
  return snapshot;
}

export function restoreAvatarImages(root, snapshot) {
  if (snapshot.size === 0) return;
  root.querySelectorAll('[data-avatar-for] > img.avatar-img').forEach((freshImg) => {
    const container = freshImg.parentElement;
    const userId = container.getAttribute('data-avatar-for');
    const oldImg = snapshot.get(userId);
    if (oldImg && oldImg.src === freshImg.src) {
      freshImg.replaceWith(oldImg);
    }
  });
}

function escapeAttr(str) {
  return String(str).replace(/"/g, '&quot;');
}

export function initialsFrom(name) {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

function hashString(str) {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = (hash * 31 + str.charCodeAt(i)) >>> 0;
  }
  return hash;
}
