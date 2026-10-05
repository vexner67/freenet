// The left column of a profile: the follow button and the people blocks.

import { api } from './api.js';
import { followersCount, followingCount } from './labels.js';
import { $, showError, hideError } from './layout.js';
import { personCell } from './people.js';

const SIDE_BLOCK_SIZE = 6;

export function initFollow(profile, me, isOwner) {
  const button = $('follow');
  const error = $('follow-error');

  let stats = profile;

  function listURL(type) {
    return `/follows?u=${encodeURIComponent(profile.nickname)}&type=${type}`;
  }

  async function renderBlock(type, total, label) {
    const block = $(`${type}-block`);

    block.hidden = total === 0;
    if (total === 0) return;

    $(`${type}-count`).textContent = label(total);
    $(`${type}-all`).href = listURL(type);

    const page = await api[type](profile.nickname, '', SIDE_BLOCK_SIZE);
    $(`${type}-grid`).replaceChildren(...page.users.map(personCell));
  }

  function render() {
    button.textContent = stats.isFollowing ? 'Отписаться' : 'Подписаться';
    button.classList.toggle('secondary', stats.isFollowing);
    $('follow-state').hidden = !stats.isFollowing;

    renderBlock('followers', stats.followersCount, followersCount).catch((e) => showError(error, e));
    renderBlock('following', stats.followingCount, followingCount).catch((e) => showError(error, e));
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

  // Guests see the blocks but no button; the owner cannot follow themselves.
  if (me && !isOwner) {
    button.hidden = false;
    button.addEventListener('click', toggle);
  }

  render();
}
