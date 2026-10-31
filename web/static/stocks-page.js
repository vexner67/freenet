// "Акции" (/stocks): the stock exchange of the enterprises as the trading
// terminals of the 2000s drew it — a running tape of prices, the index of
// the exchange, the companies with their charts of the day, and the card
// of a company (?firm=<id>): its candles, the order book and the panel of
// a deal. Shares are bought and sold of the broker account, which is
// topped up from the votes and taken back to them in a mini window over
// its plaque. The list of companies has "+1" and "−1": a deal in one press.
// The tabs of the market (?tab=) keep the portfolio, the news and the rules
// apart, and the long text of a window sits behind its "?". The server
// counts the hours and the prices; the page asks again every half minute,
// blinks what moved and only shows what the server said.

import { api, currentUser, session, started } from './api.js';
import {
  balanceAmount, firmEventAbout, firmEventName, firmKindName, firmPictures, formatDateTime, groupedVotes, hundredths, hundredthsOf, isGoodEvent, priceChange, sharePercent, stockHour, stockTick, votesAmount,
} from './labels.js';
import { $, el, fullName, onScreen, pagePath, replaceInPlace, renderChrome, showError, hideError } from './layout.js';
import { toast } from './modal.js';
import { candleChart, lineChart, sparkline } from './charts.js';
import { chips, isOpen, popover, segmented, stepper } from './controls.js';
import { flash, floatText, gameIcon, gameWindow } from './game.js';
import { coins, dealTick } from './sounds.js';
import { tr } from './i18n.js';

const firmOfAddress = () => new URLSearchParams(location.search).get('firm') ?? '';
const answered = session.get() ? started(api.stocks(firmOfAddress())) : null;

let state = null;
let me = null;
let balance = 0;
let busy = false;
// The hours of the chart of a company, kept while the page is redrawn.
let range = 24;
// The panel of a deal: buying or selling, at the market or at a price.
let side = 'buy';
let mode = 'market';
// The price of an order picked in the order book, 0 — the best now.
let limitPrice = 0;

const money = (n) => tr('{0} гол.', hundredths(n));
const companyOf = (id) => state.companies.find((c) => c.firm === id);
const holdingOf = (id) => state.holdings.find((h) => h.firm === id) ?? { shares: 0, free: 0 };
const ordersOf = (id) => state.orders.filter((o) => o.firm === id);
const person = (p) => (p ? el('a', { href: pagePath(p), textContent: fullName(p) }) : tr('—'));
const hint = (text) => el('p', { className: 'hint firm-hint', textContent: text });
const firmIcon = (kind, size) => el('img', { className: 'firm-icon', src: firmPictures[kind], alt: '', width: size, height: size });
const companyPath = (id) => `/stocks?firm=${encodeURIComponent(id)}`;
const tabOfAddress = () => new URLSearchParams(location.search).get('tab') ?? 'market';

const pictures = {
  index: '/static/icons/game/index.svg', firm: '/static/icons/game/firm.svg', wallet: '/static/icons/game/wallet.svg', share: '/static/icons/game/share.svg',
  clock: '/static/icons/game/clock.svg', news: '/static/icons/game/news.svg', help: '/static/icons/game/help.svg', candles: '/static/icons/game/candles.svg',
  book: '/static/icons/game/book.svg', deal: '/static/icons/game/deal.svg', log: '/static/icons/game/log.svg', crown: '/static/icons/game/crown.svg',
  profit: '/static/icons/game/profit.svg',
};
const win = (picture, heading, nodes, help = '') => gameWindow(pictures[picture], heading, nodes, help);

function showBalance(votes) {
  balance = votes;
}

function said(text) {
  // A sum in "гол." at the end of a sentence would give it two dots.
  $('done').textContent = text.replace(/\.\.$/, '.');
  $('done').hidden = !text;
}

// trend is the change of a price: an arrow, colored, with the percent.
function trend(now, before, withSum = false) {
  if (!before) return el('span', { className: 'stock-flat', textContent: tr('—') });
  const up = now > before;
  const down = now < before;
  const arrow = up ? '▲ ' : down ? '▼ ' : '';
  const sum = withSum ? `${up ? '+' : down ? '−' : ''}${hundredths(Math.abs(now - before))} ` : '';
  return el('span', { className: up ? 'stock-up' : down ? 'stock-down' : 'stock-flat', textContent: `${arrow}${sum}(${priceChange(now, before)})` });
}

// --- the address of the page ---

function go(href, isTab = false) {
  const firm = firmOfAddress();
  history.pushState(null, '', href);
  hideError($('error'));
  said('');
  if (firm === firmOfAddress()) show();
  else load(firmOfAddress());
  if (!isTab) window.scrollTo(0, 0);
}

