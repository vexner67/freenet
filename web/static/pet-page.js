// "Питомец" (/pet), a tamagotchi: a person adopts one pet in the shop for
// votes, feeds it (for votes) and plays with it (free, once a while), or
// it gets hungry and sad by itself. Care and fights give it experience,
// and its level makes it stronger. The pet fights the pets of others —
// one found by the rating or one called by a nickname (?fight=<nickname>
// from a profile): the server decides the fight, the page plays it hit by
// hit. A win brings votes and rating. The administrators set the prices
// in the admin panel (/admin/pets).

import { api, currentUser, session, started } from './api.js';
import {
  formatDateTime, formatPostDate, groupedVotes, petFightResult, petHitText, petKindName, petKindText, petKinds,
  petLevelProgress, petMood, petWait, plural, votesAmount, votesFontSize,
} from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { petImage } from './pet.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.pet()) : null;
const fought = session.get() ? started(api.petFights()).catch(() => ({ fights: [] })) : null;
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

// A hit of a fight lasts so long on the page.
const hitPause = 650;

let me = null;
let shop = null;
let pet = null;
let chosen = '';
let busy = false;
let waitTimer = 0;

function showBalance(votes) {
  $('votes').textContent = groupedVotes(votes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('balance').hidden = false;
}

function setBusy(on) {
  busy = on;
  for (const id of ['feed', 'find', 'call', 'adopt-button']) $(id).disabled = on;
  if (pet) showActions();
}

// act runs an action of the page: errors go above it, the balance follows.
async function act(run) {
  if (busy) return null;
  hideError($('error'));
  setBusy(true);
  try {
    const answer = await run();
    if (answer?.balance !== undefined) showBalance(answer.balance);
    return answer;
  } catch (e) {
    showError($('error'), e);
    return null;
  } finally {
    setBusy(false);
  }
}

// The shop.

function kindCard(kind) {
  const price = shop.prices[kind] ?? 0;
  const card = el('div', { className: 'pet-kind' },
    el('div', { className: 'pet-kind-picture' }, petImage(kind, 64)),
    el('div', { className: 'pet-kind-name', textContent: petKindName(kind) }),
    el('div', { className: 'pet-kind-about', textContent: petKindText(kind) }),
    el('div', { className: 'pet-kind-price', textContent: votesAmount(price) }),
    button(tr('Выбрать'), () => choose(kind)));
  card.dataset.kind = kind;

  return card;
}

function choose(kind) {
  chosen = kind;
  for (const card of $('kinds').children) card.classList.toggle('pet-kind-chosen', card.dataset.kind === kind);
  $('adopt').hidden = false;
  $('adopt-button').textContent = tr('Завести за {0}', votesAmount(shop.prices[kind] ?? 0));
  if (!$('adopt-name').value) $('adopt-name').value = petKindName(kind);
  $('adopt-name').focus();
}

function showShop() {
  $('kinds').replaceChildren(...petKinds.map(kindCard));
  $('home').hidden = true;
  $('shop').hidden = false;
}

// The home of the pet.

function bar(id, value) {
  $(`${id}-bar`).style.width = `${Math.max(0, Math.min(100, value))}%`;
}

function showActions() {
  const full = pet.satiety >= 100;
  $('feed').textContent = shop.meal > 0 ? tr('Покормить за {0}', votesAmount(shop.meal)) : tr('Покормить');
  $('feed').disabled = busy || full;
  $('feed').title = full ? tr('Питомец не голоден') : '';

  const wait = pet.playAt ? (Date.parse(pet.playAt) - Date.now()) / 1000 : 0;
  $('play').disabled = busy || wait > 0;
  $('play-wait').textContent = wait > 0 ? tr('поиграть можно через {0}', petWait(wait)) : '';

  clearTimeout(waitTimer);
  if (wait > 0) waitTimer = setTimeout(showActions, 1000);
}

function statRow(label, value) {
  return el('tr', {}, el('td', { className: 'label', textContent: label }), el('td', { textContent: value }));
}

function showStats() {
  $('stats').replaceChildren(
    statRow(tr('Атака:'), String(pet.attack)),
    statRow(tr('Защита:'), String(pet.defense)),
    statRow(tr('Здоровье:'), String(pet.health)),
    statRow(tr('Скорость:'), String(pet.speed)),
    statRow(tr('Сильный удар:'), `${pet.crit}%`),
    statRow(tr('Рейтинг:'), groupedVotes(pet.rating)),
    statRow(tr('Бои:'), tr('побед: {0}, поражений: {1}', pet.wins, pet.losses)),
    statRow(tr('Живёт у вас:'), tr('с {0}', formatDateTime(pet.bornAt))),
  );
}

function stageClass() {
  const low = Math.min(pet.satiety, pet.joy);
  if (low >= 70) return 'pet-stage pet-happy';
  if (low < 15) return 'pet-stage pet-sad';
  return 'pet-stage';
}

function showPet(next) {
  pet = next;
  $('stage').className = stageClass();
  $('stage').replaceChildren(petImage(pet.kind, 96));
  $('name').textContent = pet.name;
  $('kind').textContent = petKindName(pet.kind);
  $('mood').textContent = petMood(pet);

  bar('satiety', pet.satiety);
  $('satiety').textContent = `${pet.satiety}%`;
  bar('joy', pet.joy);
  $('joy').textContent = `${pet.joy}%`;
  $('level').textContent = String(pet.level);
  bar('experience', petLevelProgress(pet));
  $('experience').textContent = `${groupedVotes(pet.experience)} / ${groupedVotes(pet.nextLevelExperience)}`;

  const left = Math.max(0, shop.fights - pet.fights);
  $('fights-left').textContent = tr('Боёв сегодня: {0} из {1}. Победа приносит {2} и рейтинг; для боя питомцу нужны сытость и настроение.',
    pet.fights, shop.fights, votesAmount(shop.win));
  $('find').disabled = busy || left === 0;
  $('call').disabled = busy || left === 0;

  showActions();
  showStats();
  $('shop').hidden = true;
  $('home').hidden = false;
}

// Fights.

function fighterName(fighter) {
  return fighter.user ? `${fighter.name} (${fighter.user.name} ${fighter.user.surname})` : fighter.name;
}

function fightRow(fight) {
  const mine = fight.attacker.user?.id === me.id;
  const result = petFightResult(fight, mine);
  const rival = mine ? fight.defender : fight.attacker;

  return el('div', { className: 'pet-fight' },
    petImage(rival.kind, 32),
    el('div', { className: 'pet-fight-text' },
      el('div', {},
        el('span', { textContent: mine ? tr('Вы вызвали ') : tr('Вас вызвал ') }),
        rival.user
          ? el('a', { href: '/' + rival.user.nickname, textContent: fighterName(rival) })
          : el('span', { textContent: fighterName(rival) })),
      el('div', { className: 'hint', textContent: formatPostDate(fight.foughtAt) })),
    el('div', { className: result.won ? 'pet-result pet-won' : 'pet-result pet-lost' },
      el('b', { textContent: result.text }),
      el('div', { className: 'hint', textContent: tr('рейтинг {0}', result.rating) })));
}

function showFights(fights) {
  $('fights').replaceChildren(...fights.map(fightRow));
  $('no-fights').hidden = fights.length > 0;
}

function fighterBox(fighter) {
  const health = el('div', { className: 'pet-bar-fill pet-health' });
  const value = el('span', { className: 'pet-arena-hp', textContent: `${fighter.health} / ${fighter.health}` });
  const box = el('div', { className: 'pet-arena-side' },
    el('div', { className: 'pet-arena-picture' }, petImage(fighter.kind, 64)),
    el('b', { textContent: fighter.name }),
    el('div', { className: 'hint', textContent: tr('{0} ур.', fighter.level) + (fighter.user ? ` · ${fighter.user.name} ${fighter.user.surname}` : '') }),
    el('div', { className: 'pet-bar pet-arena-bar' }, health),
    value);

  return {
    box,
    hit(left) {
      health.style.width = `${(100 * left) / fighter.health}%`;
      value.textContent = `${left} / ${fighter.health}`;
      box.classList.remove('pet-hurt');
      if (!still) {
        void box.offsetWidth;
        box.classList.add('pet-hurt');
      }
    },
  };
}

const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function playFight(fight) {
  const attacker = fighterBox(fight.attacker);
  const defender = fighterBox(fight.defender);
  const log = el('div', { className: 'pet-log' });
  const end = el('div', { className: 'pet-arena-result', hidden: true });

  $('arena').replaceChildren(
    el('div', { className: 'pet-arena-ring' }, attacker.box, el('div', { className: 'pet-arena-vs', textContent: 'VS' }), defender.box),
    log, end);
  $('arena').hidden = false;

  for (const hit of fight.hits) {
    if (!still) await pause(hitPause);
    (hit.byAttacker ? defender : attacker).hit(hit.byAttacker ? hit.defenderHealth : hit.attackerHealth);
    log.prepend(el('div', { className: hit.crit ? 'pet-log-line pet-log-crit' : 'pet-log-line', textContent: petHitText(hit, fight.attacker, fight.defender) }));
  }

  const result = petFightResult(fight, true);
  end.className = result.won ? 'pet-arena-result pet-won' : 'pet-arena-result pet-lost';
  end.textContent = result.won
    ? tr('Победа! {0} и рейтинг {1}.', votesAmount(shop.win), result.rating)
    : tr('Поражение. Рейтинг {0}.', result.rating);
  end.hidden = false;
}

async function fight(nickname) {
  // The balance waits for the end of the fight: it would tell who won.
  const answer = (await act(() => api.fightPet(nickname).then((fought) => ({ fought }))))?.fought;
  if (!answer) return;

  setBusy(true);
  try {
    await playFight(answer.fight);
  } finally {
    setBusy(false);
  }
  showPet(answer.pet);
  showBalance(answer.balance);
  api.petFights().then(({ fights }) => showFights(fights)).catch(() => {});
}

// Rename and release.

function openRename() {
  const modal = openModal(tr('Переименовать питомца'), { tag: 'form' });
  const input = el('input', { type: 'text', maxLength: 20, value: pet.name, required: true });
  modal.body.append(el('p', {}, el('label', { textContent: tr('Новое имя:') })), input);
  modal.actions.append(button(tr('Сохранить'), null, { type: 'submit' }), button(tr('Отмена'), modal.close, { secondary: true }));
  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    modal.close();
    const renamed = await act(() => api.renamePet(input.value));
    if (renamed) showPet(renamed.pet);
  });
  input.select();
}

