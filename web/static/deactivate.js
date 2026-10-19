// openvk's "Удаление страницы" box: why you leave (one of its reasons or
// your own words), "Удалить страницу" and "Отмена". A deleted page can be
// restored until it is erased.

import { api } from './api.js';
import { deactivationReasons } from './labels.js';
import { el, profilePath, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const MAX_LENGTH = 1000;

export function openDeactivate(me) {
  const modal = openModal(tr('Удаление страницы'), { tag: 'form' });
  const text = el('textarea', { maxLength: MAX_LENGTH, placeholder: tr('Ваше сообщение'), ariaLabel: tr('Ваше сообщение') });
  const error = el('div', { className: 'error', hidden: true });
  const remove = button(tr('Удалить страницу'), null, { type: 'submit' });

  // A reason puts its words in the message, as openvk does.
  const reasons = deactivationReasons.map(([title, words]) => el('label', { className: 'modal-option' },
    el('input', { type: 'radio', name: 'reason', onchange: () => { text.value = words; } }),
    title,
  ));

  modal.body.append(
    error,
    el('p', { className: 'hint', textContent: tr('Мы сожалеем, что Вы хотите удалить свою страницу. Поэтому, Вы можете указать причину удаления и Ваше сообщение по этому поводу. Мы читаем Ваши отзывы и пытаемся сделать сайт лучше!') }),
    el('b', { textContent: tr('Пожалуйста, укажите причину удаления Вашей страницы') }),
    ...reasons,
    text,
  );
  modal.actions.append(remove, button(tr('Отмена'), modal.close, { secondary: true }));

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    remove.disabled = true;

    try {
      await api.deactivate(text.value);
      location.href = profilePath(me);
    } catch (e) {
      showError(error, e);
      remove.disabled = false;
    }
  });
}
