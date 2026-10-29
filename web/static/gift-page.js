// A collectible gift at /gift/<id>, as Telegram's: the card with its
// model, backdrop and symbol and how rare each is, who has it, its sale or
// auction with the bid form, the history of who had it and, for its
// owner, "Продать", "Аукцион" and "Передать". openvk has nothing like it.

import { api, currentUser, started } from './api.js';
import { balanceAmount, auctionLeft, collectibleName, formatDateTime, giftBackdrops, giftHistoryLine, giftIncome, giftModels, giftSymbols, GIFT_FEE_PERCENT, label, PRIME_GIFT_FEE_PERCENT, traitChance, transferNickname, votesCount } from './labels.js';
import { collectibleCard, giftsPath } from './gifts.js';
import { $, el, fullName, hideError, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const id = decodeURIComponent(location.pathname.split('/')[2] ?? '');
const error = $('error');
// The gift and the catalogue — how rare each trait is — are asked with
// the page.
const read = started(api.collectible(id));
const catalog = started(api.giftCatalog()).catch(() => ({}));

const AUCTION_HOURS = [1, 3, 6, 12, 24, 48, 72, 168];

// person is a link to a page with its mark, or what stands for nobody.
function person(p, nobody) {
  if (!p) return el('span', { textContent: nobody });

  return el('span', {}, el('a', { href: pagePath(p), textContent: fullName(p) }), verifiedMark(p));
}

function row(name, ...value) {
  return el('tr', {}, el('th', { textContent: name }), el('td', {}, ...value));
}

// trait is a trait with how often it falls: "оникс · 10 %".
function trait(list, weights, name) {
  const chance = traitChance(weights, name);

  return el('span', {}, label(list, name) || name, chance ? el('span', { className: 'hint', textContent: ` · ${chance}` }) : null);
}

function showTraits(c, cat) {
  const { gift } = c;

  $('traits').replaceChildren(
    row(tr('Владелец:'), c.site ? el('b', { textContent: 'Freenet' }) : person(c.owner, tr('страница удалена'))),
    row(tr('Модель:'), trait(giftModels, cat.models, gift.model)),
    row(tr('Фон:'), trait(giftBackdrops, cat.backdrops, gift.backdrop)),
    row(tr('Узор:'), trait(giftSymbols, cat.symbols, gift.symbol)),
    row(tr('Номер:'), `#${gift.number}`),
  );
}

// reload shows the gift again after a deal.
async function reload(me, notice) {
  try {
    show(await api.collectible(id), me, await catalog);
    if (notice) {
      $('notice').textContent = notice;
      $('notice').hidden = false;
    }
  } catch (e) {
    showError(error, e);
  }
}

// act runs a deal: the button is off meanwhile and an error is shown.
async function act(control, place, run) {
  hideError(place);
  control.disabled = true;

  try {
    await run();
  } catch (e) {
    showError(place, e.message === 'not enough votes' ? tr('У вас недостаточно голосов.') : e);
    control.disabled = false;
  }
}

// buy asks before buying at the price shown.
function buy(c, me) {
  const price = c.gift.salePrice;
  const modal = openModal(tr('Купить подарок'));
  const place = el('div', { className: 'error', hidden: true });
  const go = button(tr('Купить за {0}', votesCount(price)), () => act(go, place, async () => {
    await api.buyGift(id, price);
    modal.close();
    await reload(me, tr('Подарок теперь ваш.'));
  }));

  modal.body.append(el('p', { textContent: tr('{0} перейдёт к вам за {1}. У вас {2}.', collectibleName(c.gift), votesCount(price), balanceAmount(me.votes ?? 0)) }), place);
  modal.actions.append(go, button(tr('Отмена'), modal.close, { secondary: true }));
}

// auctionPart is the auction: the bids and, for others, the bid form.
function auctionPart(c, me) {
  const a = c.auction;
  const rows = [
    row(tr('Продавец:'), a.site ? el('b', { textContent: 'Freenet' }) : person(a.seller, tr('страница удалена'))),
    row(tr('Начальная ставка:'), votesCount(a.start)),
    row(tr('Текущая ставка:'), a.bids ? el('span', {}, el('b', { textContent: votesCount(a.topBid) }), ' — ', person(a.bidder, tr('страница удалена'))) : tr('ставок пока нет')),
    row(tr('Ставок:'), String(a.bids)),
    row(tr('До конца:'), `${auctionLeft(a.endsAt)} (${formatDateTime(a.endsAt)})`),
  ];
  const part = [el('div', { className: 'section-title', textContent: tr('Аукцион') }), el('table', { className: 'collectible-traits' }, el('tbody', {}, ...rows))];

  if (a.mine) part.push(el('p', { className: 'collectible-lead', textContent: tr('Ваша ставка — самая высокая.') }));
  if (!me || c.mine) return part;

  const amount = el('input', { type: 'number', min: a.minBid, max: 1000000, value: String(a.minBid), required: true });
  const place = el('div', { className: 'error', hidden: true });
  const go = button(tr('Сделать ставку'), null, { type: 'submit' });
  const form = el('form', { className: 'collectible-form' },
    el('label', {}, tr('Ваша ставка, голосов: '), amount), go,
    el('div', { className: 'hint', textContent: tr('Не меньше {0}. Голоса ставки держатся, пока вас не перебьют, — тогда они вернутся. Ставка в последние 5 минут продлевает аукцион на 5 минут.', votesCount(a.minBid)) }),
    place);

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    act(go, place, async () => {
      await api.bidGift(id, Number(amount.value));
      await reload(me, tr('Ставка сделана.'));
    });
  });
  part.push(form);

  return part;
}

