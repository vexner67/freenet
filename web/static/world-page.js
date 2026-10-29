// "Мир" (/world): the map of the open world and the panel of the province
// chosen on it. A province is chosen by ?province=<n> too.

import { api, currentUser, started } from './api.js';
import { worldPopulation, worldResource, worldTerrain } from './labels.js';
import { $, el, renderChrome, showError } from './layout.js';
import { segmented } from './controls.js';
import { tr } from './i18n.js';
import { createWorldMap, loadMap } from './world-map.js';

const answered = started(api.world());

let atlas = null;
let map = null;
let chosen = null;
let mode = 'relief';

const query = () => new URLSearchParams(location.search);
const hint = (text) => el('p', { className: 'hint', textContent: text });

function province(id) {
  return atlas.provinces[id] ?? null;
}

// panel is what the map tells of the province chosen.
function panel() {
  const box = $('world-panel');
  const p = chosen === null ? null : province(chosen);
  if (!p) {
    box.replaceChildren(hint(tr('Выберите провинцию на карте.')));
    return;
  }

  const fact = (name, value) => el('tr', {}, el('td', { className: 'world-fact-name', textContent: name }), el('td', { textContent: value }));

  box.replaceChildren(...[
    el('div', { className: 'world-panel-name', textContent: p.name }),
    p.freeCity ? el('div', { className: 'world-badge', textContent: tr('Вольный город') }) : null,
    el('table', { className: 'world-facts' },
      fact(tr('Местность'), worldTerrain(p.terrain)),
      fact(tr('Население'), worldPopulation(p.population ?? 0)),
      fact(tr('Ресурсы'), p.resources?.length ? p.resources.map(worldResource).join(', ') : tr('нет')),
      p.coastal ? fact(tr('Побережье'), tr('есть выход к морю')) : null),
  ].filter(Boolean));
}

function choose(p) {
  chosen = p ? p.id : null;
  map.select(chosen);
  panel();
  const q = query();
  if (chosen === null) q.delete('province');
  else q.set('province', String(chosen));
  history.replaceState(null, '', `${location.pathname}${q.size ? `?${q}` : ''}`);
}

function hover(p) {
  $('world-status').textContent = p ? `${p.name} · ${worldTerrain(p.terrain)}` : '';
}

function mapView() {
  const frame = el('div', { className: 'world-frame' },
    el('div', { className: 'world-map', id: 'world-map' },
      el('div', { className: 'world-zoom' },
        el('button', { type: 'button', className: 'world-zoom-button', textContent: '+', title: tr('Приблизить'), onclick: () => map.zoom(1.6) }),
        el('button', { type: 'button', className: 'world-zoom-button', textContent: '−', title: tr('Отдалить'), onclick: () => map.zoom(0.625) }),
        el('button', { type: 'button', className: 'world-zoom-button', textContent: '⤢', title: tr('Развернуть'), onclick: () => full(!frame.classList.contains('world-full')) }))),
    el('div', { className: 'world-panel', id: 'world-panel' }));

  const full = (on) => {
    frame.classList.toggle('world-full', on);
    document.body.classList.toggle('world-fixed', on);
  };
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') full(false);
  });

  const modes = segmented([
    ['relief', tr('Рельеф')],
    ['resources', tr('Ресурсы')],
  ], mode, (next) => {
    mode = next;
    map.setMode(next);
  });

  return [el('div', { className: 'world-bar' }, modes.node, el('span', { className: 'world-status', id: 'world-status' })), frame];
}

async function startMap(worldId) {
  try {
    const loaded = await loadMap(worldId);
    atlas = loaded.atlas;
    map = createWorldMap($('world-map'), { worldId, atlas, raster: loaded.raster, onSelect: choose, onHover: hover });
    const asked = Number(query().get('province'));
    if (query().has('province') && province(asked)) {
      choose(province(asked));
      map.show(asked);
    } else {
      panel();
    }
  } catch (e) {
    showError($('error'), e);
  }
}

renderChrome(await currentUser().catch(() => null));

try {
  const { world } = await answered;
  $('world').hidden = false;
  $('world-view').replaceChildren(...mapView());
  await startMap(world.id);
} catch (e) {
  if (e.status === 404) {
    $('closed').hidden = false;
  } else {
    showError($('error'), e);
  }
}
