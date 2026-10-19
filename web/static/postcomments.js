// The comments under a post of a wall in openvk's "Микроблог": the three
// newest, "Показать остальные комментарии (N)" leading to the page of the
// post, and a field to answer in. The page of the post shows them all
// (post.js).

import { api } from './api.js';
import { otherComments } from './labels.js';
import { autosize, el } from './layout.js';
import { commentNode } from './comments.js';
import { tr } from './i18n.js';

// lastComments draws them for post, which came with lastComments. Options:
//   commenter: the signed-in viewer, null for anonymous; with a filled-in
//     profile they answer here, unless the comments are closed;
//   moderates: the viewer keeps the wall clean and deletes any comment, as
//     the author of the post does;
//   path: the page of the post;
//   canLike, onError: as of a post;
//   onChanged() runs when a comment is added or deleted.
// It returns the node, open() that shows the field ("Комментировать") and
// canOpen() that tells whether there is a field to show.
export function lastComments(post, { commenter = null, moderates = false, path, canLike = false, onError, onChanged } = {}) {
  const shown = post.lastComments ?? [];
  // Those on the page of the post only: what is not shown here.
  let others = Math.max(0, post.comments - shown.length);

  const more = el('a', { className: 'post-comments-more', href: path });
  const list = el('div', { className: 'post-comments-list' });
  const text = el('textarea', { className: 'post-comment-text', rows: 1, maxLength: 60000, placeholder: tr('Комментировать...') });
  const submit = el('button', { type: 'submit', className: 'button', textContent: tr('Отправить') });
  const form = el('form', { className: 'post-comment-form', hidden: true }, text, submit);
  const node = el('div', { className: 'post-comments' }, more, list, form);

  const mayWrite = () => Boolean(commenter?.profileCompleted) && !post.commentsClosed;

  function render() {
    more.textContent = otherComments(others);
    more.hidden = others === 0;
    if (!mayWrite()) form.hidden = true;
    else if (list.children.length > 0) form.hidden = false;
    node.hidden = more.hidden && list.children.length === 0 && form.hidden;
  }

  const deletes = (comment) => Boolean(commenter) && (comment.mine || post.mine || moderates || (post.wall?.id ?? post.author.id) === commenter.id);

  function draw(comment) {
    return commentNode(comment, {
      canLike,
      onError,
      href: path,
      onDelete: deletes(comment) && remove,
    });
  }

  async function remove(comment, drawn) {
    try {
      await api.deleteComment(comment.id);
    } catch (e) {
      onError?.(e);
      return;
    }

    drawn.remove();
    post.comments--;
    render();
    onChanged?.();
  }

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!text.value.trim()) return;

    submit.disabled = true;

    try {
      list.append(draw(await api.createComment(post.id, text.value)));
      text.value = '';
      autosize(text);
      post.comments++;
      render();
      onChanged?.();
    } catch (e) {
      onError?.(e);
    } finally {
      submit.disabled = false;
    }
  });

  list.append(...shown.map(draw));
  render();

  return {
    node,
    render,
    canOpen: () => mayWrite() && form.hidden,
    open() {
      form.hidden = false;
      node.hidden = false;
      text.focus();
    },
  };
}
