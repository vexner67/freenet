// "Мои Голоса" at /votes, a small bank of votes: "Счёт" (the balance,
// what came in and went out in 30 days and the statement, all of it or
// ?flow=in|out), "Перевести" (?act=transfer, ?to=<nickname> names the
// recipient; the fee is shown as the votes are typed), "Пополнить"
// (?act=top-up: a voucher), "Получить" (?act=receive: the link that opens a
// transfer to you), "Рейтинг" (?act=rating: openvk's "Повысить
// рейтинг", for votes), "Вклад" (?act=deposit; the old ?act=bank too)
// and "Кредит" (?act=loan).

import { api, currentUser, started } from './api.js';
import { balanceVotes, balanceWord, balanceAmount, MAX_RATING, RATING_PRICE, bankRate, bankVotes, TRANSFER_FEE_PERCENT, formatDateTime, isEndless, plural, rubles, transferFee, signedVotes, transferNickname, transferURL, voteOperation, voteRate, votesAmount, votesFontSize, voucherGave } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { notify } from './modal.js';
import { tr } from './i18n.js';
import { isNickname, recipientPicker } from './recipient.js';

const params = new URLSearchParams(location.search);
const tabs = { transfer: initTransfer, 'top-up': initTopUp, receive: initReceive, rating: initRating, deposit: initBank, loan: initBank };
const asked = params.get('act') === 'bank' ? 'deposit' : params.get('act');
const act = Object.hasOwn(tabs, asked) ? asked : 'account';
const flow = ['in', 'out'].includes(params.get('flow')) ? params.get('flow') : '';
const error = $('error');

// The statement and the rate are asked with the page, before who is
// signed in is known.
const statement = act === 'account' ? started(api.voteStatement('', flow)) : null;
const rate = act === 'account' ? api.voteRate().then((r) => r.kopecks, () => 0) : Promise.resolve(0);
const bonus = act === 'account' ? started(api.bonus()) : null;
const bankState = ['deposit', 'loan'].includes(act) ? started(api.bank()) : null;

// say shows the notice above the tab: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  notify($('notice'), title);
}

function showBalance(votes, kopecks) {
  $('votes').textContent = balanceVotes(votes);
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = balanceWord(votes);
  // The endless balance of an administrator is worth no sum.
  $('votes-rubles').textContent = kopecks && !isEndless(votes) ? `≈ ${rubles(votes * kopecks)}` : '';
}

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
    el('div', { className: 'bank-op-about' }, ...about),
    el('div', { className: 'bank-op-sum' },
      el('b', { className: op.amount < 0 ? 'bank-out' : 'bank-in', textContent: signedVotes(op.amount) }),
      el('div', { className: 'bank-op-balance', textContent: tr('остаток {0}', balanceAmount(op.balance)) })));
}

// showBonus is the daily bonus: a button while it waits for the caller
// today, else when the next one comes or why there is none.
function showBonus(state, me, kopecks) {
  const claim = $('bonus-claim');

  $('bonus-text').textContent = !state.eligible
    ? tr('Бонус получают страницы старше недели с заполненной анкетой.')
    : state.taken
      ? tr('Сегодня вы его уже получили. Следующий — завтра.')
      : tr('Сегодня — {0}.', votesAmount(state.votes));
  $('bonus-streak').textContent = state.eligible && state.streak > 0
    ? tr('Заходите каждый день: бонус растёт до семикратного. Дней подряд: {0} из {1}.', Math.min(state.streak, 7), 7)
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
      showBalance(got.balance, kopecks());
      showBonus({ ...state, taken: true }, me, kopecks);
      say(tr('Бонус получен'), tr('Вам начислено: {0}.', votesAmount(got.votes)));
    } catch (e) {
      showError(error, e);
    } finally {
      claim.disabled = false;
    }
  };
}

async function initAccount(me) {
  showBalance(me.votes ?? 0, 0);
  let rated = 0;
  bonus.then((state) => showBonus(state, me, () => rated), () => {});
  $('flow-' + (flow || 'all')).classList.add('active');

  rate.then((kopecks) => {
    if (!kopecks) return;
    rated = kopecks;
    $('vote-rate').textContent = tr('Курс: {0}.', voteRate(kopecks));
    $('vote-rate').hidden = false;
    showBalance(me.votes ?? 0, kopecks);
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
      const raised = await api.raiseRating(Number(percent.value));
      me.votes = raised.balance;
      me.completeness = Math.min(page() + raised.rating - (me.rating ?? 0), MAX_RATING);
      me.rating = raised.rating;
      $('rating-balance').textContent = balanceAmount(raised.balance);
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
function showBank(bank, balance) {
  const { settings: s, account: a } = bank;
  const owed = Math.ceil(a.debt / 100);

  for (const b of document.querySelectorAll('.bank-balance-votes')) b.textContent = balanceAmount(balance);
  $('deposit-state').textContent = a.deposit
    ? tr('На вкладе {0}.', bankVotes(a.deposit))
    : tr('Вклада пока нет.');
  $('deposit-terms').textContent = tr('Ставка {0} на сумму до {1}; положить можно до {1}.', bankRate(s.depositRate), votesAmount(s.depositMost));
  $('deposit-take').disabled = a.deposit < 100;

  if (a.debt) {
    $('loan-state').replaceChildren(
      tr('Вы должны {0} — взято {1}.', bankVotes(a.debt), votesAmount(a.borrowed)), ' ',
      a.overdue
        ? el('b', { className: 'bank-out', textContent: tr('Срок прошёл: банк сам заберёт долг с баланса и вклада.') })
        : tr('Вернуть до {0}.', formatDateTime(a.dueAt)));
    $('loan-go').textContent = tr('Погасить');
    $('loan-amount').placeholder = tr('всё — {0}', votesAmount(owed));
    $('loan-hint').textContent = tr('Пустое поле — погасить весь долг.');
  } else {
    $('loan-state').textContent = tr('Кредита нет.');
    $('loan-go').textContent = tr('Взять кредит');
    $('loan-amount').placeholder = tr('сколько голосов');
    $('loan-hint').textContent = tr('Кредит дают страницам старше недели с заполненной анкетой, по одному за раз.');
  }
  $('loan-terms').textContent = tr('До {0} на {1} под {2}, после срока — {3}.',
    votesAmount(s.loanMost), `${s.loanDays} ${plural(s.loanDays, ['день', 'дня', 'дней'])}`, bankRate(s.loanRate), bankRate(s.penaltyRate));
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
  showBank(bank, me.votes ?? 0);

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
      showBank(bank, moved.balance);
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
  (tabs[act] ?? initAccount)(me);
}
