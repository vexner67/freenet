// The jackpot of the casino: a win of the jackpot of the casino (set by the
// administrators, 1000 votes by default) and more in "Огонёк", "Прицел",
// "Ракетка" or "Покер" gets a message box, as openvk says anything that
// matters: the sum won and "Закрыть".

import { el } from './layout.js';
import { button, openModal } from './modal.js';
import { votesCount } from './labels.js';
import { tr } from './i18n.js';

// jackpot shows the win of votes in a message box.
export function jackpot(votes) {
  const box = openModal(tr('Джекпот'));

  box.body.append(el('b', { textContent: tr('Вы выиграли {0}!', votesCount(votes)) }));
  box.actions.append(button(tr('Закрыть'), box.close));
}
