// "Мои Голоса" at /votes, a small bank of votes in the look of the games of
// money: "Счёт" (the card, the balance, the daily bonus, what came in and
// went out in 30 days and the statement, all of it or ?flow=in|out), "Карта"
// (?act=card: the virtual card and its covers), "Перевести" (?act=transfer,
// ?to=<nickname> names the recipient; the fee is shown as the votes are
// typed), "Пополнить" (?act=top-up: a voucher), "Получить" (?act=receive: the
// link that opens a transfer to you), "Рейтинг" (?act=rating: openvk's
// "Повысить рейтинг", for votes), "Вклад" (?act=deposit; the old ?act=bank
// too) and "Кредит" (?act=loan).

import { api, currentUser, started } from './api.js';
import { balanceVotes, balanceWord, balanceAmount, MAX_RATING, RATING_PRICE, bankRate, bankVotes, cardCovers, cardNumber, grownDeposit, groupedVotes, TRANSFER_FEE_PERCENT, formatDateTime, isEndless, plural, rubles, transferFee, signedVotes, transferNickname, transferURL, voteOperation, voteRate, votesAmount, votesFontSize, voucherGave } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { chips, meter } from './controls.js';
import { countTo, flash, floatText, gameIcon, helpButton } from './game.js';
import { notify, toast } from './modal.js';
import { coins, dealTick, fanfare } from './sounds.js';
import { tr } from './i18n.js';
import { isNickname, recipientPicker } from './recipient.js';

const params = new URLSearchParams(location.search);
const tabs = { card: initCard, transfer: initTransfer, 'top-up': initTopUp, receive: initReceive, rating: initRating, deposit: initBank, loan: initBank };
const asked = params.get('act') === 'bank' ? 'deposit' : params.get('act');
const act = Object.hasOwn(tabs, asked) ? asked : 'account';
const flow = ['in', 'out'].includes(params.get('flow')) ? params.get('flow') : '';
const error = $('error');

// The statement and the rate are asked with the page, before who is
// signed in is known.
const statement = act === 'account' ? started(api.voteStatement('', flow)) : null;
const rate = act === 'account' ? api.voteRate().then((r) => r.kopecks, () => 0) : Promise.resolve(0);
const bonus = act === 'account' ? started(api.bonus()) : null;
const bankState = started(api.bank());
const cardState = ['account', 'card'].includes(act) ? started(api.bankCard()).then((r) => r.card) : Promise.resolve(null);
const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
const BONUS_DAYS = 7;
// How far the page looks ahead at a deposit.
const FORECAST_DAYS = 30;

// say shows the notice above the tab: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  notify($('notice'), title);
}

let shownVotes = null;
let rated = 0;

