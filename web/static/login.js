import { api, currentUser } from './api.js';
import { settle, signedInAs } from './accounts.js';
import { keepInvite } from './invite.js';
import { $, accountLinks, avatarURL, el, fullName, renderChrome, showAccounts, homePath, showError, hideError } from './layout.js';
import { tr } from './i18n.js';

const emailForm = $('email-form');
const codeForm = $('code-form');
const twoFactorForm = $('two-factor-form');
const error = $('error');

let email = '';

// The picture to read, when the site asks for one; each is good for one
// try, so another comes after any answer.
let captcha = { required: false };

async function showCaptcha() {
  captcha = await api.captcha();
  $('captcha').hidden = !captcha.required;
  $('captcha-answer').required = captcha.required;
  $('captcha-answer').value = '';
  if (captcha.required) $('captcha-image').src = `data:image/svg+xml,${encodeURIComponent(captcha.image)}`;
}

async function sendCode() {
  try {
    await api.requestCode(email, captcha.id ?? '', $('captcha-answer').value.trim());
  } catch (e) {
    await showCaptcha().catch(() => {});
    throw e;
  }

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

// enter opens the site as the one just signed in.
async function enter() {
  const me = await api.me();
  // An account that waited in this browser has a new session now.
  settle(me);
  signedInAs(me.id);
  location.href = me.profileCompleted ? homePath(me) : '/edit?welcome=1';
}

// ticket is the sign-in waiting for the code of the authenticator app.
let ticket = '';

codeForm.addEventListener('submit', (event) => {
  event.preventDefault();
  withButton($('code-submit'), async () => {
    const answer = await api.verifyCode(email, $('code').value.trim());
    if (!answer.twoFactorTicket) {
      await enter();
      return;
    }

    ticket = answer.twoFactorTicket;
    codeForm.hidden = true;
    twoFactorForm.hidden = false;
    $('two-factor-code').focus();
  });
});

twoFactorForm.addEventListener('submit', (event) => {
  event.preventDefault();
  withButton($('two-factor-submit'), async () => {
    await api.verifyTwoFactor(ticket, $('two-factor-code').value.trim());
    await enter();
  });
});

$('two-factor-back').addEventListener('click', () => {
  hideError(error);
  ticket = '';
  $('two-factor-code').value = '';
  twoFactorForm.hidden = true;
  emailForm.hidden = false;
  $('email').focus();
});

$('change-email').addEventListener('click', () => {
  hideError(error);
  codeForm.hidden = true;
  emailForm.hidden = false;
  showCaptcha().catch((e) => showError(error, e));
  $('email').focus();
});

// showInviter says who invites, openvk's "<b>Анна</b> приглашает вас в
// OpenVK"; a nickname of nobody says nothing.
async function showInviter(nickname) {
  if (!nickname) return;

  const inviter = await api.profile(nickname).catch(() => null);
  if (!inviter || inviter.deactivated) return;

  $('invited').replaceChildren(
    el('img', { className: 'invited-photo', src: avatarURL(inviter, 'small'), alt: '' }),
    el('span', {}, el('b', { textContent: fullName(inviter) }), tr(' приглашает вас во Freenet.')));
  $('invited').hidden = false;
}

// A new code takes a new picture, so "Прислать снова" goes back to it.
$('resend').addEventListener('click', () => {
  codeForm.hidden = true;
  emailForm.hidden = false;
  withButton($('email-submit'), showCaptcha);
  ($('captcha').hidden ? $('email') : $('captcha-answer')).focus();
});
$('captcha-new').addEventListener('click', () => withButton($('email-submit'), showCaptcha));

const me = await currentUser().catch(() => null);
if (me) {
  location.replace(homePath(me));
} else {
  renderChrome(null);
  showCaptcha().catch((e) => showError(error, e));

  // The accounts set aside in this browser: a click signs in as one.
  showAccounts(() => {
    const waiting = accountLinks();

    $('accounts-list').replaceChildren(...waiting);
    $('accounts').hidden = waiting.length === 0;
  });
  showInviter(keepInvite());
  $('email').focus();
}
