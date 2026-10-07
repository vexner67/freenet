// openvk's "Поделиться" box: the audience (your wall, the only one here),
// your comment, "Отправить" and "Отмена".

import { api } from './api.js';
import { el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { postPath } from './posts.js';

const MAX_LENGTH = 1000;

// openShare shares post; onShared(repost) runs once it is on the wall.
export function openShare(post, onShared) {
  const modal = openModal('Поделиться', { tag: 'form' });
  const text = el('textarea', { maxLength: MAX_LENGTH, placeholder: '…', ariaLabel: 'Ваш комментарий' });
  const error = el('div', { className: 'error', hidden: true });
  const send = button('Отправить', null, { type: 'submit' });

  modal.body.append(
    error,
    el('b', { textContent: 'Аудитория' }),
    el('label', { className: 'modal-option' }, el('input', { type: 'radio', checked: true }), 'на стену'),
    el('b', { textContent: 'Ваш комментарий' }),
    text,
  );
  modal.actions.append(send, button('Отмена', modal.close, { secondary: true }));

  // done says where the repost is, as openvk's "Запись появится на вашей стене".
  function done(repost) {
    modal.body.replaceChildren(el('div', {},
      'Запись опубликована на вашей стене. ',
      el('a', { href: postPath(repost), textContent: 'Перейти к записи' }),
    ));
    modal.actions.replaceChildren(button('Закрыть', modal.close));
  }

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    try {
      const repost = await api.repost(post.id, text.value);
      onShared(repost);
      done(repost);
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  text.focus();
}
