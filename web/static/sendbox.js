// "Отправить сообщение" of a page, as openvk's window (showUserDialog in
// Web/static/js/messages/related.js): the photo of the person with "В
// диалог" under it, their name and whether they are online, the field
// with the emoji and what may be attached at its right; "Закрыть" and
// "Отправить" under it. Sent, the window closes and says so. The message
// is sealed as in the dialog (e2ee.js).

import { initAttachments } from './attach.js';
import { initAudios } from './audio.js';
import { initDocuments } from './docs.js';
import { ensureKey, KEY_READY, sendSealed } from './e2ee.js';
import { initEmoji } from './emoji.js';
import { keyStateText, presence } from './labels.js';
import { avatarURL, el, fullName, hideError, showError } from './layout.js';
import { button, openModal, toast } from './modal.js';
import { tr } from './i18n.js';

// openSendBox opens the window to write to profile as me.
export function openSendBox(profile, me) {
  const dialog = '/im/' + encodeURIComponent(profile.nickname);
  const modal = openModal(tr('Отправить сообщение'));
  const error = el('div', { className: 'error', hidden: true });
  const text = el('textarea', { className: 'send-box-text', maxLength: 60000, placeholder: tr('Введите сообщение'), ariaLabel: tr('Сообщение') });
  const entry = el('a', { className: 'emoji-entry', title: tr('Смайлы') });
  const picker = el('div', { className: 'emoji-picker-wrap', hidden: true },
    el('div', { className: 'emoji-picker-scroll' }, el('div', { className: 'emoji-picker-groups' })),
    el('div', { className: 'emoji-picker-footer' },
      el('a', { className: 'emoji-main-tab active', title: tr('Смайлы') }, el('span', { className: 'emoji-icon' })),
      el('div', { className: 'emoji-tabs-nav' }, el('div', { className: 'emoji-category-nav' }))));
  const photo = el('a', { className: 'send-box-attach attach-photo', title: tr('Фотография') });
  const audio = el('a', { className: 'send-box-attach attach-audio', title: tr('Аудиозапись') });
  const doc = el('a', { className: 'send-box-attach attach-document', title: tr('Документ') });
  const file = el('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif,image/webp', multiple: true, hidden: true });
  const photos = el('div', { className: 'attachments', hidden: true });
  const documents = el('div', { className: 'doc-attached', hidden: true });
  const audios = el('div', { className: 'audio-list audio-attached', hidden: true });

  modal.box.classList.add('send-box');
  modal.body.append(error, el('div', { className: 'send-box-centre' },
    el('div', { className: 'send-box-left' },
      el('a', { href: '/' + profile.nickname }, el('img', { src: avatarURL(profile, 'large'), alt: '' })),
      el('a', { href: dialog, textContent: tr('В диалог') })),
    el('div', { className: 'send-box-right' },
      el('a', { className: 'send-box-name', href: '/' + profile.nickname, textContent: fullName(profile) }),
      el('div', { className: 'send-box-online', textContent: presence(profile) }),
      el('div', { className: 'messenger-field' }, text, entry, picker),
      photos, documents, audios,
      el('div', { className: 'send-box-attachments' }, photo, audio, doc),
      file)));

  const attached = initAttachments({ link: photo, input: file, list: photos, error, onChange: () => {} });
  const docs = initDocuments({ link: doc, list: documents });
  const songs = initAudios({ link: audio, list: audios });
  initEmoji({ link: entry, picker, input: text, bar: el('span'), stickersLink: el('a'), stickersPanel: el('div'), showStickers: () => {} });

  const send = button(tr('Отправить'), async () => {
    hideError(error);
    if (!text.value.trim() && attached.count() === 0 && docs.count() === 0 && songs.count() === 0) return;
    if (attached.busy()) {
      showError(error, tr('Дождитесь, пока загрузятся фотографии.'));
      return;
    }

    send.disabled = true;
    try {
      // Until this browser has its key nothing can be sealed.
      const state = await ensureKey(me.id);
      if (state !== KEY_READY) {
        showError(error, keyStateText(state));
        send.disabled = false;
        return;
      }

      await sendSealed(me.id, profile.nickname, text.value, { photos: attached.ids(), documents: docs.ids(), audios: songs.ids() });
      modal.close();
      toast(tr('Сообщение отправлено!'));
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  modal.actions.append(button(tr('Закрыть'), modal.close, { secondary: true }), send);
  text.focus();
}
