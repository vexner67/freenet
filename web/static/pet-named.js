// The frame of BIG 2Z, the pet named after churkez228: its shadow under a
// lock until 22:30 in Moscow, then a blinking "Открыть". The pet falls out
// of a chest with a flash and stands in its pose with what it is beside it.

import { api } from './api.js';
import { petRarityName } from './labels.js';
import { $, el, hideError, showError } from './layout.js';
import { button } from './modal.js';
import { petImage } from './pet.js';
import { tr } from './i18n.js';

const fallTakes = 2600;

function left(ms) {
  const seconds = Math.ceil(ms / 1000);
  const part = (n) => String(n).padStart(2, '0');

  return `${part(Math.floor(seconds / 3600))}:${part(Math.floor(seconds / 60) % 60)}:${part(seconds % 60)}`;
}

function stat(text) {
  return el('li', { textContent: text });
}

// show plays the fall and calls done with the pet when it is taken.
function show(pet, kind, done) {
  const stats = kind.stats ?? {};
  const about = el('div', { className: 'named-about' },
    el('span', { className: 'pet-rarity pet-rarity-named', textContent: petRarityName('named') }),
    el('div', { className: 'named-title', textContent: 'BIG 2Z' }),
    el('p', { textContent: tr('Золотые рога, цепь с двумя Z и молнии в кулаках. Гроза арены, названный в честь churkez228: каждый третий его удар проходит сквозь защиту. Выдаётся всем и один раз.') }),
    el('ul', { className: 'named-stats' },
      stat(tr('Атака: {0}', stats.attack)), stat(tr('Защита: {0}', stats.defense)),
      stat(tr('Здоровье: {0}', stats.health)), stat(tr('Скорость: {0}', stats.speed))),
    button(tr('Забрать'), () => {
      scene.remove();
      done(pet);
    }));
  const scene = el('div', { className: 'named-scene' },
    el('div', { className: 'named-rays' }),
    el('div', { className: 'named-flash' }),
    el('img', { className: 'named-chest', src: '/static/pets/cases/gold.svg', alt: '' }),
    el('img', { className: 'named-pose', src: '/static/pets/big2z-pose.svg', alt: 'BIG 2Z' }),
    about);
  document.body.append(scene);
  setTimeout(() => scene.classList.add('named-done'), fallTakes);
}

// showNamedPet draws the frame unless the person has the pet; taken gets
// the pet.
export async function showNamedPet(kind, taken) {
  const named = await api.namedPet();
  if (named.claimed) return;

  const frame = $('named-pet');
  const opensAt = new Date(named.opensAt);
  const status = el('div', { className: 'named-status' });
  const shadow = el('span', { className: 'named-shadow' }, petImage('big2z', 96));
  const lock = el('span', { className: 'named-lock', textContent: '🔒' });
  const open = button(tr('Открыть'), async () => {
    open.disabled = true;
    try {
      const { pet } = await api.claimNamedPet();
      hideError($('error'));
      frame.hidden = true;
      show(pet, kind, taken);
    } catch (e) {
      open.disabled = false;
      showError($('error'), e);
    }
  });
  open.classList.add('named-open');
  open.hidden = true;

  frame.setAttribute('aria-label', tr('Рамка BIG 2Z'));
  frame.replaceChildren(
    el('div', { className: 'named-picture' }, shadow, lock),
    el('div', { className: 'named-text' },
      el('div', { className: 'named-title', textContent: '? ? ?' }),
      el('div', { textContent: tr('Именной питомец в честь churkez228') }),
      status, open));
  frame.hidden = false;

  const tick = () => {
    const rest = opensAt - Date.now();
    if (rest > 0) {
      status.textContent = tr('Откроется в 22:30 по Москве, через {0}', left(rest));
      return;
    }
    clearInterval(timer);
    status.textContent = tr('Он уже здесь. Открывайте!');
    lock.hidden = true;
    open.hidden = false;
  };
  const timer = setInterval(tick, 1000);
  tick();
}
