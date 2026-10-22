// "Freenet Prime" at /prime, the subscription of the site: what it gives,
// buying a month, three months or a year for votes into the treasury,
// giving it to another as a present and «Невидимка» for one who has it.
// openvk has nothing like it.

import { api, currentUser, started } from './api.js';
import { formatDateTime, primePeriods, primeSaving, transferNickname, votesAmount } from './labels.js';
import { $, el, fullName, hideError, pagePath, renderChrome, showError } from './layout.js';
import { tr } from './i18n.js';

const error = $('error');
// The prices are asked with the page, before who is signed in is known.
const prices = started(api.primePrices());

// say shows the notice above the page: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  $('notice').scrollIntoView({ block: 'nearest' });
}

// showState is until when the caller has Prime, and the balance.
function showState(me) {
  const until = me.primeUntil && new Date(me.primeUntil) > new Date() ? me.primeUntil : '';

  $('state').textContent = until
    ? tr('У вас Freenet Prime до {0}.', formatDateTime(until))
    : tr('У вас пока нет Freenet Prime.');
  $('balance').textContent = votesAmount(me.votes ?? 0);
  $('invisible-part').hidden = !until;
  $('invisible').checked = Boolean(me.primeInvisible);
}

// plan is a period to buy: its name, price and what it saves.
function plan(me, cost, [period, name, months]) {
  const price = cost[period];
  const saving = primeSaving(cost, months, price);
  const buy = el('button', { className: 'button', type: 'button', textContent: tr('Купить за {0}', votesAmount(price)) });

  buy.addEventListener('click', async () => {
    hideError(error);
    $('notice').hidden = true;
    buy.disabled = true;

    try {
      const bought = await api.buyPrime(period);
      Object.assign(me, { votes: bought.balance, primeUntil: bought.until, prime: true });
      showState(me);
      say(tr('Freenet Prime ваш'), tr('Подписка действует до {0}.', formatDateTime(bought.until)));
    } catch (e) {
      showError(error, e);
    } finally {
      buy.disabled = false;
    }
  });

  return el('div', { className: 'prime-plan' },
    el('b', { className: 'prime-plan-name', textContent: name }),
    el('div', { className: 'prime-plan-price', textContent: votesAmount(price) }),
    el('div', { className: 'prime-plan-saving', textContent: saving ? tr('выгода {0} %', saving) : ' ' }),
    buy);
}

// initGive is "Подарить подписку": the recipient by nickname or link.
function initGive(me) {
  const form = $('give');
  const period = $('give-period');

  period.replaceChildren(...primePeriods.map(([value, name]) => el('option', { value, textContent: name })));

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = form.querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const nickname = transferNickname($('give-to').value);
      const recipient = await api.profile(nickname);
      const given = await api.givePrime(nickname, period.value);
      me.votes = given.balance;
      showState(me);
      $('give-to').value = '';
      say(tr('Подписка подарена'), el('a', { href: pagePath(recipient), textContent: fullName(recipient) }),
        tr(' получает Freenet Prime до {0}.', formatDateTime(given.until)));
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });
}

// initInvisible turns «Невидимка» on and off at once.
function initInvisible(me) {
  const box = $('invisible');

  box.addEventListener('change', async () => {
    hideError(error);
    box.disabled = true;

    try {
      await api.setPrimeInvisible(box.checked);
      me.primeInvisible = box.checked;
    } catch (e) {
      box.checked = !box.checked;
      showError(error, e);
    } finally {
      box.disabled = false;
    }
  });
}

const me = await currentUser().catch(() => null);
if (!me) {
  location.replace('/');
} else {
  renderChrome(me);
  $('loading').hidden = true;

  try {
    const cost = await prices;
    $('plans').replaceChildren(...primePeriods.map((p) => plan(me, cost, p)));
    showState(me);
    initGive(me);
    initInvisible(me);
    $('prime').hidden = false;
  } catch (e) {
    showError(error, e);
  }
}
