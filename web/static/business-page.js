// "Бизнес" (/business), a tab of "Работа": a page buys businesses for votes
// and runs them — goods, their price, workers, repairs, advertising,
// levels. A business works by itself hour after hour; its cash piles up in
// the till and is taken out for votes. Run badly — dear goods, no goods or
// no workers while the wages and the rent go on — it runs into debt and
// goes bankrupt. The server counts the hours; the page only shows what it
// said. The administrators tune the businesses in the admin panel
// (/admin/business).

import { api, currentUser, session, started } from './api.js';
import { businessLevelCost, businessName, formatDateTime, groupedVotes, hundredths, plural, repairCost, restockCost, votesAmount, votesFontSize } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const answered = session.get() ? started(api.businesses()) : null;

let busy = false;

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

// act runs an action and shows the businesses it left.
async function act(run, text) {
  if (busy) return;
  busy = true;
  hideError($('error'));

  try {
    const done = await run();
    if (done.balance > 0 || done.spent > 0 || done.got > 0) showBalance(done.balance);
    said(text(done));
    show(done.businesses);
  } catch (e) {
    showError($('error'), e);
  } finally {
    busy = false;
  }
}

function fact(label, value, warn = false) {
  return el('tr', {},
    el('td', { className: 'business-label', textContent: label }),
    el('td', { className: warn ? 'business-warn' : '', textContent: value }),
  );
}

function numberField(value, min, max) {
  return el('input', { type: 'number', className: 'business-number', value: String(value), min: String(min), max: String(max), step: '1' });
}

function businessBlock(b, plan) {
  const kind = b.kind;
  const name = businessName(kind);
  const storage = plan.storage * b.level;
  const staffMost = plan.staff * b.level;
  const till = plan.till * b.level;
  const ad = b.adUntil && new Date(b.adUntil) > new Date();
  const profit = b.income - b.costs;

  const facts = el('table', { className: 'business-facts' },
    fact(tr('Уровень:'), tr('{0} из {1}', b.level, plan.levels)),
    fact(tr('Товар:'), tr('{0} из {1} шт.', groupedVotes(b.stock), groupedVotes(storage)), b.stock <= 0),
    fact(tr('Цена товара:'), tr('{0} гол. за шт. (обычная — {1})', hundredths(b.price), hundredths(plan.fair)), b.price > plan.fair),
    fact(tr('Работники:'), tr('{0} из {1}', b.staff, staffMost), b.staff === 0),
    fact(tr('Состояние помещения:'), `${hundredths(b.condition)} %`, b.condition < 5000),
    b.inDebt
      ? fact(tr('Долг:'), tr('{0} гол. (разорение — с {1})', hundredths(-b.cash), groupedVotes(plan.debt * b.level)), true)
      : fact(tr('В кассе:'), tr('{0} гол. (вмещает {1})', hundredths(b.cash), groupedVotes(till))),
    fact(tr('Покупателей в час:'), tr('{0}, обслуживают {1}', b.customers.toFixed(1).replace('.', ','), b.served.toFixed(1).replace('.', ','))),
    fact(tr('За час:'), tr('выручка {0}, расходы {1} — итого {2} гол.', hundredths(b.income), hundredths(b.costs), hundredths(profit)), profit < 0),
    fact(tr('Реклама:'), ad ? tr('до {0}', formatDateTime(b.adUntil)) : tr('нет')),
    fact(tr('Вложено и выведено:'), tr('{0} / {1}', votesAmount(b.invested), votesAmount(b.collected))),
  );

  const block = el('div', { className: 'business-card' },
    el('div', { className: 'section-title', textContent: name }),
  );

  if (b.bankrupt) {
    const close = button(tr('Закрыть бизнес'), () => act(() => api.runBusiness(kind, 'sell'), () => tr('Бизнес «{0}» закрыт.', name)));
    block.append(
      el('p', { className: 'business-bankrupt', textContent: tr('Бизнес разорился: долги превысили допустимое. Его можно только закрыть — тогда его можно купить снова.') }),
      facts, el('div', { className: 'business-actions' }, close),
    );
    return block;
  }

  // Price and staff.
  const price = el('input', { type: 'text', className: 'business-number', value: hundredths(b.price), inputMode: 'decimal' });
  const staff = numberField(b.staff, 0, staffMost);
  const manage = button(tr('Сохранить'), () => {
    const cents = Math.round(Number(price.value.trim().replace(',', '.')) * 100) || 0;
    act(() => api.runBusiness(kind, 'manage', { price: cents, staff: Number(staff.value) || 0 }), () => tr('Цена и работники сохранены.'));
  });

  // Goods.
  const room = Math.max(storage - b.stock, 0);
  const units = numberField(room, 1, room || 1);
  const costHint = el('span', { className: 'hint' });
  const showCost = () => { costHint.textContent = tr('стоит {0}', votesAmount(restockCost(Number(units.value) || 0, plan.cost))); };
  units.addEventListener('input', showCost);
  showCost();
  const restock = button(tr('Закупить'), () => act(
    () => api.runBusiness(kind, 'restock', { amount: Number(units.value) || 0 }),
    (d) => tr('Товар закуплен за {0}.', votesAmount(d.spent)),
  ));
  restock.disabled = room === 0;

  const actions = el('div', { className: 'business-actions' });
  if (b.level < plan.levels) {
    actions.append(button(tr('Улучшить за {0}', votesAmount(businessLevelCost(b.level, plan.upgrade))), () => act(
      () => api.runBusiness(kind, 'upgrade'), () => tr('Бизнес «{0}» улучшен.', name),
    )));
  }
  if (b.condition < 10000) {
    actions.append(button(tr('Ремонт за {0}', votesAmount(repairCost(b.condition, plan.repair))), () => act(
      () => api.runBusiness(kind, 'repair'), () => tr('Помещение отремонтировано.'),
    )));
  }
  actions.append(button(tr('Реклама за {0}', votesAmount(plan.adPrice)), () => act(
    () => api.runBusiness(kind, 'advertise'), () => tr('Реклама запущена: +{0} % покупателей на {1} ч.', plan.adBoost, plan.adHours),
  )));
  if (b.cash >= 100) {
    actions.append(button(tr('Забрать выручку'), () => act(
      () => api.runBusiness(kind, 'collect'), (d) => tr('Из кассы забрано {0}, налог — {1}.', votesAmount(d.got), votesAmount(d.tax)),
    )));
  }
  actions.append(button(b.inDebt ? tr('Погасить долг') : tr('Пополнить кассу'), () => deposit(b, name)));
  if (!b.inDebt) actions.append(button(tr('Продать'), () => sell(b, name)));

  block.append(
    facts,
    el('div', { className: 'business-row' },
      el('span', { className: 'business-label', textContent: tr('Цена, гол. за шт.:') }), price,
      el('span', { className: 'business-label', textContent: tr('Работники:') }), staff, manage,
    ),
    el('div', { className: 'business-row' },
      el('span', { className: 'business-label', textContent: tr('Товар, шт.:') }), units, restock, costHint,
    ),
    actions,
  );

  return block;
}

