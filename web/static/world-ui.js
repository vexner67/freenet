// What the pages of "Мир" show alike: a state with its flag, a line of the
// news, the builder of a flag and the colors of a state.

import { COLORS, fitFlag, flagEmblems, flagNode, flagPatterns, patternColors } from './flag.js';
import { flagEmblemNames, flagPatternNames, formatDateTime, worldEventParts } from './labels.js';
import { el, fullName, pagePath } from './layout.js';
import { tr } from './i18n.js';

export const statePath = (id) => `/world/state/${encodeURIComponent(id)}`;

export const title = (text) => el('div', { className: 'section-title', textContent: text });

// stateLink is a state as a link: its flag and name.
export function stateLink(state, width = 21) {
  return el('a', { className: 'world-state-link', href: statePath(state.id) }, flagNode(state.flag, width), ' ', state.name);
}

export const personLink = (p) => (p ? el('a', { href: pagePath(p), textContent: fullName(p) }) : tr('—'));

// eventNode is a line of the news; states gives the flags of those still
// standing, provinces the names of the provinces.
export function eventNode(e, states, provinces) {
  const province = (n) => provinces?.[n]?.name || tr('провинция №{0}', n);
  const parts = worldEventParts(e, province).map((part) => {
    if (typeof part === 'string') return part;
    if (part.person) return personLink(part.person);
    const state = states.find((s) => s.id === part.state);
    return state ? stateLink(state, 15) : el('b', { textContent: part.name });
  });

  return el('div', { className: 'world-event' },
    el('span', { className: 'world-event-at', textContent: formatDateTime(e.at) }),
    el('span', { className: 'world-event-text' }, ...parts));
}

// swatches are the colors of a palette to choose from: prefix is the class
// of their background (flag-b, state-b).
function swatches(prefix, value, onchange) {
  const node = el('span', { className: 'world-swatches' });
  const draw = (current) => node.replaceChildren(...Array.from({ length: COLORS }, (_, c) => el('button', {
    type: 'button',
    className: `world-swatch ${prefix}${c}${c === current ? ' chosen' : ''}`,
    title: tr('Цвет {0}', c + 1),
    onclick: () => {
      draw(c);
      onchange(c);
    },
  })));
  draw(value);

  return node;
}

function select(options, value, onchange) {
  const node = el('select', {}, ...options.map(([v, text]) => el('option', { value: v, textContent: text, selected: v === value })));
  node.addEventListener('change', () => onchange(node.value));

  return node;
}

// flagEditor builds a flag and the color of a state: { node, get } with
// get() → { flag, color }.
export function flagEditor(start, startColor) {
  let flag = fitFlag(start);
  let color = startColor;
  const preview = el('span', { className: 'world-flag-preview' });
  const slots = el('div', { className: 'world-flag-colors' });
  const emblemRow = el('div', { className: 'world-flag-row' });
  const row = (label, ...control) => el('div', { className: 'world-flag-row' }, el('span', { className: 'world-flag-label', textContent: label }), ...control);

  function draw() {
    preview.replaceChildren(flagNode(flag, 120));
    slots.replaceChildren(...flag.colors.map((c, i) => row(
      patternColors(flag.pattern) > 1 ? tr('Цвет {0}:', i + 1) : tr('Цвет:'),
      swatches('flag-b', c, (next) => {
        flag.colors[i] = next;
        draw();
      }),
    )));
    emblemRow.replaceChildren(...(flag.emblem ? [
      row(tr('Цвет эмблемы:'), swatches('flag-b', flag.emblemColor, (next) => {
        flag.emblemColor = next;
        draw();
      })),
      row(tr('Место:'), select([['center', tr('в центре')], ['canton', tr('в крыже')]], flag.emblemAt, (next) => {
        flag.emblemAt = next;
        draw();
      })),
    ] : []));
  }

  const node = el('div', { className: 'world-flag-editor' },
    preview,
    el('div', { className: 'world-flag-controls' },
      row(tr('Узор:'), select(flagPatterns.map(([p]) => [p, flagPatternNames[p]]), flag.pattern, (next) => {
        flag = fitFlag({ ...flag, pattern: next });
        draw();
      })),
      slots,
      row(tr('Эмблема:'), select(flagEmblems.map((e) => [e, flagEmblemNames[e]]), flag.emblem, (next) => {
        flag = { ...flag, emblem: next, emblemAt: flag.emblemAt || 'center' };
        draw();
      })),
      emblemRow,
      row(tr('Цвет на карте:'), swatches('state-b', color, (next) => {
        color = next;
      }))));
  draw();

  return { node, get: () => ({ flag: { ...flag, colors: [...flag.colors] }, color }) };
}