// showBalance runs the balance up to votes wherever the page shows it: the
// strip on top, the big number and the card.
function showBalance(votes) {
  const from = shownVotes ?? votes;
  const whole = isEndless(from) || isEndless(votes);
  shownVotes = votes;

  countTo($('hud-votes'), whole ? votes : from, votes, balanceVotes);
  countTo($('votes'), whole ? votes : from, votes, balanceVotes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = balanceWord(votes);
  // The endless balance of an administrator is worth no sum.
  $('votes-rubles').textContent = rated && !isEndless(votes) ? `≈ ${rubles(votes * rated)}` : '';
  for (const sum of document.querySelectorAll('.bank-card-sum')) sum.textContent = balanceAmount(votes);
}

// showRating is the rating of the page on the strip on top.
function showRating(me) {
  $('hud-rating').textContent = `${me.completeness ?? me.rating ?? 0} %`;
}

// reward is the answer of the page to money moved: a number flying up from
// what was pressed, a flash and the sound of coins.
function reward(node, text, tone = 'gold') {
  floatText(node, text, tone);
  flash(node.closest('.game-window') ?? node, tone);
  coins();
}

// quick fills input from a row of chips [label, amount], the page hearing
// it as if typed.
function quick(place, input, list) {
  $(place).replaceChildren(chips(list.map(([label, amount]) => [label, () => {
    input.value = String(amount());
    input.dispatchEvent(new Event('input', { bubbles: true }));
    dealTick();
  }])));
}

const help = (place, text) => $(place).replaceChildren(helpButton(text));

const coverOf = (id) => cardCovers.find((cover) => cover.id === id) ?? cardCovers[0];

// cardSince is when a card was issued, as a card writes it: 10/26.
function cardSince(card) {
  const at = new Date(card.issuedAt);
  return `${String(at.getMonth() + 1).padStart(2, '0')}/${String(at.getFullYear()).slice(2)}`;
}

// cardNode is the card with its two sides; a click turns it over.
function cardNode(card, me) {
  const cover = coverOf(card.cover);
  const name = fullName(me).toUpperCase();
  const art = () => el('img', { className: 'bank-card-art', src: cover.src, alt: '' });
  const node = el('button', { type: 'button', className: `bank-card${cover.dark ? ' ink-dark' : ''}`, title: tr('Перевернуть'), ariaLabel: tr('Перевернуть') },
    el('span', { className: 'bank-card-inner' },
      el('span', { className: 'bank-card-face' }, art(),
        el('span', { className: 'bank-card-brand', textContent: 'freenet' }),
        el('span', { className: 'bank-card-chip' }),
        el('span', { className: 'bank-card-number', textContent: cardNumber(card.number) }),
        el('span', { className: 'bank-card-since', textContent: tr('с {0}', cardSince(card)) }),
        el('span', { className: 'bank-card-holder', textContent: name }),
        el('span', { className: 'bank-card-sum', textContent: balanceAmount(me.votes ?? 0) })),
      el('span', { className: 'bank-card-face bank-card-back' }, art(),
        el('span', { className: 'bank-card-strip' }),
        el('span', { className: 'bank-card-sign', textContent: name }),
        el('span', { className: 'bank-card-note', textContent: tr('Голоса Freenet. Карта показывает ваш счёт и не хранит денег отдельно.') }))));

  node.addEventListener('click', () => turnCard(node));
  return node;
}

function turnCard(node) {
  node?.classList.toggle('flipped');
  dealTick();
}

// rollNumber spins the digits of a new card and stops them a group at a time.
function rollNumber(node, number, done) {
  if (still) {
    done();
    return;
  }
  const from = performance.now();
  const timer = setInterval(() => {
    const fixed = Math.floor((performance.now() - from) / 250);
    const digits = [...number].map((digit, i) => (i < fixed * 4 ? digit : String(Math.floor(Math.random() * 10))));
    node.textContent = cardNumber(digits.join(''));
    dealTick(fixed % 2 === 0);
    if (fixed >= 4) {
      clearInterval(timer);
      done();
    }
  }, 70);
}

// blankNode stands for a card not issued yet, with the button that issues it.
function blankNode(me, place) {
  const issue = el('button', { type: 'button', className: 'button game-go', textContent: tr('Оформить карту') });

  issue.addEventListener('click', async () => {
    hideError(error);
    issue.disabled = true;
    try {
      const card = (await api.issueBankCard()).card;
      showCard(card, me, place);
      const node = place.querySelector('.bank-card');
      rollNumber(node.querySelector('.bank-card-number'), card.number, () => {
        fanfare();
        node.classList.add('shine');
        floatText(node, tr('Карта ваша!'), 'gold');
      });
    } catch (e) {
      showError(error, e);
      issue.disabled = false;
    }
  });

  return el('div', { className: 'bank-card blank' },
    el('div', { className: 'bank-card-blank' },
      gameIcon('/static/icons/game/card.svg', 24),
      el('b', { textContent: tr('Виртуальная карта Freenet') }),
      el('span', { textContent: tr('Свой номер и обложка на выбор. Бесплатно.') }),
      issue));
}

// showCard puts the card in place and tells the page it changed.
function showCard(card, me, place) {
  place.replaceChildren(card ? cardNode(card, me) : blankNode(me, place));
  onCard(card);
}

let onCard = () => {};

// operationRow is a line of the statement: when, what and with whom, the
// votes it brought or took and the balance after it.
function operationRow(op, me) {
  const { title, role } = voteOperation(op, me.id);
  const about = [el('div', { className: 'bank-op-title', textContent: title })];

  if (role && op.peer) {
    about.push(el('div', { className: 'bank-op-peer' }, `${role} `,
      el('a', { href: pagePath(op.peer), textContent: fullName(op.peer) }), verifiedMark(op.peer)));
  }
  if (op.kind === 'transfer' && op.note) {
    about.push(el('div', { className: 'bank-op-note', textContent: `«${op.note}»` }));
  }
  about.push(el('div', { className: 'bank-op-date', textContent: formatDateTime(op.createdAt) }));

  return el('div', { className: 'bank-op' },
    gameIcon(op.amount < 0 ? '/static/icons/game/out.svg' : '/static/icons/game/in.svg', 24),
    el('div', { className: 'bank-op-about' }, ...about),
    el('div', { className: 'bank-op-sum' },
      el('b', { className: op.amount < 0 ? 'bank-out' : 'bank-in', textContent: signedVotes(op.amount) }),
      el('div', { className: 'bank-op-balance', textContent: tr('остаток {0}', balanceAmount(op.balance)) })));
}

// showBonus is the daily bonus: a row of seven days, a button while it waits
// for the caller today, else when the next one comes or why there is none.
function showBonus(state, me) {
  const claim = $('bonus-claim');
  const days = state.eligible ? Math.min(state.streak, BONUS_DAYS) : 0;

  $('bonus-pips').replaceChildren(...Array.from({ length: BONUS_DAYS }, (_, i) => {
    const kind = i < days ? ' done' : i === days && state.eligible && !state.taken ? ' next' : '';
    return el('span', { className: `bank-pip${kind}`, textContent: `×${i + 1}` });
  }));
  $('bonus-text').textContent = !state.eligible
    ? tr('Бонус получают страницы старше недели с заполненной анкетой.')
    : state.taken
      ? tr('Сегодня вы его уже получили. Следующий — завтра.')
      : tr('Сегодня — {0}.', votesAmount(state.votes));
  $('bonus-streak').textContent = state.eligible && state.streak > 0
    ? tr('Дней подряд: {0} из {1}.', days, BONUS_DAYS)
    : '';
  claim.textContent = tr('Получить {0}', votesAmount(state.votes));
  claim.hidden = !state.eligible || state.taken;
  $('bonus').hidden = false;

  claim.onclick = async () => {
    hideError(error);
    claim.disabled = true;
    try {
      const got = await api.claimBonus();
      me.votes = got.balance;
      reward(claim, `+${groupedVotes(got.votes)}`);
      showBalance(got.balance);
      showBonus({ ...state, taken: true, streak: state.streak + 1 }, me);
    } catch (e) {
      showError(error, e);
    } finally {
      claim.disabled = false;
    }
  };
}

// showFlows is the strip of what came in against what went out.
function showFlows(income, expense) {
  if (!income && !expense) return;
  $('flow-bar-in').style.width = `${(income / (income + expense)) * 100}%`;
  $('flow-bar').hidden = false;
}

// initCard is the card with its tools and the covers to choose from: a
// cover goes on at once, and comes off again if the server refuses it.
async function initCard(me) {
  help('card-help', [
    tr('У каждой карты свой номер из 16 цифр: он начинается с 1919 и больше ни у кого не повторится.'),
    tr('Карта бесплатна и одна на страницу. Она показывает ваш счёт, а денег отдельно не хранит.'),
  ]);

  const place = $('card-big');
  let card = null;
  const showCovers = () => {
    $('covers-hint').textContent = card ? tr('Нажмите на обложку, и она сразу окажется на карте.') : tr('Сначала оформите карту.');
    $('card-tools').hidden = !card;
    $('card-covers').replaceChildren(...cardCovers.map((cover) => {
      const node = el('button', {
        type: 'button', disabled: !card, className: `bank-cover${card?.cover === cover.id ? ' active' : ''}`,
        onclick: () => choose(cover),
      }, el('img', { src: cover.src, alt: '' }), el('span', { textContent: cover.title }));
      return node;
    }));
  };
  onCard = (next) => {
    card = next;
    showCovers();
  };

  const choose = async (cover) => {
    if (cover.id === card.cover) return;
    hideError(error);
    const before = card;
    showCard({ ...card, cover: cover.id }, me, place);
    place.querySelector('.bank-card').classList.add('shine');
    dealTick();
    try {
      card = (await api.setBankCardCover(cover.id)).card;
    } catch (e) {
      showCard(before, me, place);
      showError(error, e);
    }
  };

  $('card-flip').addEventListener('click', () => turnCard(place.querySelector('.bank-card')));
  $('card-copy').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(card.number);
      toast(tr('Номер скопирован'));
    } catch {
      toast(cardNumber(card.number));
    }
  });

  try {
    showCard(await cardState, me, place);
  } catch (e) {
    showError(error, e);
  }
}

