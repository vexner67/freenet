// "Бизнес" (/business), a tab of "Заработок": the economy of the
// enterprises as the browser games of the 2000s had it — pixel pictures of
// the enterprises and the goods, tiles and plaques. The tabs: "Мои
// предприятия" (tiles of those the page holds shares of, the news of the
// city, and founding a new one by its picture; ?kind= picks one), the card
// of an enterprise (?firm=<id>, its own tabs in &part=: production,
// improvements, the orders of the city, the exchange, the report, the
// shareholders), "Заказы города" (?tab=contracts), "Биржа" (the prices of
// the goods; &good= — the orders of one) and "Справочник" (the chains from
// the raw materials to the shop); the shares are traded in "Акции"
// (/stocks). Nothing is a form of fields: the plaques of the card — the
// cash, the workers, the level, the machines, the share — open mini
// windows of steppers, sliders and quick amounts over themselves. The
// server counts the hours; the page asks for the city again when an hour
// ends and only shows what the server said.

import { api, currentUser, session, started } from './api.js';
import {
  balanceVotes, balanceWord, balanceAmount, firmEventAbout, firmEventName, firmHour, firmKindName, firmPictures, formatDateTime, goodName, goodPictures, groupedVotes, hundredths,
  hundredthsOf, improvements, isGoodEvent, sharePercent, votesAmount,
} from './labels.js';
import { $, el, fullName, onScreen, pagePath, replaceInPlace, renderChrome, showError, hideError } from './layout.js';
import { button, openModal, toast } from './modal.js';
import { lineChart, sparkline } from './charts.js';
import { chips, isOpen, meter, popover, segmented, slider, stepper } from './controls.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.firms()) : null;

let state = null;
let me = null;
let balance = 0;
let busy = false;
let asked = 0; // when refresh last asked for the city
// The kind chosen to found, kept while the page is redrawn.
let chosen = '';

function showBalance(votes) {
  balance = votes;
  $('votes').textContent = balanceVotes(votes);
  $('votes-word').textContent = balanceWord(votes);
  $('balance').hidden = false;
}

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

// money is a sum in hundredths of a vote: "12,5 гол.".
const money = (n) => tr('{0} гол.', hundredths(n));
const decimal = (n) => String(Math.round(n * 100) / 100).replace('.', ',');

// signed is a sum that may be a loss: green with a plus, red with a minus.
function signed(n) {
  return el('span', { className: n > 0 ? 'firm-up' : n < 0 ? 'firm-down' : '', textContent: (n > 0 ? '+' : '') + money(n) });
}

const specOf = (kind) => state.catalog.find((c) => c.kind === kind);
const planOf = (kind) => state.settings.plans.find((p) => p.kind === kind);
const quoteOf = (good) => state.quotes.find((q) => q.good === good);
const firmOf = (id) => state.firms.find((f) => f.id === id);
const held = (f) => f.holders.find((h) => h.user.id === me.id)?.shares ?? 0;
const directs = (f) => f.director?.id === me.id;

// Whether the storage of an enterprise buys a good (what it makes of or a
// shop sells) and sells it (what it makes, or a shop to the city).
const buys = (spec, good) => spec.inputs.some((i) => i.good === good) || spec.sells.includes(good);
const sells = (spec, good) => spec.output === good || spec.sells.includes(good);
// retailed is whether the city buys a good from the shops.
const retailed = (good) => state.catalog.some((s) => s.sells.includes(good));

// productivity is what the user service counts of a wage: the square root
// of it against the standard, from 0,5 to 1,5 (FirmRules.Productivity).
const productivity = (wage) => Math.min(Math.max(Math.sqrt(wage / state.settings.wage), 0.5), 1.5);

// --- the address of the page ---

function where() {
  const q = new URLSearchParams(location.search);
  return {
    tab: q.get('tab') ?? 'mine', firm: q.get('firm') ?? '', part: q.get('part') ?? 'work', good: q.get('good') ?? '', kind: q.get('kind') ?? '',
  };
}

// go opens another address of the page; a tab keeps the page where it is,
// as the tabs of the other pages do, another link opens it at its top.
function go(href, isTab = false) {
  history.pushState(null, '', href);
  hideError($('error'));
  said('');
  show();
  if (!isTab) window.scrollTo(0, 0);
}

// follow opens a link of the page without loading it again.
function follow(event) {
  if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  go(event.currentTarget.getAttribute('href'), Boolean(event.currentTarget.closest('.tabs')));
}

const firmPath = (f, part = '') => `/business?firm=${encodeURIComponent(f.id)}${part ? `&part=${part}` : ''}`;
const link = (text, href, className = '') => el('a', { href, className, textContent: text, onclick: follow });

// --- the pieces of the page ---

const firmIcon = (kind, size) => el('img', { className: 'firm-icon', src: firmPictures[kind], alt: '', width: size, height: size });
const goodIcon = (good, size = 16) => el('img', { className: 'firm-icon', src: goodPictures[good], alt: '', width: size, height: size });

// goodLink is a good with its picture, a link to its orders.
function goodLink(good) {
  return el('a', { className: 'firm-good', href: `/business?tab=exchange&good=${encodeURIComponent(good)}`, onclick: follow },
    goodIcon(good), el('span', { textContent: goodName(good) }));
}

// firmLink is an enterprise with its picture.
function firmLink(f) {
  return el('a', { className: 'firm-good', href: firmPath(f), onclick: follow }, firmIcon(f.kind, 16), el('span', { textContent: f.name }));
}

const person = (p) => (p ? el('a', { href: pagePath(p), textContent: fullName(p) }) : tr('—'));
const hint = (text) => el('p', { className: 'hint firm-hint', textContent: text });
const title = (text) => el('div', { className: 'section-title', textContent: text });

// bar is a strip filled to part of whole.
function bar(part, whole) {
  const fill = el('span', { className: 'firm-bar-fill' });
  fill.style.width = `${whole > 0 ? Math.min(100, Math.round((part / whole) * 100)) : 0}%`;
  return el('span', { className: 'firm-bar' }, fill);
}

// table is a table of fixed columns: [heading, width in pixels or 0 for
// the rest, whether it holds numbers]; a wide one scrolls inside itself on
// a narrow screen.
function table(columns, rows, footer = null) {
  const cell = (tag, c, i) => {
    const node = c instanceof Node ? el(tag, {}, c) : el(tag, { textContent: String(c ?? '') });
    if (columns[i][2]) node.className = 'firm-num';
    return node;
  };
  const cols = columns.map(([, width]) => {
    const col = el('col');
    if (width) col.style.width = `${width}px`;
    return col;
  });

  return el('div', { className: 'firm-scroll' },
    el('table', { className: 'list-table firm-table' },
      el('colgroup', {}, ...cols),
      el('thead', {}, el('tr', {}, ...columns.map((c, i) => cell('th', c[0], i)))),
      el('tbody', {}, ...rows.map((cells) => el('tr', {}, ...cells.map((c, i) => cell('td', c, i))))),
      footer ? el('tfoot', {}, el('tr', {}, ...footer.map((c, i) => cell('td', c, i)))) : null,
    ));
}

// line is a row of a grid: its label and what follows it.
function line(label, ...controls) {
  return [el('div', { className: 'firm-label', textContent: label }), el('div', { className: 'firm-control' }, ...controls.filter(Boolean))];
}

// grid is rows of a label and controls, aligned in two columns; with
// submit it is a form.
function grid(lines, submit = null) {
  const node = el(submit ? 'form' : 'div', { className: 'firm-grid' }, ...lines.flat());
  if (submit) {
    node.addEventListener('submit', (event) => {
      event.preventDefault();
      submit();
    });
  }
  return node;
}

function field(props, className = 'firm-number') {
  return el('input', { className: `text ${className}`, ...props });
}

const note = () => el('span', { className: 'firm-note' });
const submit = (text) => button(text, null, { type: 'submit' });

// act does an action, shows the city it left and says what was done. The
// error goes to box (an open dialog's) or over the tabs.
async function act(action, body, text, box = null) {
  if (busy) return false;
  busy = true;
  const error = box ?? $('error');
  hideError(error);
  said('');

  try {
    const done = await api.runFirms(action, body);
    if (done.spent > 0 || done.got > 0) showBalance(done.balance);
    state = done.firms;
    // What an action leads to takes the place of the page it was done on:
    // "back" goes to the page before, not to an enterprise no longer there.
    if (action === 'found') history.replaceState(null, '', firmPath({ id: done.firm }));
    if (action === 'liquidate') history.replaceState(null, '', '/business');
    show();
    const message = typeof text === 'function' ? text(done) : text;
    said(message);
    // The page stays where the action was done; what was done is said
    // over the tabs and, when they are out of sight, at the bottom.
    if (message && !onScreen($('done'))) toast(message);
    return true;
  } catch (e) {
    showError(error, e);
    if (!onScreen(error)) toast(error.textContent);
    return false;
  } finally {
    busy = false;
  }
}

// dialog opens a box of an action with its own place for errors.
function dialog(heading, ...body) {
  const modal = openModal(heading);
  modal.box.classList.add('firm-dialog');
  const error = el('div', { className: 'error', hidden: true });
  modal.body.replaceChildren(error, ...body.filter(Boolean));
  return { modal, error };
}

