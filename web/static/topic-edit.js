// openvk's "Редактирование темы" at /topic/<id>/edit: the title and
// "Настройки темы" — "Закрепить тему" for the admins, "Комментировать
// могут только администраторы" for them in a topic of the community,
// "Закрыть тему" — with "Удалить тему" and "Сохранить".

import { api, currentUser } from './api.js';
import { $, fullName, pagePath, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { boardPath, boardTitle, topicPath } from './topics.js';

const topicID = decodeURIComponent(location.pathname.slice('/topic/'.length, -'/edit'.length));

function fill(topic, club, admin) {
  document.title = `Редактирование темы «${topic.title}» | Freenet`;
  $('title').replaceChildren(...boardTitle(club, 'Редактирование темы'));
  $('topic-title').textContent = topic.title;
  $('topic-author').textContent = fullName(topic.author);
  $('topic-author').href = pagePath(topic.author);

  $('title-input').value = topic.title;
  $('pinned').checked = topic.pinned;
  $('restricted').checked = topic.restricted;
  $('closed').checked = topic.closed;
  $('pinned-label').hidden = !admin;
  $('restricted-label').hidden = !admin || !topic.fromWall;
}

// confirmDelete asks as openvk's message box does before the topic goes.
function confirmDelete(topic, club) {
  const modal = openModal('Удалить тему');
  modal.body.textContent = 'Вы уверены, что хотите удалить эту тему со всеми сообщениями?';
  modal.actions.append(
    button('Удалить', async () => {
      try {
        await api.deleteTopic(topic.id);
        location.assign(boardPath(club));
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button('Отмена', modal.close, { secondary: true }),
  );
}

const me = await currentUser().catch(() => null);
renderChrome(me);

if (!me) {
  location.replace('/');
} else {
  try {
    const { topic, club, role } = await api.topic(topicID);
    const admin = role === 'admin' || role === 'owner';

    fill(topic, club, admin);
    $('content').hidden = false;

    $('delete').addEventListener('click', () => confirmDelete(topic, club));
    $('form').addEventListener('submit', async (event) => {
      event.preventDefault();
      hideError($('error'));
      $('save').disabled = true;

      try {
        await api.editTopic(topic.id, {
          title: $('title-input').value,
          closed: $('closed').checked,
          pinned: $('pinned').checked,
          restricted: $('restricted').checked,
        });
        location.assign(topicPath(topic));
      } catch (e) {
        showError($('error'), e);
        $('save').disabled = false;
      }
    });
  } catch (e) {
    $('title').textContent = e.status === 404 ? 'Тема не найдена' : 'Ошибка';
    showError($('error'), e);
  }
}
