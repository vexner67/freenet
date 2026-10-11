// openvk's polls: the block of a poll under the words of a post, where a
// reader votes and sees how others did, and the box "Новый опрос" that
// attaches one to a post being written.

import { api } from './api.js';
import { daysCount, pollAsksVote, pollMeta, pollPercent, pollVoters } from './labels.js';
import { el, pagePath } from './layout.js';
import { openLikers } from './likers.js';
import { button, openModal } from './modal.js';

export const MAX_OPTIONS = 10;
const MAX_LINE = 128;
const MAX_DAYS = 31;

// pollNode is the poll of post (a post or the post a repost shares, its
// poll in post.poll): the options to choose from for whoever has not
// voted, the results otherwise. canVote: the reader is signed in; a guest
// sees the options but cannot choose, as on openvk.
export function pollNode(post, { canVote = false, onError } = {}) {
  const node = el('div', { className: 'poll' });
  let busy = false;

  async function change(call) {
    if (busy) return;
    busy = true;

    try {
      post.poll = await call();
    } catch (e) {
      onError?.(e);
    } finally {
      busy = false;
      render();
    }
  }

  const vote = (options) => change(() => api.votePoll(post.id, options));

  // form: a click on an option votes at once; with several to choose, a
  // button does, once something is chosen.
  function form(poll) {
    const inputs = poll.options.map((option, i) => el('input', {
      type: poll.multiple ? 'checkbox' : 'radio',
      name: 'poll-' + post.id,
      disabled: !canVote,
      onchange: () => {
        if (poll.multiple) cast.disabled = !inputs.some((input) => input.checked);
        else vote([i]);
      },
    }));
    const cast = button('Проголосовать!', () => vote(inputs.flatMap((input, i) => (input.checked ? [i] : []))));
    cast.disabled = true;

    return el('div', { className: 'poll-options' },
      ...poll.options.map((option, i) => el('div', { className: 'poll-option' }, el('label', {}, inputs[i], ' ' + option.name))),
      poll.multiple && canVote ? el('div', { className: 'poll-cast' }, cast) : null,
    );
  }

  // results: each option with its share as a bar; its name leads to who
  // chose it, unless the poll is anonymous, and is bold when the reader did.
  function results(poll) {
    const mayRetract = canVote && poll.voted.length > 0 && !poll.locked && !poll.ended;

    return el('div', { className: 'poll-results' },
      mayRetract ? el('a', { className: 'poll-retract', textContent: 'Отменить голос', onclick: () => change(() => api.retractVote(post.id)) }) : null,
      ...poll.options.map((option, i) => {
        const listed = !poll.anonymous && option.votes > 0;
        const fill = el('div', { className: 'poll-result-fill' });
        const percent = pollPercent(option.votes, poll.voters);
        // Set through the CSSOM: the CSP forbids only the attribute.
        fill.style.width = percent + '%';

        return el('div', { className: 'poll-result' },
          el(listed ? 'a' : 'span', {
            className: 'poll-result-name' + (poll.voted.includes(i) ? ' chosen' : ''),
            textContent: option.name,
            onclick: listed ? () => openVoters(post.id, option, i) : null,
          }),
          el('div', { className: 'poll-result-row' },
            el('div', { className: 'poll-result-bar' }, fill, el('span', { className: 'poll-result-count', textContent: String(option.votes) })),
            el('b', { className: 'poll-result-percent', textContent: percent + '%' }),
          ),
        );
      }),
    );
  }

  function render() {
    const { poll } = post;

    node.replaceChildren(
      el('h4', { textContent: poll.title }),
      pollAsksVote(poll) ? form(poll) : results(poll),
      el('div', { className: 'poll-meta' },
        el('div', { textContent: pollVoters(poll.voters) }),
        el('div', { className: 'poll-kind', textContent: pollMeta(poll) }),
      ),
    );
  }

  render();

  return node;
}

// openVoters is openvk's "Список проголосовавших" of an option, a box as
// of those who like a post.
function openVoters(postID, option, place) {
  openLikers(option.votes, postID, (id, cursor, limit) => api.pollVoters(id, place, cursor, limit), {
    head: () => 'Список проголосовавших » ' + option.name,
    people: (page) => page.users.map((user) => ({ user, href: pagePath(user) })),
  });
}

