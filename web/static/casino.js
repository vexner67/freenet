// "Казино" (/casino): three games for votes, chosen by the tabs over them.
// "Огонёк": a spin costs votes and wins one of the prizes, votes again —
// the administrators tune how likely each is in the admin panel. A light
// runs over the prizes, slowing down, and stops on the one won.
// "Прицел" (?game=aim, once ?game=bet): the player bets as many votes as
// they like on a multiplier and gets the bet multiplied or loses it; a
// marker runs over a strip of 0 to 100 and stops on the roll, a win in the
// green zone in the middle, as wide as the chance.
// "Ставки" (?game=events): events the administrators make, each with its
// outcomes; people stake votes on an outcome, and once it is resolved
// those who guessed share all staked. "Популярное" are the events taking
// stakes, the most staked on first. At the right is the balance, as in
// "Голоса" of the settings.

import { api, currentUser, started } from './api.js';
import { aimZone, betAmount, betChance, betEventOdds, betEventState, betEventShare, betMultiplier, betOdds, betPrize, betRoll, casinoChance, casinoPlan, plural, spinsCount, votesCount, wagersCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const answered = started(api.casino());

// Without motion the light lands at once.
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

let cells = [];
let lit = 0;
const session = { spins: 0, spent: 0, won: 0 };
const wagers = { count: 0, spent: 0, won: 0 };
let balance = 0;
let playing = false;
let aimShown = false;
let signedIn = false;
// The section of "Ставки" shown and the one being read, so that a late
// answer does not draw over a newer one.
let section = '';
let reading = 0;
// refreshBet redraws the form of "Ставка" once it is made.
let refreshBet = () => {};

const sleep = (ms) => new Promise((resolve) => { setTimeout(resolve, ms); });

function showBoard(prizes) {
  cells = prizes.map((votes) => el('div', { className: votes > 0 ? 'casino-cell' : 'casino-cell casino-empty' },
    el('span', { className: 'casino-votes', textContent: votes > 0 ? String(votes) : '×' }),
    el('span', { className: 'casino-unit', textContent: votes > 0 ? plural(votes, ['голос', 'голоса', 'голосов']) : tr('Пусто') }),
  ));
  $('board').replaceChildren(...cells);
  light(0);
}

function light(i) {
  cells[lit]?.classList.remove('lit');
  lit = i;
  cells[lit]?.classList.add('lit');
}

// setPlaying locks both games while a spin or a bet is under way: each
// answer sets the balance, and one that came after the other would show an
// old one; the tabs and the stake stay as they are too.
function setPlaying(on) {
  playing = on;
  $('spin').disabled = on;
  for (const control of $('bet-form').querySelectorAll('input, button')) control.disabled = on;
  for (const control of $('events').querySelectorAll('input, button')) control.disabled = on;
  for (const tab of tabs()) tab.setAttribute('aria-disabled', String(on));
  refreshBet();
}

function showBalance(votes) {
  balance = votes;
  $('votes').textContent = votes;
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('balance').hidden = false;
}

// run moves the light round the board to the prize won.
async function run(prize) {
  cells.forEach((c) => c.classList.remove('won'));
  if (!still) {
    for (const step of casinoPlan(cells.length, lit, prize)) {
      await sleep(step.delay);
      light(step.cell);
    }
  }
  light(prize);
  cells[prize].classList.add('won');
}

function showResult(votes) {
  const result = $('result');

  result.className = votes > 0 ? 'casino-result casino-win' : 'casino-result';
  result.textContent = votes > 0 ? tr('Вы выиграли {0}!', votesCount(votes)) : tr('В этот раз ничего. Попробуйте ещё!');
  result.hidden = false;

  $('session').textContent = tr('За этот заход: {0}, потрачено {1}, выиграно {2}.',
    spinsCount(session.spins), votesCount(session.spent), votesCount(session.won));
  $('session').hidden = false;
}

async function spin(price) {
  if (playing) return;

  hideError($('error'));
  $('result').hidden = true;
  setPlaying(true);

  try {
    const won = await api.spinCasino();

    // The price is gone at once, the prize comes when the light stops.
    showBalance(won.balance - won.votes);
    await run(won.prize);
    session.spins++;
    session.spent += price;
    session.won += won.votes;
    showBalance(won.balance);
    showResult(won.votes);
  } catch (e) {
    showError($('error'), e);
  } finally {
    setPlaying(false);
  }
}

// tabs are the links that switch the game or the section of "Ставки".
function tabs() {
  return [$('tab-wheel'), $('tab-aim'), $('tab-events'), $('section-all'), $('section-popular')];
}

// The game shown: "wheel", "aim" or "events", by the address; "bet" is
// the name "Прицел" had.
function gameOf(url) {
  const game = new URL(url, location.href).searchParams.get('game');

  if (game === 'aim' || game === 'bet') return aimShown ? 'aim' : 'wheel';

  return game === 'events' ? 'events' : 'wheel';
}

function sectionOf(url) {
  return new URL(url, location.href).searchParams.get('section') === 'popular' ? 'popular' : 'all';
}

function showGame(url) {
  const game = gameOf(url);

  $('tab-wheel').classList.toggle('active', game === 'wheel');
  $('tab-aim').classList.toggle('active', game === 'aim');
  $('tab-events').classList.toggle('active', game === 'events');
  $('wheel-game').hidden = game !== 'wheel';
  $('aim-game').hidden = game !== 'aim';
  $('events-game').hidden = game !== 'events';
  if (game === 'events') showEvents(sectionOf(url));
}

// The tabs switch the game without a reload: the balance and the tally of
// the visit stay.
function watchTabs() {
  for (const tab of tabs()) {
    tab.addEventListener('click', (event) => {
      if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey) return;
      event.preventDefault();
      if (playing) return;
      history.pushState(null, '', tab.href);
      showGame(tab.href);
    });
  }
  addEventListener('popstate', () => showGame(location.href));
  $('casino-tabs').hidden = false;
}

