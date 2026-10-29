// "Знакомства" (/dating): a person fills in a card — what they look for,
// whom and a few words about themselves — and is shown the cards of those
// who would like to meet them, one at a time: "Нравится" or "Пропустить".
// A like answered with a like is a match: both are told and may write.
// Tabs: the cards (/dating), the matches (?tab=matches) and one's own card
// (?tab=card), which a newcomer starts with.

import { api, currentUser, started } from './api.js';
import { datingGoalName, datingGoals, datingSeekName, datingSeeks, formatPostDate } from './labels.js';
import { $, avatarURL, el, fullName, hideError, profilePath, renderChrome, replaceInPlace, showError, verifiedMark } from './layout.js';
import { button, notify } from './modal.js';
import { tr } from './i18n.js';

const answered = started(api.dating());
const tabs = ['deck', 'matches', 'card'];
const asked = new URLSearchParams(location.search).get('tab');

let state = null;
let tab = tabs.includes(asked) ? asked : 'deck';

function showTabs() {
  const names = {
    deck: tr('Анкеты'),
    matches: state.matches.length ? tr('Симпатии ({0})', state.matches.length) : tr('Симпатии'),
    card: tr('Моя анкета'),
  };
  $('tabs').replaceChildren(...tabs.map((t) => el('a', {
    className: t === tab ? 'active' : '',
    href: t === 'deck' ? '/dating' : `/dating?tab=${t}`,
    textContent: names[t],
  })));
}

function person(user, ...more) {
  return el('div', { className: 'dating-person' },
    el('a', { href: profilePath(user) }, el('img', { className: 'dating-avatar', src: avatarURL(user, 'small'), alt: '' })),
    el('div', {},
      el('a', { className: 'post-name', href: profilePath(user), textContent: fullName(user) }),
      verifiedMark(user),
      ...more));
}

function writeLink(user) {
  return el('a', { href: '/im/' + encodeURIComponent(user.nickname), textContent: tr('Написать сообщение') });
}

// deckNodes is the first card of the deck, with what to answer it; the
// others wait under it.
function deckNodes() {
  if (!state.card) {
    return [el('p', { className: 'hint' }, tr('Сначала заполните '), el('a', { href: '/dating?tab=card', textContent: tr('свою анкету') }), '.')];
  }
  if (!state.card.on) {
    return [el('p', { className: 'hint' }, tr('Ваша анкета выключена: вы не видите чужих, а они — вашу. '),
      el('a', { href: '/dating?tab=card', textContent: tr('Включить') }))];
  }

  const card = state.deck[0];
  if (!card) {
    return [el('p', { className: 'hint', textContent: tr('Анкеты закончились. Загляните позже — появятся новые.') })];
  }

  const like = button(tr('Нравится'), () => answer(card, true, [like, skip]));
  const skip = button(tr('Пропустить'), () => answer(card, false, [like, skip]), { secondary: true });

  return [el('div', { className: 'dating-card' },
    el('a', { href: profilePath(card.user) },
      el('img', { className: 'dating-photo', src: avatarURL(card.user, 'large'), alt: '' })),
    el('div', { className: 'dating-about' },
      el('a', { className: 'dating-name', href: profilePath(card.user), textContent: fullName(card.user) }),
      verifiedMark(card.user),
      el('div', { className: 'dating-goal', textContent: tr('Ищет: {0}', datingGoalName(card.goal)) }),
      card.about && el('p', { className: 'dating-text', textContent: card.about }),
      el('div', { className: 'dating-actions' }, like, skip)))];
}

async function answer(card, like, buttons) {
  for (const b of buttons) b.disabled = true;
  try {
    const { matched } = await api.answerDating(card.user.id, like);
    hideError($('error'));
    state.deck = state.deck.filter((c) => c !== card);
    if (matched) {
      state.matches = [{ user: card.user, matchedAt: new Date().toISOString() }, ...state.matches];
      showTabs();
    }
    draw(matched && el('div', { className: 'notice' },
      tr('Взаимная симпатия с {0}! ', fullName(card.user)), writeLink(card.user)));
    // The cards come again when these are answered: there may be new ones.
    if (!state.deck.length && !matched) reload();
  } catch (e) {
    for (const b of buttons) b.disabled = false;
    showError($('error'), e);
  }
}

function matchesNodes() {
  if (!state.matches.length) {
    return [el('p', { className: 'hint', textContent: tr('Взаимных симпатий пока нет. Когда тот, кто вам понравился, ответит тем же, он появится здесь.') })];
  }

  return state.matches.map((m) => person(m.user,
    el('div', { className: 'dating-goal', textContent: tr('Взаимная симпатия: {0}', formatPostDate(m.matchedAt)) }),
    writeLink(m.user)));
}

function options(values, name, chosen) {
  return values.map((v) => el('option', { value: v, textContent: name(v), selected: v === chosen }));
}

function cardNodes() {
  const card = state.card ?? { goal: 'chat', seek: '', about: '', on: true };
  const goal = el('select', { id: 'dating-goal' }, ...options(datingGoals, datingGoalName, card.goal));
  const seek = el('select', { id: 'dating-seek' }, ...options(datingSeeks, datingSeekName, card.seek));
  const about = el('textarea', { id: 'dating-about', maxLength: 300, value: card.about, placeholder: tr('Пара слов о себе: увлечения, музыка, город…') });
  const on = el('input', { type: 'checkbox', id: 'dating-on', checked: card.on });
  const note = el('span', { className: 'dating-saved' });

  const save = button(tr('Сохранить'), async () => {
    save.disabled = true;
    try {
      const saved = await api.saveDating({ goal: goal.value, seek: seek.value, about: about.value, on: on.checked });
      hideError($('error'));
      const first = !state.card;
      state.card = saved;
      note.textContent = tr('Сохранено.');
      notify(note);
      if (first || saved.on) reload();
    } catch (e) {
      showError($('error'), e);
    } finally {
      save.disabled = false;
    }
  });

  const row = (label, control) => el('tr', {}, el('th', {}, el('label', { htmlFor: control.id, textContent: label })), el('td', { className: 'control' }, control));

  return [el('div', { className: 'container-gray' },
    !state.card && el('div', { className: 'notice', textContent: tr('Расскажите, кого вы ищете, — и вам покажут анкеты тех, кто ищет таких, как вы. Анкета показывает только имя и фото со страницы.') }),
    el('h4', { textContent: tr('Моя анкета') }),
    el('table', { className: 'form' },
      row(tr('Цель:'), goal),
      row(tr('Ищу:'), seek),
      row(tr('О себе:'), about),
      el('tr', {}, el('th'), el('td', { className: 'control' }, el('label', {}, on, tr(' Показывать мою анкету')))),
      el('tr', {}, el('th'), el('td', { className: 'control' }, save, ' ', note))))];
}

function draw(lead = null) {
  const nodes = { deck: deckNodes, matches: matchesNodes, card: cardNodes }[tab]();
  replaceInPlace($('dating'), ...[lead, ...nodes].filter(Boolean));
}

async function reload() {
  try {
    state = await api.dating();
    showTabs();
    if (tab !== 'card') draw();
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser();
renderChrome(me);

try {
  state = await answered;
  if (!state.card && !asked) tab = 'card';
  showTabs();
  draw();
  $('dating').hidden = false;
} catch (e) {
  showError($('error'), e);
}
