// "Арена" of "Питомцы" (/pet?arena=1): what the pets do together. The
// tasks of the day, three drawn for each person, pay votes and, all done,
// shards and a bonus for the days in a row; the boss of the week all pets
// hit, its prize split by the damage, more to the one who finishes it;
// the tournament of the week on Sunday at 20:00 with bets of the onlookers;
// the season of the rating, a month, whose best ten are paid; the badges;
// and the market where people sell pets to each other. The server settles
// the season and the tournaments when the arena is opened.

import {
  formatDateTime, formatPostDate, groupedVotes, petAchievementName, petAchievementText, petAchievements, petFightStory, petKindName,
  petQuestName, petRarityName, votesAmount,
} from './labels.js';
import { el } from './layout.js';
import { button, openModal } from './modal.js';
import { petImage } from './pet.js';
import { gameWindow } from './game.js';
import { tr } from './i18n.js';

// The rules of the arena, as the user service has them
// (user/internal/user/pet_arena.go, pet_market.go).
const minBet = 10;
const maxBet = 100000;
export const minPrices = { common: 50, rare: 200, epic: 800, legendary: 3000 };
export const marketFee = 10;

// ownerLink is the page of the owner of a pet, or "страница удалена".
function ownerLink(user) {
  if (!user) return el('span', { className: 'hint', textContent: tr('страница удалена') });
  return el('a', { href: '/' + user.nickname, textContent: `${user.name} ${user.surname}` });
}

function progressBar(value, max, className) {
  const fill = el('div', { className: `pet-bar-fill ${className}` });
  fill.style.width = `${max ? Math.round(100 * Math.min(value, max) / max) : 0}%`;
  return el('div', { className: 'pet-bar', 'aria-hidden': 'true' }, fill);
}

// petSelect is a list of the pets of the viewer that may do something.
function petSelect(pets) {
  return el('select', {}, ...pets.map((p) => el('option', { value: p.id, textContent: `${p.name} (${petKindName(p.kind)}, ${tr('{0} ур.', p.level)})` })));
}

// story is a link that shows how a fight went under it.
function story(fight) {
  const box = el('div', { className: 'pet-fight-story', hidden: true });
  const tell = el('a', { className: 'pet-fight-tell', textContent: tr('Как прошёл бой') });
  tell.addEventListener('click', () => {
    if (!box.firstChild) {
      box.append(el('div', { className: 'pet-story' }, ...petFightStory(fight).map((line) =>
        el('div', { className: 'pet-log-line' + (line.end ? ' pet-log-end' : line.crit ? ' pet-log-crit' : ''), textContent: line.text }))));
    }
    box.hidden = !box.hidden;
    tell.textContent = box.hidden ? tr('Как прошёл бой') : tr('Скрыть');
  });
  return [tell, box];
}

// The tasks of the day.

function questsBlock(arena) {
  const rows = arena.quests.map((q) => el('tr', {},
    el('td', { className: 'pet-bar-label', textContent: petQuestName(q.kind) }),
    el('td', {}, progressBar(q.progress, q.goal, q.progress >= q.goal ? 'pet-experience' : 'pet-joy')),
    el('td', { className: 'pet-bar-value', textContent: q.progress >= q.goal ? tr('готово') : `${q.progress} / ${q.goal}` })));
  return [
    el('table', { className: 'pet-bars pet-quests' }, ...rows),
    el('p', { className: 'hint', textContent: tr('Все задания подряд: {0} дн.', arena.streak) }),
  ];
}

// The boss of the week.

