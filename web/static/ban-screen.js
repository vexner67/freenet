// The blocked page, over the whole screen, as a plain notice of the sites
// of the 2000s: a window of the site with the photo, the reason, the term
// and what is left of it. It looks every little while whether the block
// is lifted and lets the person out at once, without a new sign-in.

import { currentUser } from './api.js';
import { formatDateTime } from './labels.js';
import { avatarURL, el, fullName, logout } from './layout.js';
import { gameIcon } from './game.js';
import { fanfare } from './sounds.js';
import { tr } from './i18n.js';

// How often the block is looked at, and how long "Блокировка снята" stays.
const checkEvery = 15_000;
const freeTime = 1_500;

// clockParts splits what is left into days, hours, minutes and seconds.
export function clockParts(ms) {
  const seconds = Math.max(0, Math.floor(ms / 1000));

  return [Math.floor(seconds / 86_400), Math.floor(seconds / 3600) % 24, Math.floor(seconds / 60) % 60, seconds % 60];
}

// clockText is what is left as "2 дн. 03:12:45".
export function clockText(ms) {
  const [days, ...time] = clockParts(ms);
  const clock = time.map((part) => String(part).padStart(2, '0')).join(':');

  return days > 0 ? tr('{0} дн. {1}', days, clock) : clock;
}

// showClock runs the term down in a line of text; onEnd is told when it
// is out.
function showClock(ban, onEnd) {
  if (!ban.until) return null;

  const left = el('b', { className: 'ban-left' });
  const to = new Date(ban.until).getTime();
  let timer = 0;

  const tick = () => {
    left.textContent = clockText(to - Date.now());
    if (Date.now() >= to) {
      clearInterval(timer);
      onEnd();
    }
  };

  timer = setInterval(tick, 1000);
  tick();

  return left;
}

// row is a line of the notice: what, and its value.
function row(label, value) {
  return el('tr', {}, el('th', { textContent: label }), el('td', {}, value));
}

export function showBanScreen(me, accounts) {
  const name = fullName(me);
  const ban = me.ban;
  const title = el('span', { textContent: tr('Страница заблокирована') });
  const look = el('button', { className: 'button', type: 'button', textContent: tr('Проверить статус') });
  const said = el('span', { className: 'ban-said', ariaLive: 'polite' });
  const box = el('div', { className: 'game-window ban-window', role: 'alertdialog', ariaModal: 'true' });
  let done = false;

  const free = () => {
    done = true;
    title.textContent = tr('Блокировка снята');
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

  const clock = showClock(ban, check);
  const gate = el('div', { className: 'ban-gate' },
    el('p', {}, el('b', { textContent: name }), tr(', ваша страница заблокирована администрацией Freenet за нарушение правил сайта.')),
    el('table', { className: 'ban-facts' },
      row(tr('Причина:'), el('div', { className: 'ban-reason', textContent: ban.reason })),
      row(tr('Дата блокировки:'), formatDateTime(ban.bannedAt)),
      row(tr('Срок:'), ban.until ? tr('до {0}', formatDateTime(ban.until)) : tr('бессрочно')),
      clock && row(tr('Осталось:'), clock)),
    el('p', { className: 'hint', textContent: tr('Пока действует блокировка, страница скрыта от других, а записи, сообщения и комментарии недоступны.') }));

  look.addEventListener('click', async () => {
    look.disabled = true;
    said.textContent = '';
    await check();
    look.disabled = false;
    if (!done) said.textContent = tr('Блокировка пока действует.');
  });

  box.append(
    el('div', { className: 'game-window-title' }, gameIcon('/static/icons/game/ban.svg', 16), title),
    el('div', { className: 'ban-body' },
      el('img', { className: 'ban-photo', src: avatarURL(me, 'large'), alt: '' }),
      gate),
    el('div', { className: 'ban-actions' }, look, said,
      accounts,
      el('a', { href: '/', textContent: tr('Выйти'), onclick: logout })));
  document.body.append(el('div', { className: 'ban-screen' }, box));
  document.documentElement.classList.add('ban-open');
  setInterval(check, checkEvery);
}
