// "Столы" of "Покер" (/casino?game=poker): no-limit Texas hold'em with
// other people. A quick game seats one at a table of the stake with most
// players; one's own table is open to all or only to those invited. Chips
// are bought with votes on sitting down and become votes again on leaving.
// The server deals, keeps the turns and the clock; the page draws the
// table — an oval of felt with the seats around, the viewer at the bottom,
// chips of the bets before each seat, the board and the pot in the middle —
// and asks for it again every second while it is open and seen.

import { api } from './api.js';
import { HOLDEM_STAKES, groupedVotes, holdemAction, holdemBetting, holdemBlinds, holdemBuyIn, holdemChips, holdemPlace, holdemStreet, playersCount, plural, pokerCard, pokerHandName, tablesCount, votesCount } from './labels.js';
import { $, avatarURL, el, fullName, hideError, pagePath, showError } from './layout.js';
import { tr } from './i18n.js';

// cardNode is a card face up, face down ("back") or a place for one, as
// the table against the dealer draws them.
function cardNode(card) {
  if (card === 'back') return el('span', { className: 'poker-card back' });

  const face = pokerCard(card);
  if (!face) return el('span', { className: 'poker-card slot' });

  const corner = (flip) => el('span', { className: flip ? 'poker-corner flip' : 'poker-corner' },
    el('span', { textContent: face.rank }), el('br'), el('span', { textContent: face.suit }));
  const node = el('span', { className: face.red ? 'poker-card poker-red' : 'poker-card' },
    corner(false), el('span', { className: 'poker-pip', textContent: face.suit }), corner(true));
  node.dataset.card = card;
  node.setAttribute('role', 'img');
  node.setAttribute('aria-label', `${face.rank}${face.suit}`);

  return node;
}

// chipsNode is a stack of chips with the amount beside it.
function chipsNode(amount, className) {
  return el('span', { className },
    el('span', { className: 'holdem-chips' }, ...holdemChips(amount).map((value) => el('span', { className: `holdem-chip chip-${value}` }))),
    el('span', { className: 'holdem-amount', textContent: groupedVotes(amount) }),
  );
}

