// "Бизнес" (/business), a tab of "Заработок": the economy of the
// enterprises as the browser games of the 2000s had it — pixel pictures of
// the enterprises and the goods, tiles, plaques of numbers and tables. The
// tabs: "Мои предприятия" (tiles of those the page holds shares of, how to
// begin, and founding a new one by its picture; ?kind= picks one), the
// card of an enterprise (?firm=<id>, its own tabs in &part=: production,
// management, the exchange, the report, the shareholders), "Биржа" (the
// prices of the goods; &good= — the orders of one), "Акции" (all
// enterprises and the shares for sale) and "Справочник" (the chains from
// the raw materials to the shop). Every form is a row of one grid — a
// label of the same width, the fields, the button — and says what it will
// do while it is filled. The server counts the hours; the page asks for the
// city again when an hour ends and only shows what the server said.

import { api, currentUser, session, started } from './api.js';
import {
  firmHour, firmKindName, firmPictures, formatDateTime, goodName, goodPictures, groupedVotes, hundredths, hundredthsOf, plural,
  sharePercent, votesAmount,
} from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.firms()) : null;

let state = null;
let me = null;
let balance = 0;
let busy = false;
// The kind chosen to found, kept while the page is redrawn.
let chosen = '';

function showBalance(votes) {
  balance = votes;
  $('votes').textContent = groupedVotes(votes);
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('balance').hidden = false;
}

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

// money is a sum in hundredths of a vote: "12,5 гол.".
const money = (n) => tr('{0} гол.', hundredths(n));
const decimal = (n) => String(Math.round(n * 100) / 100).replace('.', ',');
const whole = (input) => {
  const n = Number(input.value);
  return Number.isInteger(n) && n > 0 ? n : 0;
};

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

function go(href) {
  history.pushState(null, '', href);
  hideError($('error'));
  said('');
  show();
  window.scrollTo(0, 0);
}

