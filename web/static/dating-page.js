// "Знакомства" (/dating): a person fills in a card — what they look for,
// whom and a few words about themselves — and is shown the cards of those
// who would like to meet them, one at a time, in a stack: the buttons, the
// arrows of the keyboard or a card thrown aside answer it. The card flies
// off at once and the server is asked behind it; a like answered with a like
// is a match: both are told, a window with fanfares opens and they may write.
// Tabs: the cards (/dating), the matches (?tab=matches) and one's own card
// (?tab=card), which a newcomer starts with.

import { api, currentUser, started } from './api.js';
import { datingGoalName, datingGoals, datingSeekName, datingSeeks, formatPostDate } from './labels.js';
import { $, avatarURL, el, fullName, hideError, profilePath, renderChrome, replaceInPlace, showError, verifiedMark } from './layout.js';
import { button, notify, openModal } from './modal.js';
import { flash, floatText, gameIcon } from './game.js';
import { meter, segmented } from './controls.js';
import { dealTick, fanfare } from './sounds.js';
import { tr } from './i18n.js';

const answered = started(api.dating());
const tabs = ['deck', 'matches', 'card'];
const asked = new URLSearchParams(location.search).get('tab');

let state = null;
let tab = tabs.includes(asked) ? asked : 'deck';
let me = null;
// The cards answered and not heard back from: a new deck must not bring
// them again.
const unanswered = new Set();
// How many cards this person has liked in a row of the deck.
let streak = 0;
let likedNow = 0;
// How far a card goes before it counts as thrown.
const throwAt = 90;
// How long an answered card takes to fly away (dating-fly in the styles).
const flyTime = 340;

function showTabs() {
  const names = {
    deck: tr('Анкеты'),
    matches: state.matches.length ? tr('Симпатии ({0})', state.matches.length) : tr('Симпатии'),
    card: tr('Моя анкета'),
  };
  const pictures = { deck: '/static/icons/game/deck.svg', matches: '/static/icons/game/match.svg', card: '/static/icons/game/profile-card.svg' };

  $('tabs').replaceChildren(...tabs.map((t) => el('a', {
    className: t === tab ? 'active' : '',
    href: t === 'deck' ? '/dating' : `/dating?tab=${t}`,
    onclick: (event) => {
      if (event.button || event.metaKey || event.ctrlKey || event.shiftKey) return;
      event.preventDefault();
      tab = t;
      history.replaceState(null, '', event.currentTarget.getAttribute('href'));
      showTabs();
      draw();
    },
  }, gameIcon(pictures[t]), names[t])));
}

function hudCell(id, icon, name, hint) {
  return el('div', { className: 'game-hud-cell', id: `hud-${id}` },
    el('span', { className: 'game-hud-name' }, gameIcon(icon), name),
    el('b', { className: 'game-hud-sum', id: `${id}-sum` }),
    el('span', { className: 'hint', textContent: hint }));
}

function showHUD() {
  if (!$('hud').childElementCount) {
    $('hud').replaceChildren(
      hudCell('deck', '/static/icons/game/deck.svg', tr('Анкет в колоде'), tr('листайте дальше')),
      hudCell('liked', '/static/icons/game/heart.svg', tr('Нравится'), tr('за этот заход')),
      hudCell('matches', '/static/icons/game/match.svg', tr('Симпатий'), tr('взаимных')),
    );
  }
  $('deck-sum').textContent = String(state.deck.length);
  $('liked-sum').textContent = String(likedNow);
  $('matches-sum').textContent = String(state.matches.length);
}

function cardNode(card, className = '') {
  return el('div', { className: `dating-card ${className}`.trim() },
    el('a', { href: profilePath(card.user), draggable: false },
      el('img', { className: 'dating-photo', src: avatarURL(card.user, 'large'), alt: '', draggable: false })),
    el('div', { className: 'dating-about' },
      el('a', { className: 'dating-name', href: profilePath(card.user), textContent: fullName(card.user) }),
      verifiedMark(card.user),
      el('div', { className: 'dating-goal', textContent: tr('Ищет: {0}', datingGoalName(card.goal)) }),
      card.about && el('p', { className: 'dating-text', textContent: card.about })),
    el('span', { className: 'dating-stamp like', textContent: tr('Нравится') }),
    el('span', { className: 'dating-stamp skip', textContent: tr('Мимо') }));
}

// stub is what stands in the place of the stack when there is no card.
function stub(picture, ...nodes) {
  return [el('div', { className: 'dating-stack' },
    el('div', { className: 'dating-card dating-stub' }, gameIcon(picture, 48), el('p', { className: 'hint' }, ...nodes)))];
}

// roundButton is one of the two big buttons of the stack, always in one
// place.
function roundButton(kind, picture, text, like) {
  return el('button', { type: 'button', className: `dating-round ${kind}`, title: text, onclick: (event) => answer(like, event.currentTarget) },
    gameIcon(picture, 32), el('span', { textContent: text }));
}

