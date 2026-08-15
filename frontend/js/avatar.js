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

// Renders an avatar as a real photo when one exists, falling back to the
// initials circle on load failure (404 = no avatar uploaded, or a transient
// error) — avoids a round-trip existence check before every render. `extraHtml`
// lets callers inject sibling content (e.g. a presence dot) inside the circle.
export function renderAvatar(userId, seed, name, { sizeClass = '', extraHtml = '' } = {}) {
  const palette = avatarPalette(seed);
  const classes = `avatar ${sizeClass}`.trim();
  return `
    <div class="${classes}" style="background:${palette.bg};color:${palette.text}" data-avatar-for="${escapeAttr(
      userId
    )}">
      <span class="avatar-fallback">${initialsFrom(name)}</span>
      <img
        class="avatar-img"
        src="${avatarUrl(userId)}"
        alt=""
        loading="lazy"
        onerror="this.remove()"
      />${extraHtml}
    </div>
  `;
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