// follow opens a link of the page without loading it again.
function follow(event) {
  if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  go(event.currentTarget.getAttribute('href'));
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
const unit = (text) => el('span', { className: 'firm-unit', textContent: text });
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
    if (action === 'found') history.pushState(null, '', firmPath({ id: done.firm }));
    if (action === 'liquidate') history.pushState(null, '', '/business');
    show();
    said(typeof text === 'function' ? text(done) : text);
    $('done').scrollIntoView({ block: 'nearest' });
    return true;
  } catch (e) {
    showError(error, e);
    error.scrollIntoView({ block: 'nearest' });
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

// clock says when the hour of the enterprises ends; the page keeps it
// going.
function clock() {
  const node = el('p', { className: 'hint firm-hint firm-clock' });
  tick(node);
  return node;
}

function tick(node) {
  if (!state?.nextHour) return;
  const left = Math.max(0, Math.ceil((Date.parse(state.nextHour) - Date.now()) / 60000));
  node.textContent = tr('Следующий рабочий час закончится в {0} — через {1} мин.', firmHour(state.nextHour), left);
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
  nodes.push(title(tr('Основать предприятие')), ...founding());
  return nodes;
}

function steps() {
  const step = (n, picture, heading, text) => el('div', { className: 'firm-step' },
    el('div', { className: 'firm-step-head' }, el('span', { className: 'firm-step-number', textContent: String(n) }), el('img', { className: 'firm-icon', src: picture, alt: '', width: 48, height: 48 })),
    el('b', { textContent: heading }),
    el('p', { textContent: text }));

  return el('div', { className: 'firm-steps' },
    step(1, '/static/firms/farm.svg', tr('Основайте предприятие'), tr('Проще всего начать с фермы, рудника, нефтевышки или лесхоза: им не нужно сырьё. Рабочие нанимаются сразу.')),
    step(2, '/static/goods/grain.svg', tr('Оно работает само'), tr('Каждый час рабочие делают продукцию, а склад продаёт её государству. Выручка копится в кассе предприятия.')),
    step(3, '/static/firms/dividends.svg', tr('Забирайте прибыль'), tr('Выплатите дивиденды — голоса из кассы придут на ваш счёт. Получать их могут страницы старше недели с заполненной анкетой.')),
  );
}

function tile(f) {
  const fact = (name, value) => el('tr', {}, el('td', { className: 'firm-tile-label', textContent: name }), value instanceof Node ? el('td', {}, value) : el('td', { textContent: value }));
  return el('a', { className: 'firm-tile', href: firmPath(f), onclick: follow },
    el('span', { className: 'firm-tile-icon' }, firmIcon(f.kind, 48)),
    el('span', { className: 'firm-tile-text' },
      el('b', { textContent: f.name }),
      el('span', { className: 'firm-tile-kind', textContent: `${firmKindName(f.kind)} · ${tr('{0} ур.', f.level)}${directs(f) ? ` · ${tr('директор')}` : ''}` }),
      el('table', { className: 'firm-tile-facts' },
        fact(tr('Касса'), money(f.cash)),
        fact(tr('За сутки'), signed(f.profit)),
        fact(tr('Акции'), `${held(f)} (${sharePercent(held(f), f.shares)})`))));
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

function firmCard(f, part) {
  if (!f) return [hint(tr('Такого предприятия нет: его ликвидировали.')), link(tr('« Мои предприятия'), '/business')];

  const plan = planOf(f.kind);
  const parts = [
    ['work', tr('Производство')],
    directs(f) ? ['manage', tr('Управление')] : null,
    directs(f) ? ['trade', tr('Заявки')] : null,
    ['report', tr('Отчёт')],
    ['holders', tr('Акционеры')],
  ].filter(Boolean);
  if (!parts.some(([p]) => p === part)) part = 'work';

  const view = {
    work: () => production(f),
    manage: () => management(f, plan),
    trade: () => orders(f),
    report: () => [report(f)],
    holders: () => holders(f),
  }[part]();

  return [
    el('div', { className: 'firm-back' }, link(tr('« Мои предприятия'), '/business')),
    header(f, plan),
    stats(f),
    clock(),
    el('div', { className: 'tabs firm-subtabs' }, ...parts.map(([p, text]) => link(text, firmPath(f, p), p === part ? 'active' : ''))),
    ...view,
  ];
}

function header(f, plan) {
  return el('div', { className: 'firm-header' },
    el('span', { className: 'firm-header-icon' }, firmIcon(f.kind, 64)),
    el('div', { className: 'firm-header-text' },
      el('div', { className: 'firm-header-name', textContent: f.name }),
      el('div', { className: 'firm-header-kind', textContent: `${firmKindName(f.kind)} · ${tr('основано {0}', formatDateTime(f.foundedAt))}` }),
      el('div', { className: 'firm-header-level' }, el('span', { textContent: tr('Уровень {0} из {1}', f.level, plan.levels) }), bar(f.level, plan.levels)),
      el('div', {}, el('span', { className: 'hint', textContent: tr('Директор: ') }), person(f.director)),
    ));
}

function stats(f) {
  const plaque = (name, value, sub) => el('div', { className: 'firm-stat' },
    el('div', { className: 'firm-stat-name', textContent: name }),
    el('div', { className: 'firm-stat-value' }, value),
    el('div', { className: 'firm-stat-sub', textContent: sub }));

  return el('div', { className: 'firm-stats' },
    plaque(tr('Касса'), money(f.cash), f.escrow > 0 ? tr('и {0} в заявках', money(f.escrow)) : tr('расходы {0} в час', money(f.upkeep + f.workers * f.wage))),
    plaque(tr('За сутки'), signed(f.profit), tr('прибыль за {0} ч.', f.hours.length)),
    plaque(tr('Рабочие'), `${f.workers} / ${f.seats}`, tr('выработка {0} %', Math.round(f.productivity * 100))),
    plaque(tr('Акция'), money(Math.floor(f.worth / f.shares)), f.sharePrice > 0 ? tr('сделка — {0}', votesAmount(f.sharePrice)) : tr('сделок ещё не было')),
  );
}

function production(f) {
  const spec = specOf(f.kind);
  const workers = f.workers * f.productivity;
  const now = spec.sells.length
    ? spec.sells.map((g) => `${goodName(g).toLowerCase()} — ${decimal(workers * spec.rate[g])}`).join(', ')
    : tr('{0} шт.', decimal(workers * spec.rate[spec.output]));

  const auto = (s) => {
    const parts = [];
    if (buys(spec, s.good)) parts.push(s.autoBuy ? tr('закупает до {0}', hundredths(s.buyMax)) : tr('не закупает'));
    if (sells(spec, s.good)) parts.push(s.autoSell ? tr('продаёт от {0}', hundredths(s.sellMin)) : tr('не продаёт'));
    return parts.join(', ');
  };
  const columns = [[tr('Товар'), 0], [tr('На складе'), 90, true], [tr('Заполнено'), 80], [tr('Сам'), 170]];
  if (directs(f)) columns.push(['', 80, true]);

  return [
    grid([
      line(tr('Работа:'), recipe(spec)),
      line(tr('Выработка:'), el('span', { textContent: rate(spec) })),
      line(tr('Сейчас в час:'), el('span', { textContent: now })),
    ]),
    title(tr('Склад')),
    table(columns, f.stock.map((s) => {
      const row = [
        goodLink(s.good),
        s.incoming > 0 ? tr('{0} (+{1})', groupedVotes(s.units), groupedVotes(s.incoming)) : groupedVotes(s.units),
        bar(s.units, s.capacity),
        auto(s),
      ];
      if (directs(f)) row.push(el('a', { href: '#', textContent: tr('Настроить'), onclick: (e) => { e.preventDefault(); stockBox(f, s); } }));
      return row;
    })),
    hint(directs(f)
      ? tr('Склад сам закупает сырьё у государства и продаёт продукцию государству по ценам не хуже тех, что вы задали. Выгоднее торговать с игроками — во вкладке «Заявки».')
      : tr('Склад сам закупает сырьё и продаёт продукцию государству.')),
  ];
}

function stockBox(f, s) {
  const spec = specOf(f.kind);
  const q = quoteOf(s.good);
  const units = field({ type: 'number', min: 1, required: true, placeholder: tr('шт.') });
  const autoBuy = el('input', { type: 'checkbox', checked: s.autoBuy });
  const buyMax = field({ type: 'text', inputMode: 'decimal', value: hundredths(s.buyMax).replace(/\u00a0/g, '') });
  const autoSell = el('input', { type: 'checkbox', checked: s.autoSell });
  const sellMin = field({ type: 'text', inputMode: 'decimal', value: hundredths(s.sellMin).replace(/\u00a0/g, '') });
  const body = { firm: f.id, good: s.good };

  const { modal, error } = dialog(goodName(s.good),
    grid([
      line(tr('На складе:'), el('span', { textContent: tr('{0} из {1}', groupedVotes(s.units), groupedVotes(s.capacity)) }), bar(s.units, s.capacity)),
      line(tr('Государство:'), el('span', { textContent: tr('покупает по {0}, продаёт по {1}', money(q.stateBid), money(q.stateAsk)) })),
      retailed(s.good) ? line(tr('Город:'), el('span', { textContent: tr('платит магазинам по {0}', money(q.cityPrice)) })) : [],
      line(tr('Сделка:'), units,
        buys(spec, s.good) ? button(tr('Купить'), () => trade('state_buy')) : null,
        button(tr('Продать'), () => trade('state_sell'))),
    ]),
    hint(tr('Цены государства — за первую штуку: чем больше с ним торгуют за сутки, тем хуже следующие.')),
    el('div', { className: 'biz-subtitle', textContent: tr('Склад сам') }),
    grid([
      buys(spec, s.good) ? line(tr('Закупать:'), el('label', { className: 'firm-check' }, autoBuy, tr(' у государства до')), buyMax, unit(tr('гол.'))) : [],
      sells(spec, s.good)
        ? line(tr('Продавать:'), el('label', { className: 'firm-check' }, autoSell, spec.sells.includes(s.good) ? tr(' городу от') : tr(' государству от')), sellMin, unit(tr('гол.')))
        : [],
    ]),
  );

  async function trade(action) {
    const n = whole(units);
    const ok = await act(action, { ...body, units: n }, () => (action === 'state_buy'
      ? tr('Куплено у государства: {0} шт.', groupedVotes(n))
      : tr('Продано государству: {0} шт.', groupedVotes(n))), error);
    if (ok) modal.close();
  }

  modal.actions.replaceChildren(
    button(tr('Сохранить'), async () => {
      const auto = {
        ...body, autoBuy: autoBuy.checked, buyMax: hundredthsOf(buyMax.value) || 0,
        autoSell: autoSell.checked, sellMin: hundredthsOf(sellMin.value) || 0,
      };
      if (await act('auto', auto, tr('Склад настроен.'), error)) modal.close();
    }),
    button(tr('Закрыть'), modal.close, { secondary: true }),
  );
}

function management(f, plan) {
  const body = { firm: f.id };
  const shares = held(f);

  const name = field({ type: 'text', maxLength: 40, required: true, value: f.name }, 'firm-name');
  const workers = field({ type: 'number', min: 0, max: f.seats, required: true, value: f.workers });
  const wage = field({ type: 'text', inputMode: 'decimal', required: true, value: hundredths(f.wage).replace(/\u00a0/g, '') });
  const staffNote = note();
  const restaff = () => {
    const w = hundredthsOf(wage.value);
    const n = Number(workers.value);
    staffNote.textContent = Number.isNaN(w) || !Number.isInteger(n)
      ? ''
      : tr('выработка {0} %, зарплаты {1} в час', Math.round(productivity(w) * 100), money(n * w));
  };
  workers.addEventListener('input', restaff);
  wage.addEventListener('input', restaff);
  restaff();

  const deposit = field({ type: 'number', min: 1, required: true });
  const depositNote = note();
  depositNote.textContent = tr('на вашем счету {0}', votesAmount(balance));

  const dividend = field({ type: 'number', min: 1, max: Math.floor(f.cash / 100), required: true });
  const dividendNote = note();
  const redivide = () => {
    const v = whole(dividend);
    dividendNote.textContent = v
      ? tr('вам — {0} из {1}', votesAmount(Math.floor((v * shares) / f.shares)), votesAmount(v))
      : tr('в кассе {0}', votesAmount(Math.floor(f.cash / 100)));
  };
  dividend.addEventListener('input', redivide);
  redivide();

  const top = f.level >= plan.levels;
  const upgrade = button(top ? tr('Высший уровень') : tr('Улучшить за {0}', money(f.upgradeCost * 100)),
    () => act('upgrade', body, tr('Предприятие улучшено до {0} ур.', f.level + 1)));
  upgrade.disabled = top;
  const upgradeNote = note();
  if (!top) {
    upgradeNote.textContent = f.cash < f.upgradeCost * 100
      ? tr('в кассе не хватает {0}', money(f.upgradeCost * 100 - f.cash))
      : tr('будет {0} мест и склад больше', plan.staff * (f.level + 1));
  }

  return [
    grid([line(tr('Название:'), name, submit(tr('Сохранить')))], () => act('rename', { ...body, name: name.value }, tr('Предприятие переименовано.'))),
    grid([
      line(tr('Рабочих:'), workers, unit(tr('из {0}', f.seats))),
      line(tr('Зарплата:'), wage, unit(tr('гол. в час'))),
      line('', submit(tr('Сохранить')), staffNote),
    ], () => act('staff', { ...body, workers: Number(workers.value), wage: hundredthsOf(wage.value) }, tr('Рабочие и зарплата изменены.'))),
    hint(tr('Норма — {0} в час, платить можно от половины до тройной нормы: чем выше зарплата, тем больше делают рабочие. Зарплаты и содержание идут из кассы каждый час, а без денег предприятие стоит.', money(state.settings.wage))),
    grid([line(tr('В кассу:'), deposit, unit(tr('гол.')), submit(tr('Вложить')), depositNote)],
      () => act('deposit', { ...body, votes: whole(deposit) }, (d) => tr('Вы вложили в кассу {0}.', votesAmount(d.spent)))),
    grid([line(tr('Дивиденды:'), dividend, unit(tr('гол.')), submit(tr('Выплатить')), dividendNote)],
      () => act('dividend', { ...body, votes: whole(dividend) }, (d) => tr('Дивиденды выплачены, ваша доля — {0}.', votesAmount(d.got)))),
    hint(tr('Дивиденды делятся из кассы между всеми акционерами по числу акций.')),
    grid([line(tr('Улучшение:'), upgrade, upgradeNote)]),
    grid([line(tr('Ликвидация:'), button(tr('Ликвидировать'), () => liquidate(f), { secondary: true }), el('span', { className: 'firm-note', textContent: tr('акционеры получат {0}', money(f.worth)) }))]),
  ];
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

function orders(f) {
  const own = state.orders.filter((o) => o.firmId === f.id);
  const nodes = [];
  nodes.push(title(tr('Заявки предприятия')));
  nodes.push(own.length
    ? table([[tr('Товар'), 0], [tr('Заявка'), 80], [tr('Цена'), 80, true], [tr('Осталось'), 80, true], ['', 70, true]], own.map((o) => [
      goodLink(o.good), o.buy ? tr('покупка') : tr('продажа'), hundredths(o.price), groupedVotes(o.units),
      el('a', { href: '#', textContent: tr('Снять'), onclick: (e) => { e.preventDefault(); act('cancel', { order: o.id }, tr('Заявка снята.')); } }),
    ]))
    : hint(tr('Заявок нет.')));

  const side = el('select', { className: 'firm-select' }, el('option', { value: 'sell', textContent: tr('продать') }), el('option', { value: 'buy', textContent: tr('купить') }));
  const good = el('select', { className: 'firm-select' }, ...f.stock.map((s) => el('option', { value: s.good, textContent: goodName(s.good) })));
  const units = field({ type: 'number', min: 1, required: true });
  const price = field({ type: 'text', inputMode: 'decimal', required: true });
  const prices = el('span', { className: 'firm-note' });
  const total = note();

  const pick = (name, value) => (value > 0
    ? el('a', { href: '#', textContent: `${name} ${hundredths(value)}`, onclick: (e) => { e.preventDefault(); price.value = hundredths(value).replace(/\u00a0/g, ''); recount(); } })
    : null);
  function recount() {
    const q = quoteOf(good.value);
    const links = [pick(tr('спрос'), q.bestBid), pick(tr('предложение'), q.bestAsk), pick(tr('гос. покупает'), q.stateBid), pick(tr('гос. продаёт'), q.stateAsk)].filter(Boolean);
    prices.replaceChildren(...links.flatMap((a, i) => (i ? [' · ', a] : [a])));
    const p = hundredthsOf(price.value);
    const n = whole(units);
    total.textContent = n && p > 0 ? tr('на сумму {0}', money(n * p)) : '';
  }
  for (const input of [side, good, units, price]) input.addEventListener('input', recount);
  recount();

  nodes.push(title(tr('Новая заявка')), grid([
    line(tr('Действие:'), side, good),
    line(tr('Количество:'), units, unit(tr('шт.'))),
    line(tr('Цена:'), price, unit(tr('гол. за шт.'))),
    line(tr('Цены сейчас:'), prices),
    line('', submit(tr('Выставить')), total),
  ], () => act('order', {
    firm: f.id, good: good.value, buy: side.value === 'buy', price: hundredthsOf(price.value), units: whole(units),
  }, (d) => {
    const dealt = d.deals.reduce((n, deal) => n + deal.units, 0);
    return dealt > 0 ? tr('Заявка выставлена, сразу исполнено {0} шт.', groupedVotes(dealt)) : tr('Заявка выставлена.');
  })));
  nodes.push(hint(tr('Заявка на покупку держит деньги кассы, на продажу — товар склада, пока её не исполнят или не снимут. Сделка идёт по цене той заявки, что выставлена раньше, а продавец платит комиссию {0} %. Нажмите на цену, чтобы подставить её.', state.settings.fee)));
  return nodes;
}

function report(f) {
  if (!f.hours.length) return hint(tr('Предприятие ещё не отработало ни часа.'));

  const hours = [...f.hours].reverse();
  const costs = (h) => h.purchases + h.wages + h.upkeep + h.fees;
  const sum = (pick) => hours.reduce((n, h) => n + pick(h), 0);
  const profit = (h) => h.income - costs(h);
  return table(
    [[tr('Час'), 0], [tr('Сделано'), 66, true], [tr('Куплено'), 66, true], [tr('Продано'), 66, true], [tr('Выручка'), 76, true], [tr('Расходы'), 76, true], [tr('Прибыль'), 90, true]],
    hours.map((h) => [firmHour(h.hour), h.made, h.bought, h.sold, hundredths(h.income), hundredths(costs(h)), signed(profit(h))]),
    [tr('Итого'), sum((h) => h.made), sum((h) => h.bought), sum((h) => h.sold), hundredths(sum((h) => h.income)), hundredths(sum(costs)), signed(sum(profit))],
  );
}

function holders(f) {
  const shares = held(f);
  const nodes = [table([[tr('Акционер'), 0], [tr('Акций'), 70, true], [tr('Доля'), 150]], f.holders.map((h) => [
    person(h.user), groupedVotes(h.shares), el('span', { className: 'firm-share' }, bar(h.shares, f.shares), sharePercent(h.shares, f.shares)),
  ]))];
  const offers = state.offers.filter((o) => o.firmId === f.id);
  nodes.push(title(tr('Продаются')), offers.length ? offersTable(offers, false) : hint(tr('Сейчас никто не продаёт акции этого предприятия.')));

  if (shares > 0) {
    const count = field({ type: 'number', min: 1, max: shares, required: true });
    const price = field({ type: 'number', min: 1, required: true });
    const got = note();
    const recount = () => {
      const n = whole(count);
      const p = whole(price);
      got.textContent = n && p ? tr('получите {0} за вычетом комиссии', votesAmount(n * p - Math.floor((n * p * state.settings.fee) / 100))) : '';
    };
    count.addEventListener('input', recount);
    price.addEventListener('input', recount);
    nodes.push(title(tr('Продать свои акции')), grid([
      line(tr('Акций:'), count, unit(tr('из {0}', shares))),
      line(tr('Цена:'), price, unit(tr('гол. за акцию'))),
      line('', submit(tr('Выставить на продажу')), got),
    ], () => act('offer', { firm: f.id, shares: whole(count), votes: whole(price) }, tr('Акции выставлены на продажу.'))),
    hint(tr('Директор — тот, у кого больше всех акций: продав много, можно потерять управление.')));
  }

  if (directs(f)) {
    const count = field({ type: 'number', min: 1, required: true });
    const price = field({ type: 'number', min: 1, required: true });
    const after = note();
    const recount = () => {
      const n = whole(count);
      const p = whole(price);
      after.textContent = n && p
        ? tr('в кассу — {0}, ваша доля станет {1}', votesAmount(n * p), sharePercent(shares, f.shares + n))
        : '';
    };
    count.addEventListener('input', recount);
    price.addEventListener('input', recount);
    nodes.push(title(tr('Выпустить новые акции')), grid([
      line(tr('Акций:'), count),
      line(tr('Цена:'), price, unit(tr('гол. за акцию'))),
      line('', submit(tr('Выпустить')), after),
    ], () => act('issue', { firm: f.id, shares: whole(count), votes: whole(price) }, tr('Новые акции выставлены на продажу.'))));
  }
  return nodes;
}

// --- "Акции" ---

function offersTable(offers, named) {
  const columns = [[tr('Продавец'), 0], [tr('Акций'), 70, true], [tr('Цена акции'), 100, true], ['', 70, true]];
  if (named) columns.unshift([tr('Предприятие'), 0]);
  return table(columns, offers.map((o) => {
    const f = firmOf(o.firmId);
    const own = o.seller?.id === me.id || (!o.seller && f && directs(f));
    return [
      ...(named ? [f ? firmLink(f) : ''] : []),
      o.seller ? person(o.seller) : tr('выпуск предприятия'), groupedVotes(o.shares), votesAmount(o.price),
      own
        ? el('a', { href: '#', textContent: tr('Снять'), onclick: (e) => { e.preventDefault(); act('cancel_offer', { offer: o.id }, tr('Предложение снято.')); } })
        : el('a', { href: '#', textContent: tr('Купить'), onclick: (e) => { e.preventDefault(); buyShares(o, f); } }),
    ];
  }));
}

function buyShares(o, f) {
  const count = field({ type: 'number', min: 1, max: o.shares, required: true, value: o.shares });
  const total = note();
  const recount = () => {
    total.textContent = tr('итого {0}', votesAmount(whole(count) * o.price));
  };
  count.addEventListener('input', recount);
  recount();

  const { modal, error } = dialog(tr('Покупка акций'),
    grid([
      line(tr('Предприятие:'), f ? firmIcon(f.kind, 16) : null, el('b', { textContent: f?.name ?? '' })),
      line(tr('Цена:'), el('span', { textContent: tr('{0} за акцию, продаётся {1}', votesAmount(o.price), groupedVotes(o.shares)) })),
      line(tr('Акций:'), count, total),
    ]));
  modal.actions.replaceChildren(
    button(tr('Купить'), async () => {
      const n = whole(count);
      if (await act('buy_shares', { offer: o.id, shares: n }, (d) => tr('Вы купили {0} акц. за {1}.', groupedVotes(n), votesAmount(d.spent)), error)) modal.close();
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

function shares() {
  const nodes = [title(tr('Все предприятия'))];
  if (!state.firms.length) {
    nodes.push(hint(tr('Ещё никто не основал предприятие.')));
  } else {
    const firms = [...state.firms].sort((a, b) => b.worth - a.worth);
    nodes.push(table(
      [[tr('Предприятие'), 0], [tr('Ур.'), 40, true], [tr('Директор'), 130], [tr('За сутки'), 90, true], [tr('Акция'), 80, true]],
      firms.map((f) => [firmLink(f), f.level, person(f.director), signed(f.profit), hundredths(Math.floor(f.worth / f.shares))]),
    ));
  }

  nodes.push(title(tr('Акции на продажу')));
  nodes.push(state.offers.length ? offersTable(state.offers, true) : hint(tr('Сейчас никто не продаёт акции.')));
  nodes.push(hint(tr('«Акция» — сколько голосов придётся на акцию, если предприятие ликвидировать: половина вложенного в основание и улучшения, касса и деньги заявок. Акционеры получают дивиденды, когда директор их выплачивает.')));
  return nodes;
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
  else if (tab === 'shares') nodes = shares();
  else if (tab === 'guide') nodes = guide();
  else nodes = myFirms();
  $('firm-view').replaceChildren(...nodes.filter(Boolean));
}

// refresh asks for the city again once its hour ends, unless something is
// being typed or a dialog is open; the clocks go on every half minute.
function refresh() {
  for (const node of document.querySelectorAll('.firm-clock')) tick(node);
  if (!state?.nextHour || Date.now() < Date.parse(state.nextHour) + 5000 || busy) return;
  const typing = document.activeElement?.closest?.('#firm-view') && document.activeElement.matches('input, select');
  if (typing || document.querySelector('.modal') || document.hidden) return;

  busy = true;
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
  for (const a of $('firm-tabs').querySelectorAll('a')) a.addEventListener('click', follow);
  window.addEventListener('popstate', () => {
    hideError($('error'));
    said('');
    show();
  });
  try {
    state = await answered;
    show();
    setInterval(refresh, 30000);
  } catch (e) {
    showError($('error'), e);
  }
}
