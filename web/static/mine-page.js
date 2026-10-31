// "Шахта" (/mine), as the mine of the BFG bot: the player picks an ore and
// digs; a dig spends energy and gives ore and experience, and an ore opens
// with experience. The ore piles up in the backpack and sells for votes
// when the player likes. A dig may meet an event — a nugget, a chest of
// votes, a collapse, an energy drink. The page is a game: the rock face is
// one big button (and the space bar), each hit answers at once with a
// number, a sound and a shake, and a series of hits is counted. The series
// changes nothing on the server: it is only there to be beaten. The energy
// comes back by itself; the page counts it on between the answers of the
// server. The administrators tune the ores, the energy and the events in
// the admin panel (/admin/mine).

import { api, currentUser, session, started } from './api.js';
import { balanceVotes, balanceWord, energyWait, experienceCount, groupedVotes, mineDug, mineEnergy, mineSold, oreName, orePrice, oreValue, oreValuePerEnergy, unitsCount, votesAmount } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { countTo, flash, floatText, gameIcon, helpButton } from './game.js';
import { coins, digCollapse, digHit, digRare, fanfare } from './sounds.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.mine()) : null;

// The log keeps the last lines of digs and sales, the newest first.
const logLength = 5;
// A hit within this time of the one before goes on the series.
const seriesGap = 3000;
const pickedKey = 'freenet.mine.ore';

// mine is the last answer of the server and when it came, so that the
// energy is counted on from it.
let mine = null;
let at = 0;
let busy = false;
// A press while a dig is on its way is kept for when it comes back, so that
// quick clicks all count.
let pressed = false;
let picked = '';
let votes = 0;
let series = 0;
let lastHit = 0;
let bestSeries = 0;

function readPicked() {
  try {
    return localStorage.getItem(pickedKey) ?? '';
  } catch {
    return '';
  }
}

function savePicked() {
  try {
    localStorage.setItem(pickedKey, picked);
  } catch {
    // Only the choice is lost.
  }
}

function showBalance(next) {
  countTo($('votes'), votes, next, balanceVotes);
  $('votes-word').textContent = balanceWord(next);
  $('balance').hidden = false;
  votes = next;
}

function log(text, win = false) {
  const lines = $('log');
  lines.prepend(el('p', { className: win ? 'mine-line mine-win' : 'mine-line', textContent: text }));
  while (lines.children.length > logLength) lines.lastChild.remove();
  lines.hidden = false;
}

const isOpen = (vein) => mine.miner.experience >= vein.experience;
const veinOf = (ore) => mine.settings.veins.find((v) => v.ore === ore);

// energy is the energy now, counted on from the last answer.
function energy() {
  return mineEnergy(mine.miner, mine.settings, (Date.now() - at) / 1000);
}

function showEnergy() {
  const { energy: now, next } = energy();
  const most = mine.settings.energy;
  const vein = veinOf(picked);
  const short = vein && now < vein.energy;

  $('energy').textContent = `${now} / ${most}`;
  $('energy-bar').style.width = `${(100 * now) / most}%`;
  $('energy-bar').className = now * 4 < most ? 'meter-fill bad' : 'meter-fill good';
  $('energy-next').textContent = next > 0 ? tr('+1 через {0}', energyWait(next)) : tr('полная');
  $('dig').classList.toggle('tired', Boolean(short));
  $('dig').setAttribute('aria-disabled', String(Boolean(short)));
  $('dig-text').textContent = short ? tr('Нет энергии') : tr('Копать!');
  $('dig-sub').textContent = short ? tr('+1 через {0}', energyWait(next)) : vein ? tr('{0}–{1} шт. за {2} энергии', groupedVotes(vein.least), groupedVotes(vein.most), vein.energy) : '';
}

function showNext() {
  const { veins } = mine.settings;
  const { experience } = mine.miner;
  const index = veins.findIndex((v) => v.experience > experience);
  const next = veins[index];

  $('next-bar').style.width = '100%';
  if (!next) {
    $('next-ore').textContent = tr('Открыты все руды');
    $('next-share').textContent = '';
    return;
  }
  const from = veins[index - 1]?.experience ?? 0;
  $('next-bar').style.width = `${(100 * (experience - from)) / (next.experience - from)}%`;
  $('next-share').textContent = `${groupedVotes(experience - from)} / ${groupedVotes(next.experience - from)}`;
  $('next-ore').textContent = tr('до руды «{0}»', oreName(next.ore));
}

function oreTile(vein) {
  const open = isOpen(vein);
  const tile = el('button', {
    type: 'button',
    className: `mine-tile${vein.ore === picked ? ' chosen' : ''}${open ? '' : ' locked'}`,
    title: [
      tr('+{0} опыта', groupedVotes(vein.gain)), orePrice(vein.price), tr('≈ {0} за энергию', oreValuePerEnergy(vein)),
    ].join(' · '),
    disabled: !open,
    onclick: () => {
      picked = vein.ore;
      savePicked();
      showOres();
      showEnergy();
    },
  },
  el('span', { className: `mine-icon mine-icon-${vein.ore}`, ariaHidden: 'true' }),
  el('b', { textContent: oreName(vein.ore) }),
  open
    ? el('span', { className: 'hint', textContent: orePrice(vein.price) })
    : el('span', { className: 'hint mine-opens' }, gameIcon('/static/icons/game/lock.svg', 12), ` ${experienceCount(vein.experience)}`));
  return tile;
}

function showOres() {
  $('ores').replaceChildren(...mine.settings.veins.map(oreTile));
  $('face-ore').className = `mine-face-ore mine-icon mine-icon-${picked}`;
}

