// "Бизнес" (/business), a tab of "Работа": a page buys businesses for votes
// and runs them as a little game. Each business is a picture of a street:
// the building with its workers in the window, people walking by and going
// in, a warehouse, a safe with the till and a billboard. Clicking a part
// opens a box of what it is and what to do with it — the price of the
// goods and levels at the building, workers at the window, goods at the
// warehouse, votes at the safe, advertising at the billboard. Worn premises
// crack, a ruined business is boarded up. The server counts the hours; the
// page only shows what it said. The administrators tune the businesses in
// the admin panel (/admin/business).

import { api, currentUser, session, started } from './api.js';
import { businessCondition, businessHours, businessLevelCost, businessName, businessWalkers, formatDateTime, groupedVotes, hundredths, plural, repairCost, restockCost, votesAmount, votesFontSize } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.businesses()) : null;

// The pictures of the businesses and of their parts.
const buildings = {
  kiosk: '/static/business/kiosk.svg',
  cafe: '/static/business/cafe.svg',
  carwash: '/static/business/carwash.svg',
  factory: '/static/business/factory.svg',
};
const customers = ['/static/business/customer-1.svg', '/static/business/customer-2.svg', '/static/business/customer-3.svg'];
const cracks = [null, '/static/business/cracks-1.svg', '/static/business/cracks-2.svg'];

// Where on the street (480×240) each building has its door and the
// window its workers stand in: x from and to, and the line they stand on.
const doors = { kiosk: 225, cafe: 295, carwash: 259, factory: 225 };
const windows = {
  kiosk: [168, 282, 150],
  cafe: [142, 270, 152],
  carwash: [136, 194, 150],
  factory: [150, 300, 128],
};

let state = null;
let busy = false;
let panel = null;

function showBalance(votes) {
  $('votes').textContent = votes;
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('balance').hidden = false;
}

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

const percent = (n, of) => `${(n / of) * 100}%`;

function picture(src, className = '') {
  return el('img', { src, alt: '', className, draggable: false });
}

// spot is a clickable part of the picture of a business.
function spot(className, title, onclick, ...children) {
  return el('button', { type: 'button', className: `biz-spot ${className}`, title, onclick }, ...children);
}

function alert(on) {
  return on ? el('span', { className: 'biz-alert', textContent: '!' }) : null;
}

function walkers(b, plan) {
  const { served, passing } = businessWalkers(b, plan);
  const all = [...Array(served).fill(true), ...Array(passing).fill(false)];
  const length = 9;

  return all.map((going, i) => {
    const node = picture(customers[i % customers.length], going ? 'biz-walker biz-walker-in' : 'biz-walker biz-walker-by');
    node.style.setProperty('--door', percent(doors[b.kind] - 8, 480));
    node.style.left = percent(20 + i * 22, 480);
    node.style.animationDelay = `${-((i * length) / all.length).toFixed(2)}s, 0s`;
    return node;
  });
}

function workers(b) {
  const [from, to, line] = windows[b.kind];
  const room = Math.floor((to - from) / 15);
  const shown = Math.min(b.staff, room);

  const region = spot('biz-workers', tr('Работники'), () => openPanel(b.kind, 'staff'),
    ...Array.from({ length: shown }, () => picture('/static/business/worker.svg')),
    b.staff > shown ? el('span', { className: 'biz-more', textContent: `×${b.staff}` }) : null,
    alert(b.staff === 0 && !b.bankrupt),
  );
  region.style.left = percent(from, 480);
  region.style.width = percent(to - from, 480);
  region.style.top = percent(line - 22, 240);
  region.style.height = percent(22, 240);

  return region;
}