function follow(event) {
  if (event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  go(event.currentTarget.getAttribute('href'), Boolean(event.currentTarget.closest('.tabs')));
}

const link = (text, href, className = '') => el('a', { href, className, textContent: text, onclick: follow });

// load asks for the exchange looking at the company firm.
async function load(firm) {
  busy = true;
  try {
    const before = new Map(state?.companies.map((c) => [c.firm, c.price]));
    state = await api.stocks(firm);
    show();
    blink(before);
  } catch (e) {
    showError($('error'), e);
  } finally {
    busy = false;
  }
}

// blink flashes the rows of the companies whose price moved.
function blink(before) {
  for (const row of document.querySelectorAll('.stock-row')) {
    const was = before.get(row.dataset.firm);
    const now = companyOf(row.dataset.firm)?.price;
    if (was && now && was !== now) flash(row, now > was ? 'good' : 'bad');
  }
}

// lastPress is what was pressed last: the rewards of a deal fly up from it.
let lastPress = null;
document.addEventListener('click', (event) => {
  lastPress = event.target.closest?.('button, a') ?? null;
}, true);

// reward answers a deal done: shares bought tick, money coming in rings.
function reward(done, body, node) {
  if (!done.shares) {
    dealTick();
    return;
  }
  if (body.buy) {
    dealTick();
    floatText(node, `+${tr('{0} акц.', groupedVotes(done.shares))}`, 'good');
    return;
  }
  coins();
  floatText(node, `+${money(done.money)}`, 'gold');
}

// act does an action and shows the exchange it left.
async function act(action, body, text, box = null) {
  if (busy) return false;
  busy = true;
  const pressed = lastPress;
  const error = box ?? $('error');
  hideError(error);
  said('');

  try {
    const done = await api.runStocks(action, { firm: firmOfAddress(), ...body });
    if (done.spent > 0 || done.got > 0) showBalance(done.balance);
    reward(done, body, pressed);
    state = done.stocks;
    show();
    const message = typeof text === 'function' ? text(done) : text;
    said(message);
    // The page stays where the deal was done; what was done is said over
    // it and, when that is out of sight, at the bottom.
    if (message && !onScreen($('done'))) toast(message);
    return true;
  } catch (e) {
    showError(error, e);
    error.hidden = false;
    if (!onScreen(error)) toast(error.textContent);
    return false;
  } finally {
    busy = false;
  }
}

// --- the plaques over the page ---

function plaque(picture, name, value, sub, onclick = null) {
  const node = el(onclick ? 'button' : 'div', { className: 'game-hud-cell' },
    el('span', { className: 'game-hud-name' }, gameIcon(pictures[picture]), name),
    el('b', { className: 'game-hud-sum' }, value),
    el('span', { className: 'hint' }, sub));
  if (onclick) {
    node.type = 'button';
    node.addEventListener('click', () => onclick(node));
  }
  return node;
}

// worth is what the shares of the caller go for at the prices now.
function worth() {
  return state.holdings.reduce((n, h) => n + h.shares * (companyOf(h.firm)?.price ?? 0), 0);
}

// indexBase is the index before its first trading hour: 1000.00.
const indexBase = 100000;

function plaques() {
  const index = state.index;
  const last = index.at(-1)?.value ?? indexBase;
  const dayAgo = index.length > 24 ? index.at(-25).value : index[0]?.value ?? indexBase;

  const day = state.holdings.reduce((n, h) => {
    const c = companyOf(h.firm);
    return n + (c?.dayAgo ? h.shares * (c.price - c.dayAgo) : 0);
  }, 0);

  return el('div', { className: 'game-hud four' },
    plaque('index', tr('Индекс Freenet'), hundredths(last), trend(last, dayAgo)),
    plaque('wallet', tr('Брокерский счёт'), money(state.account), el('span', { className: 'stock-link', textContent: tr('пополнить или вывести ▾') }), accountBox),
    plaque('share', tr('Мои акции'), money(worth()), day ? el('span', { className: day > 0 ? 'stock-up' : 'stock-down', textContent: tr('{0} за сутки', `${day > 0 ? '+' : '−'}${hundredths(Math.abs(day))}`) }) : tr('компаний: {0}', state.holdings.length)),
    plaque('clock', tr('Торговый час'), clock(), tr('до пересчёта цен')),
  );
}

function clock() {
  const node = el('span', { className: 'stock-clock' });
  tick(node);
  return node;
}

function tick(node) {
  const left = Math.max(0, Math.ceil((Date.parse(state.nextHour) - Date.now()) / 60000));
  node.textContent = tr('{0} мин.', left);
}

// accountBox is the mini window of the broker account: votes in and out.
function accountBox(anchor) {
  const pop = popover(anchor, tr('Брокерский счёт'));
  if (!pop) return;

  let way = 'in';
  const most = () => (way === 'in' ? balance : Math.floor(state.account / 100));
  const amount = stepper({ value: Math.min(100, most()), min: 1, max: Math.max(1, most()), onchange: () => recount(), wide: true });
  const after = el('div', { className: 'pop-note' });
  const go = el('button', { type: 'button', className: 'button' });

  function recount() {
    const n = amount.get();
    amount.limit(1, Math.max(1, most()));
    after.textContent = way === 'in'
      ? tr('На счету станет {0}, у вас останется {1}', money(state.account + n * 100), balanceAmount(balance - n))
      : tr('На счету останется {0}, вам придёт {1}', money(state.account - n * 100), votesAmount(n));
    go.textContent = way === 'in' ? tr('Пополнить на {0}', votesAmount(n)) : tr('Вывести {0}', votesAmount(n));
    go.disabled = most() < 1;
  }

  const ways = segmented([['in', tr('Пополнить')], ['out', tr('Вывести')]], way, (v) => {
    way = v;
    recount();
  });
  go.addEventListener('click', async () => {
    const n = amount.get();
    const action = way === 'in' ? 'account_deposit' : 'account_withdraw';
    const text = way === 'in' ? tr('Счёт пополнен на {0}.', votesAmount(n)) : tr('С брокерского счёта выведено {0}.', votesAmount(n));
    if (await act(action, { votes: n }, text, pop.error)) pop.close();
  });

  pop.body.append(
    el('div', { className: 'pop-row' }, ways.node),
    el('div', { className: 'pop-row' }, amount.node),
    el('div', { className: 'pop-row' }, chips([
      ['10', () => amount.set(10)], ['100', () => amount.set(100)], ['1 000', () => amount.set(1000)], [tr('всё'), () => amount.set(most())],
    ])),
    after,
    el('div', { className: 'pop-actions' }, go),
    el('div', { className: 'pop-note', textContent: tr('На ваших голосах: {0}. Выводить голоса могут страницы старше недели с заполненной анкетой.', balanceAmount(balance)) }),
  );
  recount();
  pop.place();
}

// --- the tape and the market ---

function ticker() {
  if (!state.companies.length) return null;
  const item = (c) => el('a', { className: 'ticker-item', href: companyPath(c.firm), onclick: follow },
    firmIcon(c.kind, 16), el('b', { textContent: c.name }), el('span', { textContent: hundredths(c.price) }), trend(c.price, c.dayAgo));
  const track = el('div', { className: 'ticker-track' }, ...state.companies.map(item), ...state.companies.map(item));
  track.style.animationDuration = `${Math.max(20, state.companies.length * 6)}s`;
  return el('div', { className: 'ticker', 'aria-hidden': 'true' }, track);
}

function market() {
  const tab = tabOfAddress();
  const tabs = [['market', tr('Рынок'), 'index'], ['portfolio', tr('Мой портфель'), 'share'], ['news', tr('Новости города'), 'news'], ['rules', tr('Правила'), 'help']];
  const view = {
    portfolio: () => [win('share', tr('Мой портфель'), [portfolio()])],
    news: () => [win('news', tr('Новости города'), [news(state.news.slice(0, 12))])],
    rules: () => [win('help', tr('Правила'), [
      el('p', { className: 'stock-rules', textContent: tr('Цена акции зависит от того, сколько стоит предприятие и сколько оно зарабатывает, от новостей и настроения рынка, а ещё — от того, сколько акций покупают и продают. Биржа сама покупает дешевле стоимости и продаёт дороже: чем больше у неё купили за сутки, тем дороже следующие акции. Продавец платит комиссию {0} %. Акционеры получают дивиденды, когда их выплачивает директор, а самый крупный акционер становится директором.', state.fee) }),
    ])],
  }[tab] ?? (() => [
    win('firm', tr('Компании'), [state.companies.length ? companies() : hint(tr('Ещё никто не основал предприятие — основайте первое в «Заработке».'))]),
    win('index', tr('Индекс Freenet'), [state.index.length > 1
      ? lineChart(state.index.map((p) => ({ time: p.hour, value: p.value })), { format: hundredths, when: stockHour, tick: stockTick })
      : hint(tr('Индекс появится, когда пройдёт первый торговый час.'))]),
  ]);

  return [
    plaques(),
    ticker(),
    el('div', { className: 'tabs game-tabs' }, ...tabs.map(([name, text, picture]) => {
      const a = link('', name === 'market' ? '/stocks' : `/stocks?tab=${name}`, name === tab || (!tabs.some((t) => t[0] === tab) && name === 'market') ? 'active' : '');
      a.append(el('img', { src: pictures[picture], alt: '', width: 16, height: 16 }), text);
      return a;
    })),
    ...view(),
  ];
}

// quick is a deal of one share at the market, in one press.
function quick(c, buy) {
  return el('button', {
    type: 'button', className: `stock-quick ${buy ? 'buy' : 'sell'}`, textContent: buy ? '+1' : '−1',
    title: buy ? tr('Купить одну акцию по рынку') : tr('Продать одну акцию по рынку'),
    disabled: !buy && holdingOf(c.firm).free < 1,
    onclick: () => act('stock_order', { firm: c.firm, buy, price: 0, shares: 1 }, (d) => {
      if (!d.shares) return tr('Заявка выставлена: она исполнится, когда найдётся цена.');
      return buy ? tr('Куплено {0} акц. за {1}.', groupedVotes(d.shares), money(d.money)) : tr('Продано {0} акц. за {1}.', groupedVotes(d.shares), money(d.money));
    }),
  });
}

function companies() {
  const head = (text, className = '') => el('th', { className, textContent: text });
  const rows = [...state.companies].sort((a, b) => b.price * b.shares - a.price * a.shares).map((c) => {
    const row = el('tr', { className: 'stock-row', onclick: (e) => !e.target.closest('a, button') && go(companyPath(c.firm)) },
      el('td', {}, el('a', { className: 'firm-good', href: companyPath(c.firm), onclick: follow }, firmIcon(c.kind, 24), el('span', {},
        el('b', { textContent: c.name }), el('br'), el('span', { className: 'hint', textContent: `${firmKindName(c.kind)}, ${tr('{0} ур.', c.level)}` })))),
      el('td', { className: 'firm-num stock-price', textContent: hundredths(c.price) }),
      el('td', { className: 'firm-num' }, trend(c.price, c.dayAgo)),
      el('td', { className: 'stock-spark' }, sparkline(c.candles.map((k) => k.close))),
      el('td', { className: 'firm-num stock-wide', textContent: groupedVotes(c.volume) }),
      el('td', { className: 'firm-num stock-wide', textContent: money(c.price * c.shares) }),
      el('td', { className: 'stock-quicks' }, quick(c, true), quick(c, false)));
    row.dataset.firm = c.firm;
    return row;
  });

  return el('div', { className: 'firm-scroll' }, el('table', { className: 'list-table stock-table' },
    el('thead', {}, el('tr', {},
      head(tr('Компания')), head(tr('Цена'), 'firm-num'), head(tr('За сутки'), 'firm-num'), head(tr('График'), 'stock-spark'),
      head(tr('Объём'), 'firm-num stock-wide'), head(tr('Капитализация'), 'firm-num stock-wide'), head(tr('Сделка')))),
    el('tbody', {}, ...rows)));
}

function portfolio() {
  if (!state.holdings.length && !state.orders.length) return hint(tr('У вас пока нет акций. Пополните брокерский счёт и выберите компанию.'));

  const rows = state.holdings.map((h) => {
    const c = companyOf(h.firm);
    if (!c) return null;
    return el('a', { className: 'stock-holding', href: companyPath(c.firm), onclick: follow },
      firmIcon(c.kind, 24),
      el('span', { className: 'stock-holding-name' }, el('b', { textContent: c.name }), el('span', { className: 'hint', textContent: tr('{0} акц., {1} от всех', groupedVotes(h.shares), sharePercent(h.shares, c.shares)) })),
      el('span', { className: 'stock-holding-worth' }, el('b', { textContent: money(h.shares * c.price) }), trend(c.price, c.dayAgo)));
  });
  return el('div', { className: 'stock-holdings' }, ...rows.filter(Boolean), state.orders.length ? myOrders(state.orders, true) : null);
}

function news(events) {
  if (!events.length) return hint(tr('В городе всё спокойно.'));
  return el('div', { className: 'stock-news' }, ...events.map((e) => {
    const firm = e.firm ? companyOf(e.firm) : null;
    const kind = firm?.kind ?? e.kinds[0] ?? 'shop';
    const going = Date.parse(e.end) > Date.now();
    return el('div', { className: `stock-news-item ${isGoodEvent(e.kind) ? 'good' : 'bad'}` },
      firmIcon(kind, 24),
      el('div', {},
        el('b', { textContent: firm ? `${firmEventName(e.kind)}: ${firm.name}` : firmEventName(e.kind) }),
        el('div', { textContent: firmEventAbout(e.kind) }),
        el('div', { className: 'hint', textContent: going && e.end !== e.start ? tr('{0}, до {1}', formatDateTime(e.start), stockHour(e.end)) : formatDateTime(e.start) })));
  }));
}

function myOrders(orders, named) {
  return el('div', { className: 'stock-orders' }, ...orders.map((o) => {
    const c = companyOf(o.firm);
    return el('div', { className: `stock-order ${o.buy ? 'buy' : 'sell'}` },
      el('span', { className: 'stock-order-side', textContent: o.buy ? tr('покупка') : tr('продажа') }),
      named && c ? el('b', { textContent: c.name }) : null,
      el('span', { textContent: o.stop
        ? tr('{0} акц.: стоп {1}, затем по {2}', groupedVotes(o.shares), hundredths(o.stop), hundredths(o.price))
        : tr('{0} акц. по {1}', groupedVotes(o.shares), hundredths(o.price)) }),
      el('button', { type: 'button', className: 'chip', textContent: tr('снять'), onclick: () => act('stock_cancel', { order: o.id, firm: firmOfAddress() }, tr('Заявка снята.')) }));
  }));
}

// --- the card of a company ---

function companyView(c) {
  if (!c) return [hint(tr('Такой компании нет: предприятие закрылось.')), link(tr('« Все компании'), '/stocks')];

  const candles = c.candles.slice(-range);
  const ranges = segmented([[24, tr('24 часа')], [72, tr('3 дня')], [168, tr('неделя')]], range, (v) => {
    range = v;
    show();
  });
  const h = holdingOf(c.firm);
  const about = state.news.filter((e) => e.firm === c.firm || (!e.firm && (!e.kinds.length || e.kinds.includes(c.kind))));

  return [
    plaques(),
    el('div', { className: 'firm-back' }, link(tr('« Все компании'), '/stocks')),
    el('div', { className: 'stock-head' },
      el('span', { className: 'firm-header-icon' }, firmIcon(c.kind, 64)),
      el('div', { className: 'firm-header-text' },
        el('div', { className: 'firm-header-name', textContent: c.name }),
        el('div', { className: 'firm-header-kind', textContent: `${firmKindName(c.kind)}, ${tr('{0} ур.', c.level)}` }),
        el('div', {}, el('span', { className: 'hint', textContent: tr('Директор: ') }), person(c.director)),
        el('div', {}, link(tr('Карточка предприятия »'), `/business?firm=${encodeURIComponent(c.firm)}`))),
      el('div', { className: 'stock-head-price' },
        el('div', { className: 'stock-big', textContent: hundredths(c.price) }),
        trend(c.price, c.dayAgo, true),
        el('div', { className: 'hint', textContent: tr('голосов за акцию') }))),
    el('div', { className: 'stock-trade' },
      win('deal', tr('Сделка'), [dealPanel(c, h)], tr('«По рынку» покупает и продаёт сразу по лучшей цене, «по своей цене» ждёт, пока найдётся такая цена.')),
      win('book', tr('Стакан'), [orderBook(c)], tr('Нажмите на строку, чтобы подставить её цену в сделку.'))),
    win('candles', tr('График цены'), [
      el('div', { className: 'stock-chart-head' }, ranges.node),
      candles.length ? candleChart(candles, {
        format: hundredths, when: stockHour, tick: stockTick,
        texts: { open: tr('откр.'), high: tr('макс.'), low: tr('мин.'), close: tr('закр.'), volume: tr('объём') },
      }) : hint(tr('Свечи появятся, когда пройдёт первый торговый час.')),
    ]),
    win('share', tr('Мои акции и заявки'), [h.shares || ordersOf(c.firm).length
      ? el('div', {},
        h.shares ? el('p', { className: 'stock-mine' }, tr('У вас {0} акц. на {1} — {2} от всех', groupedVotes(h.shares), money(h.shares * c.price), sharePercent(h.shares, c.shares))) : null,
        ordersOf(c.firm).length ? myOrders(ordersOf(c.firm), false) : null)
      : hint(tr('У вас нет акций этой компании.'))]),
    win('profit', tr('Показатели'), [facts(c)]),
    win('log', tr('Лента сделок'), [tape(c)]),
    win('crown', tr('Выкуп акций'), [tenders(c, h)], tr('Предложите акционерам продать вам акции не дешевле {0} за штуку. Деньги удерживаются со счёта на сутки, непотраченное вернётся. Кто соберёт больше акций, чем у директора, сам станет директором.', hundredths(c.price))),
    win('news', tr('Новости'), [news(about.slice(0, 8))]),
  ];
}

// tenders are the offers to buy the shares of the company out: each
// shareholder sells to one as much as it likes, and who comes to hold the
// most directs the enterprise. Below — the form of an offer of one's own.
function tenders(c, h) {
  const offers = state.tenders.filter((t) => t.firm === c.firm);
  const mine = offers.find((t) => t.user.id === me.id);
  const rows = offers.map((t) => {
    const own = t.user.id === me.id;
    const error = el('div', { className: 'error', hidden: true });
    const shares = stepper({ value: Math.max(1, Math.min(h.free, t.shares)), min: 1, max: Math.max(1, Math.min(h.free, t.shares)) });
    const action = own
      ? el('button', { type: 'button', className: 'button', textContent: tr('Отозвать'), onclick: () => act('tender_cancel', { firm: c.firm, order: t.id }, tr('Предложение отозвано, деньги вернулись на счёт.'), error) })
      : h.free > 0
        ? el('span', { className: 'tender-sell' }, shares.node, el('button', {
          type: 'button', className: 'button', textContent: tr('Продать'),
          onclick: () => act('tender_sell', { firm: c.firm, order: t.id, shares: shares.get() }, (d) => tr('Продано {0} акц. за {1}.', groupedVotes(d.shares), money(d.money)), error),
        }))
        : null;
    return el('div', { className: `stock-tender ${own ? 'own' : ''}`.trim() },
      el('div', { className: 'stock-tender-text' },
        el('div', {}, person(t.user), ' ', tr('покупает {0} акц. по {1}', groupedVotes(t.shares), hundredths(t.price))),
        el('div', { className: 'hint', textContent: tr('{0} к цене, до {1}', priceChange(t.price, c.price), stockHour(t.until)) })),
      action, error);
  });

  return el('div', { className: 'stock-tenders' },
    offers.length ? el('div', {}, ...rows) : hint(tr('Никто не предлагает выкупить акции этой компании.')),
    mine ? null : tenderForm(c, h));
}

// tenderForm offers to buy shares of the company of their holders at a
// price no lower than a share goes for now; the money is kept on the
// account for a day.
function tenderForm(c, h) {
  const others = Math.max(0, c.shares - c.pool - c.float - h.shares);
  if (!others) return null;

  const error = el('div', { className: 'error', hidden: true });
  const plain = (n) => hundredths(n).replace(/\u00a0/g, '');
  const shares = stepper({ value: Math.min(others, Math.ceil(c.shares / 2)), min: 1, max: others, onchange: () => recount(), wide: true });
  const priceInput = el('input', { type: 'text', inputMode: 'decimal', className: 'step-value wide', value: plain(Math.round(c.price * 1.2)) });
  const sum = el('div', { className: 'deal-summary' });
  const go = el('button', { type: 'button', className: 'button deal-go', textContent: tr('Предложить выкуп') });
  const price = () => hundredthsOf(priceInput.value);
  // A stop order sells a little below its stop and buys a little above,
  // so that it deals when it wakes even if the price runs on.
  const stopLimit = () => Math.max(1, Math.round(price() * (side === 'buy' ? 1.05 : 0.95)));
  const stopOff = () => (side === 'buy' ? price() <= c.price : price() >= c.price);

  function recount() {
    const total = shares.get() * price();
    sum.replaceChildren(
      el('div', {}, el('span', { textContent: tr('Будет удержано') }), el('b', { textContent: money(total) })),
      el('div', {}, el('span', { textContent: tr('На счету останется') }), el('b', { textContent: money(state.account - total) })));
    go.disabled = price() < c.price || total > state.account;
  }
  priceInput.addEventListener('input', recount);
  go.addEventListener('click', () => act('tender', { firm: c.firm, shares: shares.get(), price: price() }, tr('Предложение отправлено акционерам.'), error));
  recount();

  return el('div', { className: 'deal buy stock-tender-form' },
    error,
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Акций') }), shares.node),
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Цена') }), el('span', { className: 'stepper' }, priceInput)),
    sum,
    go);
}

function facts(c) {
  // A day of profit a share: what a price is measured against.
  const earnings = c.shares ? Math.trunc(c.profit / c.shares) : 0;
  const fact = (name, value) => el('div', { className: 'stock-fact' }, el('span', { textContent: name }), el('b', {}, value));
  return el('div', { className: 'stock-facts' },
    fact(tr('Стоимость'), hundredths(c.book)),
    fact(tr('С прибылью'), hundredths(c.fair)),
    fact(tr('Прибыль за сутки'), el('span', { className: c.profit >= 0 ? 'stock-up' : 'stock-down', textContent: money(c.profit) })),
    fact(tr('Сделок за сутки'), tr('{0} акц.', groupedVotes(c.volume))),
    fact(tr('Акционеров'), String(c.holders)),
    fact(tr('Акций всего'), groupedVotes(c.shares)),
    fact(tr('Продаёт биржа'), groupedVotes(c.pool + c.float)),
    fact(tr('Капитализация'), money(c.price * c.shares)),
    fact(tr('Прибыль на акцию (EPS)'), money(earnings)),
    fact(tr('P/E'), earnings > 0 ? (c.price / (earnings * 365)).toFixed(1) : tr('—')),
  );
}

// tape is the last deals of the company, the newest first.
function tape(c) {
  if (!c.tape?.length) return hint(tr('Сделок пока не было.'));

  return el('div', { className: 'stock-tape' }, ...c.tape.slice().reverse().map((t, i, all) => {
    const before = all[i + 1]?.price ?? t.price;
    return el('div', { className: 'stock-tape-row' },
      el('span', { textContent: formatDateTime(t.at) }),
      el('b', { className: t.price >= before ? 'stock-up' : 'stock-down', textContent: hundredths(t.price) }),
      el('span', { textContent: tr('{0} акц.', groupedVotes(t.shares)) }));
  }));
}

// pick sets the price of a row of the order book in the panel of a deal:
// buying what is sold there, selling to who buys.
function pick(price, buy) {
  mode = 'limit';
  side = buy ? 'buy' : 'sell';
  limitPrice = price;
  show();
}

function orderBook(c) {
  const most = Math.max(1, ...state.asks.map((l) => l.shares), ...state.bids.map((l) => l.shares));
  const row = (price, shares, who, buy) => {
    const fill = el('span', { className: 'book-fill' });
    fill.style.width = `${shares ? Math.min(100, (shares / most) * 100) : 0}%`;
    return el('button', {
      type: 'button', className: `book-row ${buy ? 'bid' : 'ask'}`, title: tr('Подставить цену'),
      onclick: () => pick(price, !buy),
    }, fill, el('span', { className: 'book-price', textContent: hundredths(price) }), el('span', { className: 'book-shares', textContent: shares ? groupedVotes(shares) : '' }), el('span', { className: 'book-who', textContent: who }));
  };

  const asks = [...state.asks].reverse().map((l) => row(l.price, l.shares, tr('{0} заяв.', l.orders), false));
  const bids = state.bids.map((l) => row(l.price, l.shares, tr('{0} заяв.', l.orders), true));
  const selling = c.pool + c.float;

  return el('div', { className: 'book' },
    el('div', { className: 'book-head' }, el('span', { textContent: tr('Цена') }), el('span', { textContent: tr('Акций') }), el('span', { textContent: tr('Кто') })),
    selling ? row(c.ask, selling, tr('биржа'), false) : null,
    ...asks,
    el('div', { className: 'book-spread' }, tr('последняя {0}', hundredths(c.price))),
    ...bids,
    row(c.bid, 0, tr('биржа'), true));
}

function dealPanel(c, h) {
  const box = el('div', { className: `deal ${side}` });
  const error = el('div', { className: 'error', hidden: true });
  const shares = stepper({ value: 1, min: 1, max: 1_000_000, onchange: () => recount(), wide: true });
  const plain = (n) => hundredths(n).replace(/\u00a0/g, '');
  const priceInput = el('input', { type: 'text', inputMode: 'decimal', className: 'step-value wide', value: plain(limitPrice || (side === 'buy' ? c.ask : c.bid)) });
  const summary = el('div', { className: 'deal-summary' });
  const go = el('button', { type: 'button', className: 'button deal-go' });

  const price = () => hundredthsOf(priceInput.value);
  // expected is what a share should go for: the price of an order, or at
  // the market the best price there is now.
  const expected = () => {
    if (mode === 'stop') return stopLimit();
    if (mode === 'limit') return price();
    if (side === 'buy') return Math.min(c.ask, c.bestAsk || Infinity);
    return Math.max(c.bid, c.bestBid);
  };
  const most = () => (side === 'buy' ? Math.floor(state.account / Math.max(1, expected())) : h.free);

  const nudge = (k) => el('button', {
    type: 'button', className: 'step-button', textContent: k < 0 ? '−' : '+',
    onclick: () => {
      const p = price() || c.price;
      priceInput.value = plain(Math.max(1, p + k * Math.max(1, Math.round(p / 100))));
      limitPrice = price();
      recount();
    },
  });
  priceInput.addEventListener('input', () => {
    limitPrice = price() || 0;
    recount();
  });

  function recount() {
    box.className = `deal ${side}`;
    const n = shares.get();
    const p = expected();
    const sum = Number.isFinite(p) && p > 0 ? n * p : 0;
    const fee = side === 'sell' ? Math.floor((sum * state.fee) / 100) : 0;
    const lines = [
      [mode === 'market' ? tr('Примерно') : tr('Сумма'), money(sum)],
      side === 'sell' ? [tr('Комиссия {0} %', state.fee), money(fee)] : null,
      side === 'buy' ? [tr('На счету останется'), money(state.account - sum)] : [tr('Придёт на счёт'), money(sum - fee)],
    ].filter(Boolean);
    summary.replaceChildren(...lines.map(([name, value]) => el('div', {}, el('span', { textContent: name }), el('b', { textContent: value }))));
    go.className = `button game-go ${side}`;
    go.textContent = side === 'buy'
      ? tr('Купить {0} акц.', groupedVotes(n))
      : tr('Продать {0} акц.', groupedVotes(n));
    // An order further from the price of the exchange than its band is
    // refused, as a real exchange does: the button waits for a price in it.
    const outOfBand = mode === 'limit' && c.bandHigh > 0 && (p < c.bandLow || p > c.bandHigh);
    band.classList.toggle('stock-down', outOfBand);
    stopHint.classList.toggle('stock-down', mode === 'stop' && stopOff());
    go.disabled = !n || outOfBand || (mode === 'stop' && stopOff()) || (side === 'sell' && n > h.free) || (side === 'buy' && sum > state.account);
  }

  const sides = segmented([['buy', tr('Купить'), 'buy'], ['sell', tr('Продать'), 'sell']], side, (v) => {
    side = v;
    limitPrice = 0;
    priceInput.value = plain(side === 'buy' ? c.ask : c.bid);
    draw();
  });
  const modes = segmented([['market', tr('по рынку')], ['limit', tr('по своей цене')], ['stop', tr('стоп')]], mode, (v) => {
    mode = v;
    draw();
  });
  const priceRow = el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Цена') }),
    el('span', { className: 'stepper' }, nudge(-1), priceInput, nudge(1)));
  const band = el('div', { className: 'hint', textContent: tr('Цена заявки — от {0} до {1}: дальше от цены биржи сделки не идут.', money(c.bandLow), money(c.bandHigh)) });

  const stopHint = el('div', { className: 'hint' });

  function draw() {
    priceRow.hidden = mode === 'market';
    band.hidden = mode !== 'limit';
    stopHint.hidden = mode !== 'stop';
    stopHint.textContent = side === 'buy'
      ? tr('Стоп-заявка на покупку: выше цены сейчас ({0}). Когда цена дойдёт до неё, заявка встанет в стакан. Цена проверяется раз в час.', money(c.price))
      : tr('Стоп-лосс: ниже цены сейчас ({0}). Когда цена упадёт до неё, акции выставятся на продажу. Цена проверяется раз в час.', money(c.price));
    recount();
  }

  go.addEventListener('click', () => {
    const n = shares.get();
    const order = { firm: c.firm, buy: side === 'buy', price: mode === 'limit' ? price() : 0, shares: n };
    if (mode === 'stop') Object.assign(order, { price: stopLimit(), stop: price() });
    act('stock_order', order, (d) => {
      if (mode === 'stop') return tr('Стоп-заявка выставлена.');
      if (d.shares === n) return side === 'buy' ? tr('Куплено {0} акц. за {1}.', groupedVotes(d.shares), money(d.money)) : tr('Продано {0} акц. за {1}.', groupedVotes(d.shares), money(d.money));
      if (d.shares > 0) return tr('Сразу исполнено {0} акц. из {1}, остальное ждёт в заявке.', groupedVotes(d.shares), groupedVotes(n));
      return tr('Заявка выставлена: она исполнится, когда найдётся цена.');
    }, error);
  });

  box.append(
    error,
    el('div', { className: 'deal-row' }, sides.node),
    el('div', { className: 'deal-row' }, modes.node),
    priceRow,
    band,
    stopHint,
    el('div', { className: 'deal-row' }, el('span', { className: 'deal-label', textContent: tr('Акций') }), shares.node),
    el('div', { className: 'deal-row' }, chips([
      ['1', () => shares.set(1)], ['10', () => shares.set(10)], ['50', () => shares.set(50)],
      [tr('макс.'), () => shares.set(Math.max(1, most()))],
    ])),
    summary,
    go,
    el('div', { className: 'hint', textContent: side === 'buy' ? tr('На счету {0}', money(state.account)) : tr('Свободно {0} акц.', groupedVotes(h.free)) }),
  );
  draw();
  return box;
}

// --- the page ---

function show() {
  const firm = firmOfAddress();
  const nodes = firm ? companyView(companyOf(firm)) : market();
  replaceInPlace($('stocks'), ...nodes.filter(Boolean));
}

// refresh asks for the exchange again, unless a mini window is open or
// something is typed; the clocks go on with it, every half minute.
function refresh() {
  for (const node of document.querySelectorAll('.stock-clock')) tick(node);
  if (!state || busy || isOpen() || document.hidden) return;
  // The card of a company holds a deal being made: it waits for the hour.
  if (firmOfAddress() && Date.now() < Date.parse(state.nextHour) + 5000) return;
  if (document.activeElement?.matches?.('#stocks input')) return;
  load(firmOfAddress());
}

me = await currentUser().catch(() => null);
renderChrome(me);

$('stocks').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
} else {
  showBalance(me.votes ?? 0);
  window.addEventListener('popstate', () => {
    hideError($('error'));
    said('');
    load(firmOfAddress());
  });
  try {
    state = await answered;
    show();
    setInterval(refresh, 30000);
  } catch (e) {
    showError($('error'), e);
  }
}
