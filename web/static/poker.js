// "Покер" of "Азартные игры" (/casino?game=poker): casino hold'em against
// the dealer, as online casinos have it. The ante deals two cards to the
// player and three to the board; the player calls with twice the ante, and
// the turn, the river and the cards of the dealer open, or folds and loses
// the ante. The server deals and judges; the page draws the table: the
// dealer at the top with his cards face down until the call, the board,
// the player, the names of the hands, and once the hand is over the five
// cards of each hand lifted. A hand dealt and not played stays on the
// table: the page shows it again when it opens.

import { api } from './api.js';
import { POKER_PAID, betAmount, groupedVotes, handsCount, pokerCard, pokerHandName, pokerOutcome, pokerPay, votesCount } from './labels.js';
import { $, el, showError, hideError } from './layout.js';
import { tr } from './i18n.js';
import { jackpot } from './jackpot.js';

// The cards each row has: two for the dealer and the player, five on the
// board.
const rows = { dealer: 2, board: 5, player: 2 };

// cardNode is a card face up, face down ("back") or a place for one.
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

// table is what each row shows of a hand: nothing yet, a hand dealt or a
// hand over.
function table(hand) {
  if (!hand) return { dealer: [], board: [], player: [] };

  const over = hand.state !== 'dealt';
  const dealer = over && hand.dealer?.length ? hand.dealer : ['back', 'back'];

  return { dealer, board: hand.board ?? [], player: hand.player ?? [] };
}