function scene(b, plan) {
  const worn = businessCondition(b.condition);
  const storage = plan.storage * b.level;
  const ad = b.adUntil && new Date(b.adUntil) > new Date();
  const working = !b.bankrupt && b.served > 0;

  const building = spot('biz-building', businessName(b.kind), () => openPanel(b.kind, 'building'),
    picture(buildings[b.kind]),
    el('span', { className: `biz-sign biz-sign-${b.kind}`, textContent: businessName(b.kind) }),
    cracks[worn] ? picture(cracks[worn], 'biz-cover') : null,
    b.bankrupt ? picture('/static/business/boards.svg', 'biz-cover') : null,
    b.bankrupt ? el('span', { className: 'biz-stamp', textContent: tr('Разорён') }) : null,
    alert(!b.bankrupt && worn === 2),
  );

  const fill = el('span', { className: 'biz-fill' });
  fill.style.width = percent(Math.min(b.stock, storage), Math.max(storage, 1));
  const warehouse = spot('biz-storage', tr('Склад'), () => openPanel(b.kind, 'storage'),
    picture('/static/business/storage.svg'),
    el('span', { className: b.stock > 0 ? 'biz-badge' : 'biz-badge biz-badge-bad' },
      b.stock > 0 ? tr('{0} шт.', groupedVotes(b.stock)) : tr('Пусто'),
      el('span', { className: 'biz-meter' }, fill),
    ),
    alert(!b.bankrupt && b.stock <= 0),
  );

  const safe = spot('biz-safe', tr('Касса'), () => openPanel(b.kind, 'till'),
    picture('/static/business/safe.svg'),
    el('span', { className: b.inDebt ? 'biz-badge biz-badge-bad' : 'biz-badge' }, tr('{0} гол.', hundredths(b.cash))),
    alert(b.inDebt),
  );

  const board = spot(ad ? 'biz-billboard biz-billboard-on' : 'biz-billboard', tr('Рекламный щит'), () => openPanel(b.kind, 'ad'),
    picture('/static/business/billboard.svg'),
    el('span', { className: 'biz-board' }, ...(ad
      ? [el('b', { textContent: businessName(b.kind) }), el('span', { textContent: tr('+{0} % покупателей', plan.adBoost) })]
      : [el('span', { textContent: tr('Здесь могла быть ваша реклама') })])),
  );

  const smoke = b.kind === 'factory' && working
    ? [0, 1, 2].map((i) => {
      const puff = picture('/static/business/smoke.svg', 'biz-smoke');
      puff.style.animationDelay = `${-i}s`;
      return puff;
    })
    : [];

  return el('div', { className: 'biz-scene' }, board, building, ...smoke, workers(b), safe, warehouse, ...walkers(b, plan));
}

function businessCard(b, plan) {
  const profit = b.income - b.costs;
  const hour = b.bankrupt
    ? el('span', { className: 'biz-hour bad', textContent: tr('разорён') })
    : el('span', { className: profit < 0 ? 'biz-hour bad' : 'biz-hour good', textContent: tr('за час: {0} гол.', (profit > 0 ? '+' : '') + hundredths(profit)) });

  return el('div', { className: 'biz-card', id: `biz-${b.kind}` },
    el('div', { className: 'biz-head' },
      picture(buildings[b.kind], 'biz-icon'),
      el('b', { textContent: businessName(b.kind) }),
      el('span', { className: 'biz-level', textContent: tr('уровень {0} из {1}', b.level, plan.levels) }),
      hour,
    ),
    scene(b, plan),
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
      spot('biz-building', tr('Купить: {0}', name), buy,
        picture('/static/business/lot.svg'),
        el('span', { className: 'biz-for-sale' }, el('b', { textContent: tr('Продаётся') }), el('span', { textContent: name })),
      ),
    ),
  );
}

function show() {
  const plans = new Map(state.settings.plans.map((p) => [p.kind, p]));
  const owned = new Set(state.businesses.map((b) => b.kind));

  $('owned').replaceChildren(...state.businesses.map((b) => businessCard(b, plans.get(b.kind))));
  const offers = state.settings.plans.filter((p) => !owned.has(p.kind));
  $('shop').replaceChildren(...offers.map(lot));
  $('shop-block').hidden = offers.length === 0;
  $('owned-hint').hidden = state.businesses.length === 0;
}

