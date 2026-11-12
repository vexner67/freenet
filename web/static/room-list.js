import { api } from './api.js';
import { avatarURL, el, fullName } from './layout.js';
import { tr } from './i18n.js';

export function roomKind(kind) {
  return { talk: tr('разговор'), watch: tr('кино'), listen: tr('музыка') }[kind] ?? '';
}
export function roomRow(room, compact = false) {
  const people = el('span', { className: 'room-row-people' });
  for (const member of room.members.slice(0, 5)) people.append(el('img', {
    src: avatarURL(member.user, 'small'), alt: fullName(member.user), title: fullName(member.user), width: 32, height: 32,
  }));
  return el('div', { className: compact ? 'room-row compact' : 'room-row' },
    el('div', {}, el('a', { href: `/room/${encodeURIComponent(room.id)}`, textContent: room.title }),
      el('div', { className: 'hint', textContent: tr('{0} · {1} из {2}', roomKind(room.kind), room.members.length, room.maxMembers) })), people);
}
// roomAccess and roomControl name who may enter and who runs the screen.
export function roomAccess(access) {
  return { everyone: tr('все желающие'), friends: tr('друзья'), link: tr('по ссылке'), club: tr('участники сообщества') }[access] ?? '';
}

// roomCover is the picture of a room in a list: the photos of up to four
// of its people, or an empty grey square while nobody is in.
function roomCover(room, path) {
  const cover = el('a', { className: 'list-cover room-cover', href: path, ariaHidden: 'true', tabIndex: -1 });
  for (const member of room.members.slice(0, 4)) {
    cover.append(el('img', { src: avatarURL(member.user, 'small'), alt: '', loading: 'lazy' }));
  }
  cover.classList.toggle('single', room.members.length === 1);
  return cover;
}

// roomListRow is a room as "Мои Группы" shows a club: the picture, the
// name, the kind, who is in and who may enter, and the actions on the right.
export function roomListRow(room, ...actions) {
  const path = `/room/${encodeURIComponent(room.id)}`;
  const people = room.members.length
    ? room.members.map((member) => fullName(member.user)).join(', ')
    : tr('пока никого');
  return el('div', { className: 'list-row club-list-row room-list-row' },
    roomCover(room, path),
    el('div', { className: 'list-info' },
      el('table', { className: 'info' },
        el('tr', {}, el('th', { textContent: tr('Название:') }), el('td', {}, el('a', { href: path, textContent: room.title }))),
        el('tr', {}, el('th', { textContent: tr('Вид:') }), el('td', { textContent: roomKind(room.kind) })),
        el('tr', {}, el('th', { textContent: tr('Участники:') }), el('td', { textContent: tr('{0} из {1}: {2}', room.members.length, room.maxMembers, people) })),
        el('tr', {}, el('th', { textContent: tr('Кто может войти:') }), el('td', { textContent: roomAccess(room.access) })),
      ),
    ),
    actions.some(Boolean) && el('div', { className: 'list-actions' }, ...actions),
  );
}
export function roomsBlock(rooms, { title = tr('Комнаты'), more = '/rooms', create = '' } = {}) {
  return el('section', { className: 'rooms-block' },
    el('div', { className: 'section-title', textContent: title }),
    el('div', { className: 'subtitle' }, el('a', { href: more, textContent: tr('Все') }),
      create && el('a', { href: create, textContent: tr('Создать комнату') })),
    ...rooms.slice(0, 3).map((room) => roomRow(room, true)));
}
export async function showFeedRooms(container) {
  try {
    const page = await api.rooms();
    container.replaceChildren(...(page.rooms.length ? [roomsBlock(page.rooms, { title: tr('Сейчас в комнатах') })] : []));
    container.hidden = !page.rooms.length;
  } catch { container.hidden = true; }
}