async function initAccount(me) {
  help('bonus-help', tr('Заходите каждый день: бонус растёт до семикратного. Пропущенный день начинает счёт сначала.'));
  cardState.then((card) => showCard(card, me, $('card-home')), (e) => showError(error, e));
  bonus.then((state) => showBonus(state, me), () => {});
  $('flow-' + (flow || 'all')).classList.add('active');

  rate.then((kopecks) => {
    if (!kopecks) return;
    rated = kopecks;
    $('vote-rate').textContent = tr('Курс: {0}.', voteRate(kopecks));
    $('vote-rate').hidden = false;
    showBalance(me.votes ?? 0);
  });

  let next = '';
  const show = (page) => {
    $('operations').append(...page.operations.map((op) => operationRow(op, me)));
    next = page.nextCursor;
    $('more').hidden = !next;
  };

  try {
    const page = await statement;
    $('income').textContent = signedVotes(page.income);
    $('expense').textContent = signedVotes(-page.expense);
    $('operations-empty').hidden = page.operations.length > 0;
    showFlows(page.income, page.expense);
    show(page);
  } catch (e) {
    showError(error, e);
  }

  $('more').addEventListener('click', async () => {
    $('more').disabled = true;
    try {
      show(await api.voteStatement(next, flow));
    } catch (e) {
      showError(error, e);
    } finally {
      $('more').disabled = false;
    }
  });
}