function bossBlock(arena, ctx) {
  const boss = arena.boss;
  const ready = ctx.pets.filter((p) => !p.salePrice && !p.bossAt);
  const out = [
  ];

  const card = el('div', { className: 'pet-card pet-boss' },
    el('div', { className: 'pet-stage' }, petImage(boss.kind, 96)),
    el('div', { className: 'pet-info' },
      el('div', { className: 'pet-name' }, el('b', { textContent: tr('{0} недели', petKindName(boss.kind)) })),
      el('table', { className: 'pet-bars' }, el('tr', {},
        el('td', { className: 'pet-bar-label', textContent: tr('Здоровье:') }),
        el('td', {}, progressBar(boss.health, boss.maxHealth, 'pet-health')),
        el('td', { className: 'pet-bar-value', textContent: `${groupedVotes(boss.health)} / ${groupedVotes(boss.maxHealth)}` })))));
  out.push(card);

  if (boss.killedAt) {
    out.push(el('p', {}, tr('Босс повержен {0}. Последний удар — ', formatDateTime(boss.killedAt)), ownerLink(boss.killedBy), '.'));
  } else if (ctx.pets.length === 0) {
    out.push(el('p', { className: 'hint', textContent: tr('Заведите питомца, чтобы бить босса.') }));
  } else if (ready.length === 0) {
    const next = ctx.pets.filter((p) => p.bossAt).map((p) => p.bossAt).sort()[0];
    out.push(el('p', { className: 'hint', textContent: next ? tr('Ваши питомцы отдыхают. Следующий удар — {0}.', formatDateTime(next)) : tr('Питомцы на рынке босса не бьют.') }));
  } else {
    const select = petSelect(ready);
    const hit = button(tr('Ударить'), () => ctx.run({ action: 'boss', petId: select.value }, (answer) => {
      if (!answer.bout) return null;
      const fight = { ...answer.bout, defender: { ...answer.bout.defender, name: petKindName(boss.kind) } };
      const [tell, box] = story(fight);
      return el('div', {}, el('b', { textContent: tr('Урон: {0}.', fight.hits.filter((h) => h.byAttacker).reduce((s, h) => s + h.damage, 0)) }), ' ', tell, box);
    }));
    hit.classList.add('pet-busy');
    out.push(el('div', { className: 'pet-stance-row' }, el('label', { textContent: tr('Питомец:') }), select, hit));
  }

  if (boss.hits.length) {
    out.push(el('table', { className: 'info pet-league-table' },
      el('tr', {}, el('th', { textContent: '#' }), el('th', { textContent: tr('Кто') }), el('th', { textContent: tr('Урон') }), el('th', { textContent: tr('Ударов') })),
      ...boss.hits.slice(0, 10).map((h, i) => el('tr', {},
        el('td', { textContent: String(i + 1) }), el('td', {}, ownerLink(h.user)),
        el('td', { textContent: groupedVotes(h.damage) }), el('td', { textContent: String(h.hits) })))));
  }

  return out;
}

// The tournament of the week.

function entrantNode(e) {
  return el('span', { className: 'pet-entrant' }, petImage(e.kind, 32), el('b', { textContent: e.name }), ' ', ownerLink(e.user));
}

function bracket(t) {
  const names = [tr('Финал'), tr('Полуфинал'), tr('Четвертьфинал'), tr('1/8 финала')];
  return t.rounds.map((round, i) => {
    const back = t.rounds.length - 1 - i;
    return el('div', { className: 'pet-round' },
      el('b', { textContent: names[back] ?? tr('Раунд {0}', i + 1) }),
      ...round.map((m) => {
        const winner = t.entrants[m.winner];
        if (m.b < 0) return el('div', { className: 'pet-match' }, entrantNode(winner), el('span', { className: 'hint', textContent: ' — ' + tr('проходит без боя') }));
        const loser = t.entrants[m.winner === m.a ? m.b : m.a];
        return el('div', { className: 'pet-match' }, entrantNode(winner), el('span', { className: 'hint', textContent: ' ' + tr('побеждает') + ' ' }),
          el('b', { textContent: loser.name }), ' ', ...(m.bout ? story(m.bout) : []));
      }));
  });
}

function placesNode(t) {
  const titles = [tr('Победитель'), tr('Финалист'), tr('Полуфиналист'), tr('Полуфиналист')];
  return el('ol', { className: 'pet-places' }, ...t.places.map((i, n) =>
    el('li', {}, el('span', { className: 'hint', textContent: titles[n] + ': ' }), entrantNode(t.entrants[i]))));
}

