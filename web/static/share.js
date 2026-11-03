// openvk's "Поделиться" box: the audience — your wall, a community you
// run ("в группу", with "От имени сообщества" and "Подпись автора") or a
// dialog or a chat ("в переписку") — your comment, with photos if you share a post,
// "Отправить" and "Отмена".

import { api } from './api.js';
import { initAttachments } from './attach.js';
import { conversationOption, sendTo, writable } from './forward.js';
import { conversationPath } from './labels.js';
import { clubRef, el, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { postPath } from './posts.js';
import { tr } from './i18n.js';

const MAX_LENGTH = 60000;

// openShare shares post, or with share a photo of an album (api.sharePhoto,
// which takes no photos of yours); onShared(repost) runs once it is on a
// wall. A post of another server, which the site cannot repost, goes with
// link, its address: after your words, where the card of the page shows it.
export function openShare(post, onShared, { share = api.repost, link = '' } = {}) {
  const ofPhoto = share !== api.repost;
  const modal = openModal(tr('Поделиться'), { tag: 'form' });
  const text = el('textarea', { maxLength: MAX_LENGTH, placeholder: '…', ariaLabel: tr('Ваш комментарий') });
  const error = el('div', { className: 'error', hidden: true });
  const send = button(tr('Отправить'), null, { type: 'submit' });
  const list = el('div', { className: 'attachments', hidden: true });
  const input = el('input', { type: 'file', accept: 'image/jpeg,image/png,image/gif,image/webp', multiple: true, hidden: true });
  const attachLink = el('a', { className: 'attach-link', textContent: tr('Прикрепить фотографию') });
  const attachments = ofPhoto ? null : initAttachments({ link: attachLink, input, list, error, onChange: () => {} });

  const audience = (value, label, checked = false) => el('label', { className: 'modal-option' },
    el('input', { type: 'radio', name: 'share-to', value, checked }), label);
  const clubs = el('select', { hidden: true, ariaLabel: tr('Группа') });
  const chats = el('select', { hidden: true, ariaLabel: tr('Переписка') });
  const fromClub = el('input', { type: 'checkbox' });
  const signed = el('input', { type: 'checkbox' });
  const signs = el('div', { className: 'share-signs', hidden: true },
    el('label', {}, fromClub, tr(' От имени сообщества')),
    el('label', {}, signed, tr(' Подпись автора')));

  modal.body.append(
    error,
    el('b', { textContent: tr('Аудитория') }),
    audience('wall', tr('на стену'), true),
    audience('club', tr('в группу')),
    clubs,
    audience('chat', tr('в переписку')),
    chats,
    el('b', { textContent: tr('Ваш комментарий') }),
    text,
    ...(ofPhoto ? [] : [list, el('div', {}, attachLink), input]),
    signs,
  );
  modal.actions.append(send, button(tr('Отмена'), modal.close, { secondary: true }));

  const target = () => modal.box.querySelector('input[name="share-to"]:checked').value;

  modal.box.addEventListener('change', async (event) => {
    if (event.target.name !== 'share-to') return;

    clubs.hidden = chats.hidden = signs.hidden = true;
    if (target() === 'club') {
      clubs.hidden = signs.hidden = false;
      await fillClubs(clubs, error);
    }
    if (target() === 'chat') {
      chats.hidden = false;
      await fillChats(chats, error);
    }
  });

  // done says where the post went, as openvk's notice after sharing.
  function done(where, href) {
    modal.body.replaceChildren(el('div', {}, where + ' ', el('a', { href, textContent: tr('Перейти') })));
    modal.actions.replaceChildren(button(tr('Закрыть'), modal.close));
  }

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    if (attachments?.busy()) {
      showError(error, tr('Дождитесь, пока загрузятся фотографии.'));
      return;
    }
    send.disabled = true;

    const photos = attachments?.ids() ?? [];

    try {
      if (link) {
        const repost = await shareLink(target(), target() === 'club' ? clubs.value : chats.value, withLink(text.value, link),
          { photos, fromClub: fromClub.checked, signed: signed.checked });
        if (target() === 'chat') {
          done(tr('Запись отправлена в переписку.'), conversationPath(chats.value));
          return;
        }
        onShared(repost);
        done(target() === 'club' ? tr('Запись опубликована в группе.') : tr('Запись опубликована на вашей стене.'), postPath(repost));
        return;
      }

      if (target() === 'chat') {
        if (!chats.value) throw tr('Выберите переписку.');
        await sendTo(chats.value, text.value, ofPhoto ? { albumPhoto: post.id } : { photos, post: post.id });
        done(ofPhoto ? tr('Фотография отправлена в переписку.') : tr('Запись отправлена в переписку.'), conversationPath(chats.value));
        return;
      }

      const where = target() === 'club' ? clubs.value : '';
      if (target() === 'club' && !where) throw tr('Выберите группу.');

      const repost = await share(post.id, text.value, {
        ...(where && { club: where, fromClub: fromClub.checked, signed: signed.checked }),
        ...(ofPhoto ? {} : { photos }),
      });
      onShared(repost);
      done(where ? tr('Запись опубликована в группе.') : tr('Запись опубликована на вашей стене.'), postPath(repost));
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  text.focus();
}

// withLink puts the address of a post of another server after your words.
function withLink(text, link) {
  const words = text.trim();
  return words ? words + '\n\n' + link : link;
}

// shareLink writes text, the address of a post of another server with your
// words, as a post on your wall or of a community where, or as a message
// to the conversation where.
async function shareLink(to, where, text, { photos, fromClub, signed }) {
  if (to === 'chat') {
    if (!where) throw tr('Выберите переписку.');
    return sendTo(where, text, { photos });
  }

  if (to === 'club') {
    if (!where) throw tr('Выберите группу.');
    return api.createClubPost(where, text, photos, { fromClub, signed });
  }

  const me = await api.me();
  return api.createPost(me.nickname, text, photos);
}

// fillClubs lists the communities you run, as openvk offers them, once.
async function fillClubs(select, error) {
  if (select.options.length) return;

  try {
    const page = await api.myClubs('', true);
    select.replaceChildren(...page.clubs.map((club) => el('option', { value: clubRef(club), textContent: club.name })));
    if (page.clubs.length === 0) select.replaceChildren(el('option', { value: '', textContent: tr('Вы не управляете ни одной группой') }));
  } catch (e) {
    showError(error, e);
  }
}

// fillChats lists your latest dialogs and chats, once.
async function fillChats(select, error) {
  if (select.options.length) return;

  try {
    const page = await api.dialogs();
    const dialogs = page.dialogs.filter(writable);
    select.replaceChildren(...dialogs.map(conversationOption));
    if (dialogs.length === 0) select.replaceChildren(el('option', { value: '', textContent: tr('У вас ещё нет переписок') }));
  } catch (e) {
    showError(error, e);
  }
}
