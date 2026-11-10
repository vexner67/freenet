// Shared screen math has no DOM or browser clocks hidden in it.
export const SEEK_THRESHOLD = 2;

export function playbackPosition(playback, serverNow) {
  const position = Number.isFinite(playback?.position) ? Math.max(0, playback.position) : 0;
  const updated = Date.parse(playback?.updatedAt ?? '');
  if (!playback?.playing || !Number.isFinite(updated) || !Number.isFinite(serverNow)) return position;
  return position + Math.max(0, serverNow - updated) / 1000;
}

export function needsRoomSeek(actual, expected) {
  return Number.isFinite(actual) && Number.isFinite(expected) && Math.abs(actual - expected) > SEEK_THRESHOLD;
}

export function roomClockOffset(serverTime, sentAt, receivedAt) {
  const server = Date.parse(serverTime);
  return Number.isFinite(server) ? server - (sentAt + receivedAt) / 2 : 0;
}

export function roomYouTubeID(raw) {
  try {
    const url = new URL(raw);
    if (url.protocol !== 'https:' && url.protocol !== 'http:') return '';
    const host = url.hostname.toLowerCase();
    let id = '';
    if (host === 'youtu.be') id = url.pathname.slice(1);
    if (['youtube.com', 'www.youtube.com', 'm.youtube.com', 'music.youtube.com', 'www.youtube-nocookie.com'].includes(host)) {
      id = url.pathname === '/watch' ? url.searchParams.get('v') : url.pathname.match(/^\/(?:embed|shorts)\/([^/]+)$/)?.[1];
    }
    return /^[a-zA-Z0-9_-]{11}$/.test(id ?? '') ? id : '';
  } catch {
    return '';
  }
}

// Exactly one participant advances a finished queue. Shared manual control
// does not mean every browser should send its own automatic "next".
export function roomPlaybackLeader(memberIDs, ownerID, control) {
  if (memberIDs.includes(ownerID)) return ownerID;
  return control === 'everyone' ? [...memberIDs].sort()[0] ?? '' : '';
}
