// The page of an article at /article/<id>, as VK's reader of "Статьи":
// the cover, the title, who wrote it, when and how long it is read, then
// the text. Anybody with the link reads it; its author edits and deletes
// it; whoever is signed in shares it on their wall.

import { api, currentUser, started } from './api.js';
import { articlePath, articleURL, blockNodes, minutesText } from './articles.js';
import { formatDateTime } from './labels.js';
import { $, avatarURL, el, fullName, pagePath, renderChrome, showError, verifiedMark } from './layout.js';
import { button, openModal, toast } from './modal.js';
import { tr } from './i18n.js';

const articleID = decodeURIComponent(location.pathname.slice('/article/'.length));

function renderArticle(article) {
  document.title = `${article.title} | Freenet`;
  $('title').textContent = tr('Статья');

  const cover = $('article-cover');
  cover.hidden = !article.cover;
  if (article.cover) cover.replaceChildren(el('img', { src: article.cover.large, alt: '' }));

  $('article-title').textContent = article.title;

  const author = article.author;
  $('article-byline').replaceChildren(
    el('a', { href: pagePath(author) }, el('img', { className: 'article-avatar', src: avatarURL(author, 'small'), alt: '' })),
    el('span', { className: 'article-byline-text' },
      el('span', {}, el('a', { className: 'article-author', href: pagePath(author), textContent: fullName(author) }), verifiedMark(author) ?? ''),
      el('span', { className: 'article-meta', textContent: [
        formatDateTime(article.createdAt),
        article.editedAt ? tr('изменено') : '',
        minutesText(article.minutes),
      ].filter(Boolean).join(' · ') })),
  );

  $('article-text').replaceChildren(...blockNodes(article.blocks));
}

function renderTools(article, me) {
  const tools = [];
  if (me?.profileCompleted) tools.push(el('a', { textContent: tr('Поделиться на стене'), onclick: () => shareOnWall(me) }));
  tools.push(el('a', { textContent: tr('Скопировать ссылку'), onclick: copyLink }));
  tools.push(el('a', { href: '/articles?user=' + encodeURIComponent(article.author.nickname), textContent: tr('Все статьи автора') }));
  if (me && me.id === article.author.id) {
    tools.push(
      el('a', { href: articlePath(article.id) + '/edit', textContent: tr('Редактировать') }),
      el('a', { textContent: tr('Удалить'), onclick: () => confirmDelete(article) }),
    );
  }

  $('article-tools').replaceChildren(...tools);
}

async function shareOnWall(me) {
  try {
    await api.createPost(me.nickname, articleURL(articleID));
    toast(tr('Статья опубликована на вашей стене.'));
  } catch (e) {
    showError($('error'), e);
  }
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(articleURL(articleID));
    toast(tr('Ссылка скопирована!'));
  } catch {
    const modal = openModal(tr('Ссылка на статью'));
    const input = el('input', { type: 'text', readOnly: true, value: articleURL(articleID) });
    modal.body.append(input);
    modal.actions.append(button(tr('Закрыть'), modal.close, { secondary: true }));
    input.select();
  }
}

// confirmDelete asks first: the posts that link to the article show the
// link alone.
function confirmDelete(article) {
  const modal = openModal(tr('Удалить статью'));
  modal.body.append(el('p', { textContent: tr('Вы уверены, что хотите удалить статью «{0}»? Записи со ссылкой на неё останутся без карточки.', article.title) }));
  modal.actions.append(
    button(tr('Удалить'), async () => {
      try {
        await api.deleteArticle(article.id);
        location.href = pagePath(article.author);
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

// The article is asked for with who is signed in, in one round trip.
const wanted = started(api.article(articleID));
const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const article = await wanted;
  renderArticle(article);
  renderTools(article, me);
  $('article').hidden = false;
} catch (e) {
  const notFound = e.status === 404 || e.status === 400;
  $('title').textContent = notFound ? tr('Статья не найдена') : tr('Ошибка');
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), notFound ? tr('Статья удалена или никогда не существовала.') : e);
}