// initTransfer is openvk's "Передача голосов", as a page of its own, the
// recipient picked as recipient.js has it.
function initTransfer(me) {
  const to = $('transfer-to');
  $('transfer-balance').textContent = balanceAmount(me.votes ?? 0);
  help('transfer-help', tr('Голоса приходят сразу, вернуть их нельзя. Комиссия — процент от суммы, не меньше 1 голоса: она списывается сверх перевода, а сколько она, форма скажет, пока вы печатаете.'));

  const picker = recipientPicker({
    to, me, people: $('transfer-people'), shown: $('transfer-recipient'),
    picked: () => $('transfer-votes').focus(),
  });

  // The fee comes on top of the votes, so the form says it beforehand: at
  // the percent the administrators set, none with Prime.
  let percent = TRANSFER_FEE_PERCENT;
  api.treasury().then((t) => { percent = me?.prime ? 0 : t.transferFee ?? percent; }, () => {});
  $('transfer-votes').addEventListener('input', () => {
    const votes = Number($('transfer-votes').value);
    const fee = Number.isSafeInteger(votes) ? transferFee(votes, percent) : 0;
    $('transfer-fee').textContent = fee ? tr('Комиссия {0}, всего спишется {1}.', votesAmount(fee), votesAmount(votes + fee)) : '';
    $('transfer-fee').hidden = !fee;
  });

  // "Всё" leaves room for the fee, which comes on top.
  const all = () => {
    const balance = me.votes ?? 0;
    let votes = Math.min(Math.floor(balance / (1 + percent / 100)), 999_999_999);
    while (votes > 0 && votes + transferFee(votes, percent) > balance) votes--;
    return votes;
  };
  quick('transfer-chips', $('transfer-votes'), [['10', () => 10], ['100', () => 100], ['1 000', () => 1000], [tr('всё'), all]]);

  to.value = params.get('to') ?? '';
  if (to.value) {
    picker.lookUp();
    $('transfer-votes').focus();
  } else {
    to.focus();
  }

  $('transfer').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = $('transfer').querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const recipient = picker.found() ?? await picker.lookUp();
      if (!recipient) {
        showError(error, isNickname(transferNickname(to.value)) ? tr('Получатель не найден.') : tr('Выберите получателя из списка.'));
        return;
      }

      const sent = Number($('transfer-votes').value);
      const left = await api.transferVotes(recipient.nickname, sent, $('transfer-note').value.trim());

      me.votes = left.votes;
      $('transfer-balance').textContent = balanceAmount(left.votes);
      reward(submit, `−${groupedVotes(sent)}`, 'bad');
      showBalance(left.votes);
      $('transfer-votes').value = '';
      $('transfer-note').value = '';
      $('transfer-fee').hidden = true;
      say(tr('Голоса переданы'), tr('Вы успешно передали '), el('b', { textContent: votesAmount(sent) }), ' — ',
        el('a', { href: pagePath(recipient), textContent: fullName(recipient) }), '. ',
        tr('Комиссия: {0}.', votesAmount(left.fee)), ' ',
        el('a', { href: '/votes', textContent: tr('На вашем счету {0}.', balanceAmount(left.votes)) }));
    } catch (e) {
      showError(error, e.status === 404 ? tr('Получатель не найден.') : e);
    } finally {
      submit.disabled = false;
    }
  });
}

