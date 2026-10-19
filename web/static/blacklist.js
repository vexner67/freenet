// The blacklist link of a profile, as on openvk: "Добавить в чёрный
// список" asks first, then the page turns into the limited one;
// "Удалить из чёрного списка" takes it back at once.

import { api } from './api.js';
import { el, fullName, showError, hideError } from './layout.js';
import { button, openModal } from './modal.js';
import { tr } from './i18n.js';

// initBlacklistLink wires the link for profile; onChange(blocked) runs
// after the blacklist changed, the page reloads to show what is left.
export function initBlacklistLink(profile, onChange) {
  const link = document.getElementById('blacklist');
  let blocked = profile.blockedByMe;

  function render() {
    link.textContent = blocked ? tr('Удалить из чёрного списка') : tr('Добавить в чёрный список');
  }

  async function unblock() {
    await api.unblock(profile.nickname);
    blocked = false;
    render();
    onChange(blocked);
  }

  function confirmBlock() {
    const modal = openModal(tr('Добавление в чёрный список'));
    const error = el('div', { className: 'error', hidden: true });
    const add = button(tr('Добавить'), async () => {
      hideError(error);
      add.disabled = true;

      try {
        await api.block(profile.nickname);
        blocked = true;
        render();
        modal.close();
        onChange(blocked);
      } catch (e) {
        showError(error, e);
        add.disabled = false;
      }
    });

    modal.body.append(
      error,
      el('div', {}, tr('Вы уверены? {0} не сможет писать вам сообщения, писать на вашей стене, ', fullName(profile)) +
        tr('комментировать ваши записи и подписываться на вас, а ваши подписки друг на друга будут удалены.')),
    );
    modal.actions.append(add, button(tr('Отмена'), modal.close, { secondary: true }));
  }

  link.addEventListener('click', () => {
    if (blocked) {
      unblock().catch((e) => showError(document.getElementById('follow-error'), e));
    } else {
      confirmBlock();
    }
  });

  render();
  link.hidden = false;
}