function tournamentBlock(arena, ctx) {
  const t = arena.tournament;
  const out = [
  ];

  if (t.held) {
    if (t.places.length) out.push(placesNode(t), ...bracket(t));
    else out.push(el('p', { className: 'hint', textContent: tr('Турнир не состоялся: записалось меньше двух питомцев. Ставки вернулись.') }));
  } else {
    out.push(el('p', {}, tr('Начало: {0}. Записались: {1} из 16.', formatDateTime(t.startsAt), t.entrants.length)));
    const mine = t.entrants.find((e) => e.user?.id === ctx.me.id);
    if (mine) {
      const leave = button(tr('Выйти из турнира'), () => ctx.run({ action: 'leave' }), { secondary: true });
      leave.classList.add('pet-busy');
      out.push(el('p', {}, tr('Ваш питомец {0} в турнире. ', mine.name), leave));
    } else if (ctx.pets.length && t.entrants.length < 16) {
      const select = petSelect(ctx.pets.filter((p) => !p.salePrice));
      const join = button(tr('Записаться'), () => ctx.run({ action: 'join', petId: select.value }));
      join.classList.add('pet-busy');
      out.push(el('div', { className: 'pet-stance-row' }, el('label', { textContent: tr('Питомец:') }), select, join));
    }
    if (t.entrants.length) out.push(el('div', { className: 'pet-entrants' }, ...t.entrants.map((e) => el('div', {}, entrantNode(e), el('span', { className: 'hint', textContent: ` ${tr('{0} ур.', e.level)}, ${tr('сила {0}', e.power)}` })))));
    out.push(betNode(t, ctx));
  }

  if (arena.last?.places.length && !t.held) {
    out.push(el('p', { className: 'hint', textContent: tr('Прошлый турнир:') }), placesNode(arena.last));
  }

  return out;
}

function betNode(t, ctx) {
  const stakes = tr('Ставок: {0}, в банке {1}.', t.bets, votesAmount(t.pool));
  if (t.betPetId) {
    const on = t.entrants.find((e) => e.petId === t.betPetId);
    return el('p', {}, tr('Ваша ставка — {0} на {1}. ', votesAmount(t.betVotes), on?.name ?? '?'), el('span', { className: 'hint', textContent: stakes }));
  }
  if (t.entrants.length < 2) return el('p', { className: 'hint', textContent: stakes });

  const select = el('select', {}, ...t.entrants.map((e) => el('option', { value: e.petId, textContent: `${e.name} (${petKindName(e.kind)})` })));
  const votes = el('input', { type: 'number', min: String(minBet), max: String(maxBet), step: '1', value: '100', className: 'pet-votes' });
  const bet = button(tr('Поставить'), () => ctx.run({ action: 'bet', petId: select.value, votes: Number(votes.value) }));
  bet.classList.add('pet-busy');
  return el('div', {},
    el('div', { className: 'pet-stance-row' }, el('label', { textContent: tr('Ставка на:') }), select, votes, bet),
    el('p', { className: 'hint', textContent: tr('От {0} до {1}, одна ставка на турнир. ', groupedVotes(minBet), groupedVotes(maxBet)) + stakes }));
}

// The season of the rating.

function placesTable(places) {
  return el('table', { className: 'info pet-league-table' },
    el('tr', {}, el('th', { textContent: '#' }), el('th', { textContent: tr('Питомец') }), el('th', { textContent: tr('Хозяин') }),
      el('th', { textContent: tr('Рейтинг') }), el('th', { textContent: tr('Награда') })),
    ...places.map((p, i) => el('tr', {},
      el('td', { textContent: String(i + 1) }),
      el('td', {}, el('span', { className: 'pet-entrant' }, petImage(p.kind, 32), p.name)),
      el('td', {}, ownerLink(p.user)),
      el('td', { textContent: String(p.rating) }),
      el('td', { textContent: votesAmount(p.prize) }))));
}

function seasonBlock(arena) {
  const out = [
    el('p', { className: 'hint', textContent: tr('Сезон кончается {0}.', formatDateTime(arena.seasonEndsAt)) }),
  ];
  out.push(arena.top.length ? placesTable(arena.top) : el('p', { className: 'hint', textContent: tr('Питомцев пока нет.') }));

  const past = arena.seasons.filter((s) => s.places.length);
  if (past.length) {
    const box = el('div', { hidden: true }, ...past.flatMap((s) => [el('p', {}, el('b', { textContent: tr('Сезон {0}', formatPostDate(s.season)) })), placesTable(s.places)]));
    const toggle = el('a', { textContent: tr('Прошлые сезоны') });
    toggle.addEventListener('click', () => { box.hidden = !box.hidden; });
    out.push(el('p', {}, toggle), box);
  }

  return out;
}

