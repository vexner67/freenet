// "Шахта" (/mine), as the mine of the BFG bot: the player picks an ore and
// digs; a dig spends energy and gives ore and experience, and an ore
// opens with experience. The ore piles up in the backpack and sells for
// votes when the player likes. A dig may meet an event — a nugget, a
// chest of votes, a collapse, an energy drink. The energy comes back by
// itself; the page counts it on between the answers of the server. The
// administrators tune the ores, the energy and the events in the admin
// panel (/admin/mine).

import { api, currentUser, session, started } from './api.js';
import { balanceVotes, balanceWord, energyWait, experienceCount, groupedVotes, mineDug, mineEnergy, mineSold, oreName, orePrice, oreValue, oreValuePerEnergy, unitsCount, votesAmount, votesFontSize } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.mine()) : null;

// The log keeps the last lines of digs and sales, the newest first.
const logLength = 8;

// mine is the last answer of the server and when it came, so that the
// energy is counted on from it.
let mine = null;
let at = 0;
let busy = false;
let rows = new Map();

function showBalance(votes) {
  $('votes').textContent = balanceVotes(votes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = balanceWord(votes);
  $('balance').hidden = false;
}

function log(text, win = false) {
  const lines = $('log');
  lines.prepend(el('p', { className: win ? 'mine-line mine-win' : 'mine-line', textContent: text }));
  while (lines.children.length > logLength) lines.lastChild.remove();
  lines.hidden = false;
}

// energy is the energy now, counted on from the last answer.
function energy() {
  return mineEnergy(mine.miner, mine.settings, (Date.now() - at) / 1000);
}

function showEnergy() {
  const { energy: now, next } = energy();
  const most = mine.settings.energy;

  $('energy').textContent = `${now} / ${most}`;
  $('energy-bar').style.width = `${(100 * now) / most}%`;
  $('energy-next').textContent = next > 0 ? tr('+1 через {0}', energyWait(next)) : tr('полная');
  for (const [ore, row] of rows) {
    const vein = mine.settings.veins.find((v) => v.ore === ore);
    row.dig.disabled = busy || now < vein.energy;
  }
}

function oreRow(vein) {
  const open = mine.miner.experience >= vein.experience;
  const dig = el('button', { className: 'button', type: 'button', textContent: tr('Копать') });
  dig.addEventListener('click', () => digOre(vein.ore));

  const facts = [
    tr('{0}–{1} шт.', groupedVotes(vein.least), groupedVotes(vein.most)),
    tr('+{0} опыта', groupedVotes(vein.gain)),
    tr('энергия: {0}', vein.energy),
    orePrice(vein.price),
    tr('≈ {0} за энергию', oreValuePerEnergy(vein)),
  ].join(' · ');

  const row = el('div', { className: open ? 'mine-ore' : 'mine-ore mine-locked' },
    el('span', { className: `mine-icon mine-icon-${vein.ore}`, ariaHidden: 'true' }),
    el('div', { className: 'mine-ore-text' },
      el('b', { textContent: oreName(vein.ore) }),
      el('div', { className: 'hint', textContent: facts }),
    ),
    open ? dig : el('span', { className: 'hint mine-opens', textContent: tr('откроется с {0}', experienceCount(vein.experience)) }),
  );
  if (open) rows.set(vein.ore, { dig });

  return row;
}

function showBackpack() {
  const { backpack, digs, earned } = mine.miner;
  const kept = mine.settings.veins.filter((v) => (backpack[v.ore] ?? 0) > 0);

  $('backpack').replaceChildren(...kept.map((vein) => {
    const units = backpack[vein.ore];
    const worth = oreValue(units, vein.price);
    const sell = el('button', { className: 'button', type: 'button', textContent: tr('Продать'), disabled: busy || worth < 1 });
    sell.addEventListener('click', () => sellOre(vein.ore));

    return el('div', { className: 'mine-item' },
      el('span', { className: `mine-icon mine-icon-${vein.ore}`, ariaHidden: 'true' }),
      el('b', { className: 'mine-item-name', textContent: oreName(vein.ore) }),
      el('span', { className: 'mine-item-units', textContent: unitsCount(units) }),
      el('span', { className: 'hint mine-item-worth', textContent: tr('≈ {0}', votesAmount(worth)) }),
      sell,
    );
  }));

  const worth = kept.reduce((sum, v) => sum + oreValue(backpack[v.ore], v.price), 0);
  $('backpack-empty').hidden = kept.length > 0;
  $('sell-all-box').hidden = kept.length === 0;
  $('backpack-worth').textContent = tr('Всё вместе — {0}', votesAmount(worth));
  $('sell-all').disabled = busy || worth < 1;
  $('totals').textContent = digs > 0 ? tr('Копок: {0}. Руда принесла {1}.', groupedVotes(digs), votesAmount(earned)) : '';
}

function show(next) {
  mine = next;
  at = Date.now();
  rows = new Map();

  $('experience').textContent = groupedVotes(mine.miner.experience);
  $('ores').replaceChildren(...mine.settings.veins.map(oreRow));
  showBackpack();
  showEnergy();
  $('status').hidden = false;
  $('backpack-block').hidden = false;
}

function setBusy(on) {
  busy = on;
  showBackpack();
  showEnergy();
}

async function digOre(ore) {
  if (busy) return;
  hideError($('error'));
  setBusy(true);

  try {
    const dug = await api.digOre(ore);
    log(mineDug(ore, dug, mine.settings), dug.event === 'chest' || dug.event === 'nugget');
    if (dug.balance > 0) showBalance(dug.balance);
    busy = false;
    show(dug.mine);
  } catch (e) {
    showError($('error'), e);
    setBusy(false);
  }
}

async function sellOre(ore) {
  if (busy) return;
  hideError($('error'));
  setBusy(true);

  try {
    const sold = await api.sellOre(ore);
    log(mineSold(sold), true);
    showBalance(sold.balance);
    busy = false;
    show(sold.mine);
  } catch (e) {
    showError($('error'), e);
    setBusy(false);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

$('mine').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
} else {
  showBalance(me.votes ?? 0);
  $('sell-all').addEventListener('click', () => sellOre(''));
  try {
    show(await answered);
    setInterval(() => {
      if (!document.hidden) showEnergy();
    }, 1000);
  } catch (e) {
    showError($('error'), e);
  }
}
