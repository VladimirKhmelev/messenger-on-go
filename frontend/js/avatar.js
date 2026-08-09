const PALETTE_SIZE = 5;

export function avatarPalette(seed) {
  const index = (hashString(seed) % PALETTE_SIZE) + 1;
  return {
    bg: `var(--avatar-bg-${index})`,
    text: `var(--avatar-text-${index})`,
  };
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
