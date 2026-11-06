// What dialogs and chats of several people share: the messages a message
// carries on, shown under its words, and VK's "Переслать" — a window that
// sends messages on into a dialog or a chat. Messages of dialogs are
// sealed (e2ee.js): they go on into dialogs only.

import { api } from './api.js';
import { sendSealed } from './e2ee.js';
import { chatTitle, conversationOf, conversationPath, conversationValue, fileSize, formatPostDate, forwardTree, messagePreview, photoMosaic, snippet, splitVideos } from './labels.js';
import { avatarURL, el, fullName, isPhone, pagePath, showError, hideError, smileyNodes } from './layout.js';
import { button, openModal, openSheet } from './modal.js';
import { dots, postPath, sharedNode, sharedPhotoNode } from './posts.js';
import { openViewer } from './viewer.js';
import { stickerNode } from './stickers.js';
import { audiosNode } from './audio.js';
import { videoNode } from './video.js';
import { tr } from './i18n.js';

// attachedNodes is what a message shows under its words, as openvk's
// MessageAttachments (Web/static/js/messages/components/message.js): the
// sticker it is, its photos in one mosaic (makimaNode), its videos, then
// its documents, songs and the post or the photo of an album it shares.
export function attachedNodes(m) {
  const photos = m.photos ?? [];
  const docs = m.documents ?? [];
  const { videos } = m.deleted ? { videos: [] } : splitVideos(m.text);

  return [
    m.sticker ? stickerNode(m.sticker) : null,
    photos.length ? el('div', { className: 'msg-attachments-container' }, makimaNode(photos)) : null,
    videos.length ? el('div', { className: 'attachments msg-attach-videos' }, ...videos.map(videoNode)) : null,
    docs.length ? el('div', { className: 'msg-attach-docs' }, ...docs.map(docNode)) : null,
    m.audios?.length ? el('div', { className: 'msg-attach-audio-player' }, audiosNode(m.audios)) : null,
    m.repost ? wallCardNode(m.repost) : null,
    m.sharedPhoto ? sharedPhotoNode(m.sharedPhoto) : null,
  ];
}

// The shapes (width / height) of the photos seen on this page, by their
// address: a photo laid out again on the same page does not move.
const shapes = new Map();

// shapeOf resolves with the shape of a photo once its picture is loaded:
// ours carry no size, so the browser tells it.
function shapeOf(src) {
  if (shapes.has(src)) return Promise.resolve(shapes.get(src));

  return new Promise((resolve) => {
    const probe = new Image();
    probe.onload = () => {
      const r = probe.naturalWidth / probe.naturalHeight;
      shapes.set(src, r);
      resolve(r);
    };
    probe.onerror = () => resolve(NaN);
    probe.src = src;
  });
}

// makimaNode is openvk's MessageMakimaGrid with the photos seen whole, as
// Telegram shows them: laid out as 4 by 3 until their pictures tell their
// shapes, then each at its own (photoMosaic); a click opens them one by one.
function makimaNode(photos) {
  const shown = photos.slice(0, 10);
  const tiles = shown.map((photo, i) => el('div', { className: 'msg-media-tile' },
    el('a', {
      className: 'msg-media-tile-link',
      href: photo.large,
      onclick: (event) => {
        event.preventDefault();
        openViewer(photos, i);
      },
    }, el('img', { className: 'media_makima', src: photo.small, alt: '' }))));
  const grid = el('div', { className: 'msg-makima-grid' });

  // On a phone the box is what the bubble leaves of the screen.
  const box = isPhone() ? Math.min(380, innerWidth - 100) : 380;
  const lay = (ratios) => {
    const mosaic = photoMosaic(ratios, { width: box });
    let next = 0;
    grid.replaceChildren(...mosaic.rows.map((row) => {
      const line = el('div', { className: 'msg-makima-row' }, ...row.widths.map((w) => {
        const tile = tiles[next++];
        // CSSOM, not a style attribute: the pages' CSP forbids those.
        tile.style.width = `${w}px`;
        return tile;
      }));
      line.style.height = `${row.height}px`;
      return line;
    }));
    grid.style.width = `${mosaic.width}px`;
  };

  lay(shown.map((p) => shapes.get(p.small)));
  if (shown.some((p) => !shapes.has(p.small))) Promise.all(shown.map((p) => shapeOf(p.small))).then(lay);

  return grid;
}

