// "Мир" (/world): the map of the open world with its states, the panel of
// the province chosen on it, the rating of the states and the news of the
// world (?tab=rating, ?tab=news). A province is chosen by ?province=<n>
// too. The world service counts the hours; the page asks again when an
// hour ends.

import { api, currentUser, started } from './api.js';
import { citizensCount, formatDateTime, groupedVotes, hundredths, provincesCount, worldPopulation, worldRanking, worldResource, worldTerrain } from './labels.js';
import { $, el, renderChrome, showError } from './layout.js';
import { segmented } from './controls.js';
import { tr } from './i18n.js';
import { createWorldMap, loadMap } from './world-map.js';
import { colonizable, foundable, mayColonize, ownerOf } from './world-rules.js';
import { eventNode, personLink, stateLink, statePath, title } from './world-ui.js';

const answered = started(api.world());
const events = started(api.worldEvents());

let me = null;
let world = null;
let atlas = null;
let map = null;
let chosen = null;
let mode = 'political';
// What the viewer's state may colonize and what a colony costs it.
let colonyCost = 0;
let news = [];

const query = () => new URLSearchParams(location.search);
const tab = () => query().get('tab') ?? 'map';
const hint = (text) => el('p', { className: 'hint', textContent: text });
const votes = (n) => tr('{0} гол.', hundredths(n));

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

// --- the tabs ---

function tabs() {
  const link = (name, text) => el('a', { href: name === 'map' ? '/world' : `/world?tab=${name}`, className: tab() === name ? 'active' : '', textContent: text });
  const own = world.viewer.stateId;
  $('world-tabs').replaceChildren(...[
    link('map', tr('Карта')),
    own ? el('a', { href: statePath(own), textContent: tr('Моё государство') }) : null,
    link('rating', tr('Рейтинг')),
    link('news', tr('Новости мира')),
    me && !own ? el('a', { className: 'tabs-side', href: '/world/found', textContent: tr('Основать государство') }) : null,
  ].filter(Boolean));
}

// --- the map ---

function province(id) {
  return atlas.provinces[id] ?? null;
}

// panel is what the map tells of the province chosen, and what may be
// done with it.
function panel() {
  const box = $('world-panel');
  const p = chosen === null ? null : province(chosen);
  if (!p) {
    box.replaceChildren(hint(tr('Выберите провинцию на карте.')));
    return;
  }

  const fact = (name, value) => el('tr', {}, el('td', { className: 'world-fact-name', textContent: name }), value instanceof Node ? el('td', {}, value) : el('td', { textContent: value }));
  const owner = ownerOf(world, p.id);
  const colony = world.colonies.find((c) => c.province === p.id);
  const settler = colony && world.states.find((s) => s.id === colony.stateId);
  const capitalOf = world.states.find((s) => s.capital === p.id);

  const nodes = [
    el('div', { className: 'world-panel-name', textContent: p.name }),
    p.freeCity ? el('div', { className: 'world-badge', textContent: tr('Вольный город') }) : null,
    capitalOf ? el('div', { className: 'world-badge', textContent: tr('Столица') }) : null,
    el('table', { className: 'world-facts' },
      fact(tr('Владелец'), owner ? stateLink(owner) : (p.freeCity ? tr('никто') : tr('ничья земля'))),
      settler ? fact(tr('Заселяет'), el('span', {}, stateLink(settler), tr(' до {0}', formatDateTime(colony.until)))) : null,
      fact(tr('Местность'), worldTerrain(p.terrain)),
      fact(tr('Население'), worldPopulation(p.population ?? 0)),
      fact(tr('Ресурсы'), p.resources?.length ? p.resources.map(worldResource).join(', ') : tr('нет')),
      p.coastal ? fact(tr('Побережье'), tr('есть выход к морю')) : null),
  ];

  if (me && !world.viewer.stateId && foundable(atlas.provinces, world).includes(p.id)) {
    nodes.push(el('a', { className: 'button', href: `/world/found?province=${p.id}`, textContent: tr('Основать здесь государство') }));
  }
  if (mayColonize(world.viewer.role) && colonizable(atlas.provinces, world, world.viewer.stateId).includes(p.id)) {
    nodes.push(el('button', {
      type: 'button', className: 'button', textContent: colonyCost ? tr('Колонизировать за {0}', votes(colonyCost)) : tr('Колонизировать'),
      onclick: (event) => colonize(event.currentTarget, p.id),
    }), hint(tr('Колонисты из казны обоснуются здесь за {0} ч.', world.rules.colonizeHours)));
  }

  box.replaceChildren(...nodes.filter(Boolean));
}

async function colonize(button, id) {
  button.disabled = true;
  try {
    await api.colonize(id);
    said(tr('Колонисты отправлены: провинция «{0}» станет вашей через {1} ч.', province(id).name, world.rules.colonizeHours));
    await reload();
  } catch (e) {
    showError($('error'), e);
    button.disabled = false;
  }
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
  const owner = p && ownerOf(world, p.id);
  $('world-status').textContent = p ? `${p.name} · ${worldTerrain(p.terrain)}${owner ? ` · ${owner.name}` : ''}` : '';
}

// marks are where the viewer's state may send colonists, for those who
// send them.
function marks() {
  return mayColonize(world.viewer.role) ? colonizable(atlas.provinces, world, world.viewer.stateId) : [];
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
    ['political', tr('Политика')],
    ['relief', tr('Рельеф')],
    ['resources', tr('Ресурсы')],
  ], mode, (next) => {
    mode = next;
    map.setMode(next);
  });

  return [el('div', { className: 'world-bar' }, modes.node, el('span', { className: 'world-status', id: 'world-status' })), frame];
}

