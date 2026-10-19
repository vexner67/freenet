// openvk's "Новая тема" at /club/<ref>/board/create: a title, the text of
// the first message with photos and, for the admins, "От имени
// сообщества"; "Создать тему" opens the new topic.

import { api, currentUser } from './api.js';
import { initAttachments } from './attach.js';
import { $, clubRef, renderChrome, showError, hideError } from './layout.js';
import { boardTitle, topicPath } from './topics.js';
import { tr } from './i18n.js';

const ref = decodeURIComponent(location.pathname.slice('/club/'.length, -'/board/create'.length));

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else {
  try {
    const club = await api.club(ref);

    document.title = tr('Новая тема | Freenet');
    $('title').replaceChildren(...boardTitle(club, tr('Новая тема')));

    if (!club.canStartTopic) {
      showError($('error'), tr('Создавать темы в этой группе могут только администраторы.'));
    } else if (!me.profileCompleted) {
      showError($('error'), tr('Сначала заполните свою страницу.'));
    } else {
      initForm(club);
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? tr('Группа не найдена') : tr('Ошибка');
    showError($('error'), e);
  }
}

function initForm(club) {
  const error = $('error');
  const admin = club.role === 'admin' || club.role === 'owner';
  const attachments = initAttachments({
    link: $('topic-attach'),
    input: $('topic-file'),
    list: $('topic-photos'),
    error,
    onChange: () => {},
  });

  $('topic-from-club-label').hidden = !admin;
  $('form').hidden = false;
  $('topic-title').focus();

  $('form').addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);

    if (attachments.busy()) {
      showError(error, tr('Дождитесь, пока загрузятся фотографии.'));
      return;
    }

    $('submit').disabled = true;

    try {
      const topic = await api.createTopic(clubRef(club), $('topic-title').value, $('topic-text').value, attachments.ids(), {
        fromClub: admin && $('topic-from-club').checked,
      });
      location.assign(topicPath(topic));
    } catch (e) {
      showError(error, e);
      $('submit').disabled = false;
    }
  });
}
