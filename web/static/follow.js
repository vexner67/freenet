// The left column of a profile: the friend link under the photo and the
// block "Друзья", as openvk's. Friends follow each other: "Добавить в
// друзья" sends a request, "Принять заявку" follows back, "Отменить
// заявку" and "Удалить из друзей" take one's follow back.

import { api } from './api.js';
import { friendsCount } from './labels.js';
import { $, showError, hideError } from './layout.js';
import { personCell } from './people.js';
import { tr } from './i18n.js';

const SIDE_BLOCK_SIZE = 6;

// friendAction is what the link under the photo says, by how the viewer
// and the owner follow each other.
function friendAction(stats) {
  if (stats.isFollowing && stats.followsMe) return tr('Удалить из друзей');
  if (stats.isFollowing) return tr('Отменить заявку');
  if (stats.followsMe) return tr('Принять заявку');
  return tr('Добавить в друзья');
}

export function initFollow(profile, me, isOwner) {
  const button = $('follow');
  const error = $('follow-error');

  let stats = profile;

  // "Кому видно моих друзей" hides the block, as openvk its "Друзья".
  const hidden = profile.access && !profile.access.friends;

  async function renderBlock() {
    const block = $('friends-block');

    block.hidden = stats.friendsCount === 0 || hidden;
    if (block.hidden) return;

    $('friends-count').textContent = friendsCount(stats.friendsCount);
    $('friends-all').href = `/friends?u=${encodeURIComponent(profile.nickname)}`;

    const page = await api.friends(profile.nickname, 'all', '', SIDE_BLOCK_SIZE);
    $('friends-grid').replaceChildren(...page.users.map((user) => personCell(user)));
  }

  function render() {
    button.textContent = friendAction(stats);
    renderBlock().catch((e) => showError(error, e));
  }

  async function toggle() {
    hideError(error);
    button.disabled = true;

    try {
      stats = await (stats.isFollowing ? api.unfollow : api.follow)(profile.nickname);
      render();
    } catch (e) {
      showError(error, e);
    } finally {
      button.disabled = false;
    }
  }

  // Guests see the block but no link; the owner has no friendship with
  // themselves.
  if (me && !isOwner) {
    button.hidden = false;
    button.addEventListener('click', toggle);
  }

  render();
}