function deposit(b, name) {
  const modal = openModal(b.inDebt ? tr('Погасить долг') : tr('Пополнить кассу'));
  const amount = numberField(b.inDebt ? Math.ceil(-b.cash / 100) : 10, 1, 1e12);
  modal.body.append(el('p', { textContent: tr('Сколько голосов положить в кассу бизнеса «{0}»?', name) }), amount);
  modal.actions.append(
    button(tr('Положить'), () => {
      modal.close();
      act(() => api.runBusiness(b.kind, 'deposit', { amount: Number(amount.value) || 0 }), (d) => tr('В кассу положено {0}.', votesAmount(d.spent)));
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

function sell(b, name) {
  const modal = openModal(tr('Продать бизнес'));
  modal.body.append(el('p', { textContent: tr('Продать бизнес «{0}» сайту за {1}? Касса уходит вместе с ним.', name, votesAmount(b.resale)) }));
  modal.actions.append(
    button(tr('Продать'), () => {
      modal.close();
      act(() => api.runBusiness(b.kind, 'sell'), (d) => tr('Бизнес «{0}» продан за {1}.', name, votesAmount(d.got)));
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

function shopItem(plan) {
  const name = businessName(plan.kind);
  return el('div', { className: 'business-offer' },
    el('b', { textContent: name }),
    el('div', { className: 'hint', textContent: tr('до {0} уровней · {1} покупателей в час на уровень · товар — {2} гол. за шт.', plan.levels, plan.demand, hundredths(plan.cost)) }),
    button(tr('Купить за {0}', votesAmount(plan.price)), () => act(
      () => api.buyBusiness(plan.kind), () => tr('Вы купили бизнес «{0}». Закупите товар — без него продаж нет.', name),
    )),
  );
}

function show(state) {
  const plans = new Map(state.settings.plans.map((p) => [p.kind, p]));
  const owned = new Set(state.businesses.map((b) => b.kind));

  $('owned').replaceChildren(...state.businesses.map((b) => businessBlock(b, plans.get(b.kind))));
  const offers = state.settings.plans.filter((p) => !owned.has(p.kind));
  $('shop').replaceChildren(...offers.map(shopItem));
  $('shop-block').hidden = offers.length === 0;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

$('business').hidden = false;
if (!me || !answered) {
  $('guest').hidden = false;
} else {
  showBalance(me.votes ?? 0);
  try {
    show(await answered);
  } catch (e) {
    showError($('error'), e);
  }
}
