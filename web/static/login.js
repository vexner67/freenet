import { api, currentUser } from './api.js';
import { settle, signedInAs } from './accounts.js';
import { $, accountLinks, renderChrome, showAccounts, homePath, showError, hideError } from './layout.js';

const emailForm = $('email-form');
const codeForm = $('code-form');
const error = $('error');

let email = '';

async function sendCode() {
  await api.requestCode(email);

  $('sent-to').textContent = email;
  emailForm.hidden = true;
  codeForm.hidden = false;
  $('code').value = '';
  $('code').focus();
}

async function withButton(button, action) {
  hideError(error);
  button.disabled = true;

  try {
    await action();
  } catch (e) {
    showError(error, e);
  } finally {
    button.disabled = false;
  }
}

emailForm.addEventListener('submit', (event) => {
  event.preventDefault();
  email = $('email').value.trim();
  withButton($('email-submit'), sendCode);
});

codeForm.addEventListener('submit', (event) => {
  event.preventDefault();
  withButton($('code-submit'), async () => {
    await api.verifyCode(email, $('code').value.trim());
    const me = await api.me();
    // An account that waited in this browser has a new session now.
    settle(me);
    signedInAs(me.id);
    location.href = me.profileCompleted ? homePath(me) : '/edit?welcome=1';
  });
});

$('change-email').addEventListener('click', () => {
  hideError(error);
  codeForm.hidden = true;
  emailForm.hidden = false;
  $('email').focus();
});

$('resend').addEventListener('click', () => withButton($('code-submit'), sendCode));

const me = await currentUser().catch(() => null);
if (me) {
  location.replace(homePath(me));
} else {
  renderChrome(null);

  // The accounts set aside in this browser: a click signs in as one.
  showAccounts(() => {
    const waiting = accountLinks();

    $('accounts-list').replaceChildren(...waiting);
    $('accounts').hidden = waiting.length === 0;
  });
  $('email').focus();
}