// initRating is openvk's "Повысить рейтинг": percents of rating for
// votes, the price shown as they are typed.
function initRating(me) {
  const percent = $('rating-percent');
  $('rating-balance').textContent = balanceAmount(me.votes ?? 0);
  // The rating of the page is what its profile shows: how full it is
  // plus the rating raised, together at most MAX_RATING. One who has it
  // is told so instead of the form, which cannot work.
  const page = () => me.completeness ?? me.rating ?? 0;
  const left = () => MAX_RATING - page();
  const full = () => left() <= 0;
  const showMeter = () => $('rating-meter').replaceChildren(meter(page(), MAX_RATING, `${page()} %`));
  help('rating-help', tr('Рейтинг прибавляется к заполненности страницы: страница с рейтингом может быть заполнена больше чем на 100 %, и тогда шкала становится золотой. Один процент рейтинга стоит {0} голоса. Рейтинг вместе с заполненностью — не больше {1} %.', RATING_PRICE, MAX_RATING));
  showMeter();
  quick('rating-chips', percent, [['+1', () => 1], ['+10', () => Math.min(10, left())], ['+100', () => Math.min(100, left())]]);
  const tellFull = () => {
    $('rating-form').hidden = true;
    say(tr('Больше повысить нельзя'), tr('Ваш рейтинг уже {0} % — больше не бывает.', MAX_RATING));
  };
  if (full()) tellFull();
  else percent.focus();

  percent.addEventListener('input', () => {
    const n = Number(percent.value);
    const fits = Number.isSafeInteger(n) && n > 0 && n <= left();
    $('rating-cost').textContent = fits ? tr('Стоимость: {0}.', votesAmount(n * RATING_PRICE)) : '';
    $('rating-cost').hidden = !fits;
  });

  $('rating').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    if (full()) {
      tellFull();
      return;
    }

    const submit = $('rating').querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const before = me.rating ?? 0;
      const raised = await api.raiseRating(Number(percent.value));
      me.votes = raised.balance;
      me.completeness = Math.min(page() + raised.rating - (me.rating ?? 0), MAX_RATING);
      me.rating = raised.rating;
      $('rating-balance').textContent = balanceAmount(raised.balance);
      reward(submit, `+${raised.rating - before} %`);
      showBalance(raised.balance);
      showRating(me);
      showMeter();
      percent.value = '';
      $('rating-cost').hidden = true;
      say(tr('Рейтинг повышен'), tr('Теперь ваш рейтинг — {0} %.', page()), ' ',
        el('a', { href: '/votes', textContent: tr('На вашем счету {0}.', balanceAmount(raised.balance)) }));
      if (full()) $('rating-form').hidden = true;
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });
}