// betting is the form of "Ставка": the amount, the multiplier, the strip
// with the winning part and the button.
function betting(bets, signedIn) {
  const amount = $('bet-amount');
  let chosen = bets.multipliers.includes(200) ? 200 : bets.multipliers[0];

  amount.min = String(bets.least);
  amount.max = String(bets.most);
  amount.value = String(Math.min(bets.most, Math.max(bets.least, 10)));
  $('bet-limits').textContent = tr('От {0} до {1} за ставку.', bets.least, votesCount(bets.most));

  const buttons = bets.multipliers.map((m) => {
    const button = el('button', { className: 'casino-cell casino-multiplier', type: 'button' },
      el('span', { className: 'casino-votes', textContent: betMultiplier(m) }),
      el('span', { className: 'casino-unit', textContent: tr('шанс {0}', betChance(betOdds(bets.return, m))) }),
    );
    button.setAttribute('role', 'radio');
    button.addEventListener('click', () => {
      if (playing) return;
      chosen = m;
      update();
    });
    button.multiplier = m;

    return button;
  });
  $('multipliers').replaceChildren(...buttons);

  const stake = () => Math.floor(Number(amount.value));

  function update() {
    const odds = betOdds(bets.return, chosen);
    const votes = stake();
    const fits = votes >= bets.least && votes <= bets.most;

    for (const b of buttons) {
      b.classList.toggle('lit', b.multiplier === chosen);
      b.setAttribute('aria-checked', String(b.multiplier === chosen));
    }
    const zone = aimZone(odds);

    $('dice-win').style.left = `${zone.low / 100}%`;
    $('dice-win').style.width = `${odds / 100}%`;
    $('bet-sum').textContent = fits
      ? tr('Шанс выигрыша {0}, выигрыш — {1}.', betChance(odds), votesCount(betPrize(votes, chosen)))
      : tr('Ставка — от {0} до {1}.', bets.least, votesCount(bets.most));
    $('bet').textContent = fits ? tr('Поставить {0}', votesCount(votes)) : tr('Поставить');
    $('bet').disabled = playing || !fits;
  }

  for (const quick of $('bet-form').querySelectorAll('[data-quick]')) {
    quick.addEventListener('click', () => {
      amount.value = String(betAmount(stake(), quick.dataset.quick, { ...bets, balance: signedIn ? balance : undefined }));
      update();
    });
  }
  amount.addEventListener('input', update);

  // roll moves the marker over the strip to the roll.
  async function roll(to) {
    const mark = $('dice-mark');

    mark.classList.remove('won', 'lost', 'near-start', 'near-end');
    mark.hidden = false;
    $('dice-roll').textContent = '';
    if (!still) {
      mark.classList.add('rolling');
      mark.style.left = '0%';
      // Two frames, so that the marker starts from the left before it runs.
      await new Promise((resolve) => { requestAnimationFrame(() => requestAnimationFrame(resolve)); });
      mark.classList.remove('rolling');
    }
    mark.style.left = `${to / 100}%`;
    if (!still) await sleep(1100);
    // The number stands over the marker, not past the ends of the strip.
    mark.classList.toggle('near-start', to < 1000);
    mark.classList.toggle('near-end', to > 9000);
    $('dice-roll').textContent = betRoll(to);
  }

  $('bet-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!signedIn || playing) return;

    const votes = stake();
    const multiplier = chosen;

    hideError($('error'));
    $('bet-result').hidden = true;
    setPlaying(true);

    try {
      const wager = await api.betCasino(votes, multiplier);

      // The bet is gone at once, the win comes when the marker stops.
      showBalance(wager.balance - wager.votes);
      await roll(wager.roll);
      $('dice-mark').classList.add(wager.won ? 'won' : 'lost');
      wagers.count++;
      wagers.spent += votes;
      wagers.won += wager.votes;
      showBalance(wager.balance);
      showWager(wager);
    } catch (e) {
      showError($('error'), e);
    } finally {
      setPlaying(false);
    }
  });

  refreshBet = update;
  if (signedIn) {
    $('bet').hidden = false;
  } else {
    $('bet-guest').hidden = false;
  }
  update();
}