// thrown lets the first card be dragged aside: past throwAt it answers,
// short of that it comes back.
function thrown(node) {
  let from = null;
  let moved = 0;

  node.addEventListener('pointerdown', (event) => {
    if (event.button) return;
    from = event.clientX;
    moved = 0;
    node.setPointerCapture(event.pointerId);
    node.classList.add('held');
  });
  node.addEventListener('pointermove', (event) => {
    if (from === null) return;
    moved = event.clientX - from;
    node.style.transform = `translateX(${moved}px) rotate(${moved / 14}deg)`;
    node.style.setProperty('--like', String(Math.max(0, Math.min(1, moved / throwAt))));
    node.style.setProperty('--skip', String(Math.max(0, Math.min(1, -moved / throwAt))));
  });
  const letGo = () => {
    if (from === null) return;
    from = null;
    node.classList.remove('held');
    if (Math.abs(moved) >= throwAt) {
      answer(moved > 0);
      return;
    }
    node.style.transform = '';
    node.style.setProperty('--like', '0');
    node.style.setProperty('--skip', '0');
  };
  node.addEventListener('pointerup', letGo);
  node.addEventListener('pointercancel', letGo);
  // A drag is no click on the photo.
  node.addEventListener('click', (event) => {
    if (Math.abs(moved) > 5) event.preventDefault();
  }, true);
}

// deckNodes is the stack of the cards with what to answer the first; the
// next ones stand under it and their photos are loaded ahead.
function deckNodes() {
  if (!state.card) {
    return stub('/static/icons/game/profile-card.svg', tr('Сначала заполните '), el('a', { href: '/dating?tab=card', textContent: tr('свою анкету') }), '.');
  }
  if (!state.card.on) {
    return stub('/static/icons/game/lock.svg', tr('Ваша анкета выключена: вы не видите чужих, а они — вашу. '),
      el('a', { href: '/dating?tab=card', textContent: tr('Включить') }));
  }

  const [card, next, after] = state.deck;
  if (!card) return stub('/static/icons/game/deck.svg', tr('Анкеты закончились. Загляните позже — появятся новые.'));

  if (after) new Image().src = avatarURL(after.user, 'large');

  const first = cardNode(card, 'dating-first');

  thrown(first);

  return [
    el('div', { className: 'dating-stack', id: 'stack' }, next && cardNode(next, 'dating-next'), first),
    el('div', { className: 'dating-actions' },
      roundButton('skip', '/static/icons/game/skip.svg', tr('Пропустить'), false),
      roundButton('like', '/static/icons/game/heart.svg', tr('Нравится'), true)),
    el('p', { className: 'dating-keys hint', textContent: tr('Клавиши ← и → или смахните анкету') }),
  ];
}

// openMatch celebrates a match: both photos and a way to write.
function openMatch(user) {
  const modal = openModal(tr('Взаимная симпатия!'));

  fanfare();
  modal.body.append(el('div', { className: 'dating-match-show' },
    el('img', { src: avatarURL(me, 'small'), alt: '' }),
    gameIcon('/static/icons/game/match.svg', 48),
    el('img', { src: avatarURL(user, 'small'), alt: '' })),
  el('p', { className: 'dating-match-text', textContent: tr('Вы понравились друг другу с {0}.', fullName(user)) }));
  modal.actions.append(
    el('a', { className: 'button', href: '/im/' + encodeURIComponent(user.nickname), textContent: tr('Написать сообщение') }),
    button(tr('Смотреть дальше'), modal.close, { secondary: true }),
  );
}

// answer takes the first card off at once: a copy of it flies away and the
// next one is there to answer; the server hears behind it. A refusal puts
// the card back.
async function answer(like, pressed = null) {
  const card = state.deck[0];
  const node = document.querySelector('.dating-first');
  if (!card || !node) return;

  const rest = node.style.transform;
  node.style.setProperty('--from', rest || 'none');
  node.classList.remove('dating-first');
  node.classList.add(like ? 'fly-like' : 'fly-skip');
  node.style.transform = '';
  setTimeout(() => node.remove(), flyTime);

  state.deck = state.deck.slice(1);
  unanswered.add(card.user.id);
  if (like) {
    likedNow += 1;
    streak += 1;
    if (pressed) floatText(pressed, '♥', 'bad');
    if (streak % 5 === 0) floatText($('hud-liked'), tr('Серия ×{0}!', streak), 'gold');
  } else {
    streak = 0;
  }
  dealTick(like);
  redeck(node);

  try {
    const { matched } = await api.answerDating(card.user.id, like);
    hideError($('error'));
    if (matched) {
      state.matches = [{ user: card.user, matchedAt: new Date().toISOString() }, ...state.matches];
      showTabs();
      showHUD();
      flash($('hud-matches'), 'gold');
      openMatch(card.user);
    }
    // The cards come again when these are answered: there may be new ones.
    if (!state.deck.length) reload();
  } catch (e) {
    state.deck = [card, ...state.deck];
    if (like) likedNow -= 1;
    showHUD();
    if (tab === 'deck') draw();
    showError($('error'), e);
  } finally {
    unanswered.delete(card.user.id);
  }
}