// poker sets up the game with the table of the casino and the hand left
// dealt; the casino gives its own parts: whether one is signed in, the
// balance, the lock of all the games, the cheer and the jackpot.
export function poker(settings, dealt, casino) {
  const amount = $('poker-amount');
  const session = { count: 0, spent: 0, won: 0 };
  // shown is what each row shows, so that only new cards fly in.
  const shown = { dealer: [], board: [], player: [] };
  let hand = dealt;

  amount.min = String(settings.least);
  amount.max = String(settings.most);
  amount.value = String(dealt?.ante ?? Math.min(settings.most, Math.max(settings.least, 10)));
  $('poker-limits').textContent = tr('Анте — от {0} до {1}, колл вдвое больше.', groupedVotes(settings.least), votesCount(settings.most));

  const ante = () => Math.floor(Number(amount.value));
  const fits = (votes) => votes >= settings.least && votes <= settings.most;

  function showPays() {
    const lit = hand ? POKER_PAID.indexOf(hand.playerHand) : -1;
    const shownPays = POKER_PAID.map((name, i) => {
      const last = i === POKER_PAID.length - 1;
      const row = el('tr', {},
        el('td', { textContent: last ? tr('Стрит и ниже') : pokerHandName(name) }),
        el('td', { textContent: tr('{0} к 1', groupedVotes(settings.pays[i] ?? 1)) }),
      );
      row.classList.toggle('lit', Boolean(hand) && (i === lit || (last && lit < 0)));

      return row;
    });
    $('poker-pays-rows').replaceChildren(...shownPays);
  }

  function showRow(name, cards, best) {
    const nodes = [];
    for (let i = 0; i < rows[name]; i++) {
      const card = cards[i] ?? '';
      const node = cardNode(card);
      // A card that was not there flies in, one after another.
      if (card && shown[name][i] !== card) {
        node.classList.add('dealt');
        node.style.animationDelay = `${nodes.filter((n) => n.classList.contains('dealt')).length * 90}ms`;
      }
      if (best?.includes(card)) node.classList.add('best');
      nodes.push(node);
    }
    shown[name] = [...cards];
    $(`poker-${name}`).replaceChildren(...nodes);
    $(`poker-${name}`).classList.toggle('judged', Boolean(best));
  }

  function draw() {
    const cards = table(hand);
    // A hand played to the end lifts the five cards of each side.
    const judged = hand && hand.state !== 'dealt' && hand.state !== 'folded';

    showRow('dealer', cards.dealer, judged ? hand.dealerBest ?? [] : null);
    showRow('board', cards.board, judged ? hand.playerBest ?? [] : null);
    showRow('player', cards.player, judged ? hand.playerBest ?? [] : null);
    $('poker-player-hand').textContent = hand?.playerHand ? pokerHandName(hand.playerHand) : '';
    $('poker-dealer-hand').textContent = judged && hand.dealerHand ? pokerHandName(hand.dealerHand) : '';
    if (!hand) {
      $('poker-bets').textContent = '';
    } else if (hand.state === 'dealt' || hand.state === 'folded') {
      $('poker-bets').textContent = tr('Анте: {0}', votesCount(hand.ante));
    } else {
      $('poker-bets').textContent = tr('Анте: {0}, колл: {1}', votesCount(hand.ante), votesCount(hand.ante * 2));
    }
    showPays();
    refresh();
  }

  // refresh sets the buttons by the hand and by whether a game is under
  // way.
  function refresh() {
    const playing = casino.playing();
    const open = hand?.state === 'dealt';
    const votes = ante();

    for (const control of $('poker-form').querySelectorAll('input, button')) control.disabled = playing || open;
    if (!casino.signedIn) return;

    $('poker-deal').hidden = open;
    $('poker-deal').textContent = fits(votes) ? tr('Раздать за {0}', votesCount(votes)) : tr('Раздать');
    $('poker-deal').disabled = playing || !fits(votes);
    $('poker-call').hidden = !open;
    $('poker-fold').hidden = !open;
    if (open) $('poker-call').textContent = tr('Колл — {0}', votesCount(hand.ante * 2));
    $('poker-call').disabled = playing;
    $('poker-fold').disabled = playing;
  }

  function showResult() {
    const result = $('poker-result');
    const won = hand.state === 'won' || hand.state === 'unqualified';

    result.className = won ? 'casino-result casino-win' : 'casino-result';
    result.textContent = pokerOutcome(hand);
    result.hidden = false;
    if (won) casino.cheer(result, hand.payout, hand.ante * 3);

    $('poker-session').textContent = tr('За этот заход: {0}, поставлено {1}, выиграно {2}.',
      handsCount(session.count), votesCount(session.spent), votesCount(session.won));
    $('poker-session').hidden = false;
  }

  // play runs a request to the server with all the games locked.
  async function play(request) {
    if (casino.playing()) return;

    hideError($('error'));
    casino.setPlaying(true);
    try {
      await request();
    } catch (e) {
      showError($('error'), e);
    } finally {
      casino.setPlaying(false);
    }
  }

  $('poker-deal').addEventListener('click', () => play(async () => {
    const votes = ante();

    hand = await api.dealPoker(votes);
    $('poker-result').hidden = true;
    session.count++;
    session.spent += votes;
    casino.showBalance(hand.balance);
    draw();
  }));

  $('poker-call').addEventListener('click', () => play(async () => {
    const over = await api.callPoker(hand.id);

    session.spent += hand.ante * 2;
    session.won += over.payout;
    hand = over;
    casino.showBalance(over.balance);
    draw();
    showResult();
    if (over.payout >= casino.jackpotFrom()) jackpot(over.payout);
  }));

  $('poker-fold').addEventListener('click', () => play(async () => {
    hand = await api.foldPoker(hand.id);
    casino.showBalance(hand.balance);
    draw();
    showResult();
  }));

  for (const quick of $('poker-form').querySelectorAll('[data-quick]')) {
    quick.addEventListener('click', () => {
      amount.value = String(betAmount(ante(), quick.dataset.quick, { ...settings, balance: casino.signedIn ? casino.balance() : undefined }));
      refresh();
    });
  }
  amount.addEventListener('input', refresh);
  $('poker-form').addEventListener('submit', (event) => event.preventDefault());

  if (!casino.signedIn) $('poker-guest').hidden = false;
  draw();

  return refresh;
}
