// "Бизнес" (/business), a tab of "Заработок": the economy of the
// enterprises in tables, as the browser games of the 2000s had it. People
// found enterprises along the chains — the raw materials, the materials,
// the goods, the shop — and run them: the workers and their wage, the
// storage that buys and sells by itself, the orders of the exchange, the
// shares. The tabs: "Мои предприятия" (those the page holds shares of and
// founding a new one; ?firm=<id> is the card of one), "Биржа" (the prices
// of the goods; &good=<good> — its orders), "Акции" (all enterprises and
// the shares for sale) and "Справочник" (what each enterprise does). The
// server counts the hours; the page only shows what it said. The
// administrators tune the economy in the admin panel (/admin/business).

import { api, currentUser, session, started } from './api.js';
import {
  firmHour, firmKindName, formatDateTime, goodName, groupedVotes, hundredths, hundredthsOf, plural, sharePercent, votesAmount,
} from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.firms()) : null;

let state = null;
let me = null;
let busy = false;

function showBalance(votes) {
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

// The address of the page: the tab, the enterprise, the good.
function where() {
  const q = new URLSearchParams(location.search);
  return { tab: q.get('tab') ?? 'mine', firm: q.get('firm') ?? '', good: q.get('good') ?? '' };
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

const link = (text, href) => el('a', { href, textContent: text, onclick: follow });
const firmLink = (f) => link(f.name, `/business?firm=${encodeURIComponent(f.id)}`);
const goodLink = (good) => link(goodName(good), `/business?tab=exchange&good=${encodeURIComponent(good)}`);
const person = (p) => (p ? el('a', { href: pagePath(p), textContent: fullName(p) }) : tr('—'));
const hint = (text) => el('p', { className: 'hint firm-hint', textContent: text });
const title = (text) => el('div', { className: 'section-title', textContent: text });

// table is a table of the page: the first column at the left, the numbers
// at the right; a wide one scrolls inside itself on a narrow screen.
function table(head, rows, footer = null) {
  const cell = (tag, c) => (c instanceof Node ? el(tag, {}, c) : el(tag, { textContent: String(c ?? '') }));
  return el('div', { className: 'firm-scroll' },
    el('table', { className: 'list-table firm-table' },
      el('thead', {}, el('tr', {}, ...head.map((h) => el('th', { textContent: h })))),
      el('tbody', {}, ...rows.map((cells) => el('tr', {}, ...cells.map((c) => cell('td', c))))),
      footer ? el('tfoot', {}, el('tr', {}, ...footer.map((c) => cell('td', c)))) : null,
    ));
}

// facts is a table of names and values, as a card of an enterprise has.
function facts(rows) {
  return el('table', { className: 'firm-facts' },
    ...rows.filter(Boolean).map(([name, value]) => el('tr', {},
      el('td', { className: 'biz-label', textContent: name }),
      value instanceof Node ? el('td', {}, value) : el('td', { textContent: String(value) }))));
}

function field(props) {
  return el('input', { className: 'text firm-number', ...props });
}

// form is a line of fields with a button; submit gets the form and
// whether the action went through.
function form(children, label, submit) {
  const node = el('form', { className: 'firm-form' }, ...children, button(label, null, { type: 'submit' }));
  node.addEventListener('submit', (event) => {
    event.preventDefault();
    submit(node);
  });
  return node;
}

const labelled = (text, input) => el('label', { className: 'firm-field' }, el('span', { textContent: text }), input);

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
    if (action === 'found') history.pushState(null, '', `/business?firm=${encodeURIComponent(done.firm)}`);
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
  const error = el('div', { className: 'error', hidden: true });
  modal.body.replaceChildren(error, ...body.filter(Boolean));
  return { modal, error };
}

function votesOf(input) {
  const n = Number(input.value);
  return Number.isInteger(n) ? n : NaN;
}

// recipe is what an enterprise does in words of the tables: "2 × зерно →
// мука, 5 шт." or, for a shop, what it sells the city.
function recipe(spec) {
  if (spec.sells.length) {
    return tr('продаёт городу: {0}', spec.sells.map((g) => `${goodName(g).toLowerCase()} — ${decimal(spec.rate[g])}`).join(', '));
  }
  const made = tr('{0}, {1} шт.', goodName(spec.output).toLowerCase(), decimal(spec.rate[spec.output]));
  if (!spec.inputs.length) return made;

  return `${spec.inputs.map((i) => `${i.units} × ${goodName(i.good).toLowerCase()}`).join(' + ')} → ${made}`;
}

function nextHour() {
  return state.nextHour ? hint(tr('Следующий рабочий час закончится в {0}.', firmHour(state.nextHour))) : null;
}

// --- "Мои предприятия" ---

function myFirms() {
  const mine = state.firms.filter((f) => held(f) > 0);
  const nodes = [title(tr('Мои предприятия'))];

  if (!mine.length) {
    nodes.push(hint(tr('У вас пока нет ни предприятий, ни акций. Основайте своё или купите акции чужих во вкладке «Акции».')));
  } else {
    nodes.push(table(
      [tr('Предприятие'), tr('Вид'), tr('Ур.'), tr('Касса'), tr('Прибыль за сутки'), tr('Мои акции')],
      mine.map((f) => [
        firmLink(f), firmKindName(f.kind), f.level, money(f.cash), money(f.profit),
        `${held(f)} (${sharePercent(held(f), f.shares)})${directs(f) ? tr(', директор') : ''}`,
      ]),
    ), nextHour());
  }

  nodes.push(title(tr('Основать предприятие')), ...founding());
  return nodes;
}

function founding() {
  const kind = el('select', { className: 'firm-select' },
    ...state.settings.plans.map((p) => el('option', { value: p.kind, textContent: `${firmKindName(p.kind)} — ${votesAmount(p.price)}` })));
  const name = field({ type: 'text', maxLength: 40, required: true, placeholder: tr('Название') });
  name.classList.replace('firm-number', 'firm-name');
  const about = hint('');
  const describe = () => {
    const spec = specOf(kind.value);
    const plan = planOf(kind.value);
    about.textContent = tr('{0}: {1} в час на рабочего, {2} рабочих на уровень, до {3} уровней.', firmKindName(spec.kind), recipe(spec), plan.staff, plan.levels);
  };
  kind.addEventListener('change', describe);
  describe();

  return [
    form([kind, name], tr('Основать'), () => act('found', { kind: kind.value, name: name.value }, tr('Предприятие «{0}» основано. Рабочие уже наняты, склад закупает сырьё сам.', name.value.trim()))),
    about,
    hint(tr('{0} % цены уходит в кассу предприятия на первые зарплаты и закупки. Вы получаете все 100 акций и становитесь директором.', state.settings.capital)),
  ];
}

// --- the card of an enterprise ---

function firmCard(f) {
  if (!f) return [hint(tr('Такого предприятия нет: его ликвидировали.')), link(tr('« Мои предприятия'), '/business')];

  const plan = planOf(f.kind);
  const shares = held(f);
  const nodes = [
    el('div', { className: 'firm-head' }, el('b', { textContent: f.name }), el('span', { className: 'biz-level', textContent: ` — ${firmKindName(f.kind)}` })),
    facts([
      [tr('Директор:'), person(f.director)],
      [tr('Основано:'), formatDateTime(f.foundedAt)],
      [tr('Уровень:'), tr('{0} из {1}', f.level, plan.levels)],
      [tr('Работа:'), recipe(specOf(f.kind)) + tr(' в час на рабочего')],
      [tr('Рабочие:'), tr('{0} из {1} мест, зарплата {2} в час, выработка {3} %', f.workers, f.seats, money(f.wage), Math.round(f.productivity * 100))],
      [tr('Касса:'), money(f.cash)],
      f.escrow > 0 ? [tr('В заявках:'), money(f.escrow)] : null,
      [tr('Содержание:'), tr('{0} в час', money(f.upkeep))],
      [tr('Прибыль за сутки:'), money(f.profit)],
      [tr('Стоимость:'), tr('{0}, на акцию — {1}', money(f.worth), money(Math.floor(f.worth / f.shares)))],
      [tr('Цена акции:'), f.sharePrice > 0 ? votesAmount(f.sharePrice) : tr('сделок ещё не было')],
      shares > 0 ? [tr('Ваши акции:'), tr('{0} из {1} ({2})', shares, f.shares, sharePercent(shares, f.shares))] : null,
    ]),
    nextHour(),
  ];

  if (directs(f)) nodes.push(title(tr('Управление')), ...management(f, plan));
  nodes.push(title(tr('Склад')), storage(f));
  if (directs(f)) nodes.push(title(tr('Заявки на бирже')), ...orders(f));
  nodes.push(title(tr('Отчёт за сутки')), report(f));
  nodes.push(title(tr('Акционеры')), ...holders(f, shares));
  return nodes;
}

function management(f, plan) {
  const body = { firm: f.id };
  const name = field({ type: 'text', maxLength: 40, required: true, value: f.name });
  name.classList.replace('firm-number', 'firm-name');
  const workers = field({ type: 'number', min: 0, max: f.seats, required: true, value: f.workers });
  const wage = field({ type: 'text', inputMode: 'decimal', required: true, value: hundredths(f.wage).replace(/ /g, '') });
  const deposit = field({ type: 'number', min: 1, required: true, placeholder: tr('голосов') });
  const dividend = field({ type: 'number', min: 1, required: true, placeholder: tr('голосов') });

  const nodes = [
    form([name], tr('Переименовать'), () => act('rename', { ...body, name: name.value }, tr('Предприятие переименовано.'))),
    form([labelled(tr('Рабочих'), workers), labelled(tr('Зарплата, гол./час'), wage)], tr('Сохранить'),
      () => act('staff', { ...body, workers: Number(workers.value), wage: hundredthsOf(wage.value) }, tr('Рабочие и зарплата изменены.'))),
    hint(tr('Норма зарплаты — {0} в час, платить можно от половины до тройной нормы: чем выше зарплата, тем больше делают рабочие. Зарплата и содержание идут из кассы каждый час, а когда денег нет, предприятие стоит.', money(state.settings.wage))),
    form([labelled(tr('В кассу'), deposit)], tr('Вложить'),
      () => act('deposit', { ...body, votes: votesOf(deposit) }, (d) => tr('Вы вложили в кассу {0}.', votesAmount(d.spent)))),
    form([labelled(tr('Акционерам'), dividend)], tr('Выплатить дивиденды'),
      () => act('dividend', { ...body, votes: votesOf(dividend) }, (d) => tr('Дивиденды выплачены, ваша доля — {0}.', votesAmount(d.got)))),
    hint(tr('Дивиденды делятся из кассы между всеми акционерами по числу акций.')),
  ];

  if (f.level < plan.levels) {
    nodes.push(el('p', { className: 'firm-buttons' },
      button(tr('Улучшить до {0} ур. за {1}', f.level + 1, money(f.upgradeCost * 100)),
        () => act('upgrade', body, tr('Предприятие улучшено до {0} ур.: больше мест для рабочих и больше склад.', f.level + 1)))));
  } else {
    nodes.push(hint(tr('Предприятие на высшем уровне.')));
  }

  nodes.push(el('p', { className: 'firm-buttons' }, button(tr('Ликвидировать'), () => liquidate(f), { secondary: true })));
  return nodes;
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

// storage is what the enterprise keeps; its director sets what the
// storage buys and sells by itself and trades with the state.
function storage(f) {
  const spec = specOf(f.kind);
  const auto = (s) => {
    const parts = [];
    if (buys(spec, s.good)) parts.push(s.autoBuy ? tr('закупать до {0}', money(s.buyMax)) : tr('не закупать'));
    if (sells(spec, s.good)) parts.push(s.autoSell ? tr('продавать от {0}', money(s.sellMin)) : tr('не продавать'));
    return parts.join(', ');
  };

  if (!directs(f)) {
    return table([tr('Товар'), tr('На складе'), tr('Вместимость')], f.stock.map((s) => [goodLink(s.good), groupedVotes(s.units), groupedVotes(s.capacity)]));
  }

  return table(
    [tr('Товар'), tr('На складе'), tr('Вместимость'), tr('Сам'), ''],
    f.stock.map((s) => [
      goodLink(s.good), s.incoming > 0 ? tr('{0} (+{1})', groupedVotes(s.units), groupedVotes(s.incoming)) : groupedVotes(s.units),
      groupedVotes(s.capacity), auto(s), el('a', { href: '#', textContent: tr('Настроить'), onclick: (e) => { e.preventDefault(); stockBox(f, s); } }),
    ]),
  );
}

function stockBox(f, s) {
  const spec = specOf(f.kind);
  const q = quoteOf(s.good);
  const units = field({ type: 'number', min: 1, required: true, placeholder: tr('шт.') });
  const autoBuy = el('input', { type: 'checkbox', checked: s.autoBuy });
  const buyMax = field({ type: 'text', inputMode: 'decimal', value: hundredths(s.buyMax).replace(/ /g, '') });
  const autoSell = el('input', { type: 'checkbox', checked: s.autoSell });
  const sellMin = field({ type: 'text', inputMode: 'decimal', value: hundredths(s.sellMin).replace(/ /g, '') });
  const body = { firm: f.id, good: s.good };

  const { modal, error } = dialog(goodName(s.good),
    facts([
      [tr('На складе:'), tr('{0} из {1}', groupedVotes(s.units), groupedVotes(s.capacity))],
      s.incoming > 0 ? [tr('Придёт по заявкам:'), groupedVotes(s.incoming)] : null,
      [tr('Государство покупает:'), tr('по {0}', money(q.stateBid))],
      [tr('Государство продаёт:'), tr('по {0}', money(q.stateAsk))],
      retailed(s.good) ? [tr('Город платит магазинам:'), tr('по {0}', money(q.cityPrice))] : null,
    ]),
    hint(tr('Цены государства — за первую штуку: чем больше с ним торгуют за сутки, тем хуже следующие.')),
    el('div', { className: 'firm-form' }, labelled(tr('Количество'), units),
      buys(spec, s.good) ? button(tr('Купить у государства'), () => trade('state_buy')) : null,
      button(tr('Продать государству'), () => trade('state_sell'))),
    el('div', { className: 'biz-subtitle', textContent: tr('Склад сам') }),
    buys(spec, s.good) ? el('div', { className: 'firm-form' }, el('label', {}, autoBuy, tr(' закупать у государства до цены')), buyMax) : null,
    sells(spec, s.good)
      ? el('div', { className: 'firm-form' }, el('label', {}, autoSell, spec.sells.includes(s.good) ? tr(' продавать городу от цены') : tr(' продавать государству от цены')), sellMin)
      : null,
  );

  async function trade(action) {
    const n = Number(units.value);
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

function orders(f) {
  const own = state.orders.filter((o) => o.firmId === f.id);
  const nodes = [];
  if (own.length) {
    nodes.push(table([tr('Товар'), tr('Заявка'), tr('Цена'), tr('Осталось'), ''], own.map((o) => [
      goodLink(o.good), o.buy ? tr('покупка') : tr('продажа'), money(o.price), groupedVotes(o.units),
      el('a', { href: '#', textContent: tr('Снять'), onclick: (e) => { e.preventDefault(); act('cancel', { order: o.id }, tr('Заявка снята.')); } }),
    ])));
  }

  const good = el('select', { className: 'firm-select' }, ...f.stock.map((s) => el('option', { value: s.good, textContent: goodName(s.good) })));
  const side = el('select', { className: 'firm-select' }, el('option', { value: 'sell', textContent: tr('продать') }), el('option', { value: 'buy', textContent: tr('купить') }));
  const price = field({ type: 'text', inputMode: 'decimal', required: true, placeholder: tr('цена, гол.') });
  const units = field({ type: 'number', min: 1, required: true, placeholder: tr('шт.') });
  nodes.push(
    form([side, good, units, price], tr('Выставить'), () => act('order', {
      firm: f.id, good: good.value, buy: side.value === 'buy', price: hundredthsOf(price.value), units: Number(units.value),
    }, (d) => {
      const dealt = d.deals.reduce((n, deal) => n + deal.units, 0);
      return dealt > 0 ? tr('Заявка выставлена, сразу исполнено {0} шт.', groupedVotes(dealt)) : tr('Заявка выставлена.');
    })),
    hint(tr('Цена — за штуку. Заявка на покупку держит деньги кассы, на продажу — товар склада, пока её не исполнят или не снимут. Сделка идёт по цене той заявки, что выставлена раньше, а продавец платит комиссию {0} %.', state.settings.fee)),
  );
  return nodes;
}

function report(f) {
  if (!f.hours.length) return hint(tr('Предприятие ещё не отработало ни часа.'));

  const hours = [...f.hours].reverse();
  const costs = (h) => h.purchases + h.wages + h.upkeep + h.fees;
  const sum = (pick) => hours.reduce((n, h) => n + pick(h), 0);
  const profit = (h) => h.income - costs(h);
  return table(
    [tr('Час'), tr('Сделано'), tr('Куплено'), tr('Продано'), tr('Выручка'), tr('Расходы'), tr('Прибыль')],
    hours.map((h) => [firmHour(h.hour), h.made, h.bought, h.sold, hundredths(h.income), hundredths(costs(h)), hundredths(profit(h))]),
    [tr('Итого'), sum((h) => h.made), sum((h) => h.bought), sum((h) => h.sold), hundredths(sum((h) => h.income)), hundredths(sum(costs)), hundredths(sum(profit))],
  );
}

function holders(f, shares) {
  const nodes = [table([tr('Акционер'), tr('Акций'), tr('Доля')], f.holders.map((h) => [person(h.user), h.shares, sharePercent(h.shares, f.shares)]))];
  const offers = state.offers.filter((o) => o.firmId === f.id);
  if (offers.length) nodes.push(el('div', { className: 'biz-subtitle', textContent: tr('Продаются') }), offersTable(offers, false));

  const count = field({ type: 'number', min: 1, required: true, placeholder: tr('акций') });
  const price = field({ type: 'number', min: 1, required: true, placeholder: tr('гол. за акцию') });
  if (shares > 0) {
    nodes.push(form([count, price], tr('Продать свои акции'), () => act('offer', { firm: f.id, shares: Number(count.value), votes: votesOf(price) }, tr('Акции выставлены на продажу.'))),
      hint(tr('Голоса за проданные акции приходят вам, кроме комиссии {0} %. Директор — тот, у кого больше всех акций.', state.settings.fee)));
  }
  if (directs(f)) {
    const issued = field({ type: 'number', min: 1, required: true, placeholder: tr('акций') });
    const issuePrice = field({ type: 'number', min: 1, required: true, placeholder: tr('гол. за акцию') });
    nodes.push(form([issued, issuePrice], tr('Выпустить новые акции'), () => act('issue', { firm: f.id, shares: Number(issued.value), votes: votesOf(issuePrice) }, tr('Новые акции выставлены на продажу.'))),
      hint(tr('Голоса за новые акции идут в кассу предприятия, а доли прежних акционеров становятся меньше.')));
  }
  return nodes;
}

// --- "Акции" ---

function offersTable(offers, named) {
  return table(
    [...(named ? [tr('Предприятие')] : []), tr('Продавец'), tr('Акций'), tr('Цена акции'), ''],
    offers.map((o) => {
      const f = firmOf(o.firmId);
      const own = o.seller?.id === me.id || (!o.seller && f && directs(f));
      return [
        ...(named ? [f ? firmLink(f) : ''] : []),
        o.seller ? person(o.seller) : tr('выпуск предприятия'), groupedVotes(o.shares), votesAmount(o.price),
        own
          ? el('a', { href: '#', textContent: tr('Снять'), onclick: (e) => { e.preventDefault(); act('cancel_offer', { offer: o.id }, tr('Предложение снято.')); } })
          : el('a', { href: '#', textContent: tr('Купить'), onclick: (e) => { e.preventDefault(); buyShares(o, f); } }),
      ];
    }),
  );
}

function buyShares(o, f) {
  const count = field({ type: 'number', min: 1, max: o.shares, required: true, value: o.shares });
  const total = el('p', {});
  const recount = () => {
    const n = Number(count.value);
    total.textContent = tr('Итого: {0}.', votesAmount(Number.isInteger(n) && n > 0 ? n * o.price : 0));
  };
  count.addEventListener('input', recount);
  recount();

  const { modal, error } = dialog(tr('Покупка акций'),
    el('p', { textContent: tr('Акции «{0}» по {1} за штуку, продаётся {2}.', f?.name ?? '', votesAmount(o.price), groupedVotes(o.shares)) }),
    el('div', { className: 'firm-form' }, labelled(tr('Акций'), count)),
    total,
  );
  modal.actions.replaceChildren(
    button(tr('Купить'), async () => {
      const n = Number(count.value);
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
      [tr('Предприятие'), tr('Вид'), tr('Ур.'), tr('Директор'), tr('Прибыль за сутки'), tr('Стоимость акции'), tr('Цена сделки')],
      firms.map((f) => [
        firmLink(f), firmKindName(f.kind), f.level, person(f.director), money(f.profit), money(Math.floor(f.worth / f.shares)),
        f.sharePrice > 0 ? votesAmount(f.sharePrice) : tr('—'),
      ]),
    ));
  }

  nodes.push(title(tr('Акции на продажу')));
  nodes.push(state.offers.length ? offersTable(state.offers, true) : hint(tr('Сейчас никто не продаёт акции.')));
  nodes.push(hint(tr('Стоимость акции — сколько на неё придётся, если предприятие ликвидировать: половина вложенного в основание и улучшения, касса и деньги заявок. Акционеры получают дивиденды, когда директор их выплачивает.')));
  return nodes;
}

// --- "Биржа" ---

const price = (n) => (n > 0 ? hundredths(n) : tr('—'));

function exchange() {
  return [
    title(tr('Котировки')),
    table(
      [tr('Товар'), tr('Гос. купит'), tr('Гос. продаст'), tr('Город'), tr('Спрос'), tr('Предложение'), tr('Сделка'), tr('На бирже за сутки, шт.')],
      state.quotes.map((q) => [
        goodLink(q.good), price(q.stateBid), price(q.stateAsk), retailed(q.good) ? price(q.cityPrice) : tr('—'), price(q.bestBid), price(q.bestAsk), price(q.last), groupedVotes(q.volume),
      ]),
    ),
    hint(tr('Цены — в голосах за штуку. Государство покупает дешевле нормы и продаёт дороже: чем больше с ним наторговали за сутки, тем хуже его цены. Спрос и предложение — лучшие заявки игроков: по ним торговать выгоднее. Город платит магазинам за хлеб, мебель и электронику.')),
  ];
}

function book(good) {
  const q = quoteOf(good);
  if (!q) return [hint(tr('Такого товара нет.')), link(tr('« Биржа'), '/business?tab=exchange')];

  const side = (buy) => state.orders.filter((o) => o.good === good && o.buy === buy)
    .sort((a, b) => (buy ? b.price - a.price : a.price - b.price) || a.placedAt.localeCompare(b.placedAt));
  const rows = (list) => list.map((o) => {
    const f = firmOf(o.firmId);
    return [money(o.price), groupedVotes(o.units), f ? firmLink(f) : ''];
  });
  const bids = side(true);
  const asks = side(false);

  return [
    el('p', {}, link(tr('« Биржа'), '/business?tab=exchange')),
    title(goodName(good)),
    facts([
      [tr('Государство:'), tr('покупает по {0}, продаёт по {1}', money(q.stateBid), money(q.stateAsk))],
      retailed(good) ? [tr('Город:'), tr('платит магазинам по {0}', money(q.cityPrice))] : null,
      [tr('Последняя сделка:'), q.last > 0 ? money(q.last) : tr('ещё не было')],
      [tr('Продано на бирже за сутки:'), tr('{0} шт.', groupedVotes(q.volume))],
    ]),
    el('div', { className: 'biz-subtitle', textContent: tr('Покупают') }),
    bids.length ? table([tr('Цена'), tr('Шт.'), tr('Предприятие')], rows(bids)) : hint(tr('Заявок на покупку нет.')),
    el('div', { className: 'biz-subtitle', textContent: tr('Продают') }),
    asks.length ? table([tr('Цена'), tr('Шт.'), tr('Предприятие')], rows(asks)) : hint(tr('Заявок на продажу нет.')),
    hint(tr('Выставить заявку можно в карточке своего предприятия, в блоке «Заявки на бирже».')),
  ];
}

// --- "Справочник" ---

function guide() {
  const plan = (kind) => planOf(kind);
  return [
    title(tr('Предприятия')),
    table(
      [tr('Предприятие'), tr('Цена'), tr('Работа в час на рабочего'), tr('Рабочих на ур.'), tr('Уровней')],
      state.catalog.map((s) => [firmKindName(s.kind), votesAmount(plan(s.kind).price), recipe(s), plan(s.kind).staff, plan(s.kind).levels]),
    ),
    title(tr('Товары')),
    table([tr('Товар'), tr('Норма цены')], state.settings.goods.map((g) => [goodLink(g.good), money(g.base)])),
    hint(tr('Цепочки: ферма → мельница → хлебозавод; лесхоз → лесопилка и рудник → сталелитейный завод → мебельная фабрика; нефтевышка → химзавод и сталь → завод электроники; хлеб, мебель и электронику город покупает в магазинах. Чем дальше по цепочке, тем дороже предприятие и тем больше оно зарабатывает.')),
    hint(tr('Предприятие с полным штатом окупает основание за несколько дней, если его продукции есть сбыт. Улучшение даёт больше мест для рабочих и больший склад. Зарплата и содержание идут из кассы каждый час — следите, чтобы в ней были деньги.')),
  ];
}

function show() {
  const { tab, firm, good } = where();
  const active = firm ? 'mine' : tab;
  for (const a of $('firm-tabs').querySelectorAll('a')) a.classList.toggle('active', a.dataset.tab === active);

  let nodes;
  if (firm) nodes = firmCard(firmOf(firm));
  else if (tab === 'exchange') nodes = good ? book(good) : exchange();
  else if (tab === 'shares') nodes = shares();
  else if (tab === 'guide') nodes = guide();
  else nodes = myFirms();
  $('firm-view').replaceChildren(...nodes.filter(Boolean));
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
  } catch (e) {
    showError($('error'), e);
  }
}