function openRelease() {
  const modal = openModal(tr('Отпустить питомца'));
  modal.body.append(el('p', { textContent: tr('Отпустить {0}? Питомец уйдёт навсегда, с уровнем и рейтингом, а голоса за него не вернутся.', pet.name) }));
  modal.actions.append(
    button(tr('Отпустить'), async () => {
      modal.close();
      if (await act(() => api.releasePet().then(() => ({})))) {
        pet = null;
        chosen = '';
        $('adopt').hidden = true;
        $('adopt-name').value = '';
        showShop();
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }));
}

function watch() {
  $('adopt').addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!chosen) return;
    const adopted = await act(() => api.adoptPet(chosen, $('adopt-name').value));
    if (adopted) {
      showPet(adopted.pet);
      showFights([]);
    }
  });
  $('feed').addEventListener('click', async () => {
    const fed = await act(() => api.feedPet());
    if (fed) showPet(fed.pet);
  });
  $('play').addEventListener('click', async () => {
    const played = await act(() => api.playPet());
    if (played) showPet(played.pet);
  });
  $('find').addEventListener('click', () => fight(''));
  $('challenge').addEventListener('submit', (event) => {
    event.preventDefault();
    const nickname = $('rival').value.trim().replace(/^@/, '');
    if (nickname) fight(nickname);
  });
  $('rename').addEventListener('click', openRename);
  $('release').addEventListener('click', openRelease);
}

me = await currentUser().catch(() => null);
renderChrome(me);
$('pet-page').hidden = false;

if (!me) {
  $('guest').hidden = false;
} else {
  try {
    const answer = await answered;
    shop = answer.shop;
    showBalance(me.votes ?? 0);
    watch();
    if (answer.pet) {
      showPet(answer.pet);
      showFights((await fought).fights);
    } else {
      showShop();
    }
    const rival = new URLSearchParams(location.search).get('fight');
    if (rival) $('rival').value = rival;
  } catch (e) {
    showError($('error'), e);
  }
}
