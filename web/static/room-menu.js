import { api } from './api.js';
import { setCounter } from './layout.js';
let started = false;
export function listenRooms() {
  if (started) return;
  started = true;
  const update = async () => {
    if (document.hidden || document.documentElement.classList.contains('shell')) return;
    try { const page = await api.rooms(); setCounter('rooms', page.rooms.filter((room) => room.friends > 0).length); }
    catch { setCounter('rooms', 0); }
  };
  update();
  const timer = setInterval(update, 15_000);
  window.addEventListener('pagehide', () => clearInterval(timer), { once: true });
}
