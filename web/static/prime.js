// "Prime" at /prime, the subscription of the site: what it gives,
// buying a month, three months or a year for votes into the treasury,
// giving it to another as a present (the recipient picked as in the bank) and «Невидимка» for one who has it;
// one who has it is not offered to buy it again until it ends.
// openvk has nothing like it.

import { api, currentUser, started } from './api.js';
import {
  balanceAmount, formatDateTime, primeBackgrounds, primeFrames, primeMoodIcons, primeMoodText, primeMoods, primeNames, primePeriods,
  primeNameClass, primeSaving, primeStyleClasses, primeTerms, transferNickname, votesAmount,
} from './labels.js';
import { $, avatarURL, el, fullName, hideError, pagePath, profilePath, renderChrome, showError, verifiedMark } from './layout.js';
import { notify } from './modal.js';
import { tr } from './i18n.js';
import { isNickname, recipientPicker } from './recipient.js';

const error = $('error');
// The prices are asked with the page, before who is signed in is known.
const prices = started(api.primePrices());

// say shows the notice above the page: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  notify($('notice'), title);
}

// showState is until when the caller has Prime, and the balance.
function showState(me) {
  const until = me.primeUntil && new Date(me.primeUntil) > new Date() ? me.primeUntil : '';

  $('state').textContent = until
    ? tr('Подписка активна до {0}. Купить её снова можно, когда она закончится.', formatDateTime(until))
    : tr('У вас пока нет Prime.');
  // One who has Prime sees what it gives, not its prices.
  $('buy-part').hidden = Boolean(until);
  $('balance').textContent = balanceAmount(me.votes ?? 0);
  $('invisible-part').hidden = !until;
  $('style-part').hidden = !until;
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
      say(tr('Prime ваш'), tr('Подписка действует до {0}.', formatDateTime(bought.until)));
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

// initGive is "Подарить подписку": the recipient picked as in the bank.
function initGive(me) {
  const form = $('give');
  const period = $('give-period');
  const picker = recipientPicker({
    to: $('give-to'), me, people: $('give-people'), shown: $('give-recipient'),
    picked: () => period.focus(),
  });

  period.replaceChildren(...primePeriods.map(([value]) => el('option', { value, textContent: primeTerms[value] })));

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = form.querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const recipient = picker.found() ?? await picker.lookUp();
      if (!recipient) {
        showError(error, isNickname(transferNickname($('give-to').value)) ? tr('Получатель не найден.') : tr('Выберите получателя из списка.'));
        return;
      }

      const given = await api.givePrime(recipient.nickname, period.value);
      me.votes = given.balance;
      showState(me);
      picker.clear();
      say(tr('Подписка подарена'), el('a', { href: pagePath(recipient), textContent: fullName(recipient) }),
        tr(' получает Prime до {0}.', formatDateTime(given.until)));
    } catch (e) {
      showError(error, e);
    } finally {
      submit.disabled = false;
    }
  });
}

// styleOf is the style the form of "Оформление страницы" has chosen.
function styleOf() {
  const chosen = (name) => $('style').querySelector(`input[name="${name}"]:checked`)?.value ?? '';

  return { name: chosen('name'), frame: chosen('frame'), background: chosen('background'), mood: chosen('mood') };
}

// showPreview dresses the small copy of the page in style.
function showPreview(me, style) {
  const preview = $('style-preview');
  const icon = primeMoodIcons[style.mood];

  preview.className = ['prime-preview', 'prime-page', ...primeStyleClasses(style)].join(' ');
  $('style-title').replaceChildren(el('span', { className: primeNameClass({ prime: true, primeStyle: style }), textContent: fullName(me) }),
    verifiedMark({ prime: true }));
  $('style-status').textContent = me.status || tr('Ваш статус');
  $('style-mood').hidden = !icon;
  if (icon) Object.assign($('style-mood'), { src: icon, title: primeMoodText(style.mood) });

  const photo = $('style-photo');
  photo.querySelectorAll('.prime-sparkles').forEach((s) => s.remove());
  if (style.frame === 'glitter') photo.append(el('span', { className: 'prime-sparkles' }), el('span', { className: 'prime-sparkles late' }));
}

// swatch is one choice of the form: a radio with what it looks like.
function swatch(group, value, name, sample, chosen) {
  return el('label', { className: 'prime-swatch' },
    el('input', { type: 'radio', name: group, value, checked: value === chosen }),
    sample,
    el('span', { textContent: name }));
}

// initStyle is "Оформление страницы": the colour of the name, the frame of
// the photo, the pattern behind the page and the smiley by the status.
function initStyle(me) {
  const form = $('style');
  const style = { name: 'amethyst', frame: 'amethyst', background: '', mood: '', ...me.primeStyle };
  const photo = avatarURL(me, 'small');

  $('style-avatar').src = photo;
  $('style-names').replaceChildren(...primeNames.map(([value, name]) => swatch('name', value, name,
    el('span', { className: primeNameClass({ prime: true, primeStyle: { name: value } }), textContent: me.name || 'Freenet' }),
    style.name)));
  $('style-frames').replaceChildren(...primeFrames.map(([value, name]) => swatch('frame', value, name,
    el('span', { className: 'prime-page prime-frame-' + value },
      el('img', { className: 'avatar prime-swatch-sample', src: photo, alt: '', width: 40, height: 40 })),
    style.frame)));
  $('style-backgrounds').replaceChildren(...primeBackgrounds.map(([value, name]) => swatch('background', value, name,
    el('span', { className: 'prime-swatch-sample prime-swatch-bg' + (value ? ' prime-bg-' + value : '') }),
    style.background)));
  $('style-moods').replaceChildren(...primeMoods.map(([value, name]) => swatch('mood', value, name,
    primeMoodIcons[value]
      ? el('img', { src: primeMoodIcons[value], alt: '', width: 32, height: 32 })
      : el('span', { className: 'prime-swatch-none', textContent: '—' }),
    style.mood)));
  showPreview(me, style);

  form.addEventListener('change', () => showPreview(me, styleOf()));
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;

    const submit = form.querySelector('button[type="submit"]');
    submit.disabled = true;

    try {
      const chosen = styleOf();
      await api.setPrimeStyle(chosen);
      me.primeStyle = chosen;
      say(tr('Оформление сохранено'), el('a', { href: profilePath(me), textContent: tr('Посмотреть свою страницу') }));
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
    initStyle(me);
    $('prime').hidden = false;
  } catch (e) {
    showError(error, e);
  }
}