// docNode is a document of a message as openvk's msg-attach-w-doc: the
// sheet with a folded corner and its name, which downloads it; a picture
// is shown as one (docImageNode).
function docNode(doc) {
  if (doc.preview) return docImageNode(doc);

  return el('div', { className: 'msg-attach-w msg-attach-w-doc' },
    el('a', { className: 'attachment_note attachment_doc', href: doc.url, download: doc.name },
      el('span', { className: 'attachment_doc_icon' }),
      el('span', { className: 'attachment_note_content' },
        el('span', { className: 'attachment_note_name', textContent: doc.name }))));
}

// docImageNode is a document that is a picture, as openvk's
// DocImageAttachment: the picture in a black box 280 by 190, its name and
// size on a dark band at its bottom; a click shows it whole.
function docImageNode(doc) {
  return el('div', { className: 'msg-attach-w msg-attach-w-doc-image' },
    el('a', {
      className: 'docGalleryItem',
      href: doc.url,
      onclick: (event) => {
        event.preventDefault();
        openViewer([{ large: doc.preview }], 0);
      },
    },
    el('img', { className: 'docGalleryItem_main_preview', src: doc.preview, alt: doc.name, loading: 'lazy' }),
    el('div', { className: 'doc_bottom_panel' },
      el('span', { className: 'doc_bottom_panel_name', textContent: doc.name }),
      el('span', { className: 'doc_bottom_panel_size', textContent: fileSize(doc.size) }))));
}

// wallCardNode is a post a message shares, as openvk's msg-attach-wall-card:
// a grey card with a bar at its left, the photo, name and date of its
// author, six lines of its words and its photos; a click opens the post.
function wallCardNode(post) {
  if (post.deleted) return el('div', { className: 'msg-attach-wall-card wall-card-deleted', textContent: tr('Запись удалена.') });
  if (post.fediverse) return sharedNode(post, { canVote: true });

  const path = pagePath(post.author);
  const href = postPath(post);
  const card = el('div', { className: 'msg-attach-wall-card' },
    el('div', { className: 'wall-card-head' },
      el('a', { className: 'wall-card-avatar-link', href: path }, el('img', { className: 'wall-card-avatar', src: avatarURL(post.author, 'small'), alt: '' })),
      el('div', { className: 'wall-card-author-info' },
        el('a', { className: 'wall-card-author-name', href: path, textContent: fullName(post.author) }),
        el('span', { className: 'wall-card-date', textContent: formatPostDate(post.createdAt) }))),
    post.text
      ? el('div', { className: 'wall-card-text' }, ...smileyNodes(post.text))
      : el('div', { className: 'wall-card-text wall-card-empty', textContent: tr('Запись') }),
    post.photos?.length ? makimaNode(post.photos) : null,
    post.documents?.length || post.audios?.length
      ? el('div', { className: 'attachments_m' }, ...(post.documents ?? []).map(docNode), audiosNode(post.audios))
      : null,
  );

  card.addEventListener('click', (event) => {
    if (event.target.closest('a, button, .audio-row')) return;
    location.href = href;
  });

  return card;
}

// forwardedNode shows the messages m carries on, as on VK: under its own
// words, each with who wrote it and when, and under each what it carried
// on itself. textOf draws a text as the page draws those of its messages.
export function forwardedNode(m, textOf) {
  const forwarded = m.forwarded ?? [];
  if (!m.forwardedCount) return null;

  const carried = (trees) => el('div', { className: 'im-forwarded' },
    ...trees.map(({ message: f, carried: under }) => el('div', { className: 'im-forwarded-message' },
      el('a', { className: 'post-name', href: '/' + f.author.nickname, textContent: fullName(f.author) }),
      el('span', { className: 'im-forwarded-date', textContent: formatPostDate(f.createdAt) }),
      splitVideos(f.text).text ? textOf(splitVideos(f.text).text) : null,
      ...attachedNodes(f),
      under.length ? carried(under) : null,
    )));

  const shown = carried(forwardTree(forwarded));
  // Those of people whose pages are gone are not shown.
  if (forwarded.length < m.forwardedCount) {
    shown.append(el('div', { className: 'im-forwarded-hint', textContent: tr('Пересланное сообщение недоступно.') }));
  }

  return shown;
}

