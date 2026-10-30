// "Питомец" (/pet), a tamagotchi: a person adopts up to ten pets in the
// shop for votes or opens a chest, for votes or shards, from which a pet of
// a rarity drops — to be taken or left for shards (?pet=<id> opens one of
// them), dresses it in clothes that add to its stats, and feeds it (for
// votes) and plays with it (free, once a while), or it gets hungry and sad
// by itself. Care and fights give it experience, and its level makes it
// stronger. The pet fights the pets of others — one of a near power found
// by the rating or one called by a nickname (?fight=<nickname> from a
// profile) — in a stance: the server decides the fight, the page plays it
// hit by hit and tells it as a story; or the owner leads it by hand, a
// move a round. A win brings votes and rating, more over a stronger rival.
// The administrators set the prices in the admin panel (/admin/pets).
// The tab "Арена" (?arena=1, pet-league.js) is what the pets do together.

import { api, currentUser, session, started } from './api.js';
import {
  balanceVotes, balanceWord, formatDateTime, formatPostDate, groupedVotes, petBonus, petCaseName, petCaseText, petElementName, petFightResult,
  petFightStory, petHitLines, petItemName, petKindName, petKindText, petKinds, petLevelProgress, petMood, petMoveName, petMoveText, petMoves,
  petOdds, petRarities, petShards, petRarityName, petSlotName, petSlots, petSpecialName, petSpecialText, petStanceName, petStanceText, petStances,
  petStatName, petStatText, petStats, petWait, votesAmount, votesFontSize,
} from './labels.js';
import { $, el, renderChrome, replaceInPlace, showError, hideError } from './layout.js';
import { button, notify, openModal } from './modal.js';
import { leagueNodes } from './pet-league.js';
import { petCaseImage, petImage, petItemImage } from './pet.js';
import { showNamedPet } from './pet-named.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.pets()) : null;
const fought = session.get() ? started(api.petFights()).catch(() => ({ fights: [] })) : null;
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

// A hit of a fight lasts so long on the page.
const hitPause = 650;

// A person keeps so many pets (user.MaxPets).
const maxPets = 10;

let me = null;
let shop = null;
let catalogue = { kinds: [], cases: [], items: [] };
let pets = [];
let keeper = { shards: 0, pity: 0, pityLimit: 40, cases: 0, casesLimit: 20, dropKind: '', dropCase: '' };
let battles = [];
let pet = null;
let chosen = '';
let busy = false;
let waitTimer = 0;
let league = false;

