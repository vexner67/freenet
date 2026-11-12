import { api, currentUser } from './api.js';
import { balanceAmount, signedVotes, voteRate } from './labels.js';
import { $, renderChrome, showError } from './layout.js';

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else if (!me.profileCompleted) {
  location.replace('/edit');
} else {
  renderChrome(me);
  $('votes').textContent = balanceAmount(me.votes ?? 0);

  try {
    const [rate, statement] = await Promise.all([api.voteRate(), api.voteStatement('', '')]);
    $('vote-rate').textContent = voteRate(rate.kopecks);
    $('income').textContent = signedVotes(statement.income);
    $('expense').textContent = signedVotes(-statement.expense);
  } catch (error) {
    showError($('error'), error);
  } finally {
    $('loading').hidden = true;
    $('summary').hidden = false;
  }
}