function showWager(wager) {
  const result = $('bet-result');

  result.className = wager.won ? 'casino-result casino-win' : 'casino-result';
  const zone = aimZone(wager.chance);

  result.textContent = wager.won
    ? tr('Выпало {0} — в зелёном поле. Вы выиграли {1}!', betRoll(wager.roll), votesCount(wager.votes))
    : tr('Выпало {0}, а зелёное поле — от {1} до {2}. Ставка проиграна.', betRoll(wager.roll), betRoll(zone.low), betRoll(zone.high));
  result.hidden = false;

  $('bet-session').textContent = tr('За этот заход: {0}, поставлено {1}, выиграно {2}.',
    wagersCount(wagers.count), votesCount(wagers.spent), votesCount(wagers.won));
  $('bet-session').hidden = false;
}

// eventMine is what the viewer staked and got back.
function eventMine(e) {
  const mine = e.outcomes.reduce((sum, o) => sum + o.mine, 0);
  if (mine === 0) return null;
  if (e.state === 'cancelled') return tr('Вам вернулось {0}.', votesCount(e.payout));
  if (e.state !== 'resolved') return tr('Вы поставили {0}.', votesCount(mine));

  return e.payout > 0 ? tr('Вы поставили {0} и получили {1}.', votesCount(mine), votesCount(e.payout)) : tr('Вы поставили {0}, но не угадали.', votesCount(mine));
}