function showBalance(votes) {
  $('votes').textContent = balanceVotes(votes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = balanceWord(votes);
  $('balance').hidden = false;
}

function setBusy(on) {
  busy = on;
  $('adopt-button').disabled = on;
  for (const b of document.querySelectorAll('.pet-busy')) b.disabled = on;
  if (pet) {
    showActions();
    showFightButtons();
  }
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

// unpaid is what an action that may cost nothing answers: a balance of 0
// is "nothing was paid", not an empty purse.
function unpaid(answer) {
  if (!answer.balance) delete answer.balance;
  return answer;
}

// The shop.

function kindOf(kind) {
  return catalogue.kinds.find((k) => k.kind === kind) ?? { kind, rarity: 'common', inShop: true };
}

// rarityBadge is the rarity of a pet: a coloured plate, as the cards of
// the games of the time had.
function rarityBadge(rarity) {
  return el('span', { className: `pet-rarity pet-rarity-${rarity}`, textContent: petRarityName(rarity) });
}

// specialNode is the element of a kind and its trick in a fight.
function specialNode(kind) {
  const of = kindOf(kind);
  if (!of.special) return null;
  return el('div', { className: 'pet-kind-special' },
    el('b', { textContent: petSpecialName(of.special) }),
    ` (${petElementName(of.element)}): ${petSpecialText(of.special)}`);
}

function kindCard(kind) {
  const price = shop.prices[kind] ?? 0;
  const card = el('div', { className: `pet-kind pet-kind-${kindOf(kind).rarity}` },
    el('div', { className: 'pet-kind-picture' }, petImage(kind, 64)),
    el('div', { className: 'pet-kind-name', textContent: petKindName(kind) }),
    rarityBadge(kindOf(kind).rarity),
    el('div', { className: 'pet-kind-about', textContent: petKindText(kind) }),
    specialNode(kind),
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
  pet = null;
  league = false;
  $('league').hidden = true;
  $('kinds').replaceChildren(...petKinds.filter((kind) => kindOf(kind).inShop).map(kindCard));
  showCases();
  $('home').hidden = true;
  $('shop').hidden = false;
  showTabs();
}

// Chests: a pet of a rarity drops from each, by its odds. The server
// draws it; the page spins a strip of pets that stops on it.

// oddsBar is the odds of a chest as a bar of the colours of rarities.
function oddsBar(odds) {
  const percents = petOdds(odds);
  const bar = el('div', { className: 'pet-odds', title: petRarities.map((r, i) => `${petRarityName(r)}: ${percents[i]}`).join(', ') });
  petRarities.forEach((rarity, i) => {
    if (!odds[i]) return;
    const part = el('div', { className: `pet-odds-part pet-rarity-${rarity}` });
    part.style.width = `${odds[i] / 10}%`;
    bar.append(part);
  });
  const legend = el('div', { className: 'pet-odds-legend' },
    ...petRarities.map((rarity, i) => (odds[i]
      ? el('span', {}, el('i', { className: `pet-rarity-${rarity}` }), `${petRarityName(rarity)} ${percents[i]}`)
      : null)));
  return el('div', {}, bar, legend);
}

// inside are the pets that drop from a chest, small, by rarity.
function inside(odds) {
  const kinds = petKinds.filter((kind) => odds[petRarities.indexOf(kindOf(kind).rarity)] > 0);
  return el('div', { className: 'pet-case-inside' },
    ...kinds.map((kind) => {
      const box = el('span', { className: `pet-case-pet pet-kind-${kindOf(kind).rarity}`, title: `${petKindName(kind)} — ${petRarityName(kindOf(kind).rarity)}` },
        petImage(kind, 32));
      return box;
    }));
}

function caseCard(chest) {
  const price = shop.cases?.[chest.id] ?? 0;
  const closed = Boolean(keeper.dropKind) || keeper.cases >= keeper.casesLimit;
  const open = button(tr('Открыть за {0}', votesAmount(price)), () => openCase(chest, false));
  const forShards = button(tr('За {0}', petShards(chest.shards)), () => openCase(chest, true));
  for (const b of [open, forShards]) b.classList.add('pet-busy');
  open.disabled = busy || closed;
  forShards.disabled = busy || closed || keeper.shards < chest.shards;
  return el('div', { className: `pet-case pet-case-${chest.id}` },
    el('div', { className: 'pet-case-picture' }, petCaseImage(chest.id, 64)),
    el('div', { className: 'pet-case-text' },
      el('div', { className: 'pet-kind-name', textContent: petCaseName(chest.id) }),
      el('div', { className: 'hint', textContent: petCaseText(chest.id) }),
      oddsBar(chest.odds),
      inside(chest.odds)),
    el('div', { className: 'pet-case-buy' }, open, forShards));
}

// showKeeper tells the shards, the chests of today and how far the chest
// that gives a legendary pet for sure is.
function showKeeper() {
  const pity = el('div', { className: 'pet-bar-fill pet-pity' });
  pity.style.width = `${(100 * keeper.pity) / keeper.pityLimit}%`;
  const waiting = keeper.dropKind
    ? el('p', {}, tr('Вас ждёт питомец из сундука: {0}. ', petKindName(keeper.dropKind)),
      el('a', { textContent: tr('Забрать или оставить'), onclick: () => showDrop(false) }))
    : null;
  $('keeper').replaceChildren(
    el('div', { className: 'pet-keeper-row' },
      el('span', { textContent: tr('Осколки: {0}', keeper.shards) }),
      el('span', { textContent: tr('Сундуков сегодня: {0} из {1}', keeper.cases, keeper.casesLimit) })),
    el('div', { className: 'pet-keeper-row' },
      el('span', { textContent: tr('Легендарный наверняка — через {0}', keeper.pityLimit - keeper.pity) }),
      el('div', { className: 'pet-bar', 'aria-hidden': 'true' }, pity)),
    ...(waiting ? [waiting] : []));
}

function showCases() {
  showKeeper();
  $('cases').replaceChildren(...catalogue.cases.map(caseCard));
}

// strip is the strip of the roulette: pets drawn by the odds of the
// chest, with the one that dropped at place.
function strip(chest, kind, place, length) {
  const kinds = [];
  for (let i = 0; i < length; i++) {
    if (i === place) {
      kinds.push(kind);
      continue;
    }
    let roll = Math.random() * 1000;
    let rarity = petRarities.findIndex((_, r) => (roll -= chest.odds[r]) < 0);
    if (rarity < 0) rarity = 0;
    const of = petKinds.filter((k) => kindOf(k).rarity === petRarities[rarity]);
    kinds.push(of[Math.floor(Math.random() * of.length)] ?? kind);
  }
  return kinds;
}

async function openCase(chest, withShards) {
  const opened = await act(() => api.openPetCase(chest.id, withShards));
  if (!opened) return;
  keeper = opened.keeper;
  showCases();
  showDrop(true);
}

// showDrop is the pet a chest gave: the roulette spins to it first when it
// has just dropped. It is taken with a name or left for its shards.
function showDrop(spin) {
  const kind = keeper.dropKind;
  const chest = catalogue.cases.find((c) => c.id === keeper.dropCase) ?? catalogue.cases[0];
  if (!kind || !chest) return;
  const rarity = kindOf(kind).rarity;
  const shards = kindOf(kind).shards ?? 0;

  const modal = openModal(petCaseName(chest.id));
  const cell = 76;
  const place = 30;
  const tape = el('div', { className: 'pet-roulette-tape' },
    ...strip(chest, kind, place, place + 5).map((k) => el('div', { className: `pet-roulette-cell pet-kind-${kindOf(k).rarity}` }, petImage(k, 64))));
  const wheel = el('div', { className: 'pet-roulette' }, tape, el('div', { className: 'pet-roulette-mark' }));
  const reveal = el('div', { className: 'pet-reveal', hidden: true });
  const error = el('div', { className: 'error', hidden: true });
  modal.body.append(spin ? wheel : '', reveal, error);
  modal.box.classList.add('pet-roulette-modal');

  const show = () => {
    const name = el('input', { type: 'text', maxLength: 20, value: petKindName(kind), required: true });
    reveal.replaceChildren(
      el('div', { className: `pet-reveal-picture pet-kind-${rarity}` }, petImage(kind, 96)),
      el('div', { className: 'pet-reveal-text' },
        el('div', { className: 'pet-reveal-title', textContent: tr('Из сундука вышел питомец: {0}!', petKindName(kind)) }),
        rarityBadge(rarity),
        el('p', { className: 'hint', textContent: petKindText(kind) }),
        specialNode(kind),
        el('label', { textContent: tr('Имя питомца:') }), name,
        pets.length >= maxPets ? el('p', { className: 'hint', textContent: tr('У вас уже десять питомцев: чтобы забрать этого, отпустите кого-нибудь.') }) : null));
    reveal.hidden = false;
    const decide = (run, done) => async () => {
      if (busy) return;
      hideError(error);
      setBusy(true);
      try {
        const answer = await run();
        modal.close();
        done(answer);
      } catch (e) {
        showError(error, e);
      } finally {
        setBusy(false);
      }
    };
    const take = button(tr('Забрать'), decide(() => api.takePetDrop(name.value), (answer) => {
      keeper = { ...keeper, dropKind: '', dropCase: '' };
      keep(answer.pet);
    }));
    take.disabled = pets.length >= maxPets;
    const leave = button(tr('Оставить за {0}', petShards(shards)), decide(() => api.leavePetDrop(), (answer) => {
      keeper = answer.keeper;
      if (pet) showTabs();
      else showShop();
    }), { secondary: true });
    modal.actions.replaceChildren(take, leave);
    if (!take.disabled) name.select();
  };

  if (!spin) {
    show();
    return;
  }

  // The tape stops with the pet under the mark, a little off its middle,
  // as a real wheel would.
  const width = wheel.clientWidth || 380;
  const shift = place * cell - width / 2 + cell / 2 + Math.round((Math.random() - 0.5) * cell * 0.6);
  if (still) {
    tape.style.transform = `translateX(${-shift}px)`;
    show();
    return;
  }
  modal.actions.append(el('span', { className: 'hint', textContent: tr('Сундук открывается…') }));
  requestAnimationFrame(() => requestAnimationFrame(() => {
    tape.classList.add('pet-roulette-spin');
    tape.style.transform = `translateX(${-shift}px)`;
  }));
  tape.addEventListener('transitionend', () => {
    wheel.classList.add('pet-roulette-done');
    show();
  }, { once: true });
}

// The tabs of the pets: one for each, and "+" for the shop while there is
// room for one more.

function showTabs() {
  const tabs = pets.map((p) => {
    const tab = el('button', { type: 'button', className: p.id === pet?.id ? 'pet-tab pet-tab-chosen' : 'pet-tab', title: petKindName(p.kind) },
      petImage(p.kind, 32, p.outfit), el('span', { textContent: p.name }));
    tab.addEventListener('click', () => { if (!busy) showPet(p); });
    return tab;
  });
  const arena = el('button', { type: 'button', className: league ? 'pet-tab pet-tab-more pet-tab-chosen' : 'pet-tab pet-tab-more' },
    el('b', { textContent: '⚔' }), el('span', { textContent: tr('Арена') }));
  arena.addEventListener('click', () => { if (!busy) showLeague(); });
  tabs.push(arena);
  if (pets.length < maxPets) {
    const more = el('button', { type: 'button', className: pet || league ? 'pet-tab pet-tab-more' : 'pet-tab pet-tab-more pet-tab-chosen' },
      el('b', { textContent: '+' }), el('span', { textContent: tr('Завести ещё') }));
    more.addEventListener('click', () => { if (!busy) showShop(); });
    tabs.push(more);
  }
  $('pet-tabs').replaceChildren(...tabs);
  $('pet-tabs').hidden = false;
  $('pets-count').replaceChildren(...countNodes());
}

// countNodes say how many pets there are and, away from the arena, where
// the quests, the boss, the tournament, the season and the market are.
function countNodes() {
  const nodes = pets.length ? [tr('Питомцев: {0} из {1}.', pets.length, maxPets)] : [];
  if (league) return nodes;

  const link = el('a', { href: '/pet?arena=1', textContent: tr('Арена') });
  link.addEventListener('click', (event) => {
    event.preventDefault();
    if (!busy) showLeague();
  });

  return [...nodes, nodes.length ? ' ' : '', link, tr(': задания дня, босс недели, турнир, сезон, достижения и рынок питомцев.')];
}

// keep puts a pet as the server answered in its place.
function keep(next) {
  const i = pets.findIndex((p) => p.id === next.id);
  if (i < 0) pets.push(next);
  else pets[i] = next;
  showPet(next);
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

// The bars of stats run up to so much: a legend in a crown at the top
// level fills them nearly to the end.
const statScale = { attack: 50, defense: 50, health: 200, speed: 30 };

let opened = '';

// powerRow is a stat as a bar of three parts, the breed, the levels and the
// clothes; a click on it tells where each comes from.
function powerRow(stat) {
  const breed = pet.breed?.[stat] ?? pet[stat];
  const clothes = pet.clothes?.[stat] ?? 0;
  const levels = Math.max(0, pet[stat] - breed - clothes);
  const scale = Math.max(statScale[stat], pet[stat]);
  const part = (className, value) => {
    const fill = el('div', { className: `pet-power-part ${className}` });
    fill.style.width = `${(100 * value) / scale}%`;
    return fill;
  };
  const about = el('div', { className: 'pet-power-about', hidden: opened !== stat },
    el('div', { textContent: petStatText(stat) }),
    el('div', { className: 'pet-power-sum' },
      el('span', {}, el('i', { className: 'pet-power-breed' }), tr('порода: {0}', breed)),
      el('span', {}, el('i', { className: 'pet-power-levels' }), tr('уровни: +{0}', levels)),
      el('span', {}, el('i', { className: 'pet-power-clothes' }), tr('одежда: +{0}', clothes))));
  const row = el('button', { type: 'button', className: opened === stat ? 'pet-power pet-power-open' : 'pet-power', 'aria-expanded': String(opened === stat) },
    el('span', { className: 'pet-power-label', textContent: petStatName(stat) }),
    el('span', { className: 'pet-power-bar' }, part('pet-power-breed', breed), part('pet-power-levels', levels), part('pet-power-clothes', clothes)),
    el('b', { className: 'pet-power-value', textContent: String(pet[stat]) }));
  row.addEventListener('click', () => {
    opened = opened === stat ? '' : stat;
    showPowers();
  });
  return el('div', {}, row, about);
}

function showPowers() {
  $('powers').replaceChildren(
    ...petStats.map(powerRow),
    el('div', { className: 'pet-power-crit' },
      el('span', { className: 'pet-power-label', textContent: tr('Сильный удар') }),
      el('span', { textContent: tr('{0}% ударов — вдвое сильнее', pet.crit) })));
}

function showStats() {
  showPowers();
  $('stats').replaceChildren(
    statRow(tr('Порода:'), `${petKindName(pet.kind)}, ${petRarityName(pet.rarity || kindOf(pet.kind).rarity)}`),
    kindOf(pet.kind).special
      ? statRow(tr('Стихия:'), petElementName(kindOf(pet.kind).element))
      : null,
    kindOf(pet.kind).special
      ? statRow(tr('Особенность:'), `${petSpecialName(kindOf(pet.kind).special)} — ${petSpecialText(kindOf(pet.kind).special)}`)
      : null,
    statRow(tr('Сила:'), groupedVotes(pet.power ?? 0)),
    statRow(tr('Рейтинг:'), groupedVotes(pet.rating)),
    statRow(tr('Бои:'), tr('побед: {0}, поражений: {1}', pet.wins, pet.losses)),
    statRow(tr('Живёт у вас:'), tr('с {0}', formatDateTime(pet.bornAt))),
  );
}

// The wardrobe: the clothes of the shop by the places they go on. A pet
// tries a thing on while the pointer is over it.

function stage(outfit) {
  $('stage').replaceChildren(petImage(pet.kind, 96, outfit));
}

// tryOn is the outfit of the pet with item instead of what is in its place.
function tryOn(item) {
  const slot = catalogue.items.find((i) => i.id === item)?.slot;
  const slotOf = (id) => catalogue.items.find((i) => i.id === id)?.slot;
  return [...(pet.outfit ?? []).filter((id) => slotOf(id) !== slot), item];
}

async function dress(item, run) {
  const dressed = await act(run);
  if (dressed) keep(dressed.pet);
  return dressed;
}

function itemCard(item) {
  const owned = (pet.wardrobe ?? []).includes(item.id);
  const worn = (pet.outfit ?? []).includes(item.id);
  let action;
  if (!owned) action = button(tr('Купить за {0}', votesAmount(item.price)), () => dress(item.id, () => api.buyPetItem(pet.id, item.id)));
  else if (worn) action = button(tr('Снять'), () => dress(item.id, () => api.dressPet(pet.id, item.id, false)), { secondary: true });
  else action = button(tr('Надеть'), () => dress(item.id, () => api.dressPet(pet.id, item.id, true)));
  action.classList.add('pet-busy');
  action.disabled = busy;

  const card = el('div', { className: worn ? 'pet-item pet-item-worn' : owned ? 'pet-item pet-item-owned' : 'pet-item', tabIndex: 0 },
    el('div', { className: 'pet-item-picture' }, petItemImage(item.id, 40)),
    el('div', { className: 'pet-item-name', textContent: petItemName(item.id) }),
    el('div', { className: 'pet-item-bonus', textContent: petBonus(item.bonus) }),
    worn ? el('div', { className: 'pet-item-state', textContent: tr('надето') }) : owned ? el('div', { className: 'pet-item-state', textContent: tr('в гардеробе') }) : null,
    action);
  const fit = () => { if (!worn) stage(tryOn(item.id)); };
  const back = () => stage(pet.outfit);
  card.addEventListener('mouseenter', fit);
  card.addEventListener('mouseleave', back);
  card.addEventListener('focusin', fit);
  card.addEventListener('focusout', back);
  return card;
}

function showWardrobe() {
  $('wardrobe').replaceChildren(...petSlots.map((slot) => el('div', { className: 'pet-slot' },
    el('div', { className: 'pet-slot-title', textContent: petSlotName(slot) }),
    el('div', { className: 'pet-slot-items' }, ...catalogue.items.filter((item) => item.slot === slot).map(itemCard)))));
}

function stageClass() {
  const low = Math.min(pet.satiety, pet.joy);
  if (low >= 70) return 'pet-stage pet-happy';
  if (low < 15) return 'pet-stage pet-sad';
  return 'pet-stage';
}

function showPet(next) {
  pet = next;
  league = false;
  $('league').hidden = true;
  $('stage').className = stageClass();
  stage(pet.outfit);
  $('name').textContent = pet.name;
  $('kind').textContent = petKindName(pet.kind);
  $('rarity').replaceChildren(rarityBadge(pet.rarity || kindOf(pet.kind).rarity));
  $('mood').textContent = petMood(pet);

  bar('satiety', pet.satiety);
  $('satiety').textContent = `${pet.satiety}%`;
  bar('joy', pet.joy);
  $('joy').textContent = `${pet.joy}%`;
  $('level').textContent = String(pet.level);
  bar('experience', petLevelProgress(pet));
  $('experience').textContent = `${groupedVotes(pet.experience)} / ${groupedVotes(pet.nextLevelExperience)}`;

  $('fights-left').textContent = tr('Боёв сегодня: {0} из {1}. Победа над равным приносит {2} и рейтинг, над сильным — больше, над слабым — меньше; соперника подбирают по силе. Для боя питомцу нужны сытость и настроение.',
    pet.fights, shop.fights, votesAmount(shop.win))
    + (pet.fatigue ? ' ' + tr('Питомец устал от боёв подряд: −{0}% к силе. Час отдыха снимает усталость.', pet.fatigue * 5) : '')
    + (pet.salePrice ? ' ' + tr('Питомец выставлен на рынок за {0} и не дерётся, пока его не снимут с продажи.', votesAmount(pet.salePrice)) : '');
  $('stance').value = pet.stance || 'defense';
  $('guard').value = pet.stance || 'defense';
  showStanceText();

  showActions();
  showFightButtons();
  showStats();
  showWardrobe();
  $('shop').hidden = true;
  $('home').hidden = false;
  showTabs();

  const battle = battleOf(pet);
  if (battle) showBattle(battle);
  else if ($('arena').dataset.pet !== pet.id) $('arena').hidden = true;
}

// Fights.

function battleOf(p) {
  return battles.find((b) => b.petId === p.id) ?? null;
}

function showFightButtons() {
  const closed = busy || pet.fights >= shop.fights || Boolean(battleOf(pet));
  $('find').disabled = closed;
  $('call').disabled = closed;
}

function showStanceText() {
  $('stance-about').textContent = petStanceText($('stance').value);
}

// fighterSide is a pet of a fight, with its owner's page under its name.
function fighterSide(fighter) {
  return el('div', { className: 'pet-fight-side' },
    petImage(fighter.kind, 32, fighter.outfit),
    el('div', { className: 'pet-fight-name' },
      el('b', { textContent: fighter.name }),
      fighter.user
        ? el('a', { className: 'hint', href: '/' + fighter.user.nickname, textContent: `${fighter.user.name} ${fighter.user.surname}` })
        : null));
}

// fightRow puts the viewer's pet on the left and the rival on the right,
// as the arena does, with the result between them.
// storyNode is the story of a fight, a line a hit.
function storyNode(lines) {
  return el('div', { className: 'pet-story' }, ...lines.map(storyLine));
}

function storyLine(line) {
  const kind = line.end ? ' pet-log-end' : line.crit ? ' pet-log-crit' : line.dodge ? ' pet-log-dodge'
    : line.special ? ' pet-log-special' : line.guard ? ' pet-log-guard' : '';
  return el('div', { className: 'pet-log-line' + kind, textContent: line.text });
}

function fightRow(fight) {
  const mine = fight.attacker.user?.id === me.id;
  const result = petFightResult(fight, mine);
  const story = el('div', { className: 'pet-fight-story', hidden: true });
  const tell = el('a', { className: 'pet-fight-tell', textContent: tr('Как прошёл бой') });
  tell.addEventListener('click', () => {
    if (!story.firstChild) story.append(storyNode(petFightStory(fight)));
    story.hidden = !story.hidden;
    tell.textContent = story.hidden ? tr('Как прошёл бой') : tr('Скрыть');
  });
  return el('div', { className: 'pet-fight-entry' }, el('div', { className: 'pet-fight' },
    fighterSide(mine ? fight.attacker : fight.defender),
    el('div', { className: 'pet-result' },
      el('b', { className: result.won ? 'pet-won' : 'pet-lost', textContent: result.text }),
      el('div', { className: 'hint', textContent: tr('рейтинг {0}', result.rating) }),
      el('div', { className: 'hint', textContent: mine ? tr('ваш вызов') : tr('вызов вам') }),
      el('div', { className: 'hint', textContent: formatPostDate(fight.foughtAt) }),
      fight.hits?.length ? tell : null),
    fighterSide(mine ? fight.defender : fight.attacker)), story);
}

function showFights(fights) {
  $('fights').replaceChildren(...fights.map(fightRow));
  $('no-fights').hidden = fights.length > 0;
}

function fighterBox(fighter) {
  const health = el('div', { className: 'pet-bar-fill pet-health' });
  const value = el('span', { className: 'pet-arena-hp', textContent: `${fighter.health} / ${fighter.health}` });
  const box = el('div', { className: 'pet-arena-side' },
    el('div', { className: 'pet-arena-picture' }, petImage(fighter.kind, 64, fighter.outfit)),
    el('b', { textContent: fighter.name }),
    el('div', { className: 'hint', textContent: tr('{0} ур.', fighter.level) + (fighter.user ? ` · ${fighter.user.name} ${fighter.user.surname}` : '') }),
    el('div', { className: 'pet-bar pet-arena-bar' }, health),
    value);

  return {
    box,
    set(left) {
      health.style.width = `${(100 * left) / fighter.health}%`;
      value.textContent = `${left} / ${fighter.health}`;
    },
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

// showHit puts the health of both after a hit; the one hurt shakes.
function showHit(attacker, defender, hit) {
  attacker.set(hit.attackerHealth);
  defender.set(hit.defenderHealth);
  if (hit.damage > 0) {
    if (hit.byAttacker) defender.hit(hit.defenderHealth);
    else attacker.hit(hit.attackerHealth);
  }
}

const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function playFight(fight) {
  const attacker = fighterBox(fight.attacker);
  const defender = fighterBox(fight.defender);
  const log = el('div', { className: 'pet-log pet-story' });
  const story = petFightStory(fight);
  const end = el('div', { className: 'pet-arena-result', hidden: true });

  $('arena').replaceChildren(
    el('div', { className: 'pet-arena-ring' }, attacker.box, el('div', { className: 'pet-arena-vs', textContent: 'VS' }), defender.box),
    log, end);
  $('arena').dataset.pet = pet.id;
  $('arena').hidden = false;

  log.append(storyLine(story[0]));
  for (const [i, hit] of fight.hits.entries()) {
    if (!still) await pause(hitPause);
    showHit(attacker, defender, hit);
    log.append(storyLine(story[i + 1]));
    log.scrollTop = log.scrollHeight;
  }
  log.append(storyLine(story.at(-1)));
  log.scrollTop = log.scrollHeight;

  const result = petFightResult(fight, true);
  end.className = result.won ? 'pet-arena-result pet-won' : 'pet-arena-result pet-lost';
  end.textContent = result.won
    ? tr('Победа! {0} и рейтинг {1}.', votesAmount(fight.reward ?? shop.win), result.rating)
    : tr('Поражение. Рейтинг {0}.', result.rating);
  end.hidden = false;
}

async function fight(nickname) {
  const stance = $('stance').value;
  if ($('manual').checked) {
    const started = (await act(() => api.startPetBattle(pet.id, nickname, stance).then((turn) => ({ turn }))))?.turn;
    if (!started) return;
    battles = [...battles.filter((b) => b.petId !== pet.id), started.battle];
    view = null;
    keep(started.pet);
    $('arena').scrollIntoView({ block: 'nearest', behavior: still ? 'auto' : 'smooth' });
    return;
  }

  // The balance waits for the end of the fight: it would tell who won.
  const answer = (await act(() => api.fightPet(pet.id, nickname, stance).then((fought) => ({ fought }))))?.fought;
  if (!answer) return;

  setBusy(true);
  try {
    await playFight(answer.fight);
  } finally {
    setBusy(false);
  }
  keep(answer.pet);
  showBalance(answer.balance);
  api.petFights().then(({ fights }) => showFights(fights)).catch(() => {});
}

// A fight by hand: the arena stays with the health of both, the story so
// far and the moves; each move plays the hits it brought. Until the end
// the fight counts as lost, so leaving it loses.

let view = null;

function showBattle(battle) {
  if (view?.petId === battle.petId && $('arena').dataset.pet === battle.petId && !$('arena').hidden) return;
  const attacker = fighterBox(battle.attacker);
  const defender = fighterBox(battle.defender);
  attacker.set(battle.attackerHealth);
  defender.set(battle.defenderHealth);
  const log = el('div', { className: 'pet-log pet-story' },
    storyLine({ text: tr('{0} ({1} ур.) вызывает на бой {2} ({3} ур.).', battle.attacker.name, battle.attacker.level, battle.defender.name, battle.defender.level) }),
    ...petHitLines(battle.attacker, battle.defender, battle.hits).map(storyLine));
  const info = el('p', { className: 'hint' });
  const moves = el('div', { className: 'pet-moves' }, ...petMoves.map((move) => {
    const b = button(petMoveName(move), () => moveBattle(move), { secondary: move === 'auto' });
    b.title = petMoveText(move);
    b.classList.add('pet-busy');
    b.disabled = busy;
    return b;
  }));
  const end = el('div', { className: 'pet-arena-result', hidden: true });
  $('arena').replaceChildren(
    el('div', { className: 'pet-arena-ring' }, attacker.box, el('div', { className: 'pet-arena-vs', textContent: 'VS' }), defender.box),
    log, info, moves, end);
  $('arena').dataset.pet = battle.petId;
  $('arena').hidden = false;
  log.scrollTop = log.scrollHeight;
  view = { petId: battle.petId, battle, attacker, defender, log, info, moves, end };
  showRound(battle);
}

function showRound(battle) {
  view.info.textContent = tr('Раунд {0} из {1}. Стойки: ваша — {2}, соперника — {3}. Победа: +{4} к рейтингу, поражение: −{5}. Пока бой не закончен, он считается проигранным.',
    Math.min(battle.round + 1, battle.rounds), battle.rounds, petStanceName(battle.attacker.stance), petStanceName(battle.defender.stance), battle.win, battle.loss);
}

async function moveBattle(move) {
  const turn = (await act(() => api.movePetBattle(pet.id, move).then((t) => ({ t }))))?.t;
  if (!turn || !view) return;

  setBusy(true);
  try {
    const { battle } = turn;
    const lines = petHitLines(battle.attacker, battle.defender, battle.hits, view.battle.hits.length);
    for (const [n, hit] of battle.hits.slice(view.battle.hits.length).entries()) {
      if (!still) await pause(hitPause);
      showHit(view.attacker, view.defender, hit);
      view.log.append(storyLine(lines[n]));
      view.log.scrollTop = view.log.scrollHeight;
    }
    view.battle = battle;
    showRound(battle);
  } finally {
    setBusy(false);
  }

  if (!turn.fight) {
    battles = battles.map((b) => (b.petId === turn.battle.petId ? turn.battle : b));
    return;
  }

  const story = petFightStory(turn.fight);
  view.log.append(storyLine(story.at(-1)));
  view.log.scrollTop = view.log.scrollHeight;
  const result = petFightResult(turn.fight, true);
  view.end.className = result.won ? 'pet-arena-result pet-won' : 'pet-arena-result pet-lost';
  view.end.textContent = result.won
    ? tr('Победа! {0} и рейтинг {1}.', votesAmount(turn.fight.reward), result.rating)
    : tr('Поражение. Рейтинг {0}.', result.rating);
  view.end.hidden = false;
  view.info.hidden = true;
  view.moves.hidden = true;
  battles = battles.filter((b) => b.petId !== turn.battle.petId);
  view = null;
  keep(turn.pet);
  showBalance(turn.balance);
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
    const renamed = await act(() => api.renamePet(pet.id, input.value).then(unpaid));
    if (renamed) keep(renamed.pet);
  });
  input.select();
}

function openRelease() {
  const modal = openModal(tr('Отпустить питомца'));
  const shards = kindOf(pet.kind).shards ?? 0;
  modal.body.append(el('p', { textContent: tr('Отпустить {0}? Питомец уйдёт навсегда, с уровнем и рейтингом, а голоса за него не вернутся — останется {1}.', pet.name, petShards(shards)) }));
  modal.actions.append(
    button(tr('Отпустить'), async () => {
      modal.close();
      const gone = pet.id;
      if (await act(() => api.releasePet(gone).then(() => ({})))) {
        keeper = { ...keeper, shards: keeper.shards + shards };
        battles = battles.filter((b) => b.petId !== gone);
        pets = pets.filter((p) => p.id !== gone);
        chosen = '';
        $('adopt').hidden = true;
        $('adopt-name').value = '';
        if (pets.length) showPet(pets[0]);
        else showShop();
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }));
}

// The arena.

// drawLeague draws the arena as the server answered, keeping the page
// where it is, with what the last action brought over it.
function drawLeague(arena, note = null) {
  const ctx = {
    me,
    pets,
    run: async (body, then) => {
      const answer = await act(() => api.runPetArena(body).then(unpaid));
      if (!answer) return;
      pets = (await api.pets().catch(() => ({ pets }))).pets;
      showTabs();
      drawLeague(answer.arena, then?.(answer) ?? null);
    },
  };
  const box = note ? el('div', { className: 'notice pet-league-note' }, note) : null;
  replaceInPlace($('league'), ...(box ? [box] : []), ...leagueNodes(arena, ctx));
  if (box) notify(box);
}

async function showLeague() {
  pet = null;
  league = true;
  $('shop').hidden = true;
  $('home').hidden = true;
  $('league').hidden = false;
  showTabs();
  const arena = await act(() => api.petArena());
  if (arena) drawLeague(arena);
}

function watch() {
  $('adopt').addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!chosen) return;
    const adopted = await act(() => api.adoptPet(chosen, $('adopt-name').value));
    if (adopted) {
      chosen = '';
      $('adopt').hidden = true;
      $('adopt-name').value = '';
      keep(adopted.pet);
    }
  });
  $('feed').addEventListener('click', async () => {
    const fed = await act(() => api.feedPet(pet.id));
    if (fed) keep(fed.pet);
  });
  $('play').addEventListener('click', async () => {
    const played = await act(() => api.playPet(pet.id).then(unpaid));
    if (played) keep(played.pet);
  });
  $('find').addEventListener('click', () => fight(''));
  $('challenge').addEventListener('submit', (event) => {
    event.preventDefault();
    const nickname = $('rival').value.trim().replace(/^@/, '');
    if (nickname) fight(nickname);
  });
  $('stance').addEventListener('change', showStanceText);
  $('guard').addEventListener('change', async () => {
    const set = await act(() => api.setPetStance(pet.id, $('guard').value).then(unpaid));
    if (set) keep(set.pet);
    else $('guard').value = pet.stance;
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
    catalogue = { kinds: answer.kinds ?? [], cases: answer.cases ?? [], items: answer.items ?? [] };
    if (answer.keeper) keeper = answer.keeper;
    battles = answer.battles ?? [];
    const stances = () => petStances.map((stance) => el('option', { value: stance, textContent: petStanceName(stance) }));
    $('stance').replaceChildren(...stances());
    $('guard').replaceChildren(...stances());
    showBalance(me.votes ?? 0);
    watch();
    pets = answer.pets;
    const wanted = new URLSearchParams(location.search).get('pet');
    if (new URLSearchParams(location.search).has('arena')) showLeague();
    else if (pets.length) showPet(pets.find((p) => p.id === wanted) ?? pets[0]);
    else showShop();
    if (keeper.dropKind) showDrop(false);
    showNamedPet(kindOf('big2z'), keep).catch(() => {});
    showFights((await fought).fights);
    const rival = new URLSearchParams(location.search).get('fight');
    if (rival) $('rival').value = rival;
  } catch (e) {
    showError($('error'), e);
  }
}