// recipe is what an enterprise does in pictures: "2 × [зерно] → [мука]",
// or for a shop what it sells the city.
function recipe(spec) {
  const parts = [];
  if (spec.sells.length) {
    spec.sells.forEach((g, i) => parts.push(i ? ', ' : '', goodIcon(g), ` ${goodName(g).toLowerCase()}`));
    parts.push(' → ', tr('город'));
  } else {
    spec.inputs.forEach((input, i) => parts.push(i ? ' + ' : '', `${input.units} × `, goodIcon(input.good), ` ${goodName(input.good).toLowerCase()}`));
    if (spec.inputs.length) parts.push(' → ');
    parts.push(goodIcon(spec.output), ` ${goodName(spec.output).toLowerCase()}`);
  }
  return el('span', { className: 'firm-recipe' }, ...parts);
}

// rate is how much an enterprise makes or sells a worker an hour.
function rate(spec) {
  if (!spec.sells.length) return tr('{0} шт. в час на рабочего', decimal(spec.rate[spec.output]));
  return tr('в час на рабочего: {0}', spec.sells.map((g) => `${goodName(g).toLowerCase()} — ${decimal(spec.rate[g])}`).join(', '));
}

// clock says when the hour of the enterprises ends: a bar of the hour
// filling up and the minutes and seconds left; the page ticks it every
// second.
function clock() {
  const node = el('div', { className: 'firm-clock' });
  tick(node);
  return node;
}

const hourLength = 3600000;

