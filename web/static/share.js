// openvk's "Поделиться" box: the audience (your wall, the only one here),
// your comment, "Отправить" and "Отмена". Esc or a click beside the box
// closes it.

import { api } from './api.js';
import { postPath } from './posts.js';
import { el, showError, hideError } from './layout.js';

const MAX_LENGTH = 1000;

// openShare shares post; onShared(repost) runs once it is on the wall.
export function openShare(post, onShared) {
  const text = el('textarea', { maxLength: MAX_LENGTH, placeholder: '…', ariaLabel: 'Ваш комментарий' });
  const error = el('div', { className: 'error', hidden: true });
  const send = el('button', { className: 'button', type: 'submit', textContent: 'Отправить' });
  const cancel = el('button', { className: 'button secondary', type: 'button', textContent: 'Отмена', onclick: hide });

  const form = el('form', { className: 'modal-box' },
    el('div', { className: 'modal-header', textContent: 'Поделиться' }),
    el('div', { className: 'modal-body' },
      error,
      el('b', { textContent: 'Аудитория' }),
      el('label', { className: 'modal-option' }, el('input', { type: 'radio', checked: true }), 'на стену'),
      el('b', { textContent: 'Ваш комментарий' }),
      text,
    ),
    el('div', { className: 'modal-actions' }, send, cancel),
  );
  const overlay = el('div', { className: 'modal' }, form);

  async function submit(event) {
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
  }

  // done says where the repost is, as openvk's "Запись появится на вашей стене".
  function done(repost) {
    form.querySelector('.modal-body').replaceChildren(el('div', {},
      'Запись опубликована на вашей стене. ',
      el('a', { href: postPath(repost), textContent: 'Перейти к записи' }),
    ));
    form.querySelector('.modal-actions').replaceChildren(
      el('button', { className: 'button', type: 'button', textContent: 'Закрыть', onclick: hide }),
    );
  }

  function onKey(event) {
    if (event.key === 'Escape') hide();
  }

  function hide() {
    overlay.remove();
    document.removeEventListener('keydown', onKey);
  }

  form.addEventListener('submit', submit);
  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) hide();
  });
  document.addEventListener('keydown', onKey);
  document.body.append(overlay);
  text.focus();
}