// showBank shows the deposit and the loan as the bank counts them now.
function showBank(bank) {
  const { settings: s, account: a } = bank;
  const owed = Math.ceil(a.debt / 100);

  $('hud-deposit').textContent = groupedVotes(Math.floor(a.deposit / 100));
  $('hud-debt').textContent = groupedVotes(owed);
  $('deposit-state').textContent = a.deposit
    ? tr('На вкладе {0}.', bankVotes(a.deposit))
    : tr('Вклада пока нет.');
  $('deposit-meter').replaceChildren(meter(a.deposit, s.depositMost * 100, tr('под проценты: {0} из {1}', groupedVotes(Math.min(Math.floor(a.deposit / 100), s.depositMost)), groupedVotes(s.depositMost))));
  $('deposit-terms').textContent = tr('Ставка {0} на сумму до {1}; положить можно до {1}.', bankRate(s.depositRate), votesAmount(s.depositMost));
  $('deposit-take').disabled = a.deposit < 100;

  if (a.debt) {
    const days = s.loanDays * 86_400_000;
    const left = new Date(a.dueAt) - Date.now();
    $('loan-state').replaceChildren(
      tr('Вы должны {0} — взято {1}.', bankVotes(a.debt), votesAmount(a.borrowed)), ' ',
      a.overdue
        ? el('b', { className: 'bank-out', textContent: tr('Срок прошёл: банк сам заберёт долг с баланса и вклада.') })
        : tr('Вернуть до {0}.', formatDateTime(a.dueAt)));
    $('loan-meter').replaceChildren(meter(days - left, days, a.overdue ? tr('срок прошёл') : tr('срок идёт'), a.overdue ? 'bad' : ''));
    $('loan-go').textContent = tr('Погасить');
    $('loan-amount').placeholder = tr('всё — {0}', votesAmount(owed));
    $('loan-hint').textContent = tr('Пустое поле — погасить весь долг.');
  } else {
    $('loan-state').textContent = tr('Кредита нет.');
    $('loan-go').textContent = tr('Взять кредит');
    $('loan-amount').placeholder = tr('сколько голосов');
    $('loan-hint').textContent = tr('Кредит дают страницам старше недели с заполненной анкетой, по одному за раз.');
  }
  $('loan-meter').hidden = !a.debt;
  $('loan-terms').textContent = tr('До {0} на {1} под {2}, после срока — {3}.',
    votesAmount(s.loanMost), `${s.loanDays} ${plural(s.loanDays, ['день', 'дня', 'дней'])}`, bankRate(s.loanRate), bankRate(s.penaltyRate));
}

// showForecast says what the amount typed does: the deposit in a month, the
// sum to pay back. A loan grows as a deposit does, with no limit.
function showForecast(bank) {
  const { settings: s, account: a } = bank;
  const typed = (input) => {
    const n = Number(input.value);
    return Number.isSafeInteger(n) && n > 0 ? n : 0;
  };

  const put = typed($('deposit-amount'));
  const sum = a.deposit + put * 100;
  const grown = grownDeposit(sum, s.depositRate, s.depositMost, FORECAST_DAYS);
  $('deposit-forecast').textContent = tr('Через {0} дней на вкладе будет {1}: это +{2}.', FORECAST_DAYS, bankVotes(grown), bankVotes(grown - sum));
  $('deposit-forecast').hidden = !sum;

  const loan = a.debt ? 0 : typed($('loan-amount'));
  $('loan-forecast').textContent = tr('Вернуть за {0} дней: около {1}.', s.loanDays, bankVotes(grownDeposit(loan * 100, s.loanRate, Infinity, s.loanDays)));
  $('loan-forecast').hidden = !loan;
}

// initBank is "Вклад" and "Кредит": put votes on the deposit and take them
// off, take a loan and pay it back.
async function initBank(me) {
  let bank;
  try {
    bank = await bankState;
  } catch (e) {
    showError(error, e);
    return;
  }
  showBank(bank);
  showForecast(bank);
  help('deposit-help', tr('Голоса на вкладе каждый день растут на процент. Проценты начисляются на сумму до предела вклада, а всё, что они принесли сверх него, копится на вкладе дальше и не сгорает.'));
  help('loan-help', tr('Кредит выручит, когда голосов не хватает: его возвращают с процентами до срока, а после срока процент больше, и банк сам забирает долг с баланса, а потом со вклада.'));
  for (const id of ['deposit-amount', 'loan-amount']) $(id).addEventListener('input', () => showForecast(bank));
  const balance = () => (isEndless(me.votes ?? 0) ? bank.settings.depositMost : me.votes ?? 0);
  quick('deposit-chips', $('deposit-amount'), [['100', () => 100], ['1 000', () => 1000], [tr('весь баланс'), () => Math.min(balance(), bank.settings.depositMost)]]);
  quick('loan-chips', $('loan-amount'), [
    [tr('половину'), () => Math.ceil((bank.account.debt ? Math.ceil(bank.account.debt / 100) : bank.settings.loanMost) / 2)],
    [tr('всё'), () => (bank.account.debt ? Math.ceil(bank.account.debt / 100) : bank.settings.loanMost)],
  ]);

  const move = async (action, input, button) => {
    hideError(error);
    $('notice').hidden = true;
    const amount = input.value.trim() === '' ? 0 : Number(input.value);
    if (!Number.isSafeInteger(amount) || amount < 0 || (amount === 0 && ['deposit', 'loan'].includes(action))) {
      input.focus();
      return;
    }

    button.disabled = true;
    try {
      const moved = await api.moveBank(action, amount);
      me.votes = moved.balance;
      bank = moved.bank;
      input.value = '';
      showBank(bank);
      showForecast(bank);
      reward(button, `${['deposit', 'repay'].includes(action) ? '−' : '+'}${groupedVotes(moved.votes)}`, ['deposit', 'repay'].includes(action) ? 'bad' : 'gold');
      showBalance(moved.balance);
      say({
        deposit: tr('Голоса положены на вклад'),
        withdraw: tr('Голоса сняты со вклада'),
        loan: tr('Кредит получен'),
        repay: bank.account.debt ? tr('Часть кредита погашена') : tr('Кредит погашен'),
      }[action], tr('{0}. На вашем счету {1}.', votesAmount(moved.votes), balanceAmount(moved.balance)));
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
      $('deposit-take').disabled = bank.account.deposit < 100;
    }
  };

  $('deposit-form').addEventListener('submit', (event) => {
    event.preventDefault();
    move('deposit', $('deposit-amount'), $('deposit-put'));
  });
  $('deposit-take').addEventListener('click', () => move('withdraw', $('deposit-amount'), $('deposit-take')));
  $('loan-form').addEventListener('submit', (event) => {
    event.preventDefault();
    move(bank.account.debt ? 'repay' : 'loan', $('loan-amount'), $('loan-go'));
  });
}

