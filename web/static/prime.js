// "Prime" at /prime, the subscription of the site, in three tabs as
// "Голоса": "Подписка" (until when, the balance, buying a month or a year
// for votes into the treasury, «Невидимка» and what Prime gives),
// "Подарить" (to another, the recipient picked as in the bank) and
// "Оформление" (the look of one's page, tried on before buying).
// One who has it is not offered to buy it again until it ends.
// openvk has nothing like it.

import { api, currentUser, started } from './api.js';
import {
  balanceAmount, formatDateTime, primeBackgrounds, primeFrames, primeMoodIcons, primeMoodText, primeMoods, primeNames, primePeriods,
  primeNameClass, primeSaving, primeStyleClasses, transferNickname, votesAmount,
} from './labels.js';
import { $, avatarURL, el, fullName, hideError, pagePath, profilePath, renderChrome, showError, verifiedMark } from './layout.js';
import { notify } from './modal.js';
import { tr } from './i18n.js';
import { isNickname, recipientPicker } from './recipient.js';

const error = $('error');
// The prices are asked with the page, before who is signed in is known.
const prices = started(api.primePrices());
const asked = new URLSearchParams(location.search).get('act');
const tab = ['gift', 'style'].includes(asked) ? asked : 'plan';

// say shows the notice above the page: a bold line and what follows it.
function say(title, ...rest) {
  $('notice').replaceChildren(el('b', { textContent: title }), ...(rest.length ? [el('br'), ...rest] : []));
  $('notice').hidden = false;
  notify($('notice'), title);
}

// activeUntil is until when me has Prime, '' when they have none.
const activeUntil = (me) => (me.primeUntil && new Date(me.primeUntil) > new Date() ? me.primeUntil : '');

// showState is until when the caller has Prime, and the balance.
function showState(me) {
  const until = activeUntil(me);

  $('state').textContent = until
    ? tr('действует до {0}', formatDateTime(until))
    : tr('не оформлена');
  // One who has Prime sees what it gives, not its prices.
  $('buy-part').hidden = Boolean(until);
  $('invisible-part').hidden = !until;
  $('invisible').checked = Boolean(me.primeInvisible);
  $('balance').textContent = balanceAmount(me.votes ?? 0);
  $('give-balance').textContent = balanceAmount(me.votes ?? 0);
}

// chosenPeriod is the period checked in box.
const chosenPeriod = (box) => box.querySelector('input:checked').value;

// periodChoices fills box with the periods as radios named name, the
// month chosen: each with its price and what it saves against months,
// and names button by label of what the chosen one costs.
function periodChoices(box, name, cost, button, label) {
  const named = () => {
    button.textContent = label(votesAmount(cost[chosenPeriod(box)]));
  };

  box.replaceChildren(...primePeriods.map(([period, title, months], i) => {
    const saving = primeSaving(cost, months, cost[period]);
    return el('label', { className: 'prime-period' },
      el('input', { type: 'radio', name, value: period, checked: i === 0 }),
      ' ', el('b', { textContent: title }),
      ' — ', votesAmount(cost[period]),
      saving ? el('span', { className: 'prime-saving', textContent: tr(' (выгода {0} %)', saving) }) : '');
  }));
  box.addEventListener('change', named);
  named();
}

// initBuy is "Оформить подписку" for oneself.
function initBuy(me, cost) {
  const form = $('buy');
  const button = $('buy-button');
  periodChoices($('buy-periods'), 'buy-period', cost, button, (votes) => tr('Оформить за {0}', votes));

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;
    button.disabled = true;

    try {
      const bought = await api.buyPrime(chosenPeriod($('buy-periods')));
      Object.assign(me, { votes: bought.balance, primeUntil: bought.until, prime: true });
      showState(me);
      say(tr('Prime ваш'), tr('Подписка действует до {0}.', formatDateTime(bought.until)), ' ',
        el('a', { href: '/prime?act=style', textContent: tr('Оформить страницу') }));
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  });
}

// initGive is "Подарить": the recipient picked as in the bank.
function initGive(me, cost) {
  const form = $('gift');
  const button = $('give-button');
  const picker = recipientPicker({
    to: $('give-to'), me, people: $('give-people'), shown: $('give-recipient'),
    picked: () => form.querySelector('input[name="give-period"]:checked')?.focus(),
  });
  periodChoices($('give-periods'), 'give-period', cost, button, (votes) => tr('Подарить за {0}', votes));

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    $('notice').hidden = true;
    button.disabled = true;

    try {
      const recipient = picker.found() ?? await picker.lookUp();
      if (!recipient) {
        showError(error, isNickname(transferNickname($('give-to').value)) ? tr('Получатель не найден.') : tr('Выберите получателя из списка.'));
        return;
      }

      const given = await api.givePrime(recipient.nickname, chosenPeriod($('give-periods')));
      me.votes = given.balance;
      showState(me);
      picker.clear();
      say(tr('Подписка подарена'), el('a', { href: pagePath(recipient), textContent: fullName(recipient) }),
        tr(' получает Prime до {0}.', formatDateTime(given.until)));
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
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
  const active = Boolean(activeUntil(me));

  $('style-about').textContent = active
    ? tr('Так ваша страница выглядит для всех, пока у вас есть Prime.')
    : tr('Примерьте оформление: сохранить его можно, когда у вас есть Prime.');
  $('style-save').hidden = !active;
  $('style-buy').hidden = active;
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
  $('tab-' + tab).classList.add('active');
  $('tab-' + tab).setAttribute('aria-current', 'page');

  try {
    const cost = await prices;
    showState(me);
    initBuy(me, cost);
    initGive(me, cost);
    initInvisible(me);
    initStyle(me);
    $('loading').hidden = true;
    $(tab === 'style' ? 'style-part' : tab).hidden = false;
  } catch (e) {
    $('loading').hidden = true;
    showError(error, e);
  }
}