function showDeal(c, me) {
  const deal = [];

  if (c.gift.salePrice) {
    deal.push(el('div', { className: 'collectible-sale' }, tr('Продаётся за '), el('b', { textContent: votesCount(c.gift.salePrice) })));
    if (me && !c.mine) deal.push(button(tr('Купить'), () => buy(c, me)));
  }
  if (c.auction) deal.push(...auctionPart(c, me));

  $('deal').replaceChildren(...deal);
}

// form is one of the owner's forms: a heading, fields and a button.
function form(title, fields, text, hint, run) {
  const place = el('div', { className: 'error', hidden: true });
  const go = button(text, null, { type: 'submit' });
  const node = el('form', { className: 'collectible-form' }, el('h4', { textContent: title }), ...fields, go, hint, place);

  node.addEventListener('submit', (event) => {
    event.preventDefault();
    act(go, place, run);
  });

  return node;
}

// showActions are the owner's: a gift on sale or at an auction is taken
// off first, the rest is sold, auctioned or given away.
function showActions(c, me, cat) {
  const forms = $('action-forms');
  const fee = me.prime ? PRIME_GIFT_FEE_PERCENT : GIFT_FEE_PERCENT;

  if (c.gift.salePrice) {
    forms.replaceChildren(form(tr('Продажа'), [], tr('Снять с продажи'),
      el('div', { className: 'hint', textContent: tr('Когда подарок купят, вы получите {0}.', votesCount(giftIncome(c.gift.salePrice, me.prime))) }),
      async () => {
        await api.unsellGift(id);
        await reload(me, tr('Подарок снят с продажи.'));
      }));
  } else if (c.auction) {
    forms.replaceChildren(c.auction.bids
      ? el('p', { textContent: tr('На аукционе уже есть ставки — отменить его нельзя. Когда он закончится, подарок достанется последней ставке, а вы получите её за вычетом комиссии {0} %.', String(fee).replace('.', ',')) })
      : form(tr('Аукцион'), [], tr('Отменить аукцион'), null, async () => {
        await api.cancelGiftAuction(id);
        await reload(me, tr('Аукцион отменён.'));
      }));
  } else {
    const price = el('input', { type: 'number', min: 1, max: 1000000, required: true });
    const income = el('div', { className: 'hint', textContent: tr('Сайт берёт комиссию {0} % с продажи.', String(fee).replace('.', ',')) });
    price.addEventListener('input', () => {
      income.textContent = price.value > 0
        ? tr('Вы получите {0} — комиссия {1} %.', votesCount(giftIncome(Number(price.value), me.prime)), String(fee).replace('.', ','))
        : tr('Сайт берёт комиссию {0} % с продажи.', String(fee).replace('.', ','));
    });

    const start = el('input', { type: 'number', min: 1, max: 1000000, required: true });
    const hours = el('select', {}, ...AUCTION_HOURS.map((h) => el('option', { value: String(h), textContent: tr('{0} ч', h), selected: h === 24 })));
    const nickname = el('input', { type: 'text', required: true, placeholder: tr('никнейм или ссылка на страницу') });

    forms.replaceChildren(
      form(tr('Продать'), [el('label', {}, tr('Цена, голосов: '), price)], tr('Выставить на продажу'), income, async () => {
        await api.sellGift(id, Number(price.value));
        await reload(me, tr('Подарок выставлен на продажу.'));
      }),
      form(tr('Аукцион'), [el('label', {}, tr('Начальная ставка: '), start), el('label', {}, tr(' Длительность: '), hours)], tr('Начать аукцион'),
        el('div', { className: 'hint', textContent: tr('Каждая ставка больше предыдущей на 5 %. Без ставок подарок останется у вас.') }), async () => {
          await api.auctionGift(id, Number(start.value), Number(hours.value));
          await reload(me, tr('Аукцион начался.'));
        }),
      form(tr('Передать'), [el('label', {}, tr('Кому: '), nickname)], tr('Передать за {0}', votesCount(cat.transferPrice ?? 0)), null, async () => {
        const to = transferNickname(nickname.value);
        await api.transferGift(id, to);
        location.assign(giftsPath({ nickname: to }));
      }),
    );
  }

  $('actions').hidden = false;
}