// actionIcons are the icons of the actions of a message, as those of
// "Прикрепить" (static/icons): the class of the link names its picture.
const actionIcons = {
  Ответить: 'action-reply',
  Реакция: 'action-reaction',
  Редактировать: 'action-edit',
  Закрепить: 'action-pin',
  Открепить: 'action-unpin',
  Переслать: 'action-forward',
  Удалить: 'action-delete',
};

// actionsNode is what can be done with a message, under its date: one
// "⋯" opening the list, as the footer of a post has, as "Прикрепить" of a form does, so
// that the actions do not stretch the message; each is a link with its
// icon. actions are [name, click] pairs; those not available are falsy.
export function actionsNode(actions) {
  const shown = actions.filter(Boolean);
  if (!shown.length) return null;

  // A phone lists them over the bottom of the screen, as its apps do.
  if (isPhone()) {
    return el('a', {
      className: 'im-message-action message-actions',
      title: tr('Действия'),
      onclick: () => openSheet(shown.map(([name, run]) => ({ text: name, run, danger: name === tr('Удалить') }))),
    }, dots());
  }

  return el('span', { className: 'attach-menu attach-auto message-actions' },
    el('a', { className: 'attach-toggle im-message-action', title: tr('Действия') }, dots()),
    el('span', { className: 'attach-list' },
      ...shown.map(([name, onclick]) => el('a', { className: `attach-link ${actionIcons[name] ?? ''}`.trim(), textContent: name, onclick }))));
}

// takenBack is a message after it was deleted for everyone: it says
// nothing.
export function takenBack(m) {
  return {
    ...m, text: '', photos: [], post: '', sticker: '', repost: undefined, albumPhoto: '', sharedPhoto: undefined, forwarded: [], forwardedCount: 0, documents: [], documentCount: 0, audios: [], audioCount: 0,
    editedAt: null, deletableUntil: '', editableUntil: '', deleted: true, reactions: [], replyTo: undefined, quoted: '',
  };
}

// conversationOption is a dialog or a chat as a choice of where to send.
export function conversationOption(d) {
  return el('option', { value: conversationValue(d), textContent: d.chat ? chatTitle(d.chat) : fullName(d.peer) });
}

// sendTo writes into the dialog or the chat a choice names: into a
// dialog sealed for its two people, me (the caller, asked for when not
// given) and the peer. forward are the messages carried on.
export async function sendTo(value, text, { me = null, photos = [], post = '', forward = [], albumPhoto = '' } = {}) {
  const { chat, ref } = conversationOf(value);
  if (chat) return api.sendChatMessage(ref, text, photos, post, forward.map((m) => m.id), albumPhoto);

  const caller = me ?? await api.me();

  return sendSealed(caller.id, ref, text, { photos, post, forward, albumPhoto });
}

// fromDialog: a message of a dialog, not of a chat.
const fromDialog = (m) => !m.chatId;

