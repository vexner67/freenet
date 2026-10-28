// "Основать государство" (/world/found): the capital is chosen on the map
// among the provinces where a state may stand (marked), then the name,
// the form of government and the flag; the state costs votes of the page.

import { api, currentUser, session, started } from './api.js';
import { governments, groupedVotes, votesAmount, worldTerrain } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { randomFlag } from './flag.js';
import { tr } from './i18n.js';
import { createWorldMap, loadMap } from './world-map.js';
import { foundable } from './world-rules.js';
import { flagEditor, statePath, title } from './world-ui.js';

const answered = started(api.world());

let world = null;
let atlas = null;
let map = null;
let allowed = new Set();
let capital = null;

const hint = (text) => el('p', { className: 'hint', textContent: text });

function chosen() {
  const p = capital === null ? null : atlas.provinces[capital];
  $('found-capital').replaceChildren(p
    ? el('span', {}, el('b', { textContent: p.name }), ` · ${worldTerrain(p.terrain)}`)
    : el('span', { className: 'hint', textContent: tr('Выберите на карте провинцию в полоску — там может стоять столица.') }));
}

function choose(p) {
  if (!p || !allowed.has(p.id)) return;
  capital = p.id;
  map.select(capital);
  chosen();
}

function form() {
  const field = (label, control) => el('tr', {}, el('td', { className: 'label', textContent: label }), el('td', {}, control));
  const name = el('input', { type: 'text', maxLength: 40, required: true });
  const adjective = el('input', { type: 'text', maxLength: 40, placeholder: tr('например, бореальский') });
  const motto = el('input', { type: 'text', maxLength: 80 });
  const government = el('select', {}, ...governments.map(([v, text]) => el('option', { value: v, textContent: text })));
  const flag = flagEditor(randomFlag(), Math.floor(Math.random() * 24));
  const error = el('div', { className: 'error', hidden: true });
  const submit = el('button', { type: 'submit', className: 'button', textContent: tr('Основать за {0}', votesAmount(world.rules.foundCost)) });

  const node = el('form', { className: 'world-found' },
    error,
    el('table', { className: 'world-form' },
      field(tr('Столица:'), el('span', { id: 'found-capital' })),
      field(tr('Название:'), name),
      field(tr('Прилагательное:'), adjective),
      field(tr('Девиз:'), motto),
      field(tr('Форма правления:'), government)),
    title(tr('Флаг и цвет')),
    flag.node,
    el('p', { className: 'hint', textContent: tr('Государство получит столицу и до {0} соседних ничьих провинций, {1} голосов в казну и {2} дн. защиты. Название потом не изменить.', world.rules.foundLands, groupedVotes(world.rules.foundTreasury), world.rules.protectionDays) }),
    submit);

  node.addEventListener('submit', async (event) => {
    event.preventDefault();
    hideError(error);
    if (capital === null) {
      showError(error, tr('Выберите столицу на карте.'));
      return;
    }

    const look = flag.get();
    submit.disabled = true;
    try {
      const { id } = await api.foundState({
        name: name.value.trim(),
        adjective: adjective.value.trim(),
        motto: motto.value.trim(),
        government: government.value,
        capital,
        color: look.color,
        flag: look.flag,
      });
      location.assign(statePath(id));
    } catch (e) {
      showError(error, e);
      submit.disabled = false;
    }
  });

  return node;
}

const me = await currentUser().catch(() => null);
renderChrome(me);

try {
  if (!me || !session.get()) {
    $('guest').hidden = false;
  } else {
    world = await answered;
    if (world.viewer.stateId) {
      location.replace(statePath(world.viewer.stateId));
    } else {
      $('found').hidden = false;
      $('found').replaceChildren(
        el('div', { className: 'world-frame' }, el('div', { className: 'world-map', id: 'world-map' })),
        form());
      chosen();

      const loaded = await loadMap(world.world.id);
      atlas = loaded.atlas;
      allowed = new Set(foundable(atlas.provinces, world));
      map = createWorldMap($('world-map'), { worldId: world.world.id, atlas, raster: loaded.raster, onSelect: choose });
      map.setWorld(world);
      map.mark([...allowed]);

      const asked = Number(new URLSearchParams(location.search).get('province'));
      if (allowed.has(asked)) {
        choose(atlas.provinces[asked]);
        map.show(asked);
      }
      if (!allowed.size) $('found').prepend(hint(tr('Свободной земли не осталось: подайте заявку на гражданство в одно из государств.')));
    }
  }
} catch (e) {
  showError($('error'), e);
}
