// The discussions of a club at /club/<ref>/board, as openvk's
// "Обсуждения": "Создать тему" for whom the club lets, a search by title,
// how many there are and the topics, pinned first, then the most recently
// active, each with how many messages and who answered last.

import { api, currentUser } from './api.js';
import { formatPostDate, resultsCount, topicMessages } from './labels.js';
import { $, avatarURL, clubRef, el, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { topicPath } from './topics.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/board'.length));
const query = new URLSearchParams(location.search).get('q') ?? '';

let cursor = '';

// topicNode is one topic of the list: its title, how many messages and,
// on the right, who answered last and when ("ответил").
function topicNode(topic) {
  const last = topic.lastMessage;

  return el('div', { className: 'topic-item' },
    el('div', { className: 'topic-main' },
      el('a', { className: 'topic-name', href: topicPath(topic), textContent: topic.title }),
      topic.pinned ? el('span', { className: 'hint', textContent: ' — закреплена' }) : '',
      topic.closed ? el('span', { className: 'hint', textContent: ' — закрыта' }) : '',
      el('div', { textContent: topicMessages(topic.messages) }),
    ),
    last ? el('div', { className: 'topic-last' },
      el('a', { href: pagePath(last.author) }, el('img', { src: avatarURL(last.author, 'small'), alt: '' })),
      el('div', {},
        el('a', { href: pagePath(last.author), textContent: fullName(last.author) }),
        el('div', { className: 'hint', textContent: `ответил ${formatPostDate(last.createdAt)}` }),
      ),
    ) : '',
  );
}

async function loadMore() {
  hideError($('error'));

  try {
    const page = await api.clubTopics(ref, { q: query, cursor });

    $('topics').append(...page.topics.map(topicNode));
    $('count').replaceChildren(el('b', { textContent: resultsCount(page.total) }));
    $('count').hidden = false;
    cursor = page.nextCursor;
    $('more').hidden = !cursor;
  } catch (e) {
    showError($('error'), e);
  }
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  const club = await api.club(ref);
  const path = '/club/' + encodeURIComponent(clubRef(club));

  document.title = `Обсуждения ${club.name} | Freenet`;
  $('title').replaceChildren(el('a', { href: path, textContent: club.name }), ' » Обсуждения');
  if (me?.profileCompleted && club.canStartTopic) {
    $('title-links').replaceChildren(el('a', { href: path + '/board/create', textContent: 'Создать тему' }));
  }

  $('q').value = query;
  $('search').hidden = false;
  $('more').addEventListener('click', loadMore);
  await loadMore();
} catch (e) {
  $('title').textContent = e.status === 404 ? 'Группа не найдена' : 'Ошибка';
  showError($('error'), e);
}
