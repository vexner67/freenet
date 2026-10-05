import { api, currentUser } from './api.js';
import { genders, relationships, worldviews, politicalViews, label, formatBirthday } from './labels.js';
import { $, el, renderChrome, showError } from './layout.js';
import { initFollow } from './follow.js';
import { initPhoto } from './photo.js';
import { initWall } from './wall.js';

const nickname = decodeURIComponent(location.pathname.slice(1));

// rows lists what to show; private marks rows others cannot see (only the
// owner gets them at all, from /me).
function rows(user, privacy) {
  return [
    ['Пол:', label(genders, user.gender), false],
    ['Семейное положение:', label(relationships(user.gender), user.relationship), privacy && !privacy.showRelationship],
    ['Дата рождения:', user.birthday ? formatBirthday(user.birthday) : '', privacy && !privacy.showBirthday],
    ['Мировоззрение:', label(worldviews, user.worldview), privacy && !privacy.showWorldview],
    ['Полит. взгляды:', label(politicalViews, user.politicalViews), privacy && !privacy.showPoliticalViews],
  ].filter(([, value]) => value);
}

function render(user, isOwner) {
  const fullName = [user.name, user.surname].filter(Boolean).join(' ');

  document.title = `${fullName} | Freenet`;
  $('title').textContent = fullName;
  $('title-aside').textContent = isOwner ? 'это вы' : '@' + user.nickname;
  $('name').textContent = fullName;

  const status = $('status');
  if (user.status) {
    status.textContent = user.status;
  } else if (isOwner) {
    status.replaceChildren(el('a', { href: '/edit', textContent: 'изменить статус' }));
    status.classList.add('empty');
  }

  const info = rows(user, isOwner ? user.privacy : null);
  $('info').replaceChildren(...info.map(([title, value, isPrivate]) => el('tr', {},
    el('th', { textContent: title }),
    el('td', {}, value, isPrivate && el('span', { className: 'private-mark', textContent: ' · видно только вам' })),
  )));
  $('no-info').hidden = info.length > 0;

  $('edit').hidden = !isOwner;
  $('profile').hidden = false;

  initWall(user, isOwner, Boolean(me));
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  // The public profile also brings the follow counters; the owner sees the
  // whole profile from /me.
  const profile = await api.profile(nickname);
  const isOwner = Boolean(me) && me.id === profile.id;

  render(isOwner ? me : profile, isOwner);
  initPhoto(isOwner ? me : profile, isOwner);
  initFollow(profile, me, isOwner);
} catch (e) {
  $('title').textContent = e.status === 404 || e.status === 400 ? 'Страница не найдена' : 'Ошибка';
  document.title = $('title').textContent + ' | Freenet';
  showError($('error'), e.status === 400 ? 'Страница не найдена.' : e);
}