// start is what is shown under the map to one with no state.
function start() {
  if (!me) return [hint(tr('Чтобы основать государство или стать гражданином, войдите на сайт.'))];
  if (world.viewer.stateId) return [];

  if (world.viewer.appliedTo) {
    const state = world.states.find((s) => s.id === world.viewer.appliedTo);
    return [el('p', { className: 'hint' }, tr('Вы подали заявку на гражданство: '), state ? stateLink(state) : '', '. ',
      el('a', { href: '#', textContent: tr('Отозвать'), onclick: withdraw }))];
  }

  const step = (n, heading, text) => el('div', { className: 'firm-step' },
    el('div', { className: 'firm-step-head' }, el('span', { className: 'firm-step-number', textContent: String(n) }), el('b', { textContent: heading })),
    el('p', { textContent: text }));

  return [
    title(tr('Как начать')),
    el('div', { className: 'firm-steps' },
      step(1, tr('Выберите землю'), tr('Найдите на карте ничью провинцию подальше от чужих столиц — она станет вашей столицей.')),
      step(2, tr('Основайте государство'), tr('Придумайте название и флаг. Основание стоит {0} голосов, из них {1} лягут в казну.', groupedVotes(world.rules.foundCost), groupedVotes(world.rules.foundTreasury))),
      step(3, tr('Растите'), tr('Провинции каждый час платят в казну, а колонисты заселяют соседние земли. Зовите друзей в граждане.'))),
    hint(tr('Можно и не основывать своё: откройте страницу государства и подайте заявку на гражданство.')),
  ];
}

async function withdraw(event) {
  event.preventDefault();
  try {
    await api.withdrawApplication();
    await reload();
  } catch (e) {
    showError($('error'), e);
  }
}

// --- the rating ---

let measure = 'lands';

function rating() {
  const table = el('table', { className: 'world-rating' });
  const value = (s) => ({
    lands: provincesCount(s.lands),
    population: worldPopulation(s.population),
    citizens: citizensCount(s.citizens),
    income: tr('{0} в час', votes(s.income)),
  })[measure];
  const draw = () => table.replaceChildren(
    el('tr', {}, el('th', { textContent: '#' }), el('th', { textContent: tr('Государство') }), el('th', { textContent: tr('Правитель') }), el('th', { textContent: '' })),
    ...worldRanking(world.states, measure).map((s, i) => el('tr', {},
      el('td', { textContent: String(i + 1) }),
      el('td', {}, stateLink(s)),
      el('td', {}, personLink(s.ruler)),
      el('td', { className: 'world-rating-value', textContent: value(s) }))),
  );
  draw();

  const measures = segmented([
    ['lands', tr('по землям')],
    ['population', tr('по населению')],
    ['citizens', tr('по гражданам')],
    ['income', tr('по доходу')],
  ], measure, (next) => {
    measure = next;
    draw();
  });

  return [title(tr('Рейтинг государств')), el('div', { className: 'world-bar' }, measures.node),
    world.states.length ? table : hint(tr('Государств пока нет. Основайте первое!'))];
}

// --- the news ---

function newsView() {
  const list = el('div', { className: 'world-events' });
  const more = el('a', { href: '#', className: 'world-more', textContent: tr('Показать ещё') });
  const draw = () => {
    list.replaceChildren(...news.map((e) => eventNode(e, world.states, atlas?.provinces)));
    more.hidden = news.length === 0 || news.length % 50 !== 0;
  };
  more.addEventListener('click', async (event) => {
    event.preventDefault();
    try {
      const page = await api.worldEvents('', news.at(-1).id);
      news.push(...page.events);
      draw();
    } catch (e) {
      showError($('error'), e);
    }
  });
  draw();

  return [title(tr('Новости мира')), news.length ? list : hint(tr('В мире пока ничего не случилось.')), more];
}

// --- the page ---

function show() {
  tabs();
  if (tab() === 'rating') {
    $('world-view').replaceChildren(...rating());
    return;
  }
  if (tab() === 'news') {
    $('world-view').replaceChildren(...newsView());
    return;
  }

  $('world-view').replaceChildren(...mapView(), ...start());
}

async function startMap() {
  if (tab() !== 'map') return;
  try {
    const loaded = await loadMap(world.world.id);
    atlas = loaded.atlas;
    map = createWorldMap($('world-map'), { worldId: world.world.id, atlas, raster: loaded.raster, onSelect: choose, onHover: hover });
    map.setWorld(world);
    map.mark(marks());
    const asked = Number(query().get('province'));
    if (query().has('province') && province(asked)) {
      choose(province(asked));
      map.show(asked);
    } else if (world.viewer.stateId) {
      const own = world.states.find((s) => s.id === world.viewer.stateId);
      if (own) map.show(own.capital);
      panel();
    } else {
      panel();
    }
  } catch (e) {
    showError($('error'), e);
  }
}

async function costs() {
  if (!mayColonize(world.viewer.role)) return;
  try {
    colonyCost = (await api.worldState(world.viewer.stateId)).colonyCost;
    if (map) panel();
  } catch {
    // The button says no price.
  }
}

// reload asks for the world again: after an action and when an hour ends.
async function reload() {
  world = await api.world();
  tabs();
  if (map) {
    map.setWorld(world);
    map.mark(marks());
    panel();
  }
  costs();
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  world = await answered;
  news = (await events).events;
  $('world').hidden = false;
  show();
  await startMap();
  costs();
  setInterval(() => {
    if (document.hidden || !world) return;
    if (Date.now() > Date.parse(world.hour) + 3600_000 + 30_000) reload().catch(() => {});
  }, 60_000);
} catch (e) {
  if (e.status === 404) {
    $('closed').hidden = false;
  } else {
    showError($('error'), e);
  }
}