// redeck draws the deck leaving the card that flies away over it.
function redeck(flying) {
  showHUD();
  if (tab !== 'deck') return;

  const nodes = deckNodes();
  const stack = nodes[0];

  replaceInPlace($('dating'), ...nodes);
  stack.append(flying);
}

function matchesNodes() {
  if (!state.matches.length) {
    return [el('p', { className: 'hint', textContent: tr('Взаимных симпатий пока нет. Когда тот, кто вам понравился, ответит тем же, он появится здесь.') })];
  }

  return [el('div', { className: 'dating-matches' }, ...state.matches.map((m) => el('div', { className: 'dating-match' },
    el('a', { href: profilePath(m.user) }, el('img', { className: 'dating-avatar', src: avatarURL(m.user, 'small'), alt: '' })),
    el('a', { className: 'post-name', href: profilePath(m.user), textContent: fullName(m.user) }),
    verifiedMark(m.user),
    el('div', { className: 'dating-goal', textContent: formatPostDate(m.matchedAt) }),
    el('a', { className: 'button', href: '/im/' + encodeURIComponent(m.user.nickname), textContent: tr('Написать') }))))];
}

function cardNodes() {
  const card = state.card ?? { goal: 'chat', seek: '', about: '', on: true };
  const goal = segmented(datingGoals.map((v) => [v, datingGoalName(v)]), card.goal, () => showFilled());
  const seek = segmented(datingSeeks.map((v) => [v, datingSeekName(v)]), card.seek, () => showFilled());
  const about = el('textarea', { id: 'dating-about', maxLength: 300, value: card.about, placeholder: tr('Пара слов о себе: увлечения, музыка, город…'), oninput: () => showFilled() });
  const left = el('span', { className: 'hint' });
  const filled = el('div', { className: 'dating-filled' });

  // showFilled says how full the card is: a fuller one gets more likes.
  function showFilled() {
    const parts = [Boolean(me.avatar?.small), seek.get() !== '', about.value.trim().length > 0, about.value.trim().length >= 40];
    const done = parts.filter(Boolean).length;

    filled.replaceChildren(meter(done, parts.length, tr('Анкета заполнена на {0}%', Math.round((done / parts.length) * 100))));
    left.textContent = tr('{0} из {1}', about.value.length, about.maxLength);
  }
  showFilled();

  const on = el('input', { type: 'checkbox', id: 'dating-on', checked: card.on });
  const note = el('span', { className: 'dating-saved' });

  const save = button(tr('Сохранить'), async () => {
    save.disabled = true;
    try {
      const saved = await api.saveDating({ goal: goal.get(), seek: seek.get(), about: about.value, on: on.checked });
      hideError($('error'));
      const first = !state.card;
      state.card = saved;
      note.textContent = tr('Сохранено.');
      notify(note);
      dealTick();
      if (first || saved.on) reload();
    } catch (e) {
      showError($('error'), e);
    } finally {
      save.disabled = false;
    }
  });

  const row = (label, ...control) => el('tr', {}, el('th', {}, el('label', { textContent: label })), el('td', { className: 'control' }, ...control));

  return [el('div', { className: 'container-gray' },
    !state.card && el('div', { className: 'notice', textContent: tr('Расскажите, кого вы ищете, — и вам покажут анкеты тех, кто ищет таких, как вы. Анкета показывает только имя и фото со страницы.') }),
    el('h4', { textContent: tr('Моя анкета') }),
    filled,
    el('table', { className: 'form' },
      row(tr('Цель:'), goal.node),
      row(tr('Ищу:'), seek.node),
      row(tr('О себе:'), about, left),
      el('tr', {}, el('th'), el('td', { className: 'control' }, el('label', {}, on, tr(' Показывать мою анкету')))),
      el('tr', {}, el('th'), el('td', { className: 'control' }, save, ' ', note))))];
}

function draw() {
  const nodes = { deck: deckNodes, matches: matchesNodes, card: cardNodes }[tab]();
  replaceInPlace($('dating'), ...nodes.filter(Boolean));
}

async function reload() {
  try {
    const fresh = await api.dating();

    state = { ...fresh, deck: fresh.deck.filter((c) => !unanswered.has(c.user.id)) };
    showTabs();
    showHUD();
    if (tab !== 'card') draw();
  } catch (e) {
    showError($('error'), e);
  }
}

document.addEventListener('keydown', (event) => {
  if (tab !== 'deck' || event.target.closest?.('input, textarea, select') || document.querySelector('.modal')) return;
  if (event.key === 'ArrowRight') answer(true);
  if (event.key === 'ArrowLeft') answer(false);
});

me = await currentUser();
renderChrome(me);

try {
  state = await answered;
  if (!state.card && !asked) tab = 'card';
  showTabs();
  showHUD();
  draw();
  $('dating').hidden = false;
} catch (e) {
  showError($('error'), e);
}
