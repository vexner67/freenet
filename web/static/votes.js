import { api, currentUser } from './api.js';
import { balanceAmount, formatDateTime, signedVotes, transferFeeText, voteOperation, voteRate, votesAmount } from './labels.js';
import { $, el, fullName, hideError, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { recipientPicker } from './recipient.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const asked = params.get('act');
const tab = ['transfer', 'top-up'].includes(asked) ? asked : 'summary';
// The statement shows all, or ?flow=in what came, ?flow=out what went.
const flow = ['in', 'out'].includes(params.get('flow')) ? params.get('flow') : '';

// operationRow is a line of "История операций": what it was, from or to
// whom, when, the votes it brought or took and the balance after it.
function operationRow(op, me) {
  const { title, role } = voteOperation(op, me.id);
  const about = [el('div', { className: 'votes-op-title', textContent: title })];

  if (role && op.peer) {
    about.push(el('div', {}, `${role} `,
      el('a', { href: pagePath(op.peer), textContent: fullName(op.peer) }), verifiedMark(op.peer)));
  }
  if (op.kind === 'transfer' && op.note) about.push(el('div', { className: 'votes-op-note', textContent: `«${op.note}»` }));
  about.push(el('div', { className: 'votes-op-date', textContent: formatDateTime(op.createdAt) }));

  return el('div', { className: 'votes-op' },
    el('div', { className: 'votes-op-about' }, ...about),
    el('div', { className: 'votes-op-sum' },
      el('b', { className: op.amount < 0 ? 'votes-out' : 'votes-in', textContent: signedVotes(op.amount) }),
      el('div', { className: 'votes-op-balance', textContent: tr('остаток {0}', balanceAmount(op.balance)) })));
}

async function summary(me) {
  $('votes').textContent = balanceAmount(me.votes ?? 0);
  $('flow-' + (flow || 'all')).classList.add('active');
  const [rate, statement] = await Promise.all([api.voteRate(), api.voteStatement('', flow)]);
  $('vote-rate').textContent = voteRate(rate.kopecks);
  $('income').textContent = signedVotes(statement.income);
  $('expense').textContent = signedVotes(-statement.expense);

  let next = '';
  const show = (page) => {
    $('operations').append(...page.operations.map((op) => operationRow(op, me)));
    next = page.nextCursor;
    $('more').hidden = !next;
  };
  $('operations-empty').hidden = statement.operations.length > 0;
  show(statement);

  $('more').addEventListener('click', async () => {
    if ($('more').classList.contains('loading')) return;
    $('more').classList.add('loading');
    try {
      show(await api.voteStatement(next, flow));
    } catch (error) {
      showError($('error'), error);
    } finally {
      $('more').classList.remove('loading');
    }
  });
}

function result(text) {
  const notice = $('votes-notice');
  notice.textContent = text;
  notice.hidden = !text;
}

function submit(form, send) {
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError($('error'));
    result('');
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      await send();
    } catch (error) {
      showError($('error'), error);
    } finally {
      button.disabled = false;
    }
  });
}

async function transfer(me) {
  const to = $('transfer-to');
  const amount = $('transfer-votes');
  const note = $('transfer-note');
  const picker = recipientPicker({ to, people: $('transfer-people'), shown: $('transfer-recipient'), me, showAvatar: true, picked: () => amount.focus() });
  to.value = params.get('to') ?? '';
  if (to.value) picker.lookUp();

  const treasury = await api.treasury();
  const percent = me.prime ? 0 : treasury.transferFee;
  const fee = () => { $('transfer-fee').textContent = transferFeeText(Number(amount.value), percent); };
  amount.addEventListener('input', fee);
  fee();
  submit($('transfer'), async () => {
    const recipient = picker.found() ?? await picker.lookUp();
    if (!recipient) {
      showError($('error'), tr('Выберите получателя из списка.'));
      return;
    }
    const votes = Number(amount.value);
    await api.transferVotes(recipient.nickname, votes, note.value.trim());
    picker.clear();
    amount.value = '';
    note.value = '';
    fee();
    result(tr('Переведено {0}', votesAmount(votes)));
  });
}

function topUp() {
  const field = $('voucher-code');
  submit($('top-up'), async () => {
    const code = field.value.trim().replace(/[ -]/g, '');
    if (!/^[a-z0-9]{24}$/i.test(code)) {
      showError($('error'), tr('Введите 24 знака ваучера.'));
      return;
    }
    const got = await api.redeemVoucher(code);
    field.value = '';
    const credited = tr('Зачислено {0}', votesAmount(got.votes));
    result(got.rating ? tr('{0}, рейтинг +{1} %', credited, got.rating) : credited);
  });
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  location.replace('/edit');
} else {
  renderChrome(me);
  $('tab-' + tab).classList.add('active');
  $('tab-' + tab).setAttribute('aria-current', 'page');

  try {
    if (tab === 'summary') await summary(me);
    if (tab === 'transfer') await transfer(me);
    if (tab === 'top-up') topUp();
    $(tab).hidden = false;
    $('votes-result').hidden = tab === 'summary';
  } catch (error) {
    showError($('error'), error);
  } finally {
    $('loading').hidden = true;
  }
}
