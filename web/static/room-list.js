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
