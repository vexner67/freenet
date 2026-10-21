// "Мои Голоса" at /votes, a small bank of votes: "Счёт" (the balance,
// what came in and went out in 30 days and the statement, all of it or
// ?flow=in|out), "Перевести" (?act=transfer, ?to=<nickname> names the
// recipient), "Пополнить" (?act=top-up: a voucher) and "Получить"
// (?act=receive: the link that opens a transfer to you).

import { api, currentUser, started } from './api.js';
import { formatDateTime, plural, rubles, signedVotes, transferNickname, transferURL, voteOperation, voteRate, votesAmount, votesFontSize, voucherGave } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError, verifiedMark } from './layout.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const tabs = { transfer: initTransfer, 'top-up': initTopUp, receive: initReceive };
const act = Object.hasOwn(tabs, params.get('act')) ? params.get('act') : 'account';
const flow = ['in', 'out'].includes(params.get('flow')) ? params.get('flow') : '';
const error = $('error');

// The statement and the rate are asked with the page, before who is
// signed in is known.
const statement = act === 'account' ? started(api.voteStatement('', flow)) : null;
const rate = act === 'account' ? api.voteRate().then((r) => r.kopecks, () => 0) : Promise.resolve(0);

// say shows the notice above the tab: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  $('notice').scrollIntoView({ block: 'nearest' });
}

function showBalance(votes, kopecks) {
  $('votes').textContent = votes;
  $('votes').style.fontSize = `${votesFontSize(votes)}px`;
  $('votes-word').textContent = plural(votes, ['голос', 'голоса', 'голосов']);
  $('votes-rubles').textContent = kopecks ? `≈ ${rubles(votes * kopecks)}` : '';
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
      el('div', { className: 'bank-op-balance', textContent: tr('остаток {0}', votesAmount(op.balance)) })));
}

async function initAccount(me) {
  showBalance(me.votes ?? 0, 0);
  $('flow-' + (flow || 'all')).classList.add('active');

  rate.then((kopecks) => {
    if (!kopecks) return;
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

// initTransfer is openvk's "Передача голосов", as a page of its own: the
// recipient is named by the address of their page and shown once found.
function initTransfer(me) {
  const to = $('transfer-to');
  const shown = $('transfer-recipient');
  $('transfer-balance').textContent = votesAmount(me.votes ?? 0);

  let found = null;
  async function lookUp() {
    const nickname = transferNickname(to.value);
    found = null;
    shown.hidden = true;
    if (!nickname) return null;

    try {
      const recipient = await api.profile(nickname);
      if (transferNickname(to.value) !== nickname) return null;
      found = recipient;
      shown.replaceChildren(el('a', { href: pagePath(recipient), textContent: fullName(recipient) }), verifiedMark(recipient));
    } catch {
      shown.replaceChildren(tr('Получатель не найден.'));
    }
    shown.hidden = false;

    return found;
  }

  to.value = params.get('to') ?? '';
  to.addEventListener('change', lookUp);
  if (to.value) {
    lookUp();
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
      const recipient = found ?? await lookUp();
      if (!recipient) {
        showError(error, tr('Получатель не найден.'));
        return;
      }

      const sent = Number($('transfer-votes').value);
      const left = await api.transferVotes(recipient.nickname, sent, $('transfer-note').value.trim());

      me.votes = left.votes;
      $('transfer-balance').textContent = votesAmount(left.votes);
      $('transfer-votes').value = '';
      $('transfer-note').value = '';
      say(tr('Голоса переданы'), tr('Вы успешно передали '), el('b', { textContent: votesAmount(sent) }), ' — ',
        el('a', { href: pagePath(recipient), textContent: fullName(recipient) }), '. ',
        el('a', { href: '/votes', textContent: tr('На вашем счету {0}.', votesAmount(left.votes)) }));
    } catch (e) {
      showError(error, e.status === 404 ? tr('Получатель не найден.') : e);
    } finally {
      submit.disabled = false;
    }
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
        el('a', { href: '/votes', textContent: tr('На вашем счету {0}.', votesAmount(got.balance)) }));
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
