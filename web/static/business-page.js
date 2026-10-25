// "Бизнес" (/business), a tab of "Заработок": a page buys businesses for votes
// and runs them as a little game of supplies, offers and workers. Each
// business is its premises seen from above in pixels (business-art.js):
// customers come in to be served, workers stand at their places, the
// storeroom fills with boxes. Clicking a part — the hall, the workers, the
// storeroom, the till, the billboard, the sign — opens a box of what it is
// and what to do with it; the same parts, the journal and the event that
// waits are links under the picture. The server counts the hours; the page
// only shows what it said. The administrators tune the businesses in the
// admin panel (/admin/business).

import { api, currentUser, session, started } from './api.js';
import {
  businessAdvice, businessEvent, businessLoad, businessMood, businessName, businessNote, businessOption,
  businessTotals, businessUnits, formatDateTime, groupedVotes, hundredths, offerName, orderCost, plural, professionName,
  professionWork, supplierAbout, supplierName, supplyName, traitName, votesAmount, votesFontSize,
} from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';
import { H, Premises, W } from './business-art.js';

const answered = session.get() ? started(api.businesses()) : null;

// The pictures of the businesses for their headings and the lots.
const buildings = {
  kiosk: '/static/business/kiosk.svg',
  cafe: '/static/business/cafe.svg',
  carwash: '/static/business/carwash.svg',
  factory: '/static/business/factory.svg',
};

let state = null;
let busy = false;
let panel = null;

