// The wall block of a profile or community page: posts by the owner and
// by others, the form for whoever may write there, paging.

import { api } from './api.js';
import { wallCount } from './labels.js';
import { $, clubRef, el, isMicroblog, showError, hideError, autosize } from './layout.js';
import { postNode as renderPost } from './posts.js';
import { initAttachments } from './attach.js';
import { initGraffiti } from './graffiti.js';
import { initDocuments } from './docs.js';
import { initPollDraft } from './poll.js';
import { initTimer, postponedNotice, showPostponedLink } from './postponed.js';
import { tr } from './i18n.js';

// openvk's limit of a post or a comment; the counter shows near the end.
const MAX_LENGTH = 60000;
const COUNTER_FROM = MAX_LENGTH - 100;

// initWall shows the wall of owner, a user or a community (owner.club), to
// me, the viewer (null if anonymous). Whoever moderates the wall, its owner
// or a community's admins, deletes and pins any post on it, others their
// own ones; who may write here the first page tells, as the owner's privacy
// or the community allows. An admin also speaks for the community, a box
// on the form ("От имени сообщества"); on a "Предложка" others suggest
// posts instead ("Предложить новость") and see their suggestions after,
// as on openvk. type is a tab of the wall page (wall-page.js): '' for all
// posts, the only one with the form, 'owner', 'others' or 'archive', the
// posts put away; query is what the posts say ("Поиск"), without a form;
// year is the year of the archive to show, 0 for every one, and onYears
// is told the years of the wall, which the archive's first page carries.
export function initWall(owner, me, { moderates = me?.id === owner.id, speaksForClub = false, type = '', query = '', year = 0, onYears } = {}) {
  const posts = $('posts');
  const error = $('wall-error');
  const isOwner = me?.id === owner.id;
  // "Микроблог" shows the newest comments under each post.
  const comments = isMicroblog();
  const load = owner.club
    ? (c) => api.clubWall(clubRef(owner), c, type, query, comments, year)
    : (c) => api.wall(owner.nickname, c, type, query, comments, year);
  const plain = !type && !query;
  // On a wall that is not open its admins always speak for the club, as on
  // openvk.
  const fromClub = () => owner.wall !== 'open' || $('post-from-club').checked;
  const create = owner.club
    ? (text, photos) => api.createClubPost(clubRef(owner), text, photos, { fromClub: fromClub(), signed: fromClub() && $('post-signed').checked, nsfw: $('post-nsfw').checked, poll: poll.value(), documents: documents.ids() })
    : (text, photos) => api.createPost(owner.nickname, text, photos, $('post-nsfw').checked, poll.value(), documents.ids(), isOwner ? price() : 0);
  let attachments;
  // documents are those of "Мои Документы" attached to the post.
  let documents;
  // poll is the poll attached to the post being written ("Опрос").
  let poll;
  // timer is VK's "Таймер": the post waits for its time.
  let timer = null;
  let canPost = false;
  // suggesting: the form suggests a post for the admins of the club.
  let suggesting = false;

  let total = 0;
  let cursor = '';

  // As openvk: "Стена  5 записей  Все".
  function updateCount() {
    $('wall-count').textContent = wallCount(total);
    $('wall-empty').hidden = total > 0 || canPost;
    if (query) $('wall-empty').textContent = tr('Ничего не найдено.');
    else if (type === 'archive') $('wall-empty').textContent = year ? tr('В архиве нет записей за {0} год.', year) : tr('В архиве пока нет ни одной записи.');
  }

  const postNode = (post) => renderPost(post, {
    viewer: me,
    moderates,
    onDelete: (moderates || post.mine) && remove,
    // The pinned post comes first: the wall is read again from the top.
    onPinned: reload,
    onArchived: archived,
    canLike: Boolean(me),
    onError: (e) => showError(error, e),
    // "Поиск" highlights what it found.
    found: query,
  });

  function reload() {
    cursor = '';
    posts.replaceChildren();
    return loadMore();
  }

  // archived: a post put away leaves the wall, one returned the archive.
  function archived(post, node) {
    if (post.archived === (type === 'archive')) return;

    node.replaceWith(el('div', { className: 'post-deleted', textContent: post.archived ? tr('Запись перенесена в архив.') : tr('Запись возвращена на стену.') }));
    total--;
    updateCount();
  }

  async function remove(post, node) {
    hideError(error);

    try {
      await api.deletePost(post.id);
    } catch (e) {
      showError(error, e);
      return;
    }

    node.replaceWith(el('div', { className: 'post-deleted', textContent: tr('Запись удалена.') }));
    total--;
    updateCount();
  }

  async function loadMore() {
    const more = $('wall-more');
    hideError(error);

    try {
      const firstPage = !cursor;
      const page = await load(cursor);

      // Without a nickname of your own you have no name to sign a post.
      if (firstPage && plain && (page.canPost || page.canSuggest) && me?.profileCompleted) showForm(!page.canPost);

      if (firstPage && page.years) onYears?.(page.years);

      total = page.total;
      cursor = page.nextCursor;
      posts.append(...page.posts.map(postNode));
      more.hidden = !cursor;
      updateCount();
    } catch (e) {
      showError(error, e);
    }
  }

  function updateCounter() {
    const left = MAX_LENGTH - $('post-text').value.length;
    $('post-counter').textContent = left <= MAX_LENGTH - COUNTER_FROM ? tr('осталось {0}', left) : '';
  }

  async function submit(event) {
    event.preventDefault();
    hideError(error);

    if (attachments.busy()) {
      showError(error, tr('Дождитесь, пока загрузятся фотографии.'));
      return;
    }

    if (timer?.active() && poll.value()) {
      showError(error, tr('К отложенной записи нельзя прикрепить опрос.'));
      return;
    }

    if (timer?.active() && price() > 0) {
      showError(error, tr('Отложенную запись нельзя сделать платной.'));
      return;
    }

    if ((timer?.active() || suggesting) && documents.count()) {
      showError(error, suggesting ? tr('К предложенной записи нельзя прикрепить документ.') : tr('К отложенной записи нельзя прикрепить документ.'));
      return;
    }

    const button = $('post-submit');
    button.disabled = true;

    try {
      if (suggesting) {
        await api.suggestClubPost(clubRef(owner), $('post-text').value, attachments.ids());
        location.href = `/club/${encodeURIComponent(clubRef(owner))}/suggested?type=my`;
        return;
      }

      if (timer?.active()) {
        posts.prepend(postponedNotice(await postpone($('post-text').value, attachments.ids()), owner));
        timer.clear();
        showPostponedLink(owner);
      } else {
        const post = await create($('post-text').value, attachments.ids());

        // Below the pinned post, which stays first.
        const pinned = posts.firstElementChild?.classList.contains('pinned') ? posts.firstElementChild : null;
        if (pinned) pinned.after(postNode(post));
        else posts.prepend(postNode(post));
        total++;
      }
      $('post-text').value = '';
      $('post-nsfw').checked = false;
      if ($('post-price')) $('post-price').value = '0';
      autosize($('post-text'));
      $('post-text').blur();
      attachments.clear();
      documents.clear();
      poll.clear();
      $('post-form').classList.remove('expanded');
      updateCount();
      updateCounter();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  // showForm shows the form to write, or with suggest to suggest a post.
  function showForm(suggest) {
    const form = $('post-form');
    const text = $('post-text');

    canPost = true;
    suggesting = suggest;
    // As on openvk: "Написать" on someone else's wall, "Предложить
    // новость" on a "Предложка".
    if (!isOwner && !owner.club) text.placeholder = tr('Написать');
    if (suggest) text.placeholder = tr('Предложить новость');
    // A suggested post is marked by the admins, if at all.
    $('post-nsfw-label').hidden = suggest;
    // Only a post on your own wall sells its photos: the votes are yours.
    // The page of a club has no such field.
    if ($('post-price-label')) $('post-price-label').hidden = !(isOwner && !owner.club);
    let showSigned = null;
    if (speaksForClub) {
      // "От имени сообщества" only where there is a choice; "Подпись
      // автора" while the post speaks for the club.
      $('post-from-club-label').hidden = owner.wall !== 'open';
      showSigned = () => { $('post-signed-label').hidden = !fromClub(); };
      $('post-from-club').addEventListener('change', showSigned);
      showSigned();
    }

    // "Таймер" postpones a post for your own wall or, for its admins, one
    // that speaks for the community: always in its name.
    if (!suggest && (owner.club ? speaksForClub : isOwner)) {
      timer = initTimer({
        link: $('post-timer'),
        box: $('post-timer-box'),
        submit: $('post-submit'),
        onChange: (on) => {
          if (on) form.classList.add('expanded');
          if (!showSigned) return;
          if (on) $('post-from-club').checked = true;
          $('post-from-club').disabled = on;
          showSigned();
        },
      });
    }

    attachments = initAttachments({
      link: $('post-attach'),
      also: [$('post-graffiti')],
      input: $('post-file'),
      list: $('post-photos'),
      error,
      onChange: () => {
        if (attachments?.count()) form.classList.add('expanded');
      },
    });
    initGraffiti({ link: $('post-graffiti'), attachments });

    documents = initDocuments({
      link: $('post-doc'),
      list: $('post-documents'),
      onChange: () => {
        if (documents.count()) form.classList.add('expanded');
      },
    });
    // A suggested post attaches no documents.
    if (suggest) $('post-doc').hidden = true;
    // "Статья" opens the editor of an article for one's own wall: the
    // article comes back to it as a post that links to it.
    $('post-article').hidden = !(isOwner && !owner.club && !suggest);

    poll = initPollDraft({
      link: $('post-poll'),
      marker: $('post-poll-marker'),
      onChange: () => {
        if (poll?.value()) form.classList.add('expanded');
      },
    });
    // A suggested post asks no poll.
    if (suggest) $('post-poll').hidden = true;

    form.hidden = false;
    form.addEventListener('submit', submit);
    text.addEventListener('input', updateCounter);
    // Ctrl+Enter (Cmd+Enter on a Mac) sends, as on the old sites.
    text.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) form.requestSubmit();
    });
  }

  // postpone keeps the post for the time the timer says.
  // price: the votes others pay for the photos and documents of the post.
  function price() {
    return Math.max(0, Math.floor(Number($('post-price')?.value) || 0));
  }

  function postpone(text, photos) {
    const nsfw = $('post-nsfw').checked;
    if (!owner.club) return api.postpone(text, photos, nsfw, timer.value());

    return api.postponeClubPost(clubRef(owner), text, photos, { signed: $('post-signed').checked, nsfw, publishAt: timer.value() });
  }

  $('wall-more').addEventListener('click', loadMore);
  if (plain && moderates && me) showPostponedLink(owner);
  if ($('wall-all')) $('wall-all').href = wallPath(owner);

  return loadMore();
}

// wallPath is the page of the wall of owner, a person or a community, as
// openvk's /wall<id>.
export function wallPath(owner) {
  if (owner.club) return '/club/' + encodeURIComponent(clubRef(owner)) + '/wall';

  return '/wall/' + encodeURIComponent(owner.nickname);
}
