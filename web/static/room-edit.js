// "Настройки" of a room, as "Редактировать группу": the name, what the room
// is for, who may enter and who runs the screen; under the form its owner
// may close it ("Вы также можете закрыть комнату"). Only the owner gets
// here; the room service checks it again.

import { api, currentUser, started } from './api.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

const id = location.pathname.split('/').filter(Boolean)[1] ?? '';
const path = `/room/${encodeURIComponent(id)}`;

function fill(room) {
  const heading = tr('Настройки');

  document.title = `${room.title} — ${heading} | Freenet`;
  $('title').replaceChildren(el('a', { href: path, textContent: room.title }), ` » ${heading}`);
  $('name').value = room.title;
  $('kind').value = room.kind;
  if (room.clubId && !$('access').querySelector('option[value="club"]')) {
    $('access').append(el('option', { value: 'club', textContent: tr('участники сообщества') }));
  }
  $('access').value = room.access;
  $('control').value = room.control;
  $('permanent-text').textContent = room.permanent ? tr('постоянная: сохраняет адрес, когда все вышли') : tr('временная: закроется через 5 минут после ухода последнего участника');
}

async function submit(event) {
  event.preventDefault();
  hideError($('error'));
  $('saved').hidden = true;
  $('submit').disabled = true;

  try {
    const result = await api.updateRoom(id, { title: $('name').value, kind: $('kind').value, access: $('access').value, control: $('control').value });
    fill(result.room);
    $('saved').hidden = false;
  } catch (e) {
    showError($('error'), e);
  } finally {
    $('submit').disabled = false;
  }
}

// confirmClose asks before the room goes: everyone inside leaves it.
function confirmClose(room) {
  const modal = openModal(tr('Закрыть комнату'));
  modal.body.textContent = tr('Вы уверены, что хотите закрыть комнату «{0}»? Все участники выйдут из неё. Это нельзя отменить.', room.title);
  modal.actions.append(
    button(tr('Закрыть'), async () => {
      try {
        await api.closeRoom(id);
        location.assign('/rooms?act=managed');
      } catch (e) {
        modal.close();
        showError($('error'), e);
      }
    }),
    button(tr('Отмена'), modal.close, { secondary: true }),
  );
}

const wanted = started(api.room(id));
const me = await currentUser().catch(() => null);
if (!me) {
  wanted.catch(() => {});
  location.replace('/');
} else {
  renderChrome(me);

  try {
    const { room } = await wanted;
    if (room.ownerId !== me.id) {
      location.replace(path);
    } else {
      fill(room);
      $('form').addEventListener('submit', submit);
      $('close').addEventListener('click', () => confirmClose(room));
      $('form').hidden = false;
    }
  } catch (e) {
    $('title').textContent = e.status === 404 ? tr('Комната не найдена') : tr('Ошибка');
    showError($('load-error'), e);
  }
}
