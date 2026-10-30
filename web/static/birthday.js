// The birthday of the owner of a page: confetti bursts over it, as a
// firework, and again on a click at the plaque (showBirthday in profile.js).
import { el, isPhone } from './layout.js';

const VOLLEYS = [[0.3, 0.32], [0.7, 0.28], [0.5, 0.45]];
const VOLLEY_GAP_MS = 350;
const PIECES = 46;

function burst(sky, x, y, pieces) {
  for (let i = 0; i < pieces; i++) {
    const piece = el('span', { className: 'birthday-piece' });
    const angle = Math.random() * 2 * Math.PI;
    const reach = 60 + Math.random() * 190;

    piece.style.left = `${x * 100}%`;
    piece.style.top = `${y * 100}%`;
    piece.style.setProperty('--dx', `${Math.round(Math.cos(angle) * reach)}px`);
    piece.style.setProperty('--dy', `${Math.round(Math.sin(angle) * reach)}px`);
    piece.style.setProperty('--turn', `${Math.round((Math.random() - 0.5) * 1440)}deg`);
    piece.style.setProperty('--fall', `${(1.8 + Math.random()).toFixed(2)}s`);
    piece.addEventListener('animationend', () => piece.remove());
    sky.append(piece);
  }
}

// celebrate fires the volleys; one who asked for less motion gets none.
export function celebrate() {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  const sky = document.querySelector('.birthday-sky') ?? document.body.appendChild(el('div', { className: 'birthday-sky' }));
  const pieces = isPhone() ? PIECES / 2 : PIECES;

  VOLLEYS.forEach(([x, y], i) => setTimeout(() => burst(sky, x, y, pieces), i * VOLLEY_GAP_MS));
}
