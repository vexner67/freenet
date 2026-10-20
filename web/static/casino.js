// "Казино" (/casino): a spin costs votes and wins one of the prizes,
// votes again — the administrators tune how likely each is in the admin
// panel. A light runs over the prizes, slowing down, and stops on the one
// won; at the right is the balance, as in "Голоса" of the settings.

import { api, currentUser, started } from './api.js';
import { casinoPlan, casinoPrize, plural, spinsCount, votesCount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const answered = started(api.casino());

// Without motion the light lands at once.
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;

let cells = [];
let lit = 0;
const session = { spins: 0, spent: 0, won: 0 };

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

function showBalance(votes) {
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
  const button = $('spin');

  hideError($('error'));
  $('result').hidden = true;
  button.disabled = true;

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
    button.disabled = false;
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { price, prizes } = await answered;

  $('price').textContent = votesCount(price);
  showBoard(prizes);

  if (me) {
    $('spin').textContent = tr('Крутить за {0}', votesCount(price));
    $('spin').hidden = false;
    $('spin').addEventListener('click', () => spin(price));
    showBalance(me.votes ?? 0);
  } else {
    $('guest').hidden = false;
  }

  $('casino').hidden = false;
} catch (e) {
  showError($('error'), e);
}