function showBalance(votes) {
  $('votes').textContent = groupedVotes(votes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('balance').hidden = false;
}

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

const percent = (n, of) => `${(n / of) * 100}%`;
const decimal = (n) => (Math.round(n * 10) / 10).toFixed(1).replace('.', ',');

function picture(src, className = '') {
  return el('img', { src, alt: '', className, draggable: false });
}

const specOf = (kind) => state.catalog.find((c) => c.kind === kind);
const planOf = (kind) => state.settings.plans.find((p) => p.kind === kind);

// stocked says whether the storeroom of b holds a sale of an offer.
function stocked(b, offer) {
  const recipe = specOf(b.kind).offers.find((o) => o.offer === offer).recipe;
  return recipe.every((i) => b.supplies.find((s) => s.supply === i.supply).units >= i.units);
}

// hourly is what a business makes an hour as it runs now: what its workers
// sell at its prices while the storeroom holds it, less the wages and the
// rent, in hundredths of a vote.
function hourly(b) {
  const income = b.offers.reduce((n, o) => n + (stocked(b, o.offer) ? o.served * o.price : 0), 0);
  return { income, costs: b.wages + b.rent, profit: income - b.wages - b.rent };
}

// The premises on the page, each with its canvas, drawn by one loop.
const premises = new Map();
const still = matchMedia('(prefers-reduced-motion: reduce)');

// fit sizes a canvas to a whole number of its pixels per picture pixel.
function fit(canvas) {
  const ratio = window.devicePixelRatio || 1;
  const width = canvas.parentElement.clientWidth || W;
  const scale = Math.max(1, Math.floor((width * ratio) / W));
  canvas.width = W * scale;
  canvas.height = H * scale;
  canvas.style.width = `${(W * scale) / ratio}px`;
  canvas.style.height = `${(H * scale) / ratio}px`;
  return scale;
}

function paint(p) {
  const ctx = p.canvas.getContext('2d');
  ctx.setTransform(p.scale, 0, 0, p.scale, 0, 0);
  ctx.imageSmoothingEnabled = false;
  p.art.draw(ctx);
}

let last = 0;
function loop(now) {
  const dt = Math.min((now - (last || now)) / 1000, 0.1);
  last = now;
  for (const [kind, p] of premises) {
    if (!p.canvas.isConnected) {
      premises.delete(kind);
      continue;
    }
    p.art.step(dt, still.matches);
    paint(p);
  }
  if (premises.size) requestAnimationFrame(loop);
  else last = 0;
}

window.addEventListener('resize', () => {
  for (const p of premises.values()) p.scale = fit(p.canvas);
});

function pointAt(p, e) {
  const box = p.canvas.getBoundingClientRect();
  return [((e.clientX - box.left) / box.width) * W, ((e.clientY - box.top) / box.height) * H];
}

// scene is the canvas of a business; the people on it keep walking when
// the business changes, only the premises are drawn anew.
function scene(b) {
  let p = premises.get(b.kind);
  if (!p) {
    const canvas = el('canvas', { className: 'biz-canvas', role: 'img' });
    p = { canvas, art: new Premises(b.kind), scale: 1 };
    canvas.addEventListener('mousemove', (e) => {
      p.art.hover = p.art.partAt(...pointAt(p, e));
      canvas.title = p.art.hover?.label ?? '';
      canvas.classList.toggle('biz-pointing', Boolean(p.art.hover));
    });
    canvas.addEventListener('mouseleave', () => { p.art.hover = null; });
    canvas.addEventListener('click', (e) => {
      const part = p.art.partAt(...pointAt(p, e));
      if (part) openPanel(b.kind, part.part);
    });
    const start = premises.size === 0;
    premises.set(b.kind, p);
    if (start) requestAnimationFrame(loop);
  }
  p.art.set(b, specOf(b.kind));
  p.canvas.setAttribute('aria-label', tr('{0}: уровень {1}', businessName(b.kind), b.level));
  const box = el('div', { className: 'biz-scene' }, p.canvas);
  requestAnimationFrame(() => {
    p.scale = fit(p.canvas);
    paint(p);
  });
  return box;
}

const parts = [
  ['building', tr('Здание')],
  ['offers', tr('Товары и цены')],
  ['staff', tr('Работники')],
  ['storage', tr('Склад')],
  ['till', tr('Касса')],
  ['ad', tr('Реклама')],
  ['journal', tr('Журнал')],
];

// links are the parts of a business as words under its picture, for those
// who would rather not aim at pixels; a part that needs care is marked, and
// an event that waits for the owner comes first.
function links(b) {
  const spec = specOf(b.kind);
  const professions = new Set(b.workers.map((w) => w.profession));
  const care = {
    building: b.condition < 4000 || Boolean(b.broken),
    staff: spec.offers.some((o) => !professions.has(o.profession)),
    storage: businessTotals(b).empty,
    till: b.inDebt,
  };
  const shown = b.bankrupt ? [parts[0], parts.at(-1)] : parts;
  const words = shown.map(([part, name]) => [part, name, care[part] && !b.bankrupt ? 'biz-care' : '']);
  if (b.event) words.unshift(['event', businessEvent(b.event).title, 'biz-care']);

  return el('div', { className: 'biz-links' }, ...words.flatMap(([part, name, className], i) => [
    i ? ' | ' : '',
    el('a', { href: '#', className, textContent: name, onclick: (e) => {
      e.preventDefault();
      openPanel(b.kind, part);
    } }),
  ]));
}

function businessCard(b, plan) {
  const { profit } = hourly(b);
  const hour = b.bankrupt
    ? el('span', { className: 'biz-hour bad', textContent: tr('разорён') })
    : el('span', { className: profit < 0 ? 'biz-hour bad' : 'biz-hour good', textContent: tr('за час: {0} гол.', (profit > 0 ? '+' : '') + hundredths(Math.round(profit))) });

  return el('div', { className: 'biz-card', id: `biz-${b.kind}` },
    el('div', { className: 'biz-head' },
      picture(buildings[b.kind], 'biz-icon'),
      el('b', { textContent: businessName(b.kind) }),
      el('span', { className: 'biz-level', textContent: tr('уровень {0} из {1}', b.level, plan.levels) }),
      hour,
    ),
    scene(b),
    links(b),
  );
}

function lot(plan) {
  const name = businessName(plan.kind);
  const buy = () => openPanel(plan.kind, 'lot');

  return el('div', { className: 'biz-card' },
    el('div', { className: 'biz-head' },
      picture(buildings[plan.kind], 'biz-icon'),
      el('b', { textContent: name }),
      el('span', { className: 'biz-level', textContent: votesAmount(plan.price) }),
    ),
    el('div', { className: 'biz-scene biz-scene-lot' },
      el('button', { type: 'button', className: 'biz-lot', title: tr('Купить: {0}', name), onclick: buy },
        picture('/static/business/lot.svg'),
        el('span', { className: 'biz-for-sale' }, el('b', { textContent: tr('Продаётся') }), el('span', { textContent: name })),
      ),
    ),
  );
}

function show() {
  const owned = new Set(state.businesses.map((b) => b.kind));

  $('owned').replaceChildren(...state.businesses.map((b) => businessCard(b, planOf(b.kind))));
  const offers = state.settings.plans.filter((p) => !owned.has(p.kind));
  $('shop').replaceChildren(...offers.map(lot));
  $('shop-block').hidden = offers.length === 0;
  $('owned-hint').hidden = state.businesses.length === 0;
}

// float shows what an action brought or cost over the picture of a business.
function float(kind, text, bad) {
  const card = document.getElementById(`biz-${kind}`);
  if (!card || !text) return;

  const node = el('span', { className: bad ? 'biz-float bad' : 'biz-float', textContent: text });
  card.querySelector('.biz-scene').append(node);
  node.addEventListener('animationend', () => node.remove());
}

// act runs an action, shows the businesses it left and fills the open
// box again, or closes it when its business is gone.
async function act(kind, run, text) {
  if (busy) return;
  busy = true;
  hideError($('error'));
  if (panel) hideError(panel.error);

  try {
    const done = await run();
    // Actions that move no votes say no balance.
    if (done.balance > 0 || done.spent > 0 || done.got > 0) showBalance(done.balance);
    state = done.businesses;
    show();

    if (done.got > 0) float(kind, `+${votesAmount(done.got)}`, false);
    else if (done.spent > 0) float(kind, `−${votesAmount(done.spent)}`, true);

    const kept = state.businesses.find((b) => b.kind === kind);
    if (panel && panel.modal.box.isConnected && kept) {
      if (panel.part === 'lot') panel.part = 'building';
      if (panel.part === 'event' && !kept.event) panel.part = 'journal';
      fillPanel(text(done));
    } else {
      panel?.modal.close();
      panel = null;
      said(text(done));
    }
  } catch (e) {
    showError(panel?.modal.box.isConnected ? panel.error : $('error'), e);
  } finally {
    busy = false;
  }
}

const run = (b, action, body, text) => act(b.kind, () => api.runBusiness(b.kind, action, body), text);

function fact(label, value, warn = false) {
  return el('tr', {},
    el('td', { className: 'biz-label', textContent: label }),
    el('td', { className: warn ? 'biz-warn' : '', textContent: value }),
  );
}

function facts(...rows) {
  return el('table', { className: 'biz-facts' }, ...rows.filter(Boolean));
}

function bar(value, of, bad = false) {
  const fill = el('span', { className: bad ? 'biz-fill bad' : 'biz-fill' });
  fill.style.width = percent(Math.max(0, Math.min(value, of)), Math.max(of, 1));
  return el('span', { className: 'biz-bar' }, fill);
}

function numberField(value, min, max) {
  return el('input', { type: 'number', className: 'biz-number', value: String(value), min: String(min), max: String(max), step: '1' });
}

// centsField is a price field in votes with hundredths: "1,8".
function centsField(cents) {
  return el('input', { type: 'text', className: 'biz-number', value: hundredths(cents), inputMode: 'decimal' });
}

const centsOf = (field) => Math.round(Number(field.value.trim().replace(/\s/g, '').replace(',', '.')) * 100) || 0;

function choice(options, value) {
  const select = el('select', { className: 'biz-select' }, ...options.map(([v, label]) => el('option', { value: v, textContent: label })));
  select.value = value;
  return select;
}

function row(...children) {
  return el('div', { className: 'biz-row' }, ...children);
}

function table(...rows) {
  return el('table', { className: 'biz-table' }, ...rows);
}

function heading(text) {
  return el('h4', { className: 'biz-subtitle', textContent: text });
}

function hint(text) {
  return el('p', { className: 'hint', textContent: text });
}

function buildingPanel(b, plan) {
  const name = businessName(b.kind);

  if (b.bankrupt) {
    return {
      title: tr('{0} — разорён', name),
      body: [
        el('p', { className: 'biz-bankrupt', textContent: tr('Бизнес разорился: долги превысили допустимое. Его можно только закрыть — тогда его можно купить снова.') }),
        facts(fact(tr('Долг:'), tr('{0} гол.', hundredths(-b.cash)), true), fact(tr('Вложено и выведено:'), tr('{0} / {1}', votesAmount(b.invested), votesAmount(b.collected)))),
      ],
      actions: [button(tr('Закрыть бизнес'), () => run(b, 'sell', {}, () => tr('Бизнес «{0}» закрыт.', name)))],
    };
  }

  const { income, costs, profit } = hourly(b);
  const { customers, served } = businessTotals(b);
  const advice = businessAdvice(b, specOf(b.kind));
  const review = b.reviewUntil && new Date(b.reviewUntil) > new Date();

  const actions = [];
  if (b.level < plan.levels) actions.push(button(tr('Улучшить за {0}', votesAmount(b.upgradeCost)), () => run(b, 'upgrade', {}, () => tr('Бизнес «{0}» улучшен.', name))));
  if (b.condition < 10000 || b.broken) actions.push(button(tr('Ремонт за {0}', votesAmount(b.repairCost)), () => run(b, 'repair', {}, () => tr('Помещение отремонтировано.'))));
  if (!b.inDebt) actions.push(button(tr('Продать'), () => sell(b, name), { secondary: true }));

  return {
    title: tr('{0} — уровень {1} из {2}', name, b.level, plan.levels),
    body: [
      advice.length ? el('ul', { className: 'biz-advice' }, ...advice.map((a) => el('li', { textContent: a }))) : null,
      facts(
        fact(tr('Покупателей в час:'), tr('{0}, обслуживают {1}', decimal(customers), decimal(served)), served < customers * 0.8),
        fact(tr('Выручка за час:'), tr('≈ {0} гол.', hundredths(Math.round(income)))),
        fact(tr('Расходы за час:'), tr('{0} гол. (зарплаты {1}, аренда {2})', hundredths(Math.round(costs)), hundredths(Math.round(b.wages)), hundredths(Math.round(b.rent)))),
        fact(tr('Итог часа:'), tr('≈ {0} гол.', hundredths(Math.round(profit))), profit < 0),
        fact(tr('Работников:'), tr('{0} из {1}', b.workers.length, b.seats)),
        review ? fact(tr('Отзыв:'), tr('хороший, +30 % покупателей до {0}', formatDateTime(b.reviewUntil))) : null,
        b.broken ? fact(tr('Сломано:'), b.brokenUntil ? tr('«{0}», техник починит к {1}', offerName(b.broken), formatDateTime(b.brokenUntil)) : tr('«{0}» — обслуживают вдвое меньше', offerName(b.broken)), true) : null,
        fact(tr('Вложено и выведено:'), tr('{0} / {1}', votesAmount(b.invested), votesAmount(b.collected))),
      ),
      el('div', { className: 'biz-label', textContent: tr('Состояние помещения: {0} %', hundredths(b.condition)) }),
      bar(b.condition, 10000, b.condition < 4000),
      hint(b.level < plan.levels
        ? tr('Улучшение: больше покупателей, склад, касса и места для работников.')
        : tr('Бизнес на высшем уровне.')),
    ],
    actions,
  };
}

// offersPanel is what a business sells: the customers of each offer, those
// served, what a sale uses and its price.
function offersPanel(b) {
  const spec = specOf(b.kind);
  const rows = b.offers.map((o) => {
    const os = spec.offers.find((x) => x.offer === o.offer);
    const price = centsField(o.price);
    const save = button(tr('Сохранить'), () => {
      run(b, 'price', { offer: o.offer, price: centsOf(price) }, () => tr('Цена «{0}» сохранена.', offerName(o.offer)));
    });
    const recipe = os.recipe.map((i) => `${supplyName(i.supply)} × ${i.units}`).join(', ');
    return el('tr', {},
      el('td', {},
        el('b', { textContent: offerName(o.offer) }),
        o.broken ? el('span', { className: 'biz-warn', textContent: tr(' — сломано') }) : '',
        el('div', { className: 'hint', textContent: tr('{0}; уходит: {1}', professionName(os.profession), recipe) }),
        el('div', { className: 'hint', textContent: tr('покупателей {0} в час, обслуживают {1}', decimal(o.demand), decimal(o.served)) }),
      ),
      el('td', { className: 'biz-cell-action' },
        row(price, save),
        el('div', { className: 'hint', textContent: tr('обычная цена {0}', hundredths(os.fair)) }),
      ),
    );
  });

  return {
    title: tr('Товары и цены'),
    body: [
      hint(tr('Цены — в голосах за одну продажу. Дороже обычной — меньше покупателей, вдвое дороже не купит никто.')),
      table(...rows),
    ],
    actions: [],
  };
}

function workerRow(b, w) {
  const fire = button(tr('Уволить'), () => run(b, 'fire', { worker: w.id }, () => tr('{0} уволен(а).', w.name)), { secondary: true });
  const bonus = button(tr('Премия {0}', votesAmount(w.bonus)), () => run(b, 'bonus', { worker: w.id }, () => tr('{0} получает премию.', w.name)));
  const raise = button(tr('Прибавка'), () => run(b, 'raise', { worker: w.id }, () => tr('{0} получает прибавку.', w.name)));
  const skill = w.nextXp > 0 ? tr('навык {0}, до следующего {1} ч.', w.skill, Math.ceil(w.nextXp - w.xp)) : tr('навык {0} — высший', w.skill);
  const about = [skill, tr('зарплата {0} в час', hundredths(w.wage))];
  if (w.trait) about.push(traitName(w.trait));

  return el('tr', {},
    el('td', {},
      el('b', { textContent: w.name }), ' — ', professionName(w.profession).toLowerCase(),
      el('div', { className: 'hint', textContent: about.join('; ') }),
      el('div', { className: w.mood < 40 ? 'hint biz-warn' : 'hint', textContent: tr('настроение: {0}', businessMood(w.mood)) }),
      professionWork(w.profession) ? el('div', { className: 'hint', textContent: professionWork(w.profession) }) : '',
    ),
    el('td', { className: 'biz-cell-action' }, b.bankrupt ? '' : el('div', { className: 'biz-buttons' }, bonus, raise, fire)),
  );
}

function candidateRow(b, c) {
  const w = c.worker;
  const hire = button(tr('Нанять'), () => run(b, 'hire', { candidate: c.id }, () => tr('{0} принят(а) на работу.', w.name)));
  hire.disabled = b.workers.length >= b.seats || b.bankrupt;
  const about = [tr('навык {0}', w.skill), tr('просит {0} в час', hundredths(w.wage))];
  if (w.trait) about.push(traitName(w.trait));

  return el('tr', {},
    el('td', {},
      el('b', { textContent: w.name }), ' — ', professionName(w.profession).toLowerCase(),
      el('div', { className: 'hint', textContent: about.join('; ') }),
      professionWork(w.profession) ? el('div', { className: 'hint', textContent: professionWork(w.profession) }) : '',
    ),
    el('td', { className: 'biz-cell-action' }, hire),
  );
}

function staffPanel(b) {
  const spec = specOf(b.kind);
  const loads = spec.professions.filter((p) => p.serve > 0).map((p) => fact(
    `${professionName(p.profession)}:`, businessLoad(b.load[p.profession]), !(b.load[p.profession] <= 1),
  ));

  return {
    title: tr('Работники — {0} из {1}', b.workers.length, b.seats),
    body: [
      facts(...loads),
      b.workers.length ? table(...b.workers.map((w) => workerRow(b, w))) : hint(tr('Работников нет: без них покупатели уходят ни с чем.')),
      heading(tr('Кандидаты')),
      hint(b.candidatesUntil ? tr('Новые придут {0}.', formatDateTime(b.candidatesUntil)) : ''),
      b.candidates.length ? table(...b.candidates.map((c) => candidateRow(b, c))) : hint(tr('Все кандидаты уже наняты — ждите новых.')),
      b.workers.length >= b.seats ? hint(tr('Свободных мест нет: улучшите бизнес.')) : null,
    ],
    actions: [],
  };
}

function supplyRow(b, s, spoil) {
  const hours = s.use > 0 ? Math.floor(s.units / s.use) : null;
  const left = hours === null ? tr('сейчас не расходуется') : tr('хватит на {0} ч.', hours);
  const about = [tr('расход {0} в час', decimal(s.use)), left];
  if (s.onWay > 0) about.push(tr('в пути {0}', businessUnits(s.onWay)));
  if (spoil) about.push(tr('портится за {0} ч.', spoil));

  return el('tr', {},
    el('td', {},
      el('b', { textContent: supplyName(s.supply) }),
      el('div', { className: 'biz-label', textContent: tr('{0} из {1}', businessUnits(s.units), businessUnits(s.storage)) }),
      bar(s.units, s.storage, s.units < 1 && s.onWay === 0),
      el('div', { className: hours !== null && hours < 3 ? 'hint biz-warn' : 'hint', textContent: about.join('; ') }),
    ),
  );
}

// orderForm buys a supply from a supplier for the votes of the owner.
function orderForm(b) {
  const supplies = choice(b.supplies.map((s) => [s.supply, supplyName(s.supply)]), b.supplies[0].supply);
  const suppliers = choice(state.suppliers.map((s) => [s.supplier, supplierName(s.supplier)]), 'reliable');
  const units = numberField(0, 1, 1e9);
  const cost = el('span', { className: 'hint' });
  const about = hint('');

  const room = () => {
    const s = b.supplies.find((x) => x.supply === supplies.value);
    return Math.max(Math.floor(s.storage - s.units - s.onWay), 0);
  };
  const update = () => {
    const s = b.supplies.find((x) => x.supply === supplies.value);
    const sup = state.suppliers.find((x) => x.supplier === suppliers.value);
    cost.textContent = tr('стоит {0}', votesAmount(orderCost(Number(units.value) || 0, s.unitCosts[suppliers.value] ?? 0)));
    about.textContent = tr('{0}: {1}; поместится ещё {2}', supplierName(sup.supplier), supplierAbout(sup), businessUnits(room()));
  };
  supplies.addEventListener('change', () => { units.value = String(room()); update(); });
  suppliers.addEventListener('change', update);
  units.addEventListener('input', update);
  units.value = String(room());
  update();

  const order = button(tr('Заказать'), () => run(b, 'order', { supply: supplies.value, supplier: suppliers.value, amount: Number(units.value) || 0 },
    (d) => tr('Заказано за {0}.', votesAmount(d.spent))));

  return [heading(tr('Закупка')), row(supplies, suppliers), row(units, order, cost), about];
}

// contractForm sets or ends the standing order of a supply: the buyer
// orders it from the till when it runs low.
function contractForm(b) {
  const buyer = b.workers.some((w) => w.profession === 'buyer');
  const supplies = choice(b.supplies.map((s) => [s.supply, supplyName(s.supply)]), b.supplies[0].supply);
  const suppliers = choice(state.suppliers.map((s) => [s.supplier, supplierName(s.supplier)]), 'reliable');
  const below = numberField(0, 1, 1e9);
  const units = numberField(0, 1, 1e9);
  const fill = () => {
    const c = b.contracts.find((x) => x.supply === supplies.value);
    const s = b.supplies.find((x) => x.supply === supplies.value);
    if (c) suppliers.value = c.supplier;
    below.value = String(c?.below ?? Math.floor(s.storage / 4));
    units.value = String(c?.units ?? Math.floor(s.storage / 2));
  };
  supplies.addEventListener('change', fill);
  fill();

  const lines = b.contracts.map((c) => el('li', { className: c.stalled ? 'biz-warn' : '', textContent: tr('{0}: {1}, когда меньше {2} — {3}{4}',
    supplyName(c.supply), businessUnits(c.units), businessUnits(c.below), supplierName(c.supplier), c.stalled ? tr(' (в кассе нет денег)') : '') }));

  return [
    heading(tr('Договоры поставок')),
    hint(buyer
      ? tr('Закупщик сам заказывает товар из кассы, когда его остаётся меньше порога.')
      : tr('Договоры ведёт закупщик — без него они не работают. Наймите его.')),
    lines.length ? el('ul', { className: 'biz-list' }, ...lines) : null,
    row(supplies, suppliers),
    row(el('span', { className: 'biz-label', textContent: tr('Когда меньше, шт.:') }), below, el('span', { className: 'biz-label', textContent: tr('заказывать, шт.:') }), units),
    row(
      button(tr('Заключить договор'), () => run(b, 'contract', { supply: supplies.value, supplier: suppliers.value, below: Number(below.value) || 0, amount: Number(units.value) || 0 },
        () => tr('Договор на «{0}» заключён.', supplyName(supplies.value)))),
      b.contracts.some((c) => c.supply === supplies.value)
        ? button(tr('Расторгнуть'), () => run(b, 'contract', { supply: supplies.value, supplier: suppliers.value, amount: 0 },
          () => tr('Договор на «{0}» расторгнут.', supplyName(supplies.value))), { secondary: true })
        : '',
    ),
  ];
}

function storagePanel(b) {
  const spec = specOf(b.kind);
  const keeper = b.workers.some((w) => w.profession === 'storekeeper');
  const orders = b.orders.map((o) => el('li', { textContent: tr('{0}: {1} от «{2}», привезут {3}{4}',
    supplyName(o.supply), businessUnits(o.units), supplierName(o.supplier), formatDateTime(o.arrives), o.fromTill ? tr(' (по договору)') : '') }));

  return {
    title: tr('Склад'),
    body: [
      table(...b.supplies.map((s) => supplyRow(b, s, spec.supplies.find((x) => x.supply === s.supply)?.spoil))),
      keeper ? null : hint(tr('Кладовщика нет: при приёмке теряется 10 % каждой поставки.')),
      orders.length ? heading(tr('В пути')) : null,
      orders.length ? el('ul', { className: 'biz-list' }, ...orders) : null,
      ...(b.bankrupt ? [] : [...orderForm(b), ...contractForm(b)]),
    ],
    actions: [],
  };
}

function tillPanel(b) {
  const amount = numberField(b.inDebt ? Math.ceil(-b.cash / 100) : 10, 1, 1e12);
  const tax = Math.ceil((Math.floor(Math.max(b.cash, 0) / 100) * b.tax) / 100);

  const actions = [];
  if (b.cash >= 100 && !b.bankrupt) {
    actions.push(button(tr('Забрать выручку'), () => run(b, 'collect', {}, (d) => tr('Из кассы забрано {0}, налог — {1}.', votesAmount(d.got), votesAmount(d.tax)))));
  }
  if (!b.bankrupt) {
    actions.push(button(b.inDebt ? tr('Погасить долг') : tr('Пополнить кассу'), () => run(b, 'deposit', { amount: Number(amount.value) || 0 },
      (d) => tr('В кассу положено {0}.', votesAmount(d.spent))), { secondary: b.cash >= 100 }));
  }

  return {
    title: tr('Касса'),
    body: [
      b.inDebt
        ? el('div', { className: 'biz-warn', textContent: tr('Долг: {0} гол. Разорение — с {1} гол.', hundredths(-b.cash), hundredths(b.debtLimit)) })
        : el('div', { className: 'biz-label', textContent: tr('В кассе: {0} из {1} гол.', hundredths(b.cash), hundredths(b.till)) }),
      b.inDebt ? bar(-b.cash, b.debtLimit, true) : bar(b.cash, b.till),
      hint(b.inDebt
        ? tr('Зарплаты, аренда и договоры идут из кассы всегда. Положите в неё голоса, пока долг не разорил бизнес.')
        : tr('Выручка сверх того, что вмещает касса, пропадает — забирайте её. Налог — {0} %{1}.', b.tax, b.cash >= 100 ? tr(', сейчас — {0}', votesAmount(tax)) : '')),
      b.bankrupt ? null : row(el('span', { className: 'biz-label', textContent: b.inDebt ? tr('Погасить, голосов:') : tr('Пополнить, голосов:') }), amount),
    ],
    actions,
  };
}

function adPanel(b, plan) {
  const ad = b.adUntil && new Date(b.adUntil) > new Date();

  return {
    title: tr('Рекламный щит'),
    body: [
      facts(
        fact(tr('Сейчас:'), ad ? tr('реклама до {0}', formatDateTime(b.adUntil)) : tr('рекламы нет')),
        fact(tr('Кампания:'), tr('+{0} % покупателей на {1} ч.', plan.adBoost, plan.adHours)),
      ),
      hint(tr('Кампании складываются: каждая продлевает рекламу. Маркетолог приводит покупателей и без неё.')),
    ],
    actions: b.bankrupt ? [] : [button(tr('Реклама за {0}', votesAmount(plan.adPrice)), () => run(b, 'advertise', {},
      () => tr('Реклама запущена: +{0} % покупателей на {1} ч.', plan.adBoost, plan.adHours)))],
  };
}

// journalPanel is what happened lately, the newest first, and the last
// hours of the business.
function journalPanel(b) {
  const notes = [...b.notes].reverse().map((n) => el('li', {}, el('span', { className: 'hint', textContent: `${formatDateTime(n.at)} ` }), businessNote(n)));
  const hours = [...b.hours].reverse().slice(0, 8).map((h) => {
    const costs = h.wages + h.rent + h.supplies + h.losses;
    const net = h.income - costs;
    return el('tr', {},
      el('td', { className: 'biz-label', textContent: formatDateTime(h.hour) }),
      el('td', { textContent: `+${hundredths(Math.round(h.income))}` }),
      el('td', { textContent: `−${hundredths(Math.round(costs))}` }),
      el('td', { className: net < 0 ? 'biz-warn' : '', textContent: hundredths(Math.round(net)) }),
      el('td', { className: 'hint', textContent: h.unserved >= 1 ? tr('ушли ни с чем: {0}', Math.floor(h.unserved)) : '' }),
    );
  });

  return {
    title: tr('Журнал'),
    body: [
      hours.length ? heading(tr('Последние часы')) : null,
      hours.length ? table(...hours) : null,
      heading(tr('События')),
      notes.length ? el('ul', { className: 'biz-list biz-notes' }, ...notes) : hint(tr('Пока ничего не случилось.')),
    ],
    actions: [],
  };
}

// eventPanel is what waits for the owner: the answers, the default one
// first, which applies by itself when the day is over.
function eventPanel(b) {
  if (!b.event) return journalPanel(b);
  const e = b.event;
  const { title, text } = businessEvent(e);

  return {
    title,
    body: [
      el('p', { textContent: text }),
      hint(tr('Если не решить до {0}, будет «{1}».', formatDateTime(e.until), businessOption(e.options[0], e))),
    ],
    actions: e.options.map((option, i) => button(businessOption(option, e), () => run(b, 'decide', { option }, () => tr('Решено: «{0}».', businessOption(option, e))), { secondary: i === 0 })),
  };
}

function lotPanel(plan) {
  const name = businessName(plan.kind);
  const spec = specOf(plan.kind);

  return {
    title: tr('Купить: {0}', name),
    body: [
      el('div', { className: 'biz-preview' }, picture(buildings[plan.kind])),
      facts(
        fact(tr('Цена:'), votesAmount(plan.price)),
        fact(tr('Уровней:'), String(plan.levels)),
        fact(tr('Продаёт:'), spec.offers.map((o) => offerName(o.offer)).join(', ')),
        fact(tr('Закупает:'), spec.supplies.map((s) => supplyName(s.supply)).join(', ')),
        fact(tr('Работники:'), tr('до {0} на уровень', plan.staff)),
        fact(tr('Аренда в час:'), tr('{0} гол. за уровень', hundredths(plan.rent))),
      ),
      hint(tr('После покупки закупите товар на складе и наймите работников — без них продаж нет.')),
    ],
    actions: [button(tr('Купить за {0}', votesAmount(plan.price)), () => act(
      plan.kind, () => api.buyBusiness(plan.kind), () => tr('Вы купили бизнес «{0}». Закупите товар и наймите работников.', name),
    ))],
  };
}

const panels = { building: buildingPanel, offers: offersPanel, staff: staffPanel, storage: storagePanel, till: tillPanel, ad: adPanel, journal: journalPanel, event: eventPanel };

// fillPanel fills the open box with the part it shows, as it is now.
function fillPanel(message = '') {
  const plan = planOf(panel.kind);
  const b = state.businesses.find((x) => x.kind === panel.kind);
  const view = panel.part === 'lot' ? lotPanel(plan) : panels[panel.part](b, plan);

  panel.modal.box.querySelector('.modal-header').textContent = view.title;
  panel.error = el('div', { className: 'error', hidden: true });
  panel.modal.body.replaceChildren(
    message ? el('div', { className: 'biz-said', textContent: message }) : '',
    panel.error,
    ...view.body.filter(Boolean),
  );
  panel.modal.actions.replaceChildren(...view.actions, button(tr('Закрыть'), panel.modal.close, { secondary: true }));
}

function openPanel(kind, part) {
  hideError($('error'));
  said('');
  const modal = openModal('');
  modal.box.classList.add('biz-dialog');
  panel = { kind, part, modal, error: null };
  fillPanel();
}

function sell(b, name) {
  panel.modal.body.replaceChildren(el('p', { textContent: tr('Продать бизнес «{0}» сайту за {1}? Касса уходит вместе с ним.', name, votesAmount(b.resale)) }));
  panel.modal.actions.replaceChildren(
    button(tr('Продать'), () => run(b, 'sell', {}, (d) => tr('Бизнес «{0}» продан за {1}.', name, votesAmount(d.got)))),
    button(tr('Отмена'), () => fillPanel(), { secondary: true }),
  );
}

const me = await currentUser().catch(() => null);
renderChrome(me);

$('business').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
} else {
  showBalance(me.votes ?? 0);
  try {
    state = await answered;
    show();
  } catch (e) {
    showError($('error'), e);
  }
}