// initTopUp is "Есть ваучер?": the serial number in four fields of six,
// as on openvk — a full field passes on to the next, Backspace in an
// empty one goes back, and a pasted number fills them all.
function initTopUp(me) {
  help('top-up-help', tr('Введите серийный номер ваучера. Обычно он указан в чеке или в сообщении. Обратите внимание, что ваучеры могут истекать и воспользоваться ими можно только один раз.'));
  const keys = [...document.querySelectorAll('.voucher-key')];
  const clean = (text) => text.replace(/[^a-z0-9]/gi, '').toUpperCase();

  function spread(text, from) {
    let rest = clean(text);

    for (let i = from; i < keys.length && rest; i++) {
      keys[i].value = rest.slice(0, 6);
      rest = rest.slice(6);
      keys[Math.min(i + (keys[i].value.length === 6 ? 1 : 0), keys.length - 1)].focus();
    }
  }

  keys.forEach((key, i) => {
    key.addEventListener('input', () => {
      key.value = clean(key.value);
      if (key.value.length === 6 && keys[i + 1]) keys[i + 1].focus();
    });
    key.addEventListener('keydown', (event) => {
      if (event.key === 'Backspace' && !key.value && keys[i - 1]) keys[i - 1].focus();
    });
    key.addEventListener('paste', (event) => {
      event.preventDefault();
      spread(event.clipboardData.getData('text'), i);
    });
  });

  $('voucher-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = $('voucher-form').querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const got = await api.redeemVoucher(keys.map((key) => key.value).join(''));

      me.votes = got.balance;
      fanfare();
      flash(submit.closest('.game-window'), 'gold');
      showBalance(got.balance);
      showRating(me);
      for (const key of keys) key.value = '';
      say(tr('Ваучер активирован'), tr('Вам начислено: {0}. Этим кодом вы больше не сможете его активировать. ', voucherGave(got)),
        el('a', { href: '/votes', textContent: tr('На вашем счету {0}.', balanceAmount(got.balance)) }));
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });

  keys[0].focus();
}

// initReceive is the link that opens a transfer to whoever signed in.
function initReceive(me) {
  const link = $('receive-link');
  link.value = transferURL(location.origin, me.nickname);
  link.addEventListener('focus', () => link.select());
  $('receive-nickname').textContent = `@${me.nickname}`;

  $('receive-copy').addEventListener('click', async () => {
    link.select();
    try {
      await navigator.clipboard.writeText(link.value);
    } catch {
      return;
    }
    say(tr('Ссылка скопирована'));
  });
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  // Without a nickname nobody can name you as a recipient: fill in first.
  location.replace('/edit');
} else {
  renderChrome(me);
  $('tab-' + act).classList.add('active');
  $(act).hidden = false;
  $('loading').hidden = true;
  $('bank-hud').hidden = false;
  showBalance(me.votes ?? 0);
  showRating(me);
  bankState.then(showBank, () => {});
  (tabs[act] ?? initAccount)(me);
}