// float shows what an action brought or cost over the safe of a business.
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

    if (panel && panel.modal.box.isConnected && (panel.part === 'lot' || state.businesses.some((b) => b.kind === kind))) {
      if (panel.part === 'lot') panel.part = 'building';
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

function fact(label, value, warn = false) {
  return el('tr', {},
    el('td', { className: 'biz-label', textContent: label }),
    el('td', { className: warn ? 'biz-warn' : '', textContent: value }),
  );
}

function facts(...rows) {
  return el('table', { className: 'biz-facts' }, ...rows);
}

function bar(value, of, bad = false) {
  const fill = el('span', { className: bad ? 'biz-fill bad' : 'biz-fill' });
  fill.style.width = percent(Math.max(0, Math.min(value, of)), Math.max(of, 1));
  return el('span', { className: 'biz-bar' }, fill);
}

function numberField(value, min, max) {
  return el('input', { type: 'number', className: 'biz-number', value: String(value), min: String(min), max: String(max), step: '1' });
}

function row(...children) {
  return el('div', { className: 'biz-row' }, ...children);
}

const decimal = (n) => n.toFixed(1).replace('.', ',');

function buildingPanel(b, plan) {
  const profit = b.income - b.costs;
  const name = businessName(b.kind);

  if (b.bankrupt) {
    return {
      title: tr('{0} — разорён', name),
      body: [
        el('p', { className: 'biz-bankrupt', textContent: tr('Бизнес разорился: долги превысили допустимое. Его можно только закрыть — тогда его можно купить снова.') }),
        facts(fact(tr('Долг:'), tr('{0} гол.', hundredths(-b.cash)), true), fact(tr('Вложено и выведено:'), tr('{0} / {1}', votesAmount(b.invested), votesAmount(b.collected)))),
      ],
      actions: [button(tr('Закрыть бизнес'), () => act(b.kind, () => api.runBusiness(b.kind, 'sell'), () => tr('Бизнес «{0}» закрыт.', name)))],
    };
  }

  const price = el('input', { type: 'text', className: 'biz-number', value: hundredths(b.price), inputMode: 'decimal' });
  const save = button(tr('Сохранить'), () => {
    const cents = Math.round(Number(price.value.trim().replace(',', '.')) * 100) || 0;
    act(b.kind, () => api.runBusiness(b.kind, 'manage', { price: cents, staff: b.staff }), () => tr('Цена товара сохранена.'));
  });

  const actions = [];
  if (b.level < plan.levels) {
    actions.push(button(tr('Улучшить за {0}', votesAmount(businessLevelCost(b.level, plan.upgrade))), () => act(
      b.kind, () => api.runBusiness(b.kind, 'upgrade'), () => tr('Бизнес «{0}» улучшен.', name),
    )));
  }
  if (b.condition < 10000) {
    actions.push(button(tr('Ремонт за {0}', votesAmount(repairCost(b.condition, plan.repair))), () => act(
      b.kind, () => api.runBusiness(b.kind, 'repair'), () => tr('Помещение отремонтировано.'),
    )));
  }
  if (!b.inDebt) actions.push(button(tr('Продать'), () => sell(b, name), { secondary: true }));

  return {
    title: tr('{0} — уровень {1} из {2}', name, b.level, plan.levels),
    body: [
      facts(
        fact(tr('Покупателей в час:'), tr('{0}, обслуживают {1}', decimal(b.customers), decimal(b.served))),
        fact(tr('Выручка за час:'), tr('{0} гол.', hundredths(b.income))),
        fact(tr('Расходы за час:'), tr('{0} гол. (зарплаты и аренда)', hundredths(b.costs))),
        fact(tr('Итог часа:'), tr('{0} гол.', hundredths(profit)), profit < 0),
        fact(tr('Вложено и выведено:'), tr('{0} / {1}', votesAmount(b.invested), votesAmount(b.collected))),
      ),
      el('div', { className: 'biz-label', textContent: tr('Состояние помещения: {0} %', hundredths(b.condition)) }),
      bar(b.condition, 10000, b.condition < 4000),
      b.level < plan.levels
        ? el('p', { className: 'hint', textContent: tr('Улучшение: больше покупателей, склад, касса и места для работников.') })
        : el('p', { className: 'hint', textContent: tr('Бизнес на высшем уровне.') }),
      row(el('span', { className: 'biz-label', textContent: tr('Цена товара, гол. за шт.:') }), price, save),
      el('p', { className: 'hint', textContent: tr('Обычная цена — {0}. Дороже — меньше покупателей, вдвое дороже не купит никто.', hundredths(plan.fair)) }),
    ],
    actions,
  };
}

function staffPanel(b, plan) {
  const most = plan.staff * b.level;
  const need = Math.ceil(b.customers / Math.max(plan.serve, 1));
  const staff = numberField(b.staff, 0, most);

  return {
    title: tr('Работники'),
    body: [
      facts(
        fact(tr('Работают:'), tr('{0} из {1}', b.staff, most), b.staff === 0),
        fact(tr('Один обслуживает:'), tr('{0} покупателей в час', plan.serve)),
        fact(tr('Зарплата:'), tr('{0} гол. в час каждому', hundredths(plan.wage))),
        fact(tr('Нужно сейчас:'), tr('{0} — на {1} покупателей в час', need, decimal(b.customers)), b.staff < need),
      ),
      b.bankrupt ? null : row(el('span', { className: 'biz-label', textContent: tr('Работников:') }), staff),
      b.bankrupt ? null : el('p', { className: 'hint', textContent: tr('Без работников покупатели проходят мимо, лишние только получают зарплату.') }),
    ],
    actions: b.bankrupt ? [] : [button(tr('Нанять и уволить'), () => act(
      b.kind, () => api.runBusiness(b.kind, 'manage', { price: b.price, staff: Number(staff.value) || 0 }), () => tr('Работников: {0}.', Number(staff.value) || 0),
    ))],
  };
}

function storagePanel(b, plan) {
  const storage = plan.storage * b.level;
  const room = Math.max(storage - b.stock, 0);
  const units = numberField(room, 1, room || 1);
  const cost = el('span', { className: 'hint' });
  const showCost = () => { cost.textContent = tr('стоит {0}', votesAmount(restockCost(Number(units.value) || 0, plan.cost))); };
  units.addEventListener('input', showCost);
  showCost();

  const hours = businessHours(b.stock, b.served);
  const restock = button(tr('Закупить'), () => act(
    b.kind, () => api.runBusiness(b.kind, 'restock', { amount: Number(units.value) || 0 }), (d) => tr('Товар закуплен за {0}.', votesAmount(d.spent)),
  ));
  restock.disabled = room === 0 || b.bankrupt;

  return {
    title: tr('Склад'),
    body: [
      el('div', { className: 'biz-label', textContent: tr('Товар: {0} из {1} шт.', groupedVotes(b.stock), groupedVotes(storage)) }),
      bar(b.stock, storage, b.stock <= 0),
      facts(
        fact(tr('Закупочная цена:'), tr('{0} гол. за шт.', hundredths(plan.cost))),
        fact(tr('Хватит:'), hours === null ? tr('товар никто не берёт') : tr('на {0} ч.', hours), hours !== null && hours < 3),
      ),
      b.bankrupt ? null : row(el('span', { className: 'biz-label', textContent: tr('Закупить, шт.:') }), units, cost),
      room === 0 ? el('p', { className: 'hint', textContent: tr('Склад полон.') }) : null,
    ],
    actions: [restock],
  };
}

function tillPanel(b, plan) {
  const till = plan.till * b.level;
  const tax = Math.ceil((Math.floor(Math.max(b.cash, 0) / 100) * state.settings.tax) / 100);
  const amount = numberField(b.inDebt ? Math.ceil(-b.cash / 100) : 10, 1, 1e12);

  const actions = [];
  if (b.cash >= 100 && !b.bankrupt) {
    actions.push(button(tr('Забрать выручку'), () => act(
      b.kind, () => api.runBusiness(b.kind, 'collect'), (d) => tr('Из кассы забрано {0}, налог — {1}.', votesAmount(d.got), votesAmount(d.tax)),
    )));
  }
  if (!b.bankrupt) {
    actions.push(button(b.inDebt ? tr('Погасить долг') : tr('Пополнить кассу'), () => act(
      b.kind, () => api.runBusiness(b.kind, 'deposit', { amount: Number(amount.value) || 0 }), (d) => tr('В кассу положено {0}.', votesAmount(d.spent)),
    ), { secondary: b.cash >= 100 }));
  }

  return {
    title: tr('Касса'),
    body: [
      b.inDebt
        ? el('div', { className: 'biz-warn', textContent: tr('Долг: {0} гол. Разорение — с {1}.', hundredths(-b.cash), groupedVotes(plan.debt * b.level)) })
        : el('div', { className: 'biz-label', textContent: tr('В кассе: {0} из {1} гол.', hundredths(b.cash), groupedVotes(till)) }),
      b.inDebt ? bar(-b.cash, plan.debt * b.level * 100, true) : bar(b.cash, till * 100),
      el('p', { className: 'hint', textContent: b.inDebt
        ? tr('Зарплаты и аренда идут из кассы всегда. Положите в неё голоса, пока долг не разорил бизнес.')
        : tr('Выручка сверх того, что вмещает касса, пропадает — забирайте её. Налог — {0} %{1}.', state.settings.tax, b.cash >= 100 ? tr(', сейчас — {0}', votesAmount(tax)) : '') }),
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
      el('p', { className: 'hint', textContent: tr('Кампании складываются: каждая продлевает рекламу.') }),
    ],
    actions: b.bankrupt ? [] : [button(tr('Реклама за {0}', votesAmount(plan.adPrice)), () => act(
      b.kind, () => api.runBusiness(b.kind, 'advertise'), () => tr('Реклама запущена: +{0} % покупателей на {1} ч.', plan.adBoost, plan.adHours),
    ))],
  };
}

function lotPanel(plan) {
  const name = businessName(plan.kind);

  return {
    title: tr('Купить: {0}', name),
    body: [
      el('div', { className: 'biz-preview' }, picture(buildings[plan.kind])),
      facts(
        fact(tr('Цена:'), votesAmount(plan.price)),
        fact(tr('Уровней:'), String(plan.levels)),
        fact(tr('Покупателей в час:'), tr('{0} на уровень', plan.demand)),
        fact(tr('Товар:'), tr('закупка {0}, обычная цена {1} гол. за шт.', hundredths(plan.cost), hundredths(plan.fair))),
        fact(tr('Расходы в час:'), tr('аренда {0}, зарплата {1} каждому', hundredths(plan.rent), hundredths(plan.wage))),
      ),
      el('p', { className: 'hint', textContent: tr('После покупки закупите товар на складе и наймите работников — без них продаж нет.') }),
    ],
    actions: [button(tr('Купить за {0}', votesAmount(plan.price)), () => act(
      plan.kind, () => api.buyBusiness(plan.kind), () => tr('Вы купили бизнес «{0}». Закупите товар — без него продаж нет.', name),
    ))],
  };
}

const panels = { building: buildingPanel, staff: staffPanel, storage: storagePanel, till: tillPanel, ad: adPanel };

// fillPanel fills the open box with the part it shows, as it is now.
function fillPanel(message = '') {
  const plan = state.settings.plans.find((p) => p.kind === panel.kind);
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
    button(tr('Продать'), () => act(b.kind, () => api.runBusiness(b.kind, 'sell'), (d) => tr('Бизнес «{0}» продан за {1}.', name, votesAmount(d.got)))),
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