function tick(node) {
  if (!state?.nextHour) return;
  const left = Math.max(0, Date.parse(state.nextHour) - Date.now());
  const seconds = Math.ceil(left / 1000);
  const countdown = `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
  node.replaceChildren(
    el('span', { className: 'hint', textContent: tr('Рабочий час закончится в {0}', firmHour(state.nextHour)) }),
    meter(hourLength - Math.min(left, hourLength), hourLength, left ? countdown : tr('считаем…')));
}

// profits are the profits of the hours of an enterprise, oldest first.
const profits = (f) => [...f.hours].sort((a, b) => a.hour.localeCompare(b.hour))
  .map((h) => h.income - h.purchases - h.wages - h.upkeep - h.fees);

// trend is a sparkline of the profits of the last hours.
const trend = (f, width) => (f.hours.length > 1 ? sparkline(profits(f), { width, height: 22 }) : null);

// watched marks a number the page compares with what it showed before:
// one that changed blinks once.
const watched = (key, node) => {
  node.dataset.watch = key;
  return node;
};
const seen = new Map();

function blink() {
  for (const node of document.querySelectorAll('[data-watch]')) {
    const before = seen.get(node.dataset.watch);
    if (before !== undefined && before !== node.textContent) node.classList.add('firm-changed');
    seen.set(node.dataset.watch, node.textContent);
  }
}

// The hour going on comes minute by minute: the city counts it only when
// it ends, but forecast tells how it ends if nothing changes, and the page
// shows that share of it which has passed. lives are the numbers drawn so,
// redrawn every second.
const lives = new Set();

// passed is the share of the hour going on that has passed, 0 to 1.
function passed() {
  if (!state?.nextHour) return 0;
  return Math.min(Math.max(1 - (Date.parse(state.nextHour) - Date.now()) / hourLength, 0), 1);
}

const hourStart = () => Date.parse(state.nextHour) - hourLength;

// sofar is what of the hour going on the report has already: the deals of
// the exchange made in it.
const sofar = (f) => f.hours.find((h) => Date.parse(h.hour) >= hourStart());

// coming is the hour going on of f as far as it has passed — each number
// of its report, and the cash — or null without a forecast.
function coming(f) {
  if (!f.forecast) return null;
  const share = passed();
  const before = sofar(f) ?? {};
  const part = (name) => Math.round((before[name] ?? 0) + ((f.forecast[name] ?? 0) - (before[name] ?? 0)) * share);
  const hour = {};
  for (const name of ['made', 'bought', 'sold', 'income', 'purchases', 'wages', 'upkeep', 'fees']) hour[name] = part(name);
  return { hour, cash: Math.round(f.cash + (f.forecastCash - f.cash) * share) };
}

// live is a node draw fills now and every second after.
function live(draw, tag = 'span') {
  const node = el(tag);
  const entry = { node, draw };
  lives.add(entry);
  node.replaceChildren(...[draw()].flat());
  return node;
}

function redraw() {
  for (const entry of lives) {
    if (entry.node.isConnected) entry.node.replaceChildren(...[entry.draw()].flat());
    else lives.delete(entry);
  }
}

// cashNow is the cash of f now, growing as the hour goes on.
const cashNow = (f) => live(() => money(coming(f)?.cash ?? f.cash));

// hourProfit is "+12 гол. за этот час" as it has come so far.
function hourProfit(f) {
  return live(() => {
    const now = coming(f);
    if (!now) return [];
    const h = now.hour;
    return [signed(h.income - h.purchases - h.wages - h.upkeep - h.fees), tr(' за этот час')];
  });
}

// --- "Мои предприятия" ---

function myFirms() {
  const mine = state.firms.filter((f) => held(f) > 0);
  const nodes = [];
  if (!mine.length) {
    nodes.push(title(tr('Как начать')), steps());
  } else {
    nodes.push(title(tr('Мои предприятия')), el('div', { className: 'firm-tiles' }, ...mine.map(tile)), clock());
  }
  nodes.push(title(tr('Рынок труда')), ...labour());
  if (state.news.length) nodes.push(title(tr('Новости города')), news(state.news.slice(0, 6)));
  nodes.push(title(tr('Основать предприятие')), ...founding());
  return nodes;
}

// labour is the labour market of the city: its people, those out of work
// and the wage the enterprises pay on average.
function labour() {
  const l = state.labour;
  const fact = (name, value) => el('div', { className: 'stock-fact' }, el('span', { textContent: name }), el('b', { textContent: value }));
  return [
    el('div', { className: 'stock-facts firm-labour' },
      fact(tr('Горожан'), groupedVotes(l.people)),
      fact(tr('Работают'), groupedVotes(l.employed)),
      fact(tr('Без работы'), groupedVotes(l.people - l.employed)),
      fact(tr('Вакансий'), groupedVotes(Math.max(l.jobs - l.employed, 0))),
      fact(tr('Средняя зарплата'), money(l.wage))),
    hint(tr('Люди приезжают в город за работой, но всех вакансий не закрывают. Безработные идут к тем, кто платит больше, а платящие меньше средней теряют рабочих.')),
  ];
}

function steps() {
  const step = (n, picture, heading, text) => el('div', { className: 'firm-step' },
    el('div', { className: 'firm-step-head' }, el('span', { className: 'firm-step-number', textContent: String(n) }), el('img', { className: 'firm-icon', src: picture, alt: '', width: 48, height: 48 })),
    el('b', { textContent: heading }),
    el('p', { textContent: text }));

  return el('div', { className: 'firm-steps' },
    step(1, '/static/firms/farm.svg', tr('Основайте предприятие'), tr('Проще всего начать с фермы, рудника, нефтевышки или лесхоза: им не нужно сырьё. Рабочих для нового предприятия город найдёт сразу.')),
    step(2, '/static/goods/grain.svg', tr('Оно работает само'), tr('Каждый час рабочие делают продукцию, а склад продаёт её государству. Выручка копится в кассе предприятия.')),
    step(3, '/static/firms/dividends.svg', tr('Забирайте прибыль'), tr('Выплатите дивиденды — голоса из кассы придут на ваш счёт. Получать их могут страницы старше недели с заполненной анкетой.')),
  );
}

function tile(f) {
  const fact = (name, value) => el('tr', {}, el('td', { className: 'firm-tile-label', textContent: name }), value instanceof Node ? el('td', {}, value) : el('td', { textContent: value }));
  const working = !idle(f) && f.workers > 0;
  return el('a', { className: working ? 'firm-tile working' : 'firm-tile', href: firmPath(f), onclick: follow },
    el('span', { className: 'firm-tile-side' },
      el('span', { className: 'firm-tile-icon' }, firmIcon(f.kind, 48)),
      trend(f, 56)),
    el('span', { className: 'firm-tile-text' },
      el('b', { textContent: f.name }),
      el('span', { className: 'firm-tile-kind', textContent: `${firmKindName(f.kind)} · ${tr('{0} ур.', f.level)}${directs(f) ? ` · ${tr('директор')}` : ''}` }),
      el('table', { className: 'firm-tile-facts' },
        fact(tr('Касса'), watched(`${f.id}:cash`, cashNow(f))),
        f.forecast ? fact(tr('Сейчас'), hourProfit(f)) : null,
        fact(tr('За сутки'), watched(`${f.id}:profit`, el('span', {}, signed(f.profit)))),
        fact(tr('Акции'), `${held(f)} (${sharePercent(held(f), f.shares)})`),
        fact(tr('Станки'), idle(f) ? el('span', { className: 'firm-down', textContent: tr('забастовка') }) : meter(100 - f.wear, 100, tr('{0} %', Math.round(100 - f.wear)), wearTone(f.wear))))));
}

// The tiers of the enterprises as "Основать предприятие" groups them.
const tiers = [[1, tr('Добыча сырья')], [2, tr('Переработка')], [3, tr('Производство товаров')], [4, tr('Торговля')]];

function founding() {
  const { kind } = where();
  if (kind && specOf(kind)) chosen = kind;
  if (!specOf(chosen)) chosen = state.catalog[0].kind;

  const picker = el('div', { className: 'firm-kinds' });
  const about = el('div', { className: 'firm-chosen' });
  const name = field({ type: 'text', maxLength: 40, required: true, placeholder: tr('например, «Колосок»') }, 'firm-name');
  const found = submit('');
  const short = note();

  function draw() {
    picker.replaceChildren(...tiers.map(([tier, heading]) => el('div', { className: 'firm-kinds-group' },
      el('div', { className: 'firm-kinds-title', textContent: heading }),
      el('div', { className: 'firm-kinds-row' }, ...state.catalog.filter((s) => s.tier === tier).map((s) => {
        const plan = planOf(s.kind);
        return el('button', {
          type: 'button', className: s.kind === chosen ? 'firm-kind chosen' : 'firm-kind', title: firmKindName(s.kind),
          onclick: () => { chosen = s.kind; draw(); },
        }, firmIcon(s.kind, 32), el('span', { className: 'firm-kind-name', textContent: firmKindName(s.kind) }),
        el('span', { className: 'firm-kind-price', textContent: votesAmount(plan.price) }));
      })))));

    const spec = specOf(chosen);
    const plan = planOf(chosen);
    about.replaceChildren(
      el('span', { className: 'firm-chosen-icon' }, firmIcon(chosen, 48)),
      el('div', { className: 'firm-chosen-text' },
        el('b', { textContent: firmKindName(chosen) }),
        el('div', {}, recipe(spec)),
        el('div', { className: 'hint', textContent: rate(spec) }),
        el('div', { className: 'hint', textContent: tr('Рабочих — {0} на уровень, уровней — {1}. В кассу при основании — {2}.', plan.staff, plan.levels, votesAmount(Math.floor((plan.price * state.settings.capital) / 100))) }),
      ));
    found.textContent = tr('Основать за {0}', votesAmount(plan.price));
    short.textContent = balance < plan.price ? tr('не хватает {0}', votesAmount(plan.price - balance)) : '';
    short.classList.toggle('firm-down', balance < plan.price);
  }
  draw();

  return [
    picker,
    about,
    grid([line(tr('Название:'), name, found, short)], () => act('found', { kind: chosen, name: name.value },
      tr('Предприятие «{0}» основано. Рабочие уже наняты — первая продукция будет к концу часа.', name.value.trim()))),
  ];
}

// --- the card of an enterprise ---

// plain is a sum in hundredths as it is typed: "12,5".
const plain = (n) => hundredths(n).replace(/ /g, '');

// priceBox is a price in hundredths with "−" and "+" a step of about a
// hundredth of it.
function priceBox(value, onchange = () => {}) {
  const input = el('input', { type: 'text', inputMode: 'decimal', className: 'step-value wide', value: plain(value) });
  const get = () => hundredthsOf(input.value);
  const nudge = (k) => el('button', {
    type: 'button', className: 'step-button', textContent: k < 0 ? '−' : '+',
    onclick: () => {
      const p = get() || value || 1;
      input.value = plain(Math.max(1, p + k * Math.max(1, Math.round(p / 100))));
      onchange(get());
    },
  });
  input.addEventListener('input', () => onchange(get()));
  return { node: el('span', { className: 'stepper' }, nudge(-1), input, nudge(1)), get, set: (n) => { input.value = plain(n); onchange(get()); } };
}

// mini is a row of a mini window.
const row = (...children) => el('div', { className: 'pop-row' }, ...children.filter(Boolean));
const popNote = (text) => el('div', { className: 'pop-note', textContent: text });
const popActions = (...buttons) => el('div', { className: 'pop-actions' }, ...buttons.filter(Boolean));

// popAct does an action of a mini window and closes it when done.
async function popAct(pop, action, body, text) {
  if (await act(action, body, text, pop.error)) pop.close();
}

const shareOf = (f) => f.price || Math.floor(f.worth / f.shares);
const idle = (f) => f.idleUntil && Date.parse(f.idleUntil) > Date.now();
const wearTone = (wear) => (wear >= 60 ? 'bad' : wear >= 30 ? 'warn' : 'good');

function firmCard(f, part) {
  if (!f) return [hint(tr('Такого предприятия нет: его ликвидировали.')), link(tr('« Мои предприятия'), '/business')];

  const plan = planOf(f.kind);
  const parts = [
    ['work', tr('Производство')],
    ['improve', tr('Улучшения')],
    ['contracts', tr('Заказы города')],
    directs(f) ? ['trade', tr('Заявки')] : null,
    directs(f) ? ['routes', tr('Поставки')] : null,
    ['report', tr('Отчёт')],
    ['holders', tr('Акционеры')],
  ].filter(Boolean);
  if (!parts.some(([p]) => p === part)) part = 'work';

  const view = {
    work: () => production(f),
    improve: () => upgrades(f),
    contracts: () => firmContracts(f),
    trade: () => orders(f),
    routes: () => routes(f),
    report: () => [report(f)],
    holders: () => holders(f),
  }[part]();

  return [
    el('div', { className: 'firm-back' }, link(tr('« Мои предприятия'), '/business')),
    header(f, plan),
    idle(f) ? el('div', { className: 'firm-alarm', textContent: tr('Забастовка: рабочие не выходят до {0}. Платите не ниже нормы — {1} в час.', firmHour(f.idleUntil), money(state.settings.wage)) }) : null,
    stats(f, plan),
    clock(),
    el('div', { className: 'tabs firm-subtabs' }, ...parts.map(([p, text]) => link(text, firmPath(f, p), p === part ? 'active' : ''))),
    ...view,
    directs(f) ? el('div', { className: 'firm-close' }, el('a', { href: '#', textContent: tr('Ликвидировать предприятие'), onclick: (e) => { e.preventDefault(); liquidate(f); } })) : null,
  ];
}

function header(f, plan) {
  const name = el('div', { className: 'firm-header-name' }, el('span', { textContent: f.name }));
  if (directs(f)) {
    const edit = el('button', { type: 'button', className: 'firm-rename', title: tr('Переименовать'), textContent: '✎' });
    edit.addEventListener('click', () => {
      const input = el('input', { type: 'text', className: 'text firm-name-edit', maxLength: 40, value: f.name });
      const save = () => {
        if (input.value.trim() && input.value.trim() !== f.name) act('rename', { firm: f.id, name: input.value }, tr('Предприятие переименовано.'));
        else show();
      };
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') save();
        if (e.key === 'Escape') show();
      });
      name.replaceChildren(input, button(tr('Сохранить'), save));
      input.focus();
      input.select();
    });
    name.append(edit);
  }

  return el('div', { className: 'firm-header' },
    el('span', { className: 'firm-header-icon' }, firmIcon(f.kind, 64)),
    el('div', { className: 'firm-header-text' },
      name,
      el('div', { className: 'firm-header-kind', textContent: `${firmKindName(f.kind)} · ${tr('основано {0}', formatDateTime(f.foundedAt))}` }),
      el('div', {}, recipe(specOf(f.kind))),
      el('div', {}, el('span', { className: 'hint', textContent: tr('Директор: ') }), person(f.director)),
    ),
    el('div', { className: 'firm-header-level' }, el('span', { textContent: tr('Уровень {0} из {1}', f.level, plan.levels) }), meter(f.level, plan.levels)));
}

// plaque is a plaque of the card; onclick opens its mini window.
function plaque(name, value, sub, onclick = null, key = '') {
  const shown = el('div', { className: 'firm-stat-value' }, value);
  const node = el(onclick ? 'button' : 'div', { className: onclick ? 'firm-stat clickable' : 'firm-stat' },
    el('div', { className: 'firm-stat-name' }, name, onclick ? el('span', { className: 'firm-stat-more', textContent: '▾' }) : null),
    key ? watched(key, shown) : shown,
    el('div', { className: 'firm-stat-sub' }, sub));
  if (onclick) {
    node.type = 'button';
    node.addEventListener('click', () => onclick(node));
  }
  return node;
}

function stats(f, plan) {
  const own = directs(f);
  return el('div', { className: 'firm-stats' },
    plaque(tr('Касса'), cashNow(f), f.forecast ? hourProfit(f) : f.escrow > 0 ? tr('и {0} в заявках', money(f.escrow)) : tr('расходы {0} в час', money(f.upkeep + f.workers * f.wage)), own ? (a) => cashBox(a, f) : null, `${f.id}:cash`),
    plaque(tr('За сутки'), el('span', { className: 'firm-stat-trend' }, signed(f.profit), trend(f, 64)), tr('прибыль за {0} ч.', f.hours.length), null, `${f.id}:profit`),
    plaque(tr('Рабочие'), `${f.workers} / ${f.wanted}`, f.wanted > f.workers ? tr('ищет ещё {0}, выработка {1} %', f.wanted - f.workers, Math.round(f.productivity * 100)) : tr('выработка {0} %', Math.round(f.productivity * 100)), own ? (a) => staffBox(a, f) : null, `${f.id}:workers`),
    plaque(tr('Уровень'), `${f.level} / ${plan.levels}`, f.upgradeCost ? tr('следующий — {0}', money(f.upgradeCost * 100)) : tr('высший'), own ? (a) => levelBox(a, f, plan) : null),
    plaque(tr('Станки'), meter(100 - f.wear, 100, tr('{0} %', Math.round(100 - f.wear)), wearTone(f.wear)), f.wear >= 30 ? tr('изношены — делают меньше') : tr('в порядке'), own ? (a) => repairBox(a, f) : null),
    plaque(tr('Акция'), money(shareOf(f)), f.float > 0 ? tr('{0} новых на бирже', groupedVotes(f.float)) : tr('торгуется в «Акциях»'), (a) => shareBox(a, f)),
  );
}

// cashBox is the mini window of the cash: votes put in, dividends paid.
function cashBox(anchor, f) {
  const pop = popover(anchor, tr('Касса предприятия'));
  if (!pop) return;
  const body = { firm: f.id };
  const cash = Math.floor(f.cash / 100);

  const put = stepper({ value: Math.min(100, Math.max(1, balance)), min: 1, max: Math.max(1, balance), onchange: () => recount(), wide: true });
  const pay = slider({ value: Math.floor(cash / 2), min: 0, max: Math.max(cash, 1), onchange: () => recount() });
  const deposit = el('div', {});
  const dividend = el('div', {});
  const putNote = popNote('');
  const payNote = popNote('');
  const putGo = el('button', { type: 'button', className: 'button', onclick: () => popAct(pop, 'deposit', { ...body, votes: put.get() }, (d) => tr('Вы вложили в кассу {0}.', votesAmount(d.spent))) });
  const payGo = el('button', { type: 'button', className: 'button', onclick: () => popAct(pop, 'dividend', { ...body, votes: pay.get() }, (d) => tr('Дивиденды выплачены, ваша доля — {0}.', votesAmount(d.got))) });

  function recount() {
    putNote.textContent = tr('Станет в кассе {0}, у вас останется {1}.', money(f.cash + put.get() * 100), balanceAmount(balance - put.get()));
    putGo.textContent = tr('Вложить {0}', votesAmount(put.get()));
    putGo.disabled = balance < 1;
    const v = pay.get();
    payNote.textContent = tr('Всем акционерам — {0}, вам — {1}.', votesAmount(v), votesAmount(Math.floor((v * held(f)) / f.shares)));
    payGo.textContent = tr('Выплатить {0}', votesAmount(v));
    payGo.disabled = v < 1;
  }

  deposit.append(
    row(put.node),
    row(chips([['10', () => put.set(10)], ['100', () => put.set(100)], ['1 000', () => put.set(1000)], [tr('всё'), () => put.set(balance)]])),
    putNote, popActions(putGo));
  dividend.append(
    row(pay.node),
    row(chips([['25 %', () => pay.set(Math.floor(cash / 4))], ['50 %', () => pay.set(Math.floor(cash / 2))], [tr('всю кассу'), () => pay.set(cash)]])),
    payNote, popActions(payGo),
    popNote(tr('Дивиденды делятся между всеми акционерами по числу акций. Получать их могут страницы старше недели с заполненной анкетой.')));

  const ways = segmented([['in', tr('Вложить')], ['out', tr('Дивиденды')]], 'in', (v) => {
    deposit.hidden = v !== 'in';
    dividend.hidden = v !== 'out';
    pop.place();
  });
  dividend.hidden = true;
  pop.body.append(row(ways.node), deposit, dividend);
  recount();
  pop.place();
}

// staffBox is the mini window of the workers and their wage.
function staffBox(anchor, f) {
  const pop = popover(anchor, tr('Рабочие'));
  if (!pop) return;
  const norm = state.settings.wage;

  const workers = stepper({ value: f.wanted, min: 0, max: f.seats, onchange: () => recount() });
  const wage = slider({ value: f.wage, min: Math.ceil(norm / 2), max: norm * 3, onchange: () => recount() });
  const wageText = el('b', {});
  const note = popNote('');
  const warn = el('div', { className: 'pop-note firm-down' });
  const go = el('button', { type: 'button', className: 'button', textContent: tr('Сохранить'), onclick: () => popAct(pop, 'staff', { firm: f.id, workers: workers.get(), wage: wage.get() }, tr('Набор рабочих и зарплата изменены.')) });
  const market = state.labour;
  const [hired, gone] = f.hours.reduce(([h, l], x) => [h + x.hired, l + x.left], [0, 0]);

  function recount() {
    const w = wage.get();
    wageText.textContent = tr('{0} в час', money(w));
    note.textContent = tr('Выработка {0} %, зарплаты {1} в час. Норма — {2}, в среднем по городу — {3}.', Math.round(productivity(w) * 100), money(Math.min(workers.get(), f.workers) * w), money(norm), money(market.wage));
    warn.textContent = [
      w < norm ? tr('Ниже нормы рабочие могут забастовать.') : '',
      w < market.wage ? tr('Ниже средней по городу рабочие уходят к тем, кто платит больше.') : '',
    ].filter(Boolean).join(' ');
  }

  pop.body.append(
    popNote(tr('Сейчас работают {0} из {1}. За сутки наняты {2}, ушли {3}.', f.workers, f.wanted, hired, gone)),
    row(el('span', { className: 'pop-label', textContent: tr('Нужно рабочих') }), workers.node, chips([[tr('все места'), () => workers.set(f.seats)]])),
    row(el('span', { className: 'pop-label', textContent: tr('Зарплата') }), wageText),
    row(wage.node),
    row(chips([[tr('½ нормы'), () => wage.set(Math.ceil(norm / 2))], [tr('норма'), () => wage.set(norm)], [tr('×2'), () => wage.set(norm * 2)], [tr('×3'), () => wage.set(norm * 3)]])),
    note, warn,
    popNote(tr('Лишних рабочих увольняют сразу, а вакансии занимают безработные горожане — сначала у тех, кто платит больше. Без работы сейчас {0}; если людей не хватает, новые приезжают в город за несколько часов.', market.people - market.employed)),
    popActions(go));
  recount();
  pop.place();
}

function levelBox(anchor, f, plan) {
  const pop = popover(anchor, tr('Уровень'));
  if (!pop) return;
  if (f.level >= plan.levels) {
    pop.body.append(popNote(tr('Предприятие на высшем уровне. Расти дальше можно улучшениями.')));
    pop.place();
    return;
  }

  const cost = f.upgradeCost * 100;
  const go = el('button', { type: 'button', className: 'button', textContent: tr('Улучшить за {0}', money(cost)), onclick: () => popAct(pop, 'upgrade', { firm: f.id }, tr('Предприятие улучшено до {0} ур.', f.level + 1)) });
  go.disabled = f.cash < cost;
  pop.body.append(
    row(meter(f.level, plan.levels, tr('{0} → {1}', f.level, f.level + 1))),
    popNote(tr('Будет {0} рабочих мест, склад больше, а содержание — {1} в час.', plan.staff * (f.level + 1), money(plan.upkeep * (f.level + 1)))),
    f.cash < cost ? el('div', { className: 'pop-note firm-down', textContent: tr('В кассе не хватает {0}.', money(cost - f.cash)) }) : null,
    popActions(go));
  pop.place();
}

function repairBox(anchor, f) {
  const pop = popover(anchor, tr('Станки'));
  if (!pop) return;
  const go = el('button', { type: 'button', className: 'button', textContent: tr('Починить за {0}', money(f.repairCost)), onclick: () => popAct(pop, 'repair', { firm: f.id }, tr('Станки починены.')) });
  go.disabled = f.wear < 1 || f.cash < f.repairCost;
  pop.body.append(
    row(meter(100 - f.wear, 100, tr('исправны на {0} %', Math.round(100 - f.wear)), wearTone(f.wear))),
    popNote(tr('Станки изнашиваются от работы, а от поломок — сразу на четверть. Изношенные больше чем на 30 % делают меньше: при полном износе — меньше половины.')),
    f.cash < f.repairCost ? el('div', { className: 'pop-note firm-down', textContent: tr('В кассе не хватает {0}.', money(f.repairCost - f.cash)) }) : null,
    popActions(go));
  pop.place();
}

// shareBox is the mini window of the share: its price, and for the
// director the new shares put up on the stock exchange.
function shareBox(anchor, f) {
  const pop = popover(anchor, tr('Акции предприятия'));
  if (!pop) return;
  pop.body.append(
    popNote(tr('Последняя цена — {0}, стоимость акции — {1}. Акций — {2}, у вас {3}.', money(shareOf(f)), money(Math.floor(f.worth / f.shares)), groupedVotes(f.shares), groupedVotes(held(f)))),
    row(el('a', { href: `/stocks?firm=${encodeURIComponent(f.id)}`, textContent: tr('Торговать на бирже »') })));

  if (directs(f)) {
    const issue = stepper({ value: f.float, min: 0, max: 1_000_000, step: 10, onchange: () => recount(), wide: true });
    const note = popNote('');
    const go = el('button', { type: 'button', className: 'button', textContent: tr('Выставить'), onclick: () => popAct(pop, 'issue', { firm: f.id, shares: issue.get() }, tr('Новые акции выставлены на биржу.')) });
    const recount = () => {
      const n = issue.get();
      note.textContent = n
        ? tr('Биржа продаст {0} новых акций не дешевле их стоимости — деньги придут в кассу. Ваша доля станет {1}.', groupedVotes(n), sharePercent(held(f), f.shares + n))
        : tr('Новых акций на бирже не будет.');
    };
    pop.body.append(
      el('div', { className: 'biz-subtitle', textContent: tr('Выпуск новых акций') }),
      row(issue.node, chips([['0', () => issue.set(0)], ['10', () => issue.set(10)], ['50', () => issue.set(50)], ['100', () => issue.set(100)]])),
      note, popActions(go));
    recount();
  }
  pop.place();
}

function production(f) {
  const spec = specOf(f.kind);
  const workers = idle(f) ? 0 : f.workers * f.productivity;
  const now = spec.sells.length
    ? spec.sells.map((g) => `${goodName(g).toLowerCase()} — ${decimal(workers * spec.rate[g])}`).join(', ')
    : tr('{0} шт.', decimal(workers * spec.rate[spec.output]));

  return [
    workshop(f, spec, workers),
    el('div', { className: 'firm-work' },
      el('span', { textContent: rate(spec) }),
      el('b', { textContent: tr('Сейчас в час: {0}', now) })),
    title(tr('Склад')),
    el('div', { className: 'firm-goods' }, ...f.stock.map((s) => goodTile(f, spec, s))),
    hint(directs(f)
      ? tr('Нажмите на товар, чтобы продать его государству, купить или настроить склад: он сам закупает сырьё и продаёт продукцию по ценам не хуже заданных — сначала у других предприятий, потом у государства.')
      : tr('Склад сам закупает сырьё и продаёт продукцию — сначала другим предприятиям, потом государству.')),
    ...(directs(f) ? offers(f) : []),
  ];
}

// workshop is the enterprise at work: its raw materials ride a belt into
// it and its goods come out, the faster the more workers it has; when no
// one works, everything stands still.
function workshop(f, spec, workers) {
  const inputs = spec.sells.length ? spec.sells : spec.inputs.map((i) => i.good);
  const made = spec.sells.length ? [] : [spec.output];
  const empty = (g) => !f.stock.some((s) => s.good === g && s.units > 0);
  const short = spec.sells.length ? inputs.every(empty) : inputs.some(empty);
  const stopped = workers <= 0 || short;
  const item = (good, i) => el('span', { className: `firm-belt-item n${i}` }, goodIcon(good, 20));
  const belt = (goods) => el('span', { className: 'firm-belt' }, ...[0, 1, 2].map((i) => item(goods[i % goods.length], i)));
  const each = (goods) => el('span', { className: 'firm-line-goods' }, ...goods.map((g) => goodIcon(g, 28)));
  const output = spec.sells.length
    ? el('img', { className: 'firm-icon', src: '/static/firms/dividends.svg', alt: '', width: 28, height: 28 })
    : each(made);
  const amount = spec.sells.length ? tr('выручка') : tr('+{0} в час', decimal(workers * spec.rate[spec.output]));

  const node = el('div', { className: stopped ? 'firm-line stopped' : 'firm-line' },
    inputs.length ? each(inputs) : el('span', { className: 'firm-line-source', textContent: tr('недра') }),
    inputs.length ? belt(inputs) : belt(made),
    el('span', { className: 'firm-line-plant' }, firmIcon(f.kind, 48), el('span', { className: 'firm-line-smoke' })),
    belt(spec.sells.length ? inputs : made),
    el('span', { className: 'firm-line-out' }, output, el('b', { textContent: idle(f) ? tr('забастовка') : workers <= 0 ? tr('нет рабочих') : short ? (spec.sells.length ? tr('нет товара') : tr('нет сырья')) : amount })));
  // The belt runs from 6 s a parcel with one worker in ten seats to 1.5 s
  // with all of them.
  const load = Math.min(1, workers / Math.max(1, f.seats));
  node.style.setProperty('--belt-time', `${(6 - 4.5 * load).toFixed(2)}s`);
  return node;
}

function goodTile(f, spec, s) {
  const badges = [];
  if (buys(spec, s.good)) badges.push(el('span', { className: s.autoBuy ? 'firm-badge on' : 'firm-badge', textContent: s.autoBuy ? tr('закупает до {0}', hundredths(s.buyMax)) : tr('не закупает') }));
  if (sells(spec, s.good)) badges.push(el('span', { className: s.autoSell ? 'firm-badge on' : 'firm-badge', textContent: s.autoSell ? tr('продаёт от {0}', hundredths(s.sellMin)) : tr('не продаёт') }));

  const tile = el(directs(f) ? 'button' : 'div', { className: directs(f) ? 'firm-good-tile clickable' : 'firm-good-tile' },
    goodIcon(s.good, 32),
    el('span', { className: 'firm-good-text' },
      el('b', { textContent: goodName(s.good) }),
      meter(s.units, s.capacity, s.incoming > 0 ? tr('{0} (+{1}) из {2}', groupedVotes(s.units), groupedVotes(s.incoming), groupedVotes(s.capacity)) : tr('{0} из {1}', groupedVotes(s.units), groupedVotes(s.capacity))),
      el('span', { className: 'firm-badges' }, ...badges)));
  if (directs(f)) {
    tile.type = 'button';
    tile.addEventListener('click', () => stockBox(tile, f, spec, s));
  }
  return tile;
}

// stockBox is the mini window of a good of the storage: a deal with the
// state and what the storage does by itself.
function stockBox(anchor, f, spec, s) {
  const pop = popover(anchor, goodName(s.good));
  if (!pop) return;
  const q = quoteOf(s.good);
  const body = { firm: f.id, good: s.good };
  const canBuy = buys(spec, s.good);

  const units = stepper({ value: Math.max(1, Math.min(100, s.units)), min: 1, max: Math.max(1, s.units, s.capacity - s.units), wide: true });
  const sell = el('button', { type: 'button', className: 'button', textContent: tr('Продать'), onclick: () => trade('state_sell') });
  const buy = canBuy ? el('button', { type: 'button', className: 'button', textContent: tr('Купить'), onclick: () => trade('state_buy') }) : null;
  function trade(action) {
    const n = units.get();
    popAct(pop, action, { ...body, units: n }, action === 'state_buy' ? tr('Куплено у государства: {0} шт.', groupedVotes(n)) : tr('Продано государству: {0} шт.', groupedVotes(n)));
  }

  const autoBuy = el('input', { type: 'checkbox', checked: s.autoBuy });
  const autoSell = el('input', { type: 'checkbox', checked: s.autoSell });
  const buyMax = priceBox(s.buyMax || q.stateAsk);
  const sellMin = priceBox(s.sellMin || q.stateBid);
  const save = el('button', {
    type: 'button', className: 'button', textContent: tr('Сохранить'),
    onclick: () => popAct(pop, 'auto', { ...body, autoBuy: autoBuy.checked, buyMax: buyMax.get() || 0, autoSell: autoSell.checked, sellMin: sellMin.get() || 0 }, tr('Склад настроен.')),
  });

  pop.body.append(
    row(meter(s.units, s.capacity, tr('{0} из {1}', groupedVotes(s.units), groupedVotes(s.capacity)))),
    popNote(retailed(s.good)
      ? tr('Государство покупает по {0}, продаёт по {1}; город платит магазинам по {2}.', money(q.stateBid), money(q.stateAsk), money(q.cityPrice))
      : tr('Государство покупает по {0}, продаёт по {1}.', money(q.stateBid), money(q.stateAsk))),
    el('div', { className: 'biz-subtitle', textContent: tr('Сделка с государством') }),
    row(units.node, chips([['10', () => units.set(10)], ['100', () => units.set(100)], [tr('весь склад'), () => units.set(s.units)]])),
    popActions(buy, sell),
    el('div', { className: 'biz-subtitle', textContent: tr('Склад сам') }),
    canBuy ? row(el('label', { className: 'firm-check' }, autoBuy, tr('закупать до')), buyMax.node) : null,
    sells(spec, s.good) ? row(el('label', { className: 'firm-check' }, autoSell, spec.sells.includes(s.good) ? tr('продавать городу от') : tr('продавать от')), sellMin.node) : null,
    popActions(save));
  pop.place();
}

function upgrades(f) {
  const own = directs(f);
  return [
    el('div', { className: 'firm-upgrades' }, ...improvements.map(([kind, name, about]) => {
      const imp = f.improvements.find((i) => i.kind === kind) ?? { rank: 0, cost: 0 };
      const pips = el('span', { className: 'firm-pips' }, ...[1, 2, 3].map((n) => el('span', { className: n <= imp.rank ? 'firm-pip on' : 'firm-pip' })));
      const go = own && imp.cost ? button(tr('Улучшить за {0}', money(imp.cost)), () => act('improve', { firm: f.id, improvement: kind }, tr('«{0}» улучшено до {1} ранга.', name, imp.rank + 1))) : null;
      if (go) go.disabled = f.cash < imp.cost;
      return el('div', { className: imp.rank ? 'firm-upgrade done' : 'firm-upgrade' },
        el('b', { textContent: name }),
        pips,
        el('span', { className: 'hint', textContent: about }),
        go ?? el('span', { className: 'hint', textContent: imp.cost ? tr('следующий ранг — {0}', money(imp.cost)) : tr('высший ранг') }));
    })),
    hint(tr('Улучшения оплачиваются из кассы и остаются с предприятием: половина их цены входит в его стоимость и в цену акции.')),
  ];
}

// --- the orders of the city ---

const left = (iso) => {
  const hours = Math.max(0, Math.round((Date.parse(iso) - Date.now()) / 3600000));
  return tr('{0} ч.', hours);
};

// contractCard is an order of the city; f is the enterprise it is looked
// at from.
function contractCard(k, f = null) {
  const taker = k.firm ? firmOf(k.firm) : null;
  const mine = taker && directs(taker);
  const unfit = !k.firm && !f && !fits(k);
  const rest = k.units - k.delivered;
  const actions = [];
  if (!k.firm && !unfit) actions.push(button(tr('Взять'), (e) => takeBox(e.currentTarget, k, f)));
  if (mine) actions.push(button(tr('Сдать'), (e) => deliverBox(e.currentTarget, k, taker)));
  if (unfit) actions.push(el('span', { className: 'firm-contract-need', textContent: tr('нужно: {0}', makers(k.good)) }));

  return el('div', { className: mine ? 'firm-contract mine' : unfit ? 'firm-contract unfit' : 'firm-contract' },
    goodIcon(k.good, 32),
    el('div', { className: 'firm-contract-text' },
      el('b', { textContent: tr('{0}: {1} шт. по {2}', goodName(k.good), groupedVotes(k.units), money(k.price)) }),
      k.firm
        ? meter(k.delivered, k.units, tr('сдано {0} из {1}', groupedVotes(k.delivered), groupedVotes(k.units)))
        : el('span', { className: 'hint', textContent: tr('Всего {0}. Ждёт исполнителя ещё {1}', money(k.units * k.price), left(k.deadline)) }),
      k.firm ? el('span', { className: 'hint' }, taker ? `${tr('Взял: {0}.', taker.name)} ` : '', tr('Сдать до {0} — осталось {1}, штраф за несданное — {2}', firmHour(k.deadline), left(k.deadline), money(Math.floor((rest * k.price) / 10)))) : null),
    el('div', { className: 'firm-contract-actions' }, ...actions));
}

function takeBox(anchor, k, f) {
  const pop = popover(anchor, tr('Взять заказ'));
  if (!pop) return;
  const able = (f ? [f] : state.firms).filter((x) => directs(x) && x.stock.some((s) => s.good === k.good));
  if (!able.length) {
    pop.body.append(popNote(tr('Ни одно ваше предприятие не держит {0} на складе.', goodName(k.good).toLowerCase())));
  } else {
    pop.body.append(
      popNote(tr('Предприятие должно сдать {0} шт. за сутки, иначе заплатит штраф — десятую часть цены несданного.', groupedVotes(k.units))),
      ...able.map((x) => row(el('button', {
        type: 'button', className: 'firm-pick', onclick: () => popAct(pop, 'take_contract', { firm: x.id, contract: k.id }, tr('Заказ взят: «{0}» сдаёт {1}.', x.name, goodName(k.good).toLowerCase())),
      }, firmIcon(x.kind, 24), el('span', { textContent: x.name }), el('span', { className: 'hint', textContent: tr('на складе {0}', groupedVotes(x.stock.find((s) => s.good === k.good).units)) })))));
  }
  pop.place();
}

function deliverBox(anchor, k, f) {
  const pop = popover(anchor, tr('Сдать заказ'));
  if (!pop) return;
  const stock = f.stock.find((s) => s.good === k.good)?.units ?? 0;
  const most = Math.min(stock, k.units - k.delivered);
  if (most < 1) {
    pop.body.append(popNote(tr('На складе нет {0}: сдавать нечего.', goodName(k.good).toLowerCase())));
    pop.place();
    return;
  }
  const units = stepper({ value: most, min: 1, max: most, onchange: () => recount(), wide: true });
  const go = el('button', { type: 'button', className: 'button', onclick: () => popAct(pop, 'deliver', { contract: k.id, units: units.get() }, () => tr('Сдано {0} шт., в кассу — {1}.', groupedVotes(units.get()), money(units.get() * k.price))) });
  const recount = () => {
    go.textContent = tr('Сдать за {0}', money(units.get() * k.price));
  };
  pop.body.append(row(units.node, chips([[tr('всё, что есть'), () => units.set(most)]])), popActions(go));
  recount();
  pop.place();
}

function firmContracts(f) {
  const own = state.contracts.filter((k) => k.firm === f.id);
  const open = state.contracts.filter((k) => !k.firm && f.stock.some((s) => s.good === k.good));
  return [
    title(tr('Взятые заказы')),
    own.length ? el('div', { className: 'firm-contracts' }, ...own.map((k) => contractCard(k, f))) : hint(tr('Предприятие не выполняет заказов.')),
    title(tr('Можно взять')),
    open.length ? el('div', { className: 'firm-contracts' }, ...open.map((k) => contractCard(k, f))) : hint(tr('Сейчас город не заказывает того, что держит это предприятие.')),
    hint(tr('Город платит за заказ на треть-половину дороже обычной цены. Предприятие берёт не больше двух заказов сразу.')),
  ];
}

// fits is whether an enterprise the caller directs keeps the good of the
// order: it can take it.
const fits = (k) => state.firms.some((x) => directs(x) && x.stock.some((s) => s.good === k.good));

// makers are the enterprises that make a good: "мельница".
const makers = (good) => state.catalog.filter((s) => s.output === good).map((s) => firmKindName(s.kind).toLowerCase()).join(', ');

// contracts are the orders of the city: those of the caller's enterprises
// first, then those they can take, and the rest grey with what they need.
function contracts() {
  const rank = (k) => (k.firm ? (directs(firmOf(k.firm) ?? {}) ? 0 : 3) : fits(k) ? 1 : 2);
  const list = [...state.contracts].sort((a, b) => rank(a) - rank(b));
  return [
    title(tr('Заказы города')),
    list.length ? el('div', { className: 'firm-contracts' }, ...list.map((k) => contractCard(k))) : hint(tr('Сейчас город ничего не заказывает. Новые заказы появляются почти каждый час.')),
    hint(tr('Город заказывает то, что делают его предприятия, на треть-половину дороже обычной цены: до 8 заказов сразу, новый — в шести часах из десяти. Взятый заказ нужно сдать за сутки со склада предприятия, а за несданное — штраф: десятая часть его цены. Серые заказы ваши предприятия взять не могут.')),
  ];
}

// news are the latest events of the city.
function news(events) {
  return el('div', { className: 'firm-news' }, ...events.map((e) => {
    const firm = e.firm ? firmOf(e.firm) : null;
    return el('div', { className: `firm-news-item ${isGoodEvent(e.kind) ? 'good' : 'bad'}` },
      firmIcon(firm?.kind ?? e.kinds[0] ?? 'shop', 24),
      el('div', {},
        el('b', { textContent: firm ? `${firmEventName(e.kind)}: ${firm.name}` : firmEventName(e.kind) }),
        el('div', { textContent: firmEventAbout(e.kind) }),
        el('div', { className: 'hint', textContent: formatDateTime(e.start) })));
  }));
}

function liquidate(f) {
  const { modal, error } = dialog(tr('Ликвидация'),
    el('p', { textContent: tr('Ликвидировать «{0}»? Предприятие закроется, заявки снимутся, а акционеры получат по акциям {1}: половину вложенного в основание и улучшения, кассу и деньги заявок.', f.name, money(f.worth)) }));
  modal.actions.replaceChildren(
    button(tr('Ликвидировать'), async () => {
      if (await act('liquidate', { firm: f.id }, (d) => tr('Предприятие «{0}» ликвидировано, ваша доля — {1}.', f.name, votesAmount(d.got)), error)) modal.close();
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// offers are the orders of other enterprises this one takes in a click:
// those that sell what it buys and those that buy what it makes. The deal
// goes at the price of the order, as many units as the order, the cash
// and the storage allow, with nothing to type.
function offers(f) {
  const spec = specOf(f.kind);
  const stockOf = (good) => f.stock.find((s) => s.good === good);
  const wanted = state.orders
    .filter((o) => o.firmId !== f.id && (o.buy ? spec.output === o.good : buys(spec, o.good)))
    .sort((a, b) => Number(a.buy) - Number(b.buy) || (a.buy ? b.price - a.price : a.price - b.price));
  const heading = title(tr('Предложения других предприятий'));
  if (!wanted.length) return [heading, hint(tr('Сейчас никто не продаёт то, что нужно вашему предприятию, и не покупает то, что оно делает.'))];

  const rows = wanted.map((o) => {
    const s = stockOf(o.good);
    const room = Math.max(0, (s?.capacity ?? 0) - (s?.units ?? 0) - (s?.incoming ?? 0));
    const units = o.buy ? Math.min(o.units, s?.units ?? 0) : Math.min(o.units, Math.floor(f.cash / o.price), room);
    const other = firmOf(o.firmId);
    const take = el('button', {
      type: 'button',
      className: `button deal-take ${o.buy ? 'sell' : 'buy'}`,
      disabled: units < 1,
      title: units < 1 ? (o.buy ? tr('На складе нет этого товара') : tr('Не хватает денег в кассе или места на складе')) : '',
      textContent: o.buy ? tr('Продать {0} шт.', groupedVotes(Math.max(units, 0))) : tr('Купить {0} шт.', groupedVotes(Math.max(units, 0))),
      onclick: () => act('order', { firm: f.id, good: o.good, buy: !o.buy, price: o.price, units }, (d) => {
        const dealt = d.deals.reduce((n, deal) => n + deal.units, 0);
        return o.buy ? tr('Продано {0} шт. по {1}.', groupedVotes(dealt), money(o.price)) : tr('Куплено {0} шт. по {1}.', groupedVotes(dealt), money(o.price));
      }),
    });
    return [
      goodLink(o.good),
      other ? firmLink(other) : '',
      el('span', { className: o.buy ? 'deal-side buy' : 'deal-side sell', textContent: o.buy ? tr('покупает') : tr('продаёт') }),
      hundredths(o.price), groupedVotes(o.units), take,
    ];
  });

  return [
    heading,
    table([[tr('Товар'), 0], [tr('Предприятие'), 0], ['', 70], [tr('Цена'), 60, true], [tr('Шт.'), 60, true], ['', 120, true]], rows),
    hint(tr('Нажмите кнопку — сделка пройдёт сразу по цене заявки. Склад, который закупает и продаёт сам, каждый час сначала берёт такие заявки и товар других предприятий по цене между ценами государства, а уже потом идёт к государству.')),
  ];
}

function orders(f) {
  const own = state.orders.filter((o) => o.firmId === f.id);
  const nodes = [...offers(f), title(tr('Заявки предприятия'))];
  nodes.push(own.length
    ? table([[tr('Товар'), 0], [tr('Заявка'), 80], [tr('Цена'), 80, true], [tr('Осталось'), 80, true], ['', 70, true]], own.map((o) => [
      goodLink(o.good), o.buy ? tr('покупка') : tr('продажа'), hundredths(o.price), groupedVotes(o.units),
      el('a', { href: '#', textContent: tr('Снять'), onclick: (e) => { e.preventDefault(); act('cancel', { order: o.id }, tr('Заявка снята.')); } }),
    ]))
    : hint(tr('Заявок нет.')));

  let good = f.stock[0].good;
  let buy = false;
  const goods = el('span', { className: 'firm-good-pick' });
  const units = stepper({ value: 100, min: 1, max: 10_000_000, onchange: () => recount(), wide: true });
  const price = priceBox(quoteOf(good).stateBid, () => recount());
  const pricesRow = el('div', { className: 'deal-row' });
  const total = popNote('');
  const go = el('button', { type: 'button', className: 'button' });

  function drawGoods() {
    goods.replaceChildren(...f.stock.map((s) => el('button', {
      type: 'button', className: s.good === good ? 'firm-good-chip active' : 'firm-good-chip', title: goodName(s.good),
      onclick: () => {
        good = s.good;
        drawGoods();
        price.set(buy ? quoteOf(good).stateAsk : quoteOf(good).stateBid);
      },
    }, goodIcon(s.good, 24), el('span', { textContent: goodName(s.good) }))));
  }

  function recount() {
    const q = quoteOf(good);
    const pick = (name, value) => (value > 0 ? [`${name} ${hundredths(value)}`, () => price.set(value)] : null);
    pricesRow.replaceChildren(chips([pick(tr('спрос'), q.bestBid), pick(tr('предложение'), q.bestAsk), pick(tr('гос. покупает'), q.stateBid), pick(tr('гос. продаёт'), q.stateAsk)].filter(Boolean)));
    const p = price.get();
    total.textContent = p > 0 ? tr('На сумму {0}', money(units.get() * p)) : '';
    go.textContent = buy ? tr('Купить {0} шт.', groupedVotes(units.get())) : tr('Продать {0} шт.', groupedVotes(units.get()));
    go.className = `button deal-go ${buy ? 'buy' : 'sell'}`;
  }
  const sides = segmented([[false, tr('Продать'), 'sell'], [true, tr('Купить'), 'buy']], buy, (v) => {
    buy = v;
    price.set(buy ? quoteOf(good).stateAsk : quoteOf(good).stateBid);
  });
  go.addEventListener('click', () => act('order', { firm: f.id, good, buy, price: price.get(), units: units.get() }, (d) => {
    const dealt = d.deals.reduce((n, deal) => n + deal.units, 0);
    return dealt > 0 ? tr('Заявка выставлена, сразу исполнено {0} шт.', groupedVotes(dealt)) : tr('Заявка выставлена.');
  }));

  drawGoods();
  const priceRow = el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Цена') }), price.node);
  nodes.push(title(tr('Новая заявка')), el('div', { className: 'firm-order' },
    el('div', { className: 'deal-row' }, sides.node),
    el('div', { className: 'deal-row' }, goods),
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Штук') }), units.node,
      chips([['10', () => units.set(10)], ['100', () => units.set(100)], ['1 000', () => units.set(1000)]])),
    priceRow, pricesRow, total, go));
  recount();
  nodes.push(hint(tr('Заявка на покупку держит деньги кассы, на продажу — товар склада, пока её не исполнят или не снимут. Сделка идёт по цене той заявки, что выставлена раньше, а продавец платит комиссию {0} %. Нажмите на цену, чтобы подставить её.', state.settings.fee)));
  return nodes;
}

// takes is whether the enterprise works with a good: makes of it or, a
// shop, sells it to the city.
const takes = (f, good) => specOf(f.kind).inputs.some((i) => i.good === good) || specOf(f.kind).sells.includes(good);

// routes are the supplies of the enterprise to others of its director,
// handed every hour by themselves, and a form of a new one: the good, the
// enterprise it goes to, from how many units kept and how many an hour.
function routes(f) {
  const nodes = [title(tr('Поставки'))];
  nodes.push(f.routes.length
    ? table([[tr('Товар'), 0], [tr('Куда'), 0], [tr('Когда на складе от'), 110, true], [tr('В час'), 70, true], ['', 70, true]], f.routes.map((r) => [
      goodLink(r.good), firmOf(r.to) ? firmLink(firmOf(r.to)) : tr('—'), groupedVotes(r.from), groupedVotes(r.units),
      el('a', { href: '#', textContent: tr('Отменить'), onclick: (e) => { e.preventDefault(); act('unroute', { firm: f.id, route: r.id }, tr('Поставка отменена.')); } }),
    ]))
    : hint(tr('Поставок нет.')));

  const goods = f.stock.map((s) => s.good).filter((g) => state.firms.some((o) => o.id !== f.id && directs(o) && takes(o, g)));
  if (!goods.length) {
    nodes.push(hint(tr('Поставлять некуда: у вас нет другого предприятия, которому нужны товары этого.')));
    return nodes;
  }

  let good = goods[0];
  const goodPick = el('span', { className: 'firm-good-pick' });
  const target = el('select', { className: 'firm-route-to' });
  const from = stepper({ value: 0, min: 0, max: 1_000_000_000, wide: true });
  const units = stepper({ value: 10, min: 1, max: 1_000_000_000, wide: true });

  function draw() {
    goodPick.replaceChildren(...goods.map((g) => el('button', {
      type: 'button', className: g === good ? 'firm-good-chip active' : 'firm-good-chip', title: goodName(g),
      onclick: () => {
        good = g;
        draw();
      },
    }, goodIcon(g, 24), el('span', { textContent: goodName(g) }))));
    target.replaceChildren(...state.firms.filter((o) => o.id !== f.id && directs(o) && takes(o, good))
      .map((o) => el('option', { value: o.id, textContent: `${o.name} (${firmKindName(o.kind).toLowerCase()})` })));
  }
  draw();

  const go = button(tr('Добавить поставку'), () => act('route', { firm: f.id, good, to: target.value, from: from.get(), units: units.get() },
    tr('Поставка добавлена: она пойдёт с началом следующего часа.')));
  nodes.push(title(tr('Новая поставка')), el('div', { className: 'firm-order' },
    el('div', { className: 'deal-row' }, goodPick),
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Куда') }), target),
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('От, шт.') }), from.node,
      chips([['0', () => from.set(0)], ['100', () => from.set(100)], ['1 000', () => from.set(1000)]])),
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('В час, шт.') }), units.node,
      chips([['10', () => units.set(10)], ['100', () => units.set(100)], ['1 000', () => units.set(1000)]])),
    go));
  nodes.push(hint(tr('Каждый час, когда на складе этого предприятия товара не меньше «от», столько, сколько указано, уходит другому вашему предприятию — до продаж и сколько вместит его склад и оплатит касса. Цена — посередине между ценами государства, без комиссии биржи: акционеры обоих предприятий ничего не теряют. Поставок у предприятия — до {0}.', 5)));
  return nodes;
}

function report(f) {
  // The hour going on is the live row on top, not one of those ended.
  const ended = f.forecast ? f.hours.filter((h) => Date.parse(h.hour) < hourStart()) : f.hours;
  if (!ended.length && !f.forecast) return hint(tr('Предприятие ещё не отработало ни часа.'));

  const hours = [...ended].reverse();
  const costs = (h) => h.purchases + h.wages + h.upkeep + h.fees;
  const sum = (pick) => hours.reduce((n, h) => n + pick(h), 0);
  const profit = (h) => h.income - costs(h);
  let total = 0;
  const points = [...hours].reverse().map((h) => ({ time: h.hour, value: (total += profit(h)) }));
  return el('div', {},
    points.length > 1 ? el('div', { className: 'biz-subtitle', textContent: tr('Прибыль с начала суток') }) : null,
    points.length > 1 ? lineChart(points, { height: 150, format: (n) => (n > 0 ? '+' : '') + hundredths(n), when: firmHour, tick: firmHour }) : null,
    table(
    [[tr('Час'), 0], [tr('Сделано'), 66, true], [tr('Куплено'), 66, true], [tr('Продано'), 66, true], [tr('Выручка'), 76, true], [tr('Расходы'), 76, true], [tr('Прибыль'), 90, true]],
    [
      ...(f.forecast ? [comingRow(f, costs, profit)] : []),
      ...hours.map((h) => [firmHour(h.hour), h.made, h.bought, h.sold, hundredths(h.income), hundredths(costs(h)), signed(profit(h))]),
    ],
    [tr('Итого'), sum((h) => h.made), sum((h) => h.bought), sum((h) => h.sold), hundredths(sum((h) => h.income)), hundredths(sum(costs)), signed(sum(profit))],
  ));
}

// comingRow is the row of the hour going on in the report, growing every
// second.
function comingRow(f, costs, profit) {
  const cell = (pick) => live(() => {
    const h = coming(f)?.hour;
    return h ? pick(h) : '';
  });
  return [
    el('span', { className: 'firm-now', textContent: tr('{0}, идёт', firmHour(new Date(hourStart()).toISOString())) }),
    cell((h) => String(h.made)), cell((h) => String(h.bought)), cell((h) => String(h.sold)),
    cell((h) => hundredths(h.income)), cell((h) => hundredths(costs(h))), cell((h) => signed(profit(h))),
  ];
}

function holders(f) {
  const offers = state.tenders.filter((t) => t.firm === f.id);
  return [
    table([[tr('Акционер'), 0], [tr('Акций'), 70, true], [tr('Доля'), 150]], f.holders.map((h) => [
      person(h.user), groupedVotes(h.shares), el('span', { className: 'firm-share' }, bar(h.shares, f.shares), sharePercent(h.shares, f.shares)),
    ])),
    offers.length ? el('div', { className: 'firm-tenders' }, ...offers.map((t) => el('div', { className: 'firm-contract' },
      el('div', { className: 'firm-contract-text' },
        el('span', {}, person(t.user), ' ', tr('предлагает выкупить {0} акц. по {1}', groupedVotes(t.shares), money(t.price))),
        el('span', { className: 'hint', textContent: tr('до {0}', firmHour(t.until)) }))))) : null,
    hint(tr('Директор — тот, у кого больше всех акций: продав много, можно потерять управление, а собрав больше директора — получить его. Акции покупают, продают и предлагают выкупить в «Акциях».')),
    el('p', {}, el('a', { href: `/stocks?firm=${encodeURIComponent(f.id)}`, textContent: tr('Акции предприятия на бирже »') })),
  ];
}

// --- "Биржа" ---

const price = (n) => (n > 0 ? hundredths(n) : tr('—'));

function exchange() {
  return [
    title(tr('Котировки')),
    table(
      [[tr('Товар'), 0], [tr('Гос. купит'), 76, true], [tr('Гос. продаст'), 86, true], [tr('Город'), 56, true], [tr('Спрос'), 60, true], [tr('Предл.'), 60, true], [tr('Продано'), 64, true]],
      state.quotes.map((q) => [
        goodLink(q.good), price(q.stateBid), price(q.stateAsk), retailed(q.good) ? price(q.cityPrice) : tr('—'),
        price(q.bestBid), price(q.bestAsk), groupedVotes(q.volume),
      ]),
    ),
    hint(tr('Цены — в голосах за штуку. Государство покупает дешевле нормы и продаёт дороже: чем больше с ним наторговали за сутки, тем хуже его цены. Спрос и предложение — лучшие заявки игроков, «Продано» — штук на бирже за сутки. Город платит магазинам за хлеб, мебель и электронику. Нажмите на товар, чтобы увидеть его заявки.')),
  ];
}

function book(good) {
  const q = quoteOf(good);
  if (!q) return [hint(tr('Такого товара нет.')), link(tr('« Биржа'), '/business?tab=exchange')];

  const side = (buy) => state.orders.filter((o) => o.good === good && o.buy === buy)
    .sort((a, b) => (buy ? b.price - a.price : a.price - b.price) || a.placedAt.localeCompare(b.placedAt));
  const list = (orders, heading, empty) => el('div', {},
    el('div', { className: 'biz-subtitle', textContent: heading }),
    orders.length
      ? table([[tr('Цена'), 60, true], [tr('Шт.'), 60, true], [tr('Предприятие'), 0]], orders.map((o) => {
        const f = firmOf(o.firmId);
        return [hundredths(o.price), groupedVotes(o.units), f ? firmLink(f) : ''];
      }))
      : hint(empty));
  const makers = state.catalog.filter((s) => s.output === good).map((s) => firmKindName(s.kind).toLowerCase());
  const users = state.catalog.filter((s) => s.inputs.some((i) => i.good === good) || s.sells.includes(good)).map((s) => firmKindName(s.kind).toLowerCase());

  return [
    el('div', { className: 'firm-back' }, link(tr('« Биржа'), '/business?tab=exchange')),
    el('div', { className: 'firm-header' },
      el('span', { className: 'firm-header-icon' }, goodIcon(good, 64)),
      el('div', { className: 'firm-header-text' },
        el('div', { className: 'firm-header-name', textContent: goodName(good) }),
        makers.length ? el('div', { className: 'firm-header-kind', textContent: tr('Делает: {0}', makers.join(', ')) }) : null,
        users.length ? el('div', { className: 'firm-header-kind', textContent: tr('Покупает: {0}', users.join(', ')) }) : null,
      )),
    grid([
      line(tr('Государство:'), el('span', { textContent: tr('покупает по {0}, продаёт по {1}', money(q.stateBid), money(q.stateAsk)) })),
      retailed(good) ? line(tr('Город:'), el('span', { textContent: tr('платит магазинам по {0}', money(q.cityPrice)) })) : [],
      line(tr('Последняя сделка:'), el('span', { textContent: q.last > 0 ? money(q.last) : tr('ещё не было') })),
      line(tr('Продано за сутки:'), el('span', { textContent: tr('{0} шт.', groupedVotes(q.volume)) })),
    ]),
    el('div', { className: 'firm-book' },
      list(side(true), tr('Покупают'), tr('Заявок на покупку нет.')),
      list(side(false), tr('Продают'), tr('Заявок на продажу нет.'))),
    hint(tr('Выставить заявку можно в карточке своего предприятия, во вкладке «Заявки».')),
  ];
}

// --- "Справочник" ---

// chains are the ways from each raw material to the shop: an enterprise,
// what it makes, the enterprise that takes it, and on, as the catalog has
// them.
function chains() {
  const out = [];
  const walk = (path, spec) => {
    const next = [...path, ['firm', spec.kind]];
    if (spec.sells.length) {
      out.push(next);
      return;
    }
    const takers = state.catalog.filter((s) => s.inputs.some((i) => i.good === spec.output) || s.sells.includes(spec.output));
    for (const taker of takers) walk([...next, ['good', spec.output]], taker);
  };
  for (const spec of state.catalog.filter((s) => s.tier === 1)) walk([], spec);
  return out;
}

function guide() {
  const node = ([type, name]) => (type === 'firm'
    ? link('', `/business?kind=${name}`, 'firm-chain-firm')
    : el('span', { className: 'firm-chain-good' }, goodIcon(name, 24), el('span', { textContent: goodName(name) })));
  const firmNode = (a, kind) => {
    a.replaceChildren(firmIcon(kind, 32), el('span', { textContent: firmKindName(kind) }));
    a.title = tr('Основать: {0}', firmKindName(kind));
    return a;
  };

  return [
    title(tr('Как начать')),
    steps(),
    title(tr('Цепочки')),
    el('div', { className: 'firm-chains' }, ...chains().map((chain) => el('div', { className: 'firm-chain' },
      ...chain.flatMap((n, i) => {
        const piece = n[0] === 'firm' ? firmNode(node(n), n[1]) : node(n);
        return i ? [el('span', { className: 'firm-chain-arrow', textContent: '→' }), piece] : [piece];
      })))),
    hint(tr('Чем дальше по цепочке, тем дороже предприятие и тем больше оно зарабатывает. Нажмите на предприятие, чтобы основать такое.')),
    title(tr('Предприятия')),
    table(
      [[tr('Предприятие'), 150], [tr('Цена'), 100, true], [tr('Работа'), 0], [tr('Рабочих'), 60, true]],
      state.catalog.map((s) => [
        el('span', { className: 'firm-good' }, firmIcon(s.kind, 16), el('span', { textContent: firmKindName(s.kind) })),
        votesAmount(planOf(s.kind).price), recipe(s), planOf(s.kind).staff,
      ]),
    ),
    title(tr('Товары')),
    table([[tr('Товар'), 0], [tr('Норма цены'), 100, true]], state.settings.goods.map((g) => [goodLink(g.good), money(g.base)])),
  ];
}

// --- the page ---

function show() {
  const { tab, firm, part, good } = where();
  const active = firm ? 'mine' : tab;
  for (const a of $('firm-tabs').querySelectorAll('a')) a.classList.toggle('active', a.dataset.tab === active);

  let nodes;
  if (firm) nodes = firmCard(firmOf(firm), part);
  else if (tab === 'exchange') nodes = good ? book(good) : exchange();
  else if (tab === 'contracts') nodes = contracts();
  else if (tab === 'guide') nodes = guide();
  else nodes = myFirms();
  replaceInPlace($('firm-view'), ...nodes.filter(Boolean));
  blink();
}

// refresh asks for the city again once its hour ends, unless something is
// being typed or a dialog is open; the clocks go on every second.
function refresh() {
  for (const node of document.querySelectorAll('.firm-clock')) tick(node);
  redraw();
  if (!state?.nextHour || Date.now() < Date.parse(state.nextHour) + 5000 || busy || Date.now() - asked < 30000) return;
  const typing = document.activeElement?.closest?.('#firm-view') && document.activeElement.matches('input, select');
  if (typing || document.querySelector('.modal') || isOpen() || document.hidden) return;

  busy = true;
  asked = Date.now();
  api.firms()
    .then((city) => {
      state = city;
      show();
    })
    .catch(() => {}) // the next tick tries again
    .finally(() => {
      busy = false;
    });
}

me = await currentUser().catch(() => null);
renderChrome(me);

$('business').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
  $('firm-tabs').hidden = true;
} else {
  showBalance(me.votes ?? 0);
  for (const a of $('firm-tabs').querySelectorAll('a[data-tab]:not([data-tab="stocks"])')) a.addEventListener('click', follow);
  window.addEventListener('popstate', () => {
    hideError($('error'));
    said('');
    show();
  });
  try {
    state = await answered;
    show();
    setInterval(refresh, 1000);
  } catch (e) {
    showError($('error'), e);
  }
}
