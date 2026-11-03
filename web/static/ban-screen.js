// The blocked page, over the whole screen, as the stern notices of the sites
// of the 2000s: a red stop sign, a warning tape, the photo stamped
// "ЗАБЛОКИРОВАН", the number of the case, the reason, a red board that
// counts the term down and what the block takes away. It looks every
// little while whether the block is lifted and lets the person out at once,
// without a new sign-in.

import { currentUser } from './api.js';
import { formatDateTime } from './labels.js';
import { avatarURL, el, fullName, logout } from './layout.js';
import { floatText, gameIcon } from './game.js';
import { denied, fanfare } from './sounds.js';
import { tr } from './i18n.js';

// How often the block is looked at, and how long "Блокировка снята" stays.
const checkEvery = 15_000;
const freeTime = 1_500;

// clockParts splits what is left into days, hours, minutes and seconds.
export function clockParts(ms) {
  const seconds = Math.max(0, Math.floor(ms / 1000));

  return [Math.floor(seconds / 86_400), Math.floor(seconds / 3600) % 24, Math.floor(seconds / 60) % 60, seconds % 60];
}

// caseNumber is the number of the case of a block: the start of the ID of
// the page and the time of the block, the same each time it is shown.
export function caseNumber(userID, bannedAt) {
  const page = userID.replaceAll('-', '').slice(-8).toUpperCase();
  const at = Math.floor(new Date(bannedAt).getTime() / 1000).toString(36).toUpperCase().slice(-5);

  return `${page.slice(0, 4)}-${page.slice(4)}/${at}`;
}

function clockCell(name) {
  const digit = el('b', { className: 'ban-digit', textContent: '00' });

  return [digit, el('div', { className: 'ban-clock-cell' }, digit, el('span', { className: 'ban-clock-name', textContent: name }))];
}

// showClock runs the term down; onEnd is told when it is out.
function showClock(ban, onEnd) {
  if (!ban.until) return el('div', { className: 'ban-forever', textContent: tr('БЕССРОЧНО') });

  const cells = [tr('дней'), tr('часов'), tr('минут'), tr('секунд')].map(clockCell);
  const fill = el('span', { className: 'ban-bar-fill' });
  const served = el('span', { className: 'ban-served' });
  const from = new Date(ban.bannedAt).getTime();
  const to = new Date(ban.until).getTime();
  let timer = 0;

  const tick = () => {
    const now = Date.now();
    const share = Math.min(1, Math.max(0, (now - from) / (to - from)));

    clockParts(to - now).forEach((part, i) => { cells[i][0].textContent = String(part).padStart(2, '0'); });
    fill.style.transform = `scaleX(${share})`;
    served.textContent = tr('Срок отбыт: {0} %', Math.floor(share * 100));
    if (now >= to) {
      clearInterval(timer);
      onEnd();
    }
  };

  timer = setInterval(tick, 1000);
  tick();

  return el('div', {},
    el('div', { className: 'ban-label', textContent: tr('До окончания блокировки:') }),
    el('div', { className: 'ban-clock' }, ...cells.map(([, cell]) => cell)),
    el('span', { className: 'ban-bar' }, fill),
    served);
}

// row is a line of the notice: what, and its value.
function row(label, value) {
  return el('tr', {}, el('th', { textContent: label }), el('td', {}, value));
}

export function showBanScreen(me, accounts) {
  const name = fullName(me);
  const ban = me.ban;
  const title = el('span', { className: 'ban-title-text', textContent: tr('Страница заблокирована') });
  const photo = el('div', { className: 'ban-photo' },
    el('img', { src: avatarURL(me, 'large'), alt: '' }),
    el('span', { className: 'ban-stamp', textContent: tr('Заблокирован') }));
  const look = el('button', { className: 'button ban-check', type: 'button', textContent: tr('Проверить статус') });
  const box = el('div', { className: 'ban-window', role: 'alertdialog', ariaModal: 'true' });
  let done = false;

  const free = () => {
    done = true;
    box.classList.add('ban-free');
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

  const gate = el('div', { className: 'ban-gate' },
    el('div', { className: 'ban-headline', textContent: tr('Доступ к странице закрыт') }),
    el('p', {}, el('b', { textContent: name }), tr(', ваша страница заблокирована администрацией Freenet за нарушение правил сайта.')),
    el('table', { className: 'ban-facts' },
      row(tr('Основание:'), el('div', { className: 'ban-reason', textContent: ban.reason })),
      row(tr('Дата блокировки:'), formatDateTime(ban.bannedAt)),
      row(tr('Срок:'), ban.until ? tr('до {0}', formatDateTime(ban.until)) : tr('бессрочно'))),
    showClock(ban, check),
    el('div', { className: 'ban-rules' },
      el('b', { textContent: tr('Пока действует блокировка:') }),
      el('ul', {},
        el('li', { textContent: tr('ваша страница скрыта от других пользователей;') }),
        el('li', { textContent: tr('записи, сообщения, комментарии и лайки недоступны;') }),
        el('li', { textContent: tr('снять блокировку может только администрация сайта.') }))),
    el('div', { className: 'ban-actions' }, look,
      accounts,
      el('a', { href: '/', textContent: tr('Выйти'), onclick: logout })));

  look.addEventListener('click', async () => {
    look.disabled = true;
    await check();
    look.disabled = false;
    if (!done) {
      denied();
      floatText(look, tr('Блокировка действует'), 'bad');
    }
  });

  box.append(
    el('div', { className: 'game-window-title ban-title' }, gameIcon('/static/icons/game/ban.svg', 20), title,
      el('b', { className: 'ban-siren', ariaHidden: 'true' })),
    el('div', { className: 'ban-tape', ariaHidden: 'true' }),
    el('div', { className: 'ban-body' },
      el('div', { className: 'ban-side' }, photo,
        el('div', { className: 'ban-case' }, tr('Дело № {0}', caseNumber(me.id, ban.bannedAt)))),
      gate));
  document.body.append(el('div', { className: 'ban-screen' }, box));
  document.documentElement.classList.add('ban-open');
  setInterval(check, checkEvery);
}
