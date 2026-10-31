// The blocked page, over the whole screen: why and for how long, a clock that
// runs down, the bars to shake while waiting. It looks every little while
// whether the block is lifted and lets the person out at once, without a new
// sign-in. The words are openvk's banned.latte.

import { api, currentUser } from './api.js';
import { banEnd, formatDateTime } from './labels.js';
import { avatarURL, el, fullName, logout } from './layout.js';
import { floatText, gameIcon, helpButton } from './game.js';
import { coins, dealTick, fanfare } from './sounds.js';
import { tr } from './i18n.js';

// How often the block is looked at, and how long "Вы свободны" stays.
const checkEvery = 15_000;
const freeTime = 1_500;
const shakesKey = 'freenet.banshakes';
const milestones = [10, 25, 50, 100, 250, 500, 1000];

function savedShakes() {
  try {
    return Number(localStorage.getItem(shakesKey)) || 0;
  } catch {
    return 0;
  }
}

function saveShakes(count) {
  try {
    localStorage.setItem(shakesKey, String(count));
  } catch {
    // The count starts over next time.
  }
}

const sayings = () => [
  tr('Решётка держится.'),
  tr('Охранник зевнул.'),
  tr('Где-то звякнули ключи.'),
  tr('Кажется, прут шатается!'),
  tr('Сосед по камере одобряет.'),
];

// clockParts splits what is left into days, hours, minutes and seconds.
export function clockParts(ms) {
  const seconds = Math.max(0, Math.floor(ms / 1000));

  return [Math.floor(seconds / 86_400), Math.floor(seconds / 3600) % 24, Math.floor(seconds / 60) % 60, seconds % 60];
}

function clockCell(name) {
  const digit = el('b', { className: 'ban-digit', textContent: '00' });

  return [digit, el('div', { className: 'ban-clock-cell' }, digit, el('span', { className: 'hint', textContent: name }))];
}

// showClock runs the time left; onEnd is told when it is out.
function showClock(ban, onEnd) {
  if (!ban.until) return el('div', { className: 'ban-forever', textContent: banEnd(ban) });

  const cells = [tr('дней'), tr('часов'), tr('минут'), tr('секунд')].map(clockCell);
  const fill = el('span', { className: 'ban-bar-fill' });
  const served = el('span', { className: 'hint' });
  const from = new Date(ban.bannedAt).getTime();
  const to = new Date(ban.until).getTime();
  let timer = 0;

  const tick = () => {
    const now = Date.now();
    const share = Math.min(1, Math.max(0, (now - from) / (to - from)));

    clockParts(to - now).forEach((part, i) => { cells[i][0].textContent = String(part).padStart(2, '0'); });
    fill.style.transform = `scaleX(${share})`;
    served.textContent = tr('Отсижено: {0} %', Math.floor(share * 100));
    if (now >= to) {
      clearInterval(timer);
      onEnd();
    }
  };

  timer = setInterval(tick, 1000);
  tick();

  return el('div', {},
    el('div', { className: 'ban-clock' }, ...cells.map(([, cell]) => cell)),
    el('span', { className: 'ban-bar' }, fill),
    served);
}

// showBars is what there is to do in the meantime.
function showBars(photo) {
  let shakes = savedShakes();
  const sum = el('b', { textContent: String(shakes) });
  const said = el('div', { className: 'ban-said', textContent: tr('Потрясите решётку, легче станет.') });

  photo.addEventListener('click', () => {
    shakes += 1;
    saveShakes(shakes);
    sum.textContent = String(shakes);
    photo.classList.remove('ban-shake');
    void photo.offsetWidth; // the animation starts over on a new press
    photo.classList.add('ban-shake');
    floatText(photo, '+1');
    if (milestones.includes(shakes)) {
      fanfare();
      said.textContent = tr('{0} раз! Рекорд камеры.', shakes);
    } else {
      dealTick();
      if (shakes % 5 === 0) said.textContent = sayings()[(shakes / 5) % 5];
    }
  });

  return el('div', { className: 'ban-shakes' }, said, el('span', { className: 'hint' }, tr('Потрясли решётку: '), sum));
}

export function showBanScreen(me, accounts) {
  const name = fullName(me);
  const ban = me.ban;
  const title = el('span', { textContent: tr('Страница заблокирована') });
  const photo = el('button', { className: 'ban-photo', type: 'button', title: tr('Потрясти решётку') },
    el('img', { src: avatarURL(me, 'large'), alt: '' }), el('span', { className: 'ban-grid' }));
  const look = el('button', { className: 'button game-go small', type: 'button', textContent: tr('Проверить сейчас') });
  const box = el('div', { className: 'ban-window', role: 'alertdialog', ariaModal: 'true' });
  let done = false;

  const free = () => {
    done = true;
    box.classList.add('ban-free');
    title.textContent = tr('Вы свободны!');
    gate.replaceChildren(el('b', { textContent: tr('Страницу разблокировали. Открываем её…') }));
    fanfare();
    setTimeout(() => location.reload(), freeTime);
  };

  // check asks whether the block is lifted; currentUser takes a new token
  // when it is.
  const check = async () => {
    if (done) return;

    const fresh = await currentUser().catch(() => me);

    if (fresh && !fresh.ban) free();
  };

  const gate = el('div', { className: 'ban-gate' },
    el('p', {}, tr('Извините, '), el('b', { textContent: name }), tr(', но вы были верискокнуты.')),
    el('p', {}, tr('А причина этому проста: ')),
    el('div', { className: 'ban-reason', textContent: ban.reason }),
    el('p', { className: 'hint', textContent: tr('Заблокировано: {0}', formatDateTime(ban.bannedAt)) }),
    showClock(ban, check),
    showBars(photo),
    el('div', { className: 'ban-actions' }, look,
      accounts,
      el('a', { href: '/', textContent: tr('выйти'), onclick: logout })));

  look.addEventListener('click', async () => {
    dealTick();
    look.disabled = true;
    await check();
    look.disabled = false;
    if (!done) {
      coins();
      floatText(look, tr('Пока нет'), 'bad');
    }
  });

  box.append(
    el('div', { className: 'game-window-title ban-title' }, gameIcon('/static/icons/game/ban.svg'), title,
      helpButton(tr('Решение о блокировке принимают администраторы сайта. Как только её снимут или выйдет срок, страница откроется сама.'))),
    el('div', { className: 'ban-body' }, photo, gate));
  document.body.append(el('div', { className: 'ban-screen' }, box));
  document.documentElement.classList.add('ban-open');
  setInterval(check, checkEvery);
}