// initPollDraft wires "Прикрепить опрос" of a post form: link opens the
// box "Новый опрос", marker shows the attached poll ("Опрос") with a
// cross to take it off; onChange runs when it comes or goes. value() is
// the poll to send with the post, null for none.
export function initPollDraft({ link, marker, onChange }) {
  let draft = null;

  function render() {
    marker.replaceChildren(...(draft ? [
      el('a', { textContent: 'Опрос', title: 'Изменить опрос', onclick: open }),
      ': ' + draft.title + ' ',
      el('a', { className: 'poll-remove', title: 'Не прикреплять', textContent: '×', onclick: () => set(null) }),
    ] : []));
    marker.hidden = !draft;
    link.hidden = Boolean(draft);
    onChange?.();
  }

  function set(poll) {
    draft = poll;
    render();
  }

  function open() {
    openPollEditor(draft, set);
  }

  link.addEventListener('click', open);
  render();

  return {
    value: () => draft,
    clear: () => set(null),
  };
}

// openPollEditor is openvk's box "Новый опрос": the title, the options
// (a click on "Добавить вариант ответа..." adds one, Backspace in an empty
// one removes it, Enter or Tab in the last adds another) and how the poll
// is held. draft is the poll attached already, to change it.
function openPollEditor(draft, onAttach) {
  const modal = openModal('Новый опрос', { tag: 'form' });
  const error = el('div', { className: 'error', hidden: true });
  const title = el('input', { type: 'text', maxLength: MAX_LINE, placeholder: 'Тема опроса', value: draft?.title ?? '' });
  const options = el('div', { className: 'poll-editor-options' });
  const adder = el('input', { type: 'text', placeholder: 'Добавить вариант ответа...', className: 'poll-editor-adder' });
  const anonymous = el('input', { type: 'checkbox', checked: Boolean(draft?.anonymous) });
  const multiple = el('input', { type: 'checkbox', checked: Boolean(draft?.multiple) });
  const locked = el('input', { type: 'checkbox', checked: Boolean(draft?.locked) });
  const expires = el('input', { type: 'checkbox', checked: Boolean(draft?.days) });
  const days = el('select', {}, ...Array.from({ length: MAX_DAYS }, (_, i) => el('option', { value: String(i + 1), textContent: daysCount(i + 1) })));
  days.value = String(draft?.days || 1);

  const inputs = () => [...options.querySelectorAll('input')];

  function updateAdder() {
    adder.hidden = inputs().length >= MAX_OPTIONS;
  }

  function addOption(value = '') {
    if (inputs().length >= MAX_OPTIONS) return null;

    const input = el('input', { type: 'text', maxLength: MAX_LINE, value });
    input.addEventListener('keydown', (event) => {
      const all = inputs();
      const last = all[all.length - 1] === input;

      if (event.key === 'Backspace' && input.value === '' && all.length > 1) {
        event.preventDefault();
        const before = all[all.indexOf(input) - 1] ?? all[1];
        input.remove();
        updateAdder();
        before.focus();
      } else if ((event.key === 'Enter' || (event.key === 'Tab' && !event.shiftKey)) && last && input.value.trim() !== '') {
        const next = addOption();
        if (next) {
          event.preventDefault();
          next.focus();
        }
      } else if (event.key === 'Enter') {
        // Enter in a field of the box does not attach the poll.
        event.preventDefault();
      }
    });
    options.append(input);
    updateAdder();

    return input;
  }

  for (const name of draft?.options ?? ['', '']) addOption(name);

  adder.addEventListener('focus', () => addOption()?.focus());
  title.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      inputs()[0]?.focus();
    }
  });

  modal.box.classList.add('poll-editor');
  modal.box.addEventListener('submit', (event) => {
    event.preventDefault();

    const names = inputs().map((input) => input.value.trim()).filter(Boolean);
    if (title.value.trim() === '') {
      error.textContent = 'Укажите тему опроса.';
    } else if (names.length === 0) {
      error.textContent = 'Добавьте хотя бы один вариант ответа.';
    } else {
      modal.close();
      onAttach({
        title: title.value.trim(),
        options: names,
        anonymous: anonymous.checked,
        multiple: multiple.checked,
        locked: locked.checked,
        days: expires.checked ? Number(days.value) : 0,
      });
      return;
    }
    error.hidden = false;
  });

  modal.body.append(
    error,
    title,
    options,
    adder,
    el('label', {}, anonymous, ' Анонимный опрос'),
    el('label', {}, multiple, ' Множественный выбор'),
    el('label', {}, locked, ' Запретить отменять свой голос'),
    el('label', { className: 'poll-editor-expires' }, expires, ' Голосование истекает через: ', days),
    el('div', { className: 'hint', textContent: 'Нажатие Backspace в пустом варианте приводит к его удалению. Tab/Enter в последнем добавляет новый.' }),
  );
  modal.actions.append(button('Прикрепить', null, { type: 'submit' }), button('Отмена', modal.close, { secondary: true }));
  title.focus();
}
