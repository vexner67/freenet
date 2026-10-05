import { api, currentUser } from './api.js';
import { $, renderChrome, profilePath, showError, hideError } from './layout.js';

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
    location.href = me.profileCompleted ? profilePath(me) : '/edit?welcome=1';
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
  location.replace(profilePath(me));
} else {
  renderChrome(null);
  $('email').focus();
}
