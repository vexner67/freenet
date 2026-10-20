// The jackpot of the casino: a win of a million votes and more in "Огонёк"
// or "Прицел" takes the whole screen — a flash, turning rays, fireworks, a
// rain of coins (in the New Year: of gifts, trees and snowflakes), the
// number counting up to the win and a button to take it. The New Year
// look (html.festive) has its own colours, words and rain. A click on the
// button, Esc or ten seconds close it; with reduced motion it only shows
// the win.

import { el } from './layout.js';
import { plural } from './labels.js';
import { tr } from './i18n.js';

// MILLION is the win that gets the jackpot.
export const MILLION = 1000000;

const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
const grouped = new Intl.NumberFormat('ru-RU');

const RAIN = ['🪙', '💰', '💎', '⭐', '🪙', '💵'];
const FESTIVE_RAIN = ['🎁', '🎄', '❄️', '⭐', '🍬', '🎅'];

function between(low, high) {
  return low + Math.random() * (high - low);
}

// rain is the pieces falling over the screen.
function rain(pieces) {
  return el('div', { className: 'jackpot-rain' }, ...Array.from({ length: 60 }, (_, i) => {
    const piece = el('span', { className: 'jackpot-piece', textContent: pieces[i % pieces.length] });

    piece.style.left = `${between(0, 100)}%`;
    piece.style.fontSize = `${Math.round(between(18, 40))}px`;
    piece.style.animationDelay = `${between(0, 4).toFixed(2)}s`;
    piece.style.setProperty('--fall', `${between(2.2, 4.2).toFixed(2)}s`);
    piece.style.setProperty('--dx', `${Math.round(between(-120, 120))}px`);
    piece.style.setProperty('--turn', `${Math.round(between(-720, 720))}deg`);

    return piece;
  }));
}

// fireworks are bursts of sparks here and there, one after another.
function fireworks() {
  return el('div', { className: 'jackpot-fireworks' }, ...Array.from({ length: 7 }, (_, b) => {
    const burst = el('div', { className: 'jackpot-burst' }, ...Array.from({ length: 18 }, (_, i) => {
      const spark = el('span', { className: 'jackpot-spark' });
      const angle = (i / 18) * Math.PI * 2;
      const reach = between(70, 130);

      spark.style.setProperty('--x', `${Math.round(Math.cos(angle) * reach)}px`);
      spark.style.setProperty('--y', `${Math.round(Math.sin(angle) * reach)}px`);

      return spark;
    }));

    burst.style.left = `${between(10, 90)}%`;
    burst.style.top = `${between(10, 60)}%`;
    burst.style.animationDelay = `${(b * 0.55 + between(0, 0.3)).toFixed(2)}s`;
    for (const spark of burst.children) spark.style.animationDelay = burst.style.animationDelay;

    return burst;
  }));
}

// countUp runs the number from 0 to votes, slowing down at the end.
function countUp(node, votes, done) {
  if (still) {
    node.textContent = grouped.format(votes);
    done();
    return;
  }

  const start = performance.now();
  const lasts = 2600;
  const step = (now) => {
    const t = Math.min(1, (now - start) / lasts);
    const eased = 1 - (1 - t) ** 4;

    node.textContent = grouped.format(Math.round(votes * eased));
    if (t < 1) {
      requestAnimationFrame(step);
    } else {
      done();
    }
  };

  requestAnimationFrame(step);
}

// jackpot shows the win of votes over the whole screen.
export function jackpot(votes) {
  const festive = document.documentElement.classList.contains('festive');
  const won = `${grouped.format(votes)} ${plural(votes, ['голос', 'голоса', 'голосов'])}`;
  const sum = el('div', { className: 'jackpot-sum', textContent: '0' });
  const take = el('button', { className: 'button jackpot-take', type: 'button', textContent: festive ? tr('Забрать подарок!') : tr('Забрать!') });

  const card = el('div', { className: 'jackpot-card' },
    el('div', { className: 'jackpot-kicker', textContent: festive ? tr('Новогодний джекпот') : tr('Джекпот') }),
    el('div', { className: 'jackpot-title', textContent: festive ? tr('Новогодний миллион!') : tr('Миллион!') }),
    sum,
    el('div', { className: 'jackpot-unit', textContent: plural(votes, ['голос', 'голоса', 'голосов']) }),
    el('div', { className: 'jackpot-note', textContent: festive ? tr('С Новым годом и новым миллионом! Вы выиграли {0}.', won) : tr('Невероятно! Вы выиграли {0}.', won) }),
    take,
  );

  const overlay = el('div', { className: festive ? 'jackpot jackpot-festive' : 'jackpot' },
    el('div', { className: 'jackpot-rays' }),
    still ? null : fireworks(),
    still ? null : rain(festive ? FESTIVE_RAIN : RAIN),
    card,
  );
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', tr('Джекпот'));

  let timer = 0;
  const close = () => {
    clearTimeout(timer);
    document.removeEventListener('keydown', onKey);
    overlay.remove();
  };
  const onKey = (e) => {
    if (e.key === 'Escape') close();
  };

  take.addEventListener('click', close);
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  take.focus();
  if (!still) navigator.vibrate?.([120, 60, 120, 60, 300]);

  countUp(sum, votes, () => {
    sum.classList.add('jackpot-done');
    timer = setTimeout(close, 10000);
  });
}