function showHistory(c) {
  $('history-title').hidden = c.history.length === 0;
  $('history').replaceChildren(...c.history.map((h) => el('div', { className: 'collectible-history' },
    el('span', { className: 'post-date', textContent: formatDateTime(h.at) }),
    el('b', { textContent: giftHistoryLine(h.kind) }),
    h.from || h.kind === 'mint' ? el('span', {}, tr(' от '), person(h.from, 'Freenet')) : null,
    h.to ? el('span', {}, tr(' к '), person(h.to, '')) : null,
    h.price ? el('span', { className: 'hint', textContent: tr(' за {0}', votesCount(h.price)) }) : null)));
}

function show(c, me, cat) {
  const name = collectibleName(c.gift);

  document.title = `${name} | Freenet`;
  $('title').replaceChildren(...(c.owner && !c.site
    ? [el('a', { href: pagePath(c.owner), textContent: fullName(c.owner) }), ' » ', el('a', { href: giftsPath(c.owner), textContent: tr('Подарки') }), ` » ${name}`]
    : [name]));
  if (c.owner && !c.site) {
    $('tab-owner').textContent = c.mine ? tr('Мои подарки') : tr('Подарки');
    $('tab-owner').href = giftsPath(c.owner);
    $('tab-owner').hidden = false;
  }
  $('tabs').hidden = false;

  $('card').replaceChildren(collectibleCard(c.gift, 'large'));
  $('name').textContent = name;
  showTraits(c, cat);
  showDeal(c, me);
  showHistory(c);
  if (c.mine) showActions(c, me, cat);
  else $('actions').hidden = true;
  $('gift').hidden = false;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  show(await read, me, await catalog);
} catch (e) {
  $('title').textContent = e.status === 404 ? tr('Подарок не найден') : tr('Подарок');
  showError(error, e.status === 404 ? tr('Такого подарка нет, или владелец скрыл свои подарки.') : e);
}
