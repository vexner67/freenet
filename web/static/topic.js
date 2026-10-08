// A topic at /topic/<id>, as openvk's "Просмотр темы": the title, who
// started it and when, "В теме N сообщений", the messages oldest first and
// the form; "Редактировать тему" for its author and the admins. Nobody
// writes in a closed topic, only the admins in a restricted one.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { commentNode } from './comments.js';
import { formatPostDate, topicMessagesCount } from './labels.js';
import { $, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { boardTitle, topicPath } from './topics.js';

const COUNTER_FROM = 900;
const MAX_LENGTH = 1000;

const topicID = decodeURIComponent(location.pathname.slice('/topic/'.length));

function renderHead(topic, club, canEdit) {
  document.title = `Тема «${topic.title}» | Freenet`;
  $('title').replaceChildren(...boardTitle(club, 'Просмотр темы'));
  if (canEdit) {
    $('title-links').replaceChildren(el('a', { href: topicPath(topic) + '/edit', textContent: 'Редактировать тему' }));
  }

  $('topic-title').textContent = topic.title;
  $('topic-author').textContent = fullName(topic.author);
  $('topic-author').href = pagePath(topic.author);
  $('topic-created').textContent = 'Создано ' + formatPostDate(topic.createdAt);
}

// writeRefusal is why the viewer may not write, null when they may.
function writeRefusal(topic, me, admin) {
  if (topic.closed) return 'Тема закрыта.';
  if (topic.restricted && !admin) return 'Комментировать могут только администраторы.';
  if (!me) return null;
  if (!me.profileCompleted) return 'Чтобы писать в обсуждениях, заполните свою страницу.';

  return null;
}

function initMessages(topic, me, admin) {
  const list = $('messages');
  const error = $('messages-error');

  let total = 0;
  let cursor = '';
  let attachments;
  // Messages sent from this page go last; pages loaded later skip them.
  const sent = new Set();

  function updateCount() {
    $('topic-count').textContent = topicMessagesCount(total);
  }

  const refusal = writeRefusal(topic, me, admin);
  const canWrite = Boolean(me) && refusal === null;

  // Its author and the admins delete a message; mine tells the author a
  // message for the community.
  const canDelete = (m) => Boolean(me) && (admin || m.mine);
  const messageNode = (m) => commentNode(m, {
    onDelete: canDelete(m) && remove,
    onReply: canWrite && reply,
    withLike: false,
  });

  // reply starts an answer as openvk's "Ответить": the name of the author
  // first.
  function reply(m) {
    const text = $('message-text');
    text.value = m.author.name + ', ' + text.value;
    text.focus();
    text.setSelectionRange(text.value.length, text.value.length);
  }

  async function remove(m, node) {
    hideError(error);

    try {
      await api.deleteTopicMessage(m.id);
    } catch (e) {
      showError(error, e);
      return;
    }

    node.replaceWith(el('div', { className: 'post-deleted', textContent: 'Сообщение удалено.' }));
    total--;
    updateCount();
  }

  async function loadMore() {
    hideError(error);

    try {
      const page = await api.topicMessages(topic.id, cursor);
      const fresh = page.messages.filter((m) => !sent.has(m.id));
      const firstSent = list.querySelector('[data-sent]');

      for (const m of fresh) list.insertBefore(messageNode(m), firstSent);

      total = page.total;
      cursor = page.nextCursor;
      $('messages-more').hidden = !cursor;
      updateCount();
    } catch (e) {
      showError(error, e);
    }
  }

  function updateCounter() {
    const left = MAX_LENGTH - $('message-text').value.length;
    $('message-counter').textContent = left <= MAX_LENGTH - COUNTER_FROM ? `осталось ${left}` : '';
  }

  async function submit(event) {
    event.preventDefault();
    hideError(error);

    if (attachments.busy()) {
      showError(error, 'Дождитесь, пока загрузятся фотографии.');
      return;
    }

    const button = $('message-submit');
    button.disabled = true;

    try {
      const m = await api.writeTopicMessage(topic.id, $('message-text').value, attachments.ids(), {
        fromClub: admin && $('message-from-club').checked,
      });
      const node = messageNode(m);

      node.dataset.sent = '';
      sent.add(m.id);
      list.append(node);
      $('message-text').value = '';
      attachments.clear();
      total++;
      updateCount();
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  if (canWrite) {
    attachments = initAttachments({
      link: $('message-attach'),
      input: $('message-file'),
      list: $('message-photos'),
      error,
      onChange: () => {},
    });
    $('message-from-club-label').hidden = !admin;
    $('message-text').addEventListener('input', updateCounter);
    $('message-form').addEventListener('submit', submit);
    $('message-form').hidden = false;
  } else if (refusal) {
    $('message-hint').textContent = refusal;
    $('message-hint').hidden = false;
  }

  $('messages-more').addEventListener('click', loadMore);

  return loadMore();
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const { topic, club, role } = await api.topic(topicID);
  const admin = role === 'admin' || role === 'owner';

  renderHead(topic, club, Boolean(me) && (topic.mine || admin));
  $('content').hidden = false;
  await initMessages(topic, me, admin);
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Тема не найдена' : 'Ошибка';
  showError($('error'), e);
}
