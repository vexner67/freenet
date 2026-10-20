// "Казино" (/casino): two games for votes, chosen by the tabs over them.
// "Огонёк": a spin costs votes and wins one of the prizes, votes again —
// the administrators tune how likely each is in the admin panel. A light
// runs over the prizes, slowing down, and stops on the one won.
// "Ставка" (?game=bet): the player bets as many votes as they like on a
// multiplier and gets the bet multiplied or loses it; a marker runs over
// a strip of 0 to 100 and stops on the roll, a win below the chance. At
// the right is the balance, as in "Голоса" of the settings.

import { api, currentUser, started } from './api.js';
import { betAmount, betChance, betMultiplier, betOdds, betPrize, betRoll, casinoPlan, plural, spinsCount, votesCount, wagersCount } from './labels.js';
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
  for (const tab of [$('tab-wheel'), $('tab-bet')]) tab.setAttribute('aria-disabled', String(on));
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

// The game shown: "wheel" or "bet", by the address.
function gameOf(url) {
  return new URL(url, location.href).searchParams.get('game') === 'bet' ? 'bet' : 'wheel';
}

function showGame(game) {
  $('tab-wheel').classList.toggle('active', game === 'wheel');
  $('tab-bet').classList.toggle('active', game === 'bet');
  $('wheel-game').hidden = game !== 'wheel';
  $('bet-game').hidden = game !== 'bet';
}

// The tabs switch the game without a reload: the balance and the tally of
// the visit stay.
function watchTabs() {
  for (const tab of [$('tab-wheel'), $('tab-bet')]) {
    tab.addEventListener('click', (event) => {
      if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey) return;
      event.preventDefault();
      if (playing) return;
      history.pushState(null, '', tab.href);
      showGame(gameOf(tab.href));
    });
  }
  addEventListener('popstate', () => showGame(gameOf(location.href)));
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
  result.textContent = wager.won
    ? tr('Выпало {0} — меньше {1}. Вы выиграли {2}!', betRoll(wager.roll), betRoll(wager.chance), votesCount(wager.votes))
    : tr('Выпало {0}, а нужно было меньше {1}. Ставка проиграна.', betRoll(wager.roll), betRoll(wager.chance));
  result.hidden = false;

  $('bet-session').textContent = tr('За этот заход: {0}, поставлено {1}, выиграно {2}.',
    wagersCount(wagers.count), votesCount(wagers.spent), votesCount(wagers.won));
  $('bet-session').hidden = false;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { price, prizes, bets } = await answered;

  $('price').textContent = votesCount(price);
  if (me) showBalance(me.votes ?? 0);
  if (bets?.multipliers?.length) {
    betting(bets, Boolean(me));
    watchTabs();
  }
  showGame(bets?.multipliers?.length ? gameOf(location.href) : 'wheel');
  showBoard(prizes);

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