function showBackpack() {
  const { backpack, digs, earned } = mine.miner;
  const kept = mine.settings.veins.filter((v) => (backpack[v.ore] ?? 0) > 0);

  $('backpack').replaceChildren(...kept.map((vein) => {
    const units = backpack[vein.ore];
    const worth = oreValue(units, vein.price);
    const sell = el('button', { className: 'button', type: 'button', textContent: tr('Продать'), disabled: worth < 1 });
    sell.addEventListener('click', () => sellOre(vein.ore, sell));

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
  $('sell-all').hidden = kept.length === 0;
  $('sell-all').textContent = tr('Продать всё за {0}', votesAmount(worth));
  $('sell-all').disabled = worth < 1;
  $('totals').textContent = digs > 0 ? tr('Копок: {0}. Руда принесла {1}.', groupedVotes(digs), votesAmount(earned)) : '';
}

// bestOpen is the dearest ore open: what a new player digs without choosing.
const bestOpen = () => mine.settings.veins.filter(isOpen).at(-1)?.ore ?? '';

function show(next) {
  const before = mine?.settings.veins.filter(isOpen).length ?? 0;
  mine = next;
  at = Date.now();
  const opened = before > 0 && mine.settings.veins.filter(isOpen).length > before;
  if (!veinOf(picked) || !isOpen(veinOf(picked)) || opened) picked = bestOpen();

  $('experience').textContent = groupedVotes(mine.miner.experience);
  showOres();
  showBackpack();
  showNext();
  showEnergy();
  $('status').hidden = false;
  $('face-block').hidden = false;
  $('backpack-block').hidden = false;

  if (opened) {
    log(tr('Открыта новая руда: {0}!', oreName(picked)), true);
    fanfare();
    flash($('face-block'), 'gold');
  }
}

// hitSeries counts a hit on, with a fanfare every ten.
function hitSeries() {
  const now = Date.now();
  series = now - lastHit < seriesGap ? series + 1 : 1;
  lastHit = now;
  bestSeries = Math.max(bestSeries, series);

  $('series').hidden = series < 2;
  $('series').textContent = tr('Серия: {0}', series);
  $('series').title = tr('Лучшая серия: {0}', bestSeries);
  if (series % 10 === 0) {
    fanfare();
    floatText($('series'), tr('Серия: {0}', series), 'gold');
  }
}

// answer shows on the rock what a dig brought.
function answer(dug) {
  const face = $('dig');
  if (dug.event === 'collapse') {
    digCollapse();
    series = 0;
    $('series').hidden = true;
    flash(face, 'bad');
    floatText(face, tr('Обвал!'), 'bad');
    return;
  }

  floatText(face, `+${groupedVotes(dug.units)}`, dug.event === 'nugget' ? 'gold' : 'good');
  if (dug.event === 'nugget' || dug.event === 'chest' || dug.event === 'drink') {
    digRare();
    flash(face, 'gold');
  }
  if (dug.event === 'chest') floatText(face, `+${votesAmount(dug.votes)}`, 'gold');
  if (dug.event === 'drink') floatText($('energy'), tr('+{0} энергии', mine.settings.events.drinkEnergy), 'good');
}

function swing() {
  const pick = $('face-pick');
  pick.classList.remove('swing');
  void pick.offsetWidth;
  pick.classList.add('swing');
}

async function digOre() {
  if (!mine || !picked) return;
  if (busy) {
    pressed = true;
    return;
  }
  if (energy().energy < veinOf(picked).energy) {
    flash($('energy').parentElement, 'bad');
    return;
  }

  hideError($('error'));
  busy = true;
  hitSeries();
  digHit(series);
  swing();
  flash($('dig'), 'hit');

  try {
    const ore = picked;
    const dug = await api.digOre(ore);
    log(mineDug(ore, dug, mine.settings), dug.event === 'chest' || dug.event === 'nugget');
    answer(dug);
    if (dug.balance > 0) showBalance(dug.balance);
    busy = false;
    show(dug.mine);
    if (pressed) {
      pressed = false;
      digOre();
    }
  } catch (e) {
    showError($('error'), e);
    busy = false;
    pressed = false;
  }
}

async function sellOre(ore, button) {
  if (busy) return;
  hideError($('error'));
  busy = true;

  try {
    const sold = await api.sellOre(ore);
    log(mineSold(sold), true);
    coins();
    floatText(button, `+${votesAmount(sold.votes)}`, 'gold');
    floatText($('votes'), `+${votesAmount(sold.votes)}`, 'gold');
    flash($('balance'), 'gold');
    showBalance(sold.balance);
    busy = false;
    show(sold.mine);
  } catch (e) {
    showError($('error'), e);
    busy = false;
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

$('mine').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
} else {
  picked = readPicked();
  votes = me.votes ?? 0;
  $('votes').textContent = balanceVotes(votes);
  showBalance(votes);
  $('mine-help').replaceChildren(helpButton(tr('Выберите руду и копайте. Каждая копка тратит энергию и даёт руду и опыт, а с опытом открываются руды дороже. Руда копится в рюкзаке — продайте её за голоса, когда захотите. Энергия восстанавливается сама. Иногда в забое случается что-нибудь неожиданное.')));
  $('dig').addEventListener('click', digOre);
  $('sell-all').addEventListener('click', () => sellOre('', $('sell-all')));
  // The space bar digs, unless something else has the keyboard.
  document.addEventListener('keydown', (event) => {
    if (event.code !== 'Space' || event.repeat || event.target.closest('button, a, input, textarea, select, .modal')) return;
    event.preventDefault();
    digOre();
  });
  try {
    show(await answered);
    setInterval(() => {
      if (!document.hidden) showEnergy();
    }, 1000);
  } catch (e) {
    showError($('error'), e);
  }
}
