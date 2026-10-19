// Answers and reactions of messages in a dialog and a chat, as on VK:
// "Ответить" puts a line over the form with whom one answers, the answer
// shows over its words the message it answers, a click leading to it; and
// anyone in the conversation puts one reaction of six on a message, shown
// under it with how many put each.

import { REACTIONS, genitiveName, quoteText, reactionCounts, reacted } from './labels.js';
import { $, el, fullName, showError } from './layout.js';
import { tr } from './i18n.js';

// quoteNode is the block over an answer's words; text is the words of the
// message it answers as the page knows them, open leads to that message.
export function quoteNode(q, text, open) {
  return el('a', {
    className: 'im-quote',
    title: tr('Перейти к сообщению'),
    onclick: () => open(q.id),
  },
  q.author && el('span', { className: 'im-quote-name', textContent: fullName(q.author) }),
  el('span', { className: 'im-quote-text', textContent: quoteText(q, text) }));
}

// replyBar runs the line over the form: start(m, author, text) answers m,
// cancel stops; replyingTo is the message answered, null for none.
export function replyBar() {
  let replying = null;

  const cancel = () => {
    replying = null;
    $('replying').hidden = true;
  };
  $('replying-cancel').addEventListener('click', cancel);

  return {
    start(m, author, text) {
      replying = m;
      $('replying-name').textContent = genitiveName(author);
      $('replying-text').textContent = quoteText({ ...m, author, photoCount: m.photos?.length ?? 0, albumPhoto: Boolean(m.albumPhoto), post: Boolean(m.post) }, text);
      $('replying').hidden = false;
      $('text').focus();
    },
    cancel,
    replyingTo: () => replying,
  };
}

// reactionNodes are the reactions under a message and, while picking,
// the six to choose from; react(m, reaction) puts the caller's.
export function reactionNodes(m, userID, picking, react) {
  const counts = reactionCounts(m.reactions, userID);
  const mine = (m.reactions ?? []).find((r) => r.userId === userID)?.reaction ?? '';
  const chip = (r, label) => el('button', {
    type: 'button',
    className: r.name === mine ? 'im-reaction mine' : 'im-reaction',
    title: r.title,
    textContent: label,
    onclick: () => react(m, r.name === mine ? '' : r.name),
  });

  return [
    counts.length > 0 && el('div', { className: 'im-reactions' }, ...counts.map((r) => chip(r, `${r.emoji} ${r.count}`))),
    picking && el('div', { className: 'im-reaction-picker' }, ...REACTIONS.map((r) => chip(r, r.emoji))),
  ].filter(Boolean);
}

// reactWith makes react for a page: the node of the message shows the
// reaction at once and goes back when the server says no; put asks the
// server, error is where to say why.
export function reactWith({ shown, userID, put, error }) {
  return async (m, reaction) => {
    const node = shown.get(m.id);
    if (!node) return;

    const before = node.message.reactions;
    node.picking = false;
    node.message = { ...node.message, reactions: reacted(before, userID, reaction) };
    node.render();

    try {
      await put(m.id, reaction);
    } catch (e) {
      node.message = { ...node.message, reactions: before };
      node.render();
      showError(error, e);
    }
  };
}
