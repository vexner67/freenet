// Editing a post in place, as on openvk's "Редактировать": the text and
// photos give way to a form with "Сохранить" and "Отмена". A repost edits
// only its own words: the shared post stays as it is.

import { api } from './api.js';
import { initAttachments } from './attach.js';
import { el, showError, hideError, autosize } from './layout.js';

const MAX_LENGTH = 60000;

// editPost opens the form in node, the post's; onSaved gets the edited post.
export function editPost(post, node, { onSaved }) {
  const isRepost = Boolean(post.repost);

  const text = el('textarea', { value: post.text, maxLength: MAX_LENGTH, placeholder: 'Текст записи' });
  const error = el('div', { className: 'error', hidden: true });
  const list = el('div', { className: 'attachments', hidden: true });
  const input = el('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif,image/webp', multiple: true, hidden: true });
  const attachLink = el('a', { className: 'attach-link', textContent: 'Прикрепить фотографию' });
  const save = el('button', { className: 'button', type: 'submit', textContent: 'Сохранить' });
  const cancel = el('button', { className: 'button secondary', type: 'button', textContent: 'Отмена' });

  const attachments = isRepost
    ? null
    : initAttachments({ link: attachLink, input, list, error, onChange: () => {}, photos: post.photos ?? [] });

  const form = el('form', { className: 'wall-form expanded post-edit' },
    text,
    isRepost ? null : list,
    el('div', { className: 'row' },
      el('span', {}, isRepost ? null : attachLink),
      el('span', { className: 'post-edit-buttons' }, save, cancel),
    ),
    input,
    error,
  );

  function close() {
    form.remove();
    node.classList.remove('editing');
  }

  async function submit(event) {
    event.preventDefault();
    hideError(error);

    if (attachments?.busy()) {
      showError(error, 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    save.disabled = true;

    try {
      const edited = await api.editPost(post.id, text.value, attachments?.ids() ?? []);
      close();
      onSaved(edited);
    } catch (e) {
      showError(error, e);
    } finally {
      save.disabled = false;
    }
  }

  form.addEventListener('submit', submit);
  cancel.addEventListener('click', close);
  text.addEventListener('keydown', (event) => {
    // Ctrl+Enter (Cmd+Enter on a Mac) saves, as the wall form sends.
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
    if (event.key === 'Escape') close();
  });

  node.classList.add('editing');
  node.querySelector('.post-content').after(form);
  autosize(text);
  text.focus();
  text.setSelectionRange(text.value.length, text.value.length);
}
