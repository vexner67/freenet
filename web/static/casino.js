// "Азартные игры" (/casino): three games for votes, chosen by the tabs over them.
// "Огонёк": a spin costs votes and wins one of the prizes, votes again —
// the administrators tune how likely each is in the admin panel. A light
// runs over the prizes, slowing down, and stops on the one won.
// "Прицел" (?game=aim, once ?game=bet): the player bets as many votes as
// they like on a multiplier and gets the bet multiplied or loses it; a
// marker runs over a strip of 0 to 100 and stops on the roll, a win in the
// green zone in the middle, as wide as the chance.
// "Ракетка" (?game=rocket): a bet launches a rocket whose multiplier grows
// from ×1; the player cashes it before it explodes and gets the bet
// multiplied by what it showed, or sets the multiplier to cash at by
// itself. The server draws where it explodes at the launch and says it once
// the round is over; the page draws the flight by its own clock, asks how
// the round is twice a second and lets the server judge the cash.
// "Ставки" (?game=events): events the administrators make, each with its
// outcomes and their odds; people stake votes on an outcome, and once it is
// resolved a stake on the outcome won brings the stake by the odds it was
// made at (older events without odds share all staked among those who
// guessed). "Популярное" are the events taking stakes, the most staked on
// first. At the right is the balance, as in
// "Голоса" of the settings. "Огонёк", "Прицел" and "Ракетка" play by themselves too
// ("Автоигра"): as many rounds as chosen or while the votes last. A win of
// the jackpot of the casino takes the whole screen (jackpot.js).

import { api, currentUser, started } from './api.js';
import { aimZone, autoplayNext, betAmount, betChance, betEventOdds, betEventState, betEventShare, betMultiplier, betPrize, betRoll, casinoChance, casinoPlan, plural, rocketMultiplier, rocketScale, rocketShown, rocketTime, rocketsCount, spinsCount, typedOdds, votesCount, votesFontSize, wagersCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';
import { jackpot } from './jackpot.js';

const answered = started(api.casino());

// jackpotFrom is the win that takes the whole screen; the casino says it.
let jackpotFrom = Infinity;

// Without motion the light lands at once.
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

let cells = [];
let lit = 0;
const session = { spins: 0, spent: 0, won: 0 };
const wagers = { count: 0, spent: 0, won: 0 };
const flights = { count: 0, spent: 0, won: 0 };
let balance = 0;
let playing = false;
let aimShown = false;
let rocketShownTab = false;
let signedIn = false;
// The section of "Ставки" shown and the one being read, so that a late
// answer does not draw over a newer one.
let section = '';
let reading = 0;
// refreshBet and refreshRocket redraw the forms of "Прицел" and "Ракетка"
// once they are made.
let refreshBet = () => {};
let refreshRocket = () => {};
// auto is "Автоигра" under way: its button, which stops it, and whether
// it is asked to.
let auto = null;
// A round of "Автоигра" lasts at least roundGap, so that the rounds keep
// under the limit of the server, 30 a minute.
const roundGap = 2100;

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
  // The button that cashes the rocket is live while it flies.
  for (const control of $('rocket-form').querySelectorAll('input, button:not(#rocket-cash)')) control.disabled = on;
  for (const control of $('events').querySelectorAll('input, button')) control.disabled = on;
  for (const game of ['wheel', 'aim', 'rocket']) {
    $(`${game}-auto-count`).disabled = on;
    // The button of "Автоигра" under way stops it.
    if (!auto || auto.button !== $(`${game}-auto-start`)) $(`${game}-auto-start`).disabled = on;
  }
  for (const tab of tabs()) tab.setAttribute('aria-disabled', String(on));
  refreshBet();
  refreshRocket();
}