// holdem sets up the tables; the casino gives whether one is signed in,
// the balance and a way to go to another address of the casino. It
// returns show, called with the address each time the poker tab shows.
export function holdem(casino) {
  let table = null;
  let tableID = '';
  let polling = 0;
  // fetchedAt and turnMs tell the clock of the turn between the answers.
  let fetchedAt = 0;
  let turnMs = 0;
  let busy = false;
  let shownBoard = [];

  const stakes = HOLDEM_STAKES.map((bb) => el('option', { value: String(bb), textContent: tr('Блайнды {0}', holdemBlinds(bb)) }));
  $('holdem-stake').replaceChildren(...stakes);
  $('holdem-stake').value = '10';
  for (let n = 2; n <= 9; n++) $('holdem-seats').append(el('option', { value: String(n), textContent: String(n) }));
  $('holdem-seats').value = '6';

  const balance = () => (casino.signedIn ? casino.balance() : undefined);
  const suggest = (input, bb) => {
    input.min = String(bb * 20);
    input.max = String(bb * 200);
    input.value = String(holdemBuyIn(bb, balance()));
  };
  const quickBuyIn = () => suggest($('holdem-quick-buyin'), Number($('holdem-stake').value));
  const ownBuyIn = () => suggest($('holdem-own-buyin'), Math.max(2, Math.floor(Number($('holdem-blind').value) || 2)));
  $('holdem-stake').addEventListener('change', quickBuyIn);
  $('holdem-blind').addEventListener('change', ownBuyIn);

  // run does a request with the buttons locked and shows its error.
  async function run(request) {
    if (busy) return;
    busy = true;
    hideError($('error'));
    try {
      await request();
    } catch (e) {
      showError($('error'), e);
    } finally {
      busy = false;
      if (table) drawControls();
    }
  }

  function open(id) {
    casino.navigate(`/casino?game=poker&table=${encodeURIComponent(id)}`);
  }

  // seated takes the answer of sitting down or leaving: the table, null
  // once it closed, and the balance.
  function seated(answer) {
    casino.showBalance(answer.balance);
    if (!answer.table) {
      casino.navigate('/casino?game=poker');
      return;
    }
    if (answer.table.id !== tableID) {
      open(answer.table.id);
      return;
    }
    take(answer.table);
  }

  $('holdem-quick').addEventListener('submit', (event) => {
    event.preventDefault();
    run(async () => seated(await api.quickHoldem(Number($('holdem-stake').value), Number($('holdem-quick-buyin').value))));
  });
  $('holdem-create').addEventListener('submit', (event) => {
    event.preventDefault();
    run(async () => seated(await api.createHoldemTable({
      name: $('holdem-name').value.trim(),
      private: $('holdem-private').checked,
      seats: Number($('holdem-seats').value),
      bigBlind: Number($('holdem-blind').value),
      buyIn: Number($('holdem-own-buyin').value),
    })));
  });

  function tableRow(t) {
    const link = el('a', { href: `/casino?game=poker&table=${encodeURIComponent(t.id)}`, textContent: t.name || tr('Стол') });
    link.addEventListener('click', (event) => {
      if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey) return;
      event.preventDefault();
      open(t.id);
    });

    return el('div', { className: 'holdem-row' },
      el('div', { className: 'holdem-row-name' }, link, t.private ? el('span', { className: 'hint', textContent: ` · ${tr('закрытый')}` }) : null),
      el('div', { className: 'hint', textContent: `${tr('Блайнды {0}', holdemBlinds(t.bigBlind))} · ${playersCount(t.players, t.seats.length)} · ${holdemStreet(t.street)}` }),
    );
  }

  async function showLobby() {
    const sections = casino.signedIn ? ['mine', 'invited', 'public'] : ['public'];
    const lists = await Promise.all(sections.map((s) => api.holdemTables(s).catch((e) => { showError($('error'), e); return { tables: [] }; })));
    const of = Object.fromEntries(sections.map((s, i) => [s, lists[i].tables ?? []]));

    for (const section of ['mine', 'invited', 'public']) {
      const tables = of[section] ?? [];
      $(`holdem-${section}`).hidden = !tables.length && section !== 'public';
      $(`holdem-${section}-count`).textContent = tablesCount(tables.length);
      $(`holdem-${section}-rows`).replaceChildren(...tables.map(tableRow));
    }
    $('holdem-public-empty').hidden = Boolean(of.public?.length);
  }

  // take is a table read: drawn whole.
  function take(t) {
    table = t;
    fetchedAt = performance.now();
    turnMs = t.turnMs ?? 0;
    draw();
  }

  function seatNode(seat, i) {
    const place = holdemPlace(i, table.seats.length, table.you);
    const node = el('div', { className: 'holdem-seat' });
    node.style.left = `${place.x}%`;
    node.style.top = `${place.y}%`;

    if (!seat.player) {
      node.classList.add('empty');
      if (casino.signedIn && table.you < 0) {
        const sit = el('button', { className: 'button', type: 'button', textContent: tr('Сесть') });
        sit.addEventListener('click', () => run(async () => seated(await api.sitHoldem(table.id, i, Number($('holdem-buyin').value)))));
        node.append(sit);
      } else {
        node.append(el('span', { className: 'holdem-free', textContent: tr('Свободно') }));
      }
      return node;
    }

    const betting = holdemBetting(table.street);
    node.classList.toggle('turn', betting && table.turn === i);
    node.classList.toggle('folded', seat.folded);
    node.classList.toggle('away', seat.away);
    node.classList.toggle('winner', seat.won > 0);
    node.classList.toggle('mine', i === table.you);

    const cards = seat.cards?.length ? seat.cards : seat.hasCards ? ['back', 'back'] : [];
    const cardNodes = cards.map((card) => {
      const c = cardNode(card);
      if (seat.best?.includes(card)) c.classList.add('best');
      return c;
    });

    const name = el('a', { className: 'holdem-name', href: pagePath(seat.player), textContent: fullName(seat.player) });
    const status = seat.won > 0 ? tr('Выигрыш +{0}', groupedVotes(seat.won))
      : seat.away ? tr('Отошёл')
        : seat.leaving ? tr('Уходит')
          : seat.allIn ? tr('Ва-банк')
            : holdemAction(seat.last);

    node.append(...[
      el('div', { className: seat.best?.length ? 'holdem-cards judged' : 'holdem-cards' }, ...cardNodes),
      el('div', { className: 'holdem-plate' },
        el('img', { className: 'holdem-avatar', src: avatarURL(seat.player, 'small'), alt: '' }),
        el('div', { className: 'holdem-who' },
          name,
          el('div', { className: 'holdem-stack', textContent: `${groupedVotes(seat.stack)} ${plural(seat.stack, ['фишка', 'фишки', 'фишек'])}` }),
        ),
      ),
      el('div', { className: 'holdem-status', textContent: status }),
      seat.hand ? el('div', { className: 'poker-hand', textContent: pokerHandName(seat.hand) }) : null,
      betting && table.turn === i ? el('div', { className: 'holdem-clock' }, el('span', { className: 'holdem-clock-bar' })) : null,
    ].filter(Boolean));
    if (table.button === i && table.street !== 'waiting') node.append(el('span', { className: 'holdem-dealer', textContent: 'D', title: tr('Дилер') }));

    return node;
  }

  // betNode is the chips a seat put in this street, between it and the
  // middle of the table.
  function betNode(seat, i) {
    if (!seat.player || !seat.bet) return null;

    const place = holdemPlace(i, table.seats.length, table.you);
    const node = chipsNode(seat.bet, 'holdem-bet');
    node.style.left = `${50 + (place.x - 50) * 0.68}%`;
    node.style.top = `${50 + (place.y - 50) * 0.62}%`;

    return node;
  }

  function draw() {
    const t = table;
    $('holdem-title').textContent = t.name || tr('Стол');
    $('holdem-info').textContent = `${tr('Блайнды {0}', holdemBlinds(t.bigBlind))} · ${playersCount(t.players, t.seats.length)}${t.private ? ` · ${tr('закрытый')}` : ''}`;
    $('holdem-street').textContent = t.street === 'waiting' ? tr('Ждём игроков: нужно хотя бы двое.') : `${holdemStreet(t.street)} · ${tr('раздача №{0}', t.hand)}`;

    const board = [0, 1, 2, 3, 4].map((k) => {
      const card = t.board[k];
      const node = cardNode(card ?? '');
      if (card && shownBoard[k] !== card) {
        node.classList.add('dealt');
        node.style.animationDelay = `${k * 90}ms`;
      }
      return node;
    });
    shownBoard = [...t.board];

    const bets = t.seats.reduce((sum, s) => sum + (s.bet || 0), 0);
    const pot = t.pot - bets;
    $('holdem-felt').replaceChildren(
      el('div', { className: 'holdem-middle' },
        el('div', { className: 'poker-cards holdem-board' }, ...board),
        pot > 0 ? chipsNode(pot, 'holdem-pot') : el('span', { className: 'holdem-pot empty' }),
        t.pot > 0 ? el('div', { className: 'holdem-pot-total', textContent: tr('Банк: {0}', groupedVotes(t.pot)) }) : null,
      ),
      ...t.seats.map(betNode).filter(Boolean),
      ...t.seats.map(seatNode),
    );

    $('holdem-invite').hidden = !(t.private && t.you >= 0);
    $('holdem-sit').hidden = !(casino.signedIn && t.you < 0);
    if (t.you < 0 && casino.signedIn && document.activeElement !== $('holdem-buyin')) suggest($('holdem-buyin'), t.bigBlind);
    $('holdem-sit-hint').textContent = tr('Фишки — от {0} до {1} голосов. Выберите свободное место.', groupedVotes(t.minBuyIn), groupedVotes(t.maxBuyIn));
    drawControls();
    tick();
  }

  // drawControls sets the buttons of the viewer by the table.
  function drawControls() {
    const t = table;
    const me = t.you >= 0 ? t.seats[t.you] : null;
    const mine = Boolean(me) && holdemBetting(t.street) && t.turn === t.you;

    $('holdem-controls').hidden = !me;
    if (!me) return;

    $('holdem-actions').hidden = !mine;
    $('holdem-back').hidden = !me.away;
    $('holdem-wait').hidden = mine || me.away;
    $('holdem-wait').textContent = me.folded && holdemBetting(t.street) ? tr('Вы сбросили карты. Ждите следующей раздачи.')
      : holdemBetting(t.street) ? tr('Ход другого игрока.') : '';
    for (const button of $('holdem-controls').querySelectorAll('button, input')) button.disabled = busy;
    if (!mine) return;

    $('holdem-check').hidden = t.toCall > 0;
    $('holdem-call').hidden = t.toCall <= 0;
    $('holdem-call').textContent = tr('Колл {0}', groupedVotes(Math.min(t.toCall, me.stack)));

    const canRaise = t.raiseMost > 0 && t.raiseLeast <= t.raiseMost;
    $('holdem-raise-box').hidden = !canRaise;
    if (canRaise) {
      const range = $('holdem-raise-range');
      const amount = $('holdem-raise-amount');
      range.min = amount.min = String(t.raiseLeast);
      range.max = amount.max = String(t.raiseMost);
      const value = Math.min(t.raiseMost, Math.max(t.raiseLeast, Number(amount.value) || 0));
      range.value = amount.value = String(value);
      raiseLabel();
    }
  }

  function raiseLabel() {
    const value = Number($('holdem-raise-amount').value);
    $('holdem-raise').textContent = (table.currentBet > 0 ? tr('Рейз до {0}', groupedVotes(value)) : tr('Ставка {0}', groupedVotes(value)));
  }

  function act(action, amount = 0) {
    run(async () => take(await api.actHoldem(table.id, action, amount)));
  }

  $('holdem-fold').addEventListener('click', () => act('fold'));
  $('holdem-check').addEventListener('click', () => act('check'));
  $('holdem-call').addEventListener('click', () => act('call'));
  $('holdem-allin').addEventListener('click', () => act('allin'));
  $('holdem-raise').addEventListener('click', () => act('raise', Math.floor(Number($('holdem-raise-amount').value))));
  $('holdem-raise-range').addEventListener('input', () => {
    $('holdem-raise-amount').value = $('holdem-raise-range').value;
    raiseLabel();
  });
  $('holdem-raise-amount').addEventListener('input', () => {
    $('holdem-raise-range').value = $('holdem-raise-amount').value;
    raiseLabel();
  });
  for (const quick of $('holdem-raise-box').querySelectorAll('[data-pot]')) {
    quick.addEventListener('click', () => {
      // A part of the pot over the call: "½ банка", "банк".
      const pot = table.pot + table.toCall;
      const value = Math.min(table.raiseMost, Math.max(table.raiseLeast, table.currentBet + Math.floor(pot * Number(quick.dataset.pot))));
      $('holdem-raise-amount').value = $('holdem-raise-range').value = String(value);
      raiseLabel();
    });
  }
  $('holdem-back').addEventListener('click', () => run(async () => take(await api.backHoldem(table.id))));
  $('holdem-leave').addEventListener('click', () => run(async () => seated(await api.leaveHoldem(table.id))));
  $('holdem-invite').addEventListener('submit', (event) => {
    event.preventDefault();
    const nicknames = $('holdem-nicknames').value.split(/[\s,]+/).map((n) => n.replace(/^@/, '')).filter(Boolean);
    if (!nicknames.length) return;
    run(async () => {
      take(await api.inviteHoldem(table.id, nicknames));
      $('holdem-nicknames').value = '';
      $('holdem-invited').hidden = false;
    });
  });
  $('holdem-to-lobby').addEventListener('click', (event) => {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey) return;
    event.preventDefault();
    casino.navigate('/casino?game=poker');
  });

  // tick runs the bar of the clock of the turn.
  function tick() {
    const bar = $('holdem-felt').querySelector('.holdem-clock-bar');
    if (!bar) return;

    const left = Math.max(0, turnMs - (performance.now() - fetchedAt));
    bar.style.width = `${Math.min(100, (left / 30000) * 100)}%`;
    bar.classList.toggle('late', left < 8000);
  }
  setInterval(() => { if (table && !$('holdem-table').hidden) tick(); }, 250);

  // poll asks for the table again while it is open and seen; the same
  // version comes back without the people.
  async function poll(id, round) {
    if (round !== polling || id !== tableID) return;
    if (!document.hidden && !$('holdem-table').hidden && !busy) {
      try {
        const answer = await api.holdemTable(id, table?.version);
        if (round !== polling) return;
        if (answer.same) {
          fetchedAt = performance.now();
          turnMs = answer.turnMs;
        } else {
          take(answer);
        }
      } catch (e) {
        if (round === polling) showError($('error'), e);
      }
    }
    setTimeout(() => poll(id, round), 1000);
  }

  async function showTable(id) {
    if (id !== tableID) {
      table = null;
      shownBoard = [];
      $('holdem-invited').hidden = true;
    }
    tableID = id;
    polling++;
    try {
      take(await api.holdemTable(id));
      setTimeout(() => poll(id, polling), 1000);
    } catch (e) {
      showError($('error'), e);
      $('holdem-table').hidden = true;
    }
  }

  if (!casino.signedIn) {
    $('holdem-forms').hidden = true;
    $('holdem-guest').hidden = false;
  }
  quickBuyIn();
  ownBuyIn();

  return function show(url) {
    const params = new URL(url, location.href).searchParams;
    const id = params.get('table') ?? '';
    $('holdem-lobby').hidden = Boolean(id);
    $('holdem-table').hidden = !id;
    if (id) {
      showTable(id);
    } else {
      tableID = '';
      polling++;
      showLobby();
      quickBuyIn();
    }
  };
}