function achievementsBlock(arena) {
  const got = new Map(arena.achievements.map((a) => [a.id, a.gotAt]));
  return [
    el('div', { className: 'pet-badges' }, ...petAchievements.map((id) => el('div', {
      className: got.has(id) ? 'pet-badge pet-badge-got' : 'pet-badge',
      title: petAchievementText(id) + (got.has(id) ? ` ${tr('Получено {0}', formatDateTime(got.get(id)))}` : ''),
    }, el('b', { textContent: petAchievementName(id) })))),
  ];
}

// The market.

function buy(listing, ctx) {
  const modal = openModal(tr('Покупка питомца'));
  modal.body.append(el('p', { textContent: tr('Купить {0} ({1}, {2} ур.) за {3}?', listing.pet.name, petKindName(listing.pet.kind), listing.pet.level, votesAmount(listing.pet.salePrice)) }));
  modal.actions.append(
    button(tr('Купить'), () => {
      modal.close();
      ctx.run({ action: 'buy', petId: listing.pet.id }, () => tr('Питомец ваш: {0}.', listing.pet.name));
    }),
    button(tr('Отмена'), modal.close, { secondary: true }));
}

function listingRow(listing, ctx) {
  const mine = listing.user?.id === ctx.me.id;
  const action = mine
    ? button(tr('Снять с продажи'), () => ctx.run({ action: 'unsell', petId: listing.pet.id }), { secondary: true })
    : button(tr('Купить'), () => buy(listing, ctx));
  action.classList.add('pet-busy');
  return el('tr', {},
    el('td', {}, el('span', { className: 'pet-entrant' }, petImage(listing.pet.kind, 32, listing.pet.outfit), el('b', { textContent: listing.pet.name }))),
    el('td', { textContent: `${petKindName(listing.pet.kind)}, ${petRarityName(listing.pet.rarity)}, ${tr('{0} ур.', listing.pet.level)}, ${tr('рейтинг {0}', listing.pet.rating)}` }),
    el('td', {}, ownerLink(listing.user)),
    el('td', { textContent: votesAmount(listing.pet.salePrice) }),
    el('td', {}, action));
}

function sellNode(ctx) {
  const ready = ctx.pets.filter((p) => !p.salePrice && !p.saleAt);
  if (!ready.length) return el('p', { className: 'hint', textContent: tr('Продать можно питомца, который у вас не меньше недели.') });

  const select = petSelect(ready);
  const price = el('input', { type: 'number', min: '1', step: '1', className: 'pet-votes' });
  const least = () => minPrices[ready.find((p) => p.id === select.value)?.rarity] ?? minPrices.common;
  const fit = () => { price.min = String(least()); if (Number(price.value) < least()) price.value = String(least()); };
  select.addEventListener('change', fit);
  fit();
  const sell = button(tr('Выставить'), () => ctx.run({ action: 'sell', petId: select.value, votes: Number(price.value) }));
  sell.classList.add('pet-busy');
  return el('div', { className: 'pet-stance-row' }, el('label', { textContent: tr('Продать:') }), select, price, sell);
}

function marketBlock(arena, ctx) {
  return [
    arena.market.length
      ? el('table', { className: 'info pet-league-table' }, ...arena.market.map((l) => listingRow(l, ctx)))
      : el('p', { className: 'hint', textContent: tr('Сейчас никто не продаёт питомцев.') }),
    sellNode(ctx),
  ];
}

// petWindow is a block of the pages of pets.
export const petWindow = gameWindow;

// leagueNodes are the blocks of the arena for ctx: { me, pets, run(body,
// then) } — run does an action and draws the arena it answers with, and
// what then(answer) returns, a node or a text, over it.
export function leagueNodes(arena, ctx) {
  const got = arena.achievements.length;

  return [
    petWindow('/static/icons/pet/quests.svg', tr('Задания дня'), questsBlock(arena)),
    petWindow('/static/icons/pet/boss.svg', tr('Босс недели'), bossBlock(arena, ctx)),
    petWindow('/static/icons/pet/cup.svg', tr('Турнир выходного дня'), tournamentBlock(arena, ctx)),
    petWindow('/static/icons/pet/season.svg', tr('Сезон'), seasonBlock(arena)),
    petWindow('/static/icons/pet/badge.svg', tr('Достижения: {0} из {1}', got, petAchievements.length), achievementsBlock(arena)),
    petWindow('/static/icons/pet/market.svg', tr('Рынок питомцев'), marketBlock(arena, ctx)),
  ];
}