// openForward carries messages on: which ones, where to ("Кому": this
// conversation, your latest dialogs and chats, or a person found by
// name), your words to go with them, "Отправить" and "Отмена". Messages
// taken back and lines of what happened in a chat have nothing to carry.
// here is the conversation of the page ({ peer } or { chat }), authorOf
// tells who wrote a message; onSent(message) runs when they went here,
// onDone always.
export function openForward({ me, messages, here, authorOf, onSent, onDone = () => {} }) {
  const forward = messages.filter((m) => !m.deleted && !m.action).sort((a, b) => (a.id < b.id ? -1 : 1));
  if (forward.length === 0) return;

  // What a dialog says stays sealed: it goes on into dialogs only.
  const sealed = forward.some(fromDialog);
  const takes = (d) => writable(d) && !(sealed && d.chat);
  const modal = openModal(forward.length > 1 ? tr('Пересылка сообщений') : tr('Пересылка сообщения'), { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const hereValue = conversationValue(here);
  // A chat I am not in anymore takes nothing from me.
  const first = takes(here) ? [conversationOption(here)] : [];
  const chats = el('select', { ariaLabel: tr('Переписка') }, ...first);
  const person = el('input', { type: 'search', maxLength: 64, placeholder: tr('Найти человека: имя, фамилия или никнейм'), ariaLabel: tr('Найти человека') });
  const text = el('textarea', { maxLength: 60000, placeholder: '…', ariaLabel: tr('Ваш комментарий') });
  const send = button(tr('Отправить'), null, { type: 'submit' });

  modal.body.append(
    error,
    el('div', { className: 'forward-list' }, ...forward.map((m) => el('div', {},
      el('b', { textContent: fullName(authorOf(m)) + ': ' }), ...smileyNodes(snippet(messagePreview(m), 80))))),
    el('b', { textContent: tr('Кому') }),
    ...(sealed ? [el('div', { className: 'hint', textContent: tr('Сообщения диалогов зашифрованы: переслать их можно только в диалог.') })] : []),
    person,
    chats,
    el('b', { textContent: tr('Ваш комментарий') }),
    text,
  );
  modal.actions.append(send, button(tr('Отмена'), modal.close, { secondary: true }));

  // The latest dialogs and chats, this one first.
  let latest = first;

  api.dialogs().then((page) => {
    latest = [...first, ...page.dialogs.filter((d) => conversationValue(d) !== hereValue && takes(d)).map(conversationOption)];
    if (!person.value.trim()) chats.replaceChildren(...latest);
  }).catch((e) => showError(error, e));

  // Anyone else is found by name, as the search of people finds them;
  // only the answer to what the field says now is shown.
  let timer;

  person.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const q = person.value.trim();

      hideError(error);
      if (!q) {
        chats.replaceChildren(...latest);
        send.disabled = false;
        return;
      }

      try {
        const page = await api.search(q);
        if (person.value.trim() !== q) return;

        const people = page.users.filter((u) => u.nickname !== me.nickname);

        chats.replaceChildren(...(people.length > 0
          ? people.map((peer) => conversationOption({ peer }))
          : [el('option', { value: '', textContent: tr('Никого не нашлось') })]));
        send.disabled = people.length === 0;
      } catch (e) {
        showError(error, e);
      }
    }, 300);
  });

  modal.box.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    send.disabled = true;

    try {
      const to = chats.value;
      if (!to) throw tr('Выберите переписку.');

      const sent = await sendTo(to, text.value, { me, forward });

      onDone();
      if (to !== hereValue) {
        modal.body.replaceChildren(el('div', {},
          forward.length > 1 ? tr('Сообщения пересланы. ') : tr('Сообщение переслано. '),
          el('a', { href: conversationPath(to), textContent: tr('Перейти') })));
        modal.actions.replaceChildren(button(tr('Закрыть'), modal.close));
        return;
      }

      modal.close();
      onSent(sent);
    } catch (e) {
      showError(error, e);
      send.disabled = false;
    }
  });

  text.focus();
}

// writable: a chat takes messages only from who is in it.
export function writable(d) {
  return !d.chat || d.chat.state === 'active';
}

// The wait that makes two clicks one double click.
const DOUBLE_CLICK_MS = 400;

// selectOnDoubleClick calls select when a message is clicked twice in a
// row — a double click of a mouse or a double tap of a finger — not on a
// link, a photo or a button in it: a single click does nothing, so that
// reading and tapping links does not select, and the double click selects
// no words of the text. While selecting() — some messages are selected —
// one click is enough, as on VK, and a double one counts once.
export function selectOnDoubleClick(node, select, selecting) {
  let last = -Infinity;
  const onMessage = (event) => !event.target.closest('a, img, button, input, textarea');

  node.addEventListener('mousedown', (event) => {
    if (event.detail > 1 && onMessage(event)) event.preventDefault();
  });
  node.addEventListener('click', (event) => {
    if (!onMessage(event)) return;

    const double = event.timeStamp - last < DOUBLE_CLICK_MS;

    if (selecting()) {
      if (double) return;
      last = event.timeStamp;
      select();
    } else if (double) {
      last = event.timeStamp;
      window.getSelection()?.removeAllRanges();
      select();
    } else {
      // Words picked out by dragging are not a first click.
      last = window.getSelection()?.toString() ? -Infinity : event.timeStamp;
    }
  });
}