function eventNode(e) {
  const taking = e.open && signedIn;
  const amount = el('input', { className: 'casino-amount', type: 'number', inputMode: 'numeric', min: '1', max: '1000000', value: '10', required: true });
  const mine = eventMine(e);

  const outcomes = e.outcomes.map((o, i) => {
    const percent = betEventShare(o.staked, e.staked);
    const bar = el('div', { className: 'bet-outcome-bar' });
    const odds = e.open ? betEventOdds(o.staked, e.staked) : '';
    const stake = taking ? el('button', { className: 'button', type: 'button', textContent: tr('Поставить') }) : null;

    bar.style.width = `${percent}%`;
    stake?.addEventListener('click', () => stakeOn(e, i, Math.floor(Number(amount.value))));

    return el('div', { className: e.winner === i ? 'bet-outcome won' : 'bet-outcome' },
      el('div', { className: 'bet-outcome-head' },
        el('b', { className: 'bet-outcome-name', textContent: o.name }),
        el('span', { className: 'bet-outcome-sum', textContent: [votesCount(o.staked), o.staked > 0 ? casinoChance(percent) : '', odds].filter(Boolean).join(' · ') }),
        stake,
      ),
      el('div', { className: 'bet-outcome-track' }, bar),
      o.mine > 0 ? el('div', { className: 'bet-outcome-mine', textContent: tr('Ваша ставка: {0}', votesCount(o.mine)) }) : null,
    );
  });

  return el('div', { className: 'bet-event' },
    el('div', { className: 'bet-event-question', textContent: e.question }),
    el('div', { className: 'bet-event-meta', textContent: tr('Поставлено: {0}. Участников: {1}. {2}', votesCount(e.staked), e.people, betEventState(e)) }),
    taking ? el('div', { className: 'casino-stake bet-event-stake' }, el('span', { className: 'casino-label', textContent: tr('Ставка:') }), amount, el('span', { textContent: tr('голосов на исход') })) : null,
    ...outcomes,
    mine ? el('div', { className: 'bet-event-mine', textContent: mine }) : null,
  );
}

// showEvents reads and draws the events of a section of "Ставки".
async function showEvents(shown) {
  $('section-all').classList.toggle('active', shown === 'all');
  $('section-popular').classList.toggle('active', shown === 'popular');
  section = shown;

  const read = ++reading;

  try {
    const { events } = await api.betEvents(shown === 'popular');
    if (read !== reading) return;

    $('events').replaceChildren(...events.map(eventNode));
    $('events-empty').textContent = shown === 'popular' ? tr('Сейчас ни одно событие не принимает ставок.') : tr('Здесь пока нет событий.');
    $('events-empty').hidden = events.length > 0;
    $('events-guest').hidden = signedIn || events.length === 0;
    setPlaying(playing);
  } catch (e) {
    if (read === reading) showError($('error'), e);
  }
}

async function stakeOn(e, outcome, amount) {
  if (!signedIn || playing) return;

  hideError($('error'));
  setPlaying(true);

  try {
    const { balance: left } = await api.stakeBet(e.id, outcome, amount);

    showBalance(left);
  } catch (err) {
    showError($('error'), err);
  } finally {
    setPlaying(false);
  }
  await showEvents(section);
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { price, prizes, bets } = await answered;

  $('price').textContent = votesCount(price);
  signedIn = Boolean(me);
  if (me) showBalance(me.votes ?? 0);
  if (bets?.multipliers?.length) {
    aimShown = true;
    $('tab-aim').hidden = false;
    betting(bets, signedIn);
  }
  // An address of the time the game was "Ставка" becomes its new one.
  if (new URL(location.href).searchParams.get('game') === 'bet') {
    history.replaceState(null, '', aimShown ? '/casino?game=aim' : '/casino');
  }
  watchTabs();
  showBoard(prizes);
  showGame(location.href);

  if (me) {
    $('spin').textContent = tr('Крутить за {0}', votesCount(price));
    $('spin').hidden = false;
    $('spin').addEventListener('click', () => spin(price));
  } else {
    $('guest').hidden = false;
  }

  $('casino').hidden = false;
} catch (e) {
  showError($('error'), e);
}