function showBalance(votes) {
  // The balance jumps when it grows.
  if (votes > balance && !$('balance').hidden && !still) {
    $('votes').classList.remove('bump');
    void $('votes').offsetWidth;
    $('votes').classList.add('bump');
  }
  balance = votes;
  $('votes').textContent = votes;
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
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

// cheer celebrates a win in the New Year casino with confetti, the more
// the bigger the win against what it cost; a big win (five times the cost
// and more) gets a big plaque.
let confetti = null;

function cheer(result, won, cost) {
  result.classList.toggle('casino-big', won >= cost * 5);
  if (still || !document.documentElement.classList.contains('festive')) return;
  if (!confetti) {
    confetti = el('div', { className: 'casino-confetti' });
    document.body.append(confetti);
  }

  const count = Math.min(140, 24 + Math.round((won / Math.max(cost, 1)) * 12));

  for (let i = 0; i < count; i++) {
    const piece = el('span', { className: 'casino-piece' });

    piece.style.left = `${Math.random() * 100}%`;
    piece.style.animationDelay = `${Math.random() * 0.5}s`;
    piece.style.setProperty('--dx', `${Math.round((Math.random() - 0.5) * 240)}px`);
    piece.style.setProperty('--turn', `${Math.round((Math.random() - 0.5) * 1440)}deg`);
    piece.style.setProperty('--fall', `${(1.8 + Math.random() * 1.6).toFixed(2)}s`);
    piece.addEventListener('animationend', () => piece.remove());
    confetti.append(piece);
  }
}

function showResult(votes, price) {
  const result = $('result');

  result.className = votes > 0 ? 'casino-result casino-win' : 'casino-result';
  if (votes > 0) cheer(result, votes, price);
  result.textContent = votes > 0 ? tr('Вы выиграли {0}!', votesCount(votes)) : tr('В этот раз ничего. Попробуйте ещё!');
  result.hidden = false;

  $('session').textContent = tr('За этот заход: {0}, потрачено {1}, выиграно {2}.',
    spinsCount(session.spins), votesCount(session.spent), votesCount(session.won));
  $('session').hidden = false;
}

async function spinOnce(price) {
  const won = await api.spinCasino();

  // The price is gone at once, the prize comes when the light stops.
  showBalance(won.balance - won.votes);
  await run(won.prize);
  session.spins++;
  session.spent += price;
  session.won += won.votes;
  showBalance(won.balance);
  showResult(won.votes, price);
  if (won.votes >= jackpotFrom) jackpot(won.votes);
}

async function spin(price) {
  if (playing) return;

  hideError($('error'));
  $('result').hidden = true;
  setPlaying(true);

  try {
    await spinOnce(price);
  } catch (e) {
    showError($('error'), e);
  } finally {
    setPlaying(false);
  }
}

// autoplay plays game ("wheel", "aim" or "rocket") round after round, each costing
// cost votes: as many rounds as chosen, while the balance pays for the next
// one, until the button stops it or a round fails.
async function autoplay(game, cost, round, count) {
  const start = $(`${game}-auto-start`);
  const state = $(`${game}-auto-state`);
  const times = Number($(`${game}-auto-count`).value);
  let played = 0;
  let next = 'go';
  let begun = 0;

  if (playing) return;
  hideError($('error'));
  auto = { button: start, stop: false };
  start.textContent = tr('Остановить');
  state.hidden = false;
  setPlaying(true);

  try {
    while (!auto.stop && (next = autoplayNext({ played, times, balance, cost })) === 'go') {
      await sleep(roundGap - (Date.now() - begun));
      if (auto.stop) break;
      state.textContent = times > 0 ? tr('Автоигра: {0} из {1}.', played + 1, times) : tr('Автоигра: {0}-я, пока есть голоса.', played + 1);
      begun = Date.now();
      await round();
      played++;
    }
  } catch (e) {
    showError($('error'), e);
  } finally {
    state.textContent = next === 'votes'
      ? tr('Автоигра остановлена: не хватает голосов. Сыграно: {0}.', count(played))
      : tr('Автоигра закончилась. Сыграно: {0}.', count(played));
    auto = null;
    start.textContent = tr('Запустить');
    setPlaying(false);
  }
}

// watchAuto starts "Автоигра" of game with its button and stops it with
// the same button.
function watchAuto(game, cost, round, count) {
  $(`${game}-auto`).hidden = false;
  $(`${game}-auto-start`).addEventListener('click', () => {
    if (auto) {
      auto.stop = true;
      auto.button.disabled = true;

      return;
    }
    const votes = cost();

    if (votes > 0) autoplay(game, votes, round, count);
  });
}

// tabs are the links that switch the game or the section of "Ставки".
function tabs() {
  return [$('tab-wheel'), $('tab-aim'), $('tab-rocket'), $('tab-events'), $('section-all'), $('section-popular')];
}

// The game shown: "wheel", "aim", "rocket" or "events", by the address;
// "bet" is the name "Прицел" had.
function gameOf(url) {
  const game = new URL(url, location.href).searchParams.get('game');

  if (game === 'aim' || game === 'bet') return aimShown ? 'aim' : 'wheel';
  if (game === 'rocket') return rocketShownTab ? 'rocket' : 'wheel';

  return game === 'events' ? 'events' : 'wheel';
}

function sectionOf(url) {
  return new URL(url, location.href).searchParams.get('section') === 'popular' ? 'popular' : 'all';
}

function showGame(url) {
  const game = gameOf(url);

  $('tab-wheel').classList.toggle('active', game === 'wheel');
  $('tab-aim').classList.toggle('active', game === 'aim');
  $('tab-rocket').classList.toggle('active', game === 'rocket');
  $('tab-events').classList.toggle('active', game === 'events');
  $('wheel-game').hidden = game !== 'wheel';
  $('aim-game').hidden = game !== 'aim';
  $('rocket-game').hidden = game !== 'rocket';
  if (game === 'rocket') drawSky(sky.ms, sky.cashedAt);
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
  let chosen = bets.multipliers.find((m) => m.multiplier === 200) ?? bets.multipliers[0];

  amount.min = String(bets.least);
  amount.max = String(bets.most);
  amount.value = String(Math.min(bets.most, Math.max(bets.least, 10)));
  $('bet-limits').textContent = tr('От {0} до {1} за ставку.', bets.least, votesCount(bets.most));

  const buttons = bets.multipliers.map((m) => {
    const button = el('button', { className: 'casino-cell casino-multiplier', type: 'button' },
      el('span', { className: 'casino-votes', textContent: betMultiplier(m.multiplier) }),
      el('span', { className: 'casino-unit', textContent: tr('шанс {0}', betChance(m.chance)) }),
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
    const odds = chosen.chance;
    const votes = stake();
    const fits = votes >= bets.least && votes <= bets.most;

    for (const b of buttons) {
      b.classList.toggle('lit', b.multiplier === chosen);
      b.setAttribute('aria-checked', String(b.multiplier === chosen));
    }
    const zone = aimZone(odds);

    // The field stands by its middle, so that a field narrower than its
    // least width grows to both sides and stays seen.
    $('dice-win').style.left = `${(zone.low + odds / 2) / 100}%`;
    $('dice-win').style.width = `${odds / 100}%`;
    $('dice-win').classList.toggle('narrow', odds < 200);
    $('bet-sum').textContent = fits
      ? tr('Шанс выигрыша {0}, выигрыш — {1}.', betChance(odds), votesCount(betPrize(votes, chosen.multiplier)))
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

  async function betOnce(votes, multiplier) {
    const wager = await api.betCasino(votes, multiplier);

    // The bet is gone at once, the win comes when the marker stops.
    showBalance(wager.balance - wager.votes);
    await roll(wager.roll);
    $('dice-mark').classList.add(wager.won ? 'won' : 'lost');
    wagers.count++;
    wagers.spent += votes;
    wagers.won += wager.votes;
    showBalance(wager.balance);
    showWager(wager, votes);
    if (wager.won && wager.votes >= jackpotFrom) jackpot(wager.votes);
  }

  $('bet-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!signedIn || playing) return;

    hideError($('error'));
    $('bet-result').hidden = true;
    setPlaying(true);

    try {
      await betOnce(stake(), chosen.multiplier);
    } catch (e) {
      showError($('error'), e);
    } finally {
      setPlaying(false);
    }
  });

  refreshBet = update;
  if (signedIn) {
    $('bet').hidden = false;
    // "Автоигра" bets the stake and the multiplier of the form, which stay
    // as they are while it plays; a stake out of the limits does not
    // start it.
    watchAuto('aim', () => {
      const votes = stake();

      return votes >= bets.least && votes <= bets.most ? votes : 0;
    }, () => betOnce(stake(), chosen.multiplier), wagersCount);
  } else {
    $('bet-guest').hidden = false;
  }
  update();
}

function showWager(wager, stake) {
  const result = $('bet-result');

  result.className = wager.won ? 'casino-result casino-win' : 'casino-result';
  if (wager.won) cheer(result, wager.votes, stake);
  const zone = aimZone(wager.chance);

  result.textContent = wager.won
    ? tr('Выпало {0} — в зелёном поле. Вы выиграли {1}!', betRoll(wager.roll), votesCount(wager.votes))
    : tr('Выпало {0}, а зелёное поле — от {1} до {2}. Ставка проиграна.', betRoll(wager.roll), betRoll(zone.low), betRoll(zone.high));
  result.hidden = false;

  $('bet-session').textContent = tr('За этот заход: {0}, поставлено {1}, выиграно {2}.',
    wagersCount(wagers.count), votesCount(wagers.spent), votesCount(wagers.won));
  $('bet-session').hidden = false;
}

// sky is what the sky of "Ракетка" shows: the milliseconds of the flight
// drawn and where the round was cashed, -1 — it was not.
const sky = { ms: 0, cashedAt: -1 };

// drawSky draws the flight of ms milliseconds: the trail from the start,
// the rocket at its end turned along it and the multiplier over it; a cash
// at cashedAt ms is a dot on the trail.
function drawSky(ms, cashedAt = -1) {
  const canvas = $('rocket-trail');
  const width = canvas.clientWidth;
  const height = canvas.clientHeight;
  if (!width) return;

  sky.ms = ms;
  sky.cashedAt = cashedAt;
  const ratio = devicePixelRatio || 1;
  if (canvas.width !== Math.round(width * ratio)) canvas.width = Math.round(width * ratio);
  if (canvas.height !== Math.round(height * ratio)) canvas.height = Math.round(height * ratio);

  const pad = 24;
  const scale = rocketScale(ms);
  const at = (t) => ({
    x: pad + (t / scale.time) * (width - 2 * pad),
    y: height - pad - ((rocketMultiplier(t) - 100) / (scale.top - 100)) * (height - 2 * pad),
  });
  const points = [];
  for (let i = 0; i <= 60; i++) points.push(at((ms * i) / 60));
  const end = points[points.length - 1];
  const before = at(Math.max(0, ms - scale.time / 40));

  const ctx = canvas.getContext('2d');
  const colour = getComputedStyle(canvas).color;
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  ctx.clearRect(0, 0, width, height);
  ctx.strokeStyle = colour;
  ctx.fillStyle = colour;
  ctx.lineWidth = 3;
  ctx.lineJoin = 'round';
  ctx.beginPath();
  points.forEach((p, i) => (i ? ctx.lineTo(p.x, p.y) : ctx.moveTo(p.x, p.y)));
  ctx.stroke();
  ctx.globalAlpha = 0.15;
  ctx.lineTo(end.x, height - pad);
  ctx.lineTo(pad, height - pad);
  ctx.fill();
  ctx.globalAlpha = 1;
  // The New Year rocket leaves sparks.
  if (document.documentElement.classList.contains('festive')) {
    points.forEach((p, i) => { if (i % 4 === 2) ctx.fillRect(p.x - 1.5, p.y - 6 - (i % 8), 3, 3); });
  }
  if (cashedAt >= 0) {
    const cash = at(cashedAt);
    ctx.beginPath();
    ctx.arc(cash.x, cash.y, 6, 0, 2 * Math.PI);
    ctx.fill();
  }

  const ship = $('rocket-ship');
  const turn = Math.atan2(end.y - before.y, end.x - before.x || 1);
  ship.style.transform = `translate(${end.x}px, ${end.y}px) translate(-50%, -50%) rotate(${ms > 0 ? turn : -Math.PI / 4}rad)`;
  $('rocket-boom').style.left = `${end.x}px`;
  $('rocket-boom').style.top = `${end.y}px`;
  $('rocket-multiplier').textContent = rocketShown(rocketMultiplier(ms));
}

// showSky marks how the flight stands: flying, cashed or crashed.
function showSky(state) {
  $('rocket-sky').classList.toggle('cashed', state === 'cashed');
  $('rocket-sky').classList.toggle('crashed', state === 'crashed');
  $('rocket-boom').hidden = state !== 'crashed';
}

// glide moves the flight from one moment to another, in milliseconds of
// the flight, drawn in duration milliseconds of the page.
async function glide(from, to, duration, cashedAt) {
  if (still || duration <= 0) {
    drawSky(to, cashedAt);
    return;
  }

  const begun = performance.now();
  await new Promise((resolve) => {
    const frame = () => {
      const part = Math.min(1, (performance.now() - begun) / duration);
      drawSky(from + (to - from) * part, cashedAt);
      if (part < 1) requestAnimationFrame(frame); else resolve();
    };
    requestAnimationFrame(frame);
  });
}

// land ends a round over: a cashed rocket flies on to where it explodes,
// quickly, and every rocket explodes there.
async function land(round, ms) {
  const crash = rocketTime(round.crash);
  const cashedAt = round.state === 'cashed' ? rocketTime(round.cashed) : -1;

  if (cashedAt >= 0) {
    showSky('cashed');
    await glide(ms, crash, Math.min(1500, crash - ms), cashedAt);
  } else {
    drawSky(crash);
  }
  showSky('crashed');
  $('rocket-multiplier').textContent = rocketShown(round.crash);
}

// flyLive draws a rocket cashed by hand while it flies, by the clock of the
// page set to the server's at the launch, until it is over: cashed with
// the button, or exploded — which the page learns by asking.
function flyLive(round, votes) {
  const start = performance.now() - round.elapsedMs;
  const cash = $('rocket-cash');

  return new Promise((resolve) => {
    let over = null;
    let shown = '';
    const asking = setInterval(async () => {
      try {
        const now = await api.rocket(round.id);

        if (now.state !== 'flying') over ??= now;
      } catch {
        // The next question may get through.
      }
    }, 500);

    const frame = () => {
      const ms = performance.now() - start;

      if (over) {
        clearInterval(asking);
        cash.hidden = true;
        cash.onclick = null;
        resolve({ round: over, ms });

        return;
      }
      drawSky(ms);
      const text = tr('Забрать {0}', votesCount(betPrize(votes, rocketMultiplier(ms))));
      if (text !== shown) cash.textContent = shown = text;
      requestAnimationFrame(frame);
    };

    cash.onclick = async () => {
      cash.disabled = true;
      try {
        over ??= await api.cashRocket(round.id);
      } catch (e) {
        showError($('error'), e);
        cash.disabled = false;
      }
    };
    cash.disabled = false;
    cash.hidden = false;
    $('rocket-launch').hidden = true;
    requestAnimationFrame(frame);
  });
}

// flyOnce launches a rocket of votes cashing by itself at auto (0 — by
// hand) and shows its flight to the end.
async function flyOnce(votes, auto) {
  const round = await api.launchRocket(votes, auto);
  let over = round;
  let ms = 0;

  // The bet is gone at once, the win comes with the cash.
  showBalance(round.balance - round.payout);
  showSky('flying');
  $('rocket-result').hidden = true;
  if (round.state === 'flying') {
    ({ round: over, ms } = await flyLive(round, votes));
  } else {
    // A rocket cashing by itself is over at the launch: its flight is
    // shown up to the cash or the explosion, at most 10 s.
    ms = rocketTime(Math.min(round.crash, round.cashed || round.crash));
    await glide(0, ms, Math.min(ms, 10000));
  }
  await land(over, ms);
  $('rocket-launch').hidden = false;

  flights.count++;
  flights.spent += votes;
  flights.won += over.payout;
  showBalance(round.state === 'flying' ? (over.balance || balance) : round.balance);
  showFlight(over, votes);
  if (over.payout >= jackpotFrom) jackpot(over.payout);
}

function showFlight(round, stake) {
  const result = $('rocket-result');
  const won = round.state === 'cashed';

  result.className = won ? 'casino-result casino-win' : 'casino-result';
  if (won) cheer(result, round.payout, stake);
  if (won) {
    result.textContent = tr('Вы забрали ставку на {0} и выиграли {1}! Ракета взорвалась на {2}.', rocketShown(round.cashed), votesCount(round.payout), rocketShown(round.crash));
  } else if (round.auto > 0) {
    result.textContent = tr('Ракета взорвалась на {0}, не долетев до {1}. Ставка проиграна.', rocketShown(round.crash), rocketShown(round.auto));
  } else {
    result.textContent = tr('Ракета взорвалась на {0}. Ставка проиграна.', rocketShown(round.crash));
  }
  result.hidden = false;

  $('rocket-session').textContent = tr('За этот заход: {0}, поставлено {1}, выиграно {2}.',
    rocketsCount(flights.count), votesCount(flights.spent), votesCount(flights.won));
  $('rocket-session').hidden = false;
}

// rocketing is the form of "Ракетка": the bet, the multiplier to cash at
// by itself, the sky and the button.
function rocketing(rocket, signedIn) {
  const amount = $('rocket-amount');
  const target = $('rocket-target');

  amount.min = String(rocket.least);
  amount.max = String(rocket.most);
  amount.value = String(Math.min(rocket.most, Math.max(rocket.least, 10)));
  $('rocket-limits').textContent = tr('От {0} до {1} за ставку. Наибольший множитель — {2}.', rocket.least, votesCount(rocket.most), betMultiplier(rocket.top));

  const stake = () => Math.floor(Number(amount.value));
  // auto is the multiplier to cash at by itself, 0 — by hand, -1 — not
  // one the rocket takes.
  const auto = () => {
    if (!target.value.trim()) return 0;
    const times = typedOdds(target.value);

    return times >= 101 && times <= rocket.top ? times : -1;
  };

  function update() {
    const votes = stake();
    const at = auto();
    const fits = votes >= rocket.least && votes <= rocket.most;

    if (!fits) $('rocket-sum').textContent = tr('Ставка — от {0} до {1}.', rocket.least, votesCount(rocket.most));
    else if (at < 0) $('rocket-sum').textContent = tr('Забрать само можно на множителе от ×1,01 до {0}.', betMultiplier(rocket.top));
    else if (at > 0) $('rocket-sum').textContent = tr('Ракета сама заберёт ставку на {0}: выигрыш — {1}.', rocketShown(at), votesCount(betPrize(votes, at)));
    else $('rocket-sum').textContent = tr('Нажмите «Забрать», пока ракета летит.');
    $('rocket-launch').textContent = fits ? tr('Поставить {0}', votesCount(votes)) : tr('Поставить');
    $('rocket-launch').disabled = playing || !fits || at < 0;
  }

  for (const quick of $('rocket-form').querySelectorAll('[data-quick]')) {
    quick.addEventListener('click', () => {
      amount.value = String(betAmount(stake(), quick.dataset.quick, { ...rocket, balance: signedIn ? balance : undefined }));
      update();
    });
  }
  amount.addEventListener('input', update);
  target.addEventListener('input', update);

  $('rocket-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!signedIn || playing || auto() < 0) return;

    hideError($('error'));
    setPlaying(true);

    try {
      await flyOnce(stake(), auto());
    } catch (e) {
      showError($('error'), e);
      $('rocket-launch').hidden = false;
    } finally {
      setPlaying(false);
    }
  });

  refreshRocket = update;
  addEventListener('resize', () => drawSky(sky.ms, sky.cashedAt));
  if (signedIn) {
    $('rocket-launch').hidden = false;
    // "Автоигра" cashes by itself: without the multiplier to cash at it
    // does not start.
    watchAuto('rocket', () => {
      const votes = stake();
      const fits = votes >= rocket.least && votes <= rocket.most;

      if (fits && auto() === 0) {
        $('rocket-auto-state').textContent = tr('Для автоигры укажите, на каком множителе ракета заберёт ставку сама.');
        $('rocket-auto-state').hidden = false;
      }

      return fits && auto() > 0 ? votes : 0;
    }, () => flyOnce(stake(), auto()), rocketsCount);
  } else {
    $('rocket-guest').hidden = false;
  }
  update();
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
  const amount = el('input', { className: 'casino-amount', type: 'number', inputMode: 'numeric', min: '1', max: '100000000', value: '10', required: true });
  const mine = eventMine(e);

  // An event has odds or, made before them, shares all staked.
  const fixed = e.outcomes.some((o) => o.odds > 0);

  const outcomes = e.outcomes.map((o, i) => {
    const percent = betEventShare(o.staked, e.staked);
    const bar = el('div', { className: 'bet-outcome-bar' });
    const odds = fixed || e.open ? betEventOdds(o, e.staked) : '';
    const stake = taking ? el('button', { className: 'button', type: 'button', textContent: tr('Поставить') }) : null;
    let mine = null;

    if (o.mine > 0) {
      mine = fixed
        ? tr('Ваша ставка: {0}, выигрыш — {1}', votesCount(o.mine), votesCount(o.potential))
        : tr('Ваша ставка: {0}', votesCount(o.mine));
    }
    bar.style.width = `${percent}%`;
    stake?.addEventListener('click', () => stakeOn(e, i, Math.floor(Number(amount.value))));

    return el('div', { className: e.winner === i ? 'bet-outcome won' : 'bet-outcome' },
      el('div', { className: 'bet-outcome-head' },
        el('b', { className: 'bet-outcome-name', textContent: o.name }),
        fixed ? el('span', { className: 'bet-outcome-odds', textContent: odds, title: tr('Коэффициент: ставка, умноженная на него, — выигрыш') }) : null,
        el('span', { className: 'bet-outcome-sum', textContent: [votesCount(o.staked), o.staked > 0 ? casinoChance(percent) : '', fixed ? '' : odds].filter(Boolean).join(' · ') }),
        stake,
      ),
      el('div', { className: 'bet-outcome-track' }, bar),
      mine ? el('div', { className: 'bet-outcome-mine', textContent: mine }) : null,
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
  const { price, prizes, bets, rocket, jackpot: from } = await answered;
  jackpotFrom = from || Infinity;

  $('price').textContent = votesCount(price);
  signedIn = Boolean(me);
  if (me) showBalance(me.votes ?? 0);
  if (bets?.multipliers?.length) {
    aimShown = true;
    $('tab-aim').hidden = false;
    betting(bets, signedIn);
  }
  if (rocket?.top) {
    rocketShownTab = true;
    $('tab-rocket').hidden = false;
    rocketing(rocket, signedIn);
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
    watchAuto('wheel', () => price, () => spinOnce(price), spinsCount);
  } else {
    $('guest').hidden = false;
  }

  $('casino').hidden = false;
  // The sky is drawn once it has its size.
  if (!$('rocket-game').hidden) drawSky(0);
} catch (e) {
  showError($('error'), e);
}
