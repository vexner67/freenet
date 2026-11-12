import { api, currentUser } from './api.js';
import { balanceAmount, signedVotes, transferFeeText, voteRate, votesAmount } from './labels.js';
import { $, hideError, renderChrome, showError } from './layout.js';
import { recipientPicker } from './recipient.js';
import { tr } from './i18n.js';

const params = new URLSearchParams(location.search);
const asked = params.get('act');
const tab = ['transfer', 'top-up'].includes(asked) ? asked : 'summary';

function balance(votes) {
  for (const id of ['votes', 'transfer-balance', 'top-up-balance']) $(id).textContent = balanceAmount(votes);
}

async function summary() {
  const [rate, statement] = await Promise.all([api.voteRate(), api.voteStatement()]);
  $('vote-rate').textContent = voteRate(rate.kopecks);
  $('income').textContent = signedVotes(statement.income);
  $('expense').textContent = signedVotes(-statement.expense);
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
    const left = await api.transferVotes(recipient.nickname, votes, note.value.trim());
    balance(left.votes);
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
    balance(got.balance);
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
  balance(me.votes ?? 0);
  $('tab-' + tab).classList.add('active');
  $('tab-' + tab).setAttribute('aria-current', 'page');

  try {
    if (tab === 'summary') await summary();
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
