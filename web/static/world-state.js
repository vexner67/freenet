// The page of a state of "Мир" (/world/state/<id>): its flag, ruler and
// numbers, its lands and colonies, the citizens with their offices, the
// budget of the last day and its news. Who may answers the applications,
// gives offices, expels and sends colonists; a citizen gives votes to the
// treasury; the ruler changes the flag and hands the state on.

import { api, currentUser, started } from './api.js';
import { citizensCount, formatDateTime, governmentName, hundredths, provincesCount, worldPopulation, worldRole, worldRoles } from './labels.js';
import { $, el, renderChrome, showError, hideError } from './layout.js';
import { flagNode } from './flag.js';
import { stepper } from './controls.js';
import { tr } from './i18n.js';
import { mayAnswer } from './world-rules.js';
import { eventNode, flagEditor, personLink, title } from './world-ui.js';

const stateId = decodeURIComponent(location.pathname.split('/')[3] ?? '');
const answered = started(api.worldState(stateId));
const worldAnswered = started(api.world());
const newsAnswered = started(api.worldEvents(stateId));

let me = null;
let view = null;
let world = null;
let news = [];
let provinces = [];

const hint = (text) => el('p', { className: 'hint', textContent: text });
const votes = (n) => tr('{0} гол.', hundredths(n));
const provinceName = (n) => provinces[n]?.name || tr('провинция №{0}', n);
const provinceLink = (n) => el('a', { href: `/world?province=${n}`, textContent: provinceName(n) });

function said(text) {
  $('done').textContent = text;
  $('done').hidden = !text;
}

// act runs an action of the page and shows the state anew.
async function act(button, run, done = '') {
  hideError($('error'));
  if (button) button.disabled = true;
  try {
    await run();
    said(done);
    await reload();
  } catch (e) {
    showError($('error'), e);
    if (button) button.disabled = false;
  }
}

const action = (text, run, done) => el('button', { type: 'button', className: 'button', textContent: text, onclick: (event) => act(event.currentTarget, run, done) });

// --- the parts of the page ---

function header() {
  const s = view.state;
  const protectedNow = Date.parse(s.protectedUntil) > Date.now();

  return el('div', { className: 'firm-header' },
    el('span', { className: 'world-state-flag' }, flagNode(s.flag, 90, s.name)),
    el('div', { className: 'firm-header-text' },
      el('span', { className: 'firm-header-name', textContent: s.name }),
      s.motto ? el('i', { className: 'world-motto', textContent: `«${s.motto}»` }) : null,
      el('span', { className: 'firm-header-kind' }, governmentName(s.government), ' · ', tr('правитель '), personLink(s.ruler)),
      el('span', { className: 'firm-header-kind', textContent: tr('Основано {0}', formatDateTime(s.foundedAt)) }),
      protectedNow ? el('span', { className: 'world-badge', textContent: tr('Под защитой до {0}', formatDateTime(s.protectedUntil)) }) : null,
      s.clubId ? el('a', { href: `/club/${encodeURIComponent(s.clubId)}`, textContent: tr('Группа государства') }) : null));
}

function stats() {
  const s = view.state;
  const stat = (name, value, sub = '') => el('div', { className: 'firm-stat' },
    el('div', { className: 'firm-stat-name', textContent: name }),
    el('div', { className: 'firm-stat-value', textContent: value }),
    el('div', { className: 'firm-stat-sub', textContent: sub }));

  return el('div', { className: 'firm-stats' },
    stat(tr('Земли'), provincesCount(s.lands), view.colonies.length ? tr('{0} заселяются', view.colonies.length) : ''),
    stat(tr('Население'), worldPopulation(s.population)),
    stat(tr('Граждане'), citizensCount(s.citizens)),
    stat(tr('Казна'), votes(s.treasury), tr('+{0} в час', votes(s.income))),
    stat(tr('Колония'), votes(view.colonyCost), tr('столько стоит следующая')));
}

// mine is what the viewer may do as who they are here.
function mine() {
  if (!me) return [hint(tr('Чтобы стать гражданином, войдите на сайт.'))];
  const v = view.viewer;
  const here = v.stateId === stateId;

  if (!v.stateId) {
    if (v.appliedTo === stateId) return [el('p', {}, tr('Вы подали заявку на гражданство. '), action(tr('Отозвать заявку'), () => api.withdrawApplication(), tr('Заявка отозвана.')))];
    if (v.appliedTo) return [hint(tr('Вы уже подали заявку в другое государство.'))];
    return [action(tr('Подать заявку на гражданство'), () => api.applyCitizenship(stateId), tr('Заявка отправлена. Её рассмотрит правитель или министр внутренних дел.'))];
  }
  if (!here) return [];

  const nodes = [el('p', {}, tr('Вы — {0} этого государства.', worldRole(v.role)))];
  const amount = stepper({ value: 100, min: 1, max: Math.max(1, me.votes ?? 1), step: 100, wide: true });
  nodes.push(el('div', { className: 'world-row' },
    el('span', { textContent: tr('Внести в казну:') }), amount.node,
    action(tr('Внести'), () => api.donateToState(amount.get()), tr('Голоса внесены в казну.'))));
  if (v.role !== 'ruler') nodes.push(action(tr('Покинуть государство'), () => api.leaveState(), tr('Вы больше не гражданин.')));

  return nodes;
}

function lands() {
  const nodes = [title(tr('Земли')), el('div', { className: 'world-lands' }, ...view.lands.flatMap((n, i) => [i ? ', ' : '', provinceLink(n)]))];
  if (view.colonies.length) {
    nodes.push(el('p', {}, tr('Заселяются: '), ...view.colonies.flatMap((c, i) => [i ? ', ' : '', provinceLink(c.province), tr(' (до {0})', formatDateTime(c.until))])));
  }
  if (view.viewer.stateId === stateId && (view.viewer.role === 'ruler' || view.viewer.role === 'minister_economy')) {
    nodes.push(hint(tr('Чтобы отправить колонистов, выберите ничью соседнюю провинцию на карте.')), el('a', { href: '/world', textContent: tr('Открыть карту') }));
  }

  return nodes;
}

function applications() {
  if (!view.applications.length) return [];

  return [title(tr('Заявки на гражданство')), el('div', { className: 'world-people' }, ...view.applications.map((a) => el('div', { className: 'world-person' },
    personLink(a.user),
    el('span', { className: 'hint', textContent: formatDateTime(a.at) }),
    action(tr('Принять'), () => api.answerApplication(a.user.id, true), tr('Новый гражданин принят.')),
    action(tr('Отклонить'), () => api.answerApplication(a.user.id, false), tr('Заявка отклонена.')))))];
}

function citizens() {
  const v = view.viewer;
  const ruler = v.stateId === stateId && v.role === 'ruler';
  const expels = v.stateId === stateId && mayAnswer(v.role);
  const minister = (role) => role.startsWith('minister_');

  const row = (c) => {
    const self = c.user.id === me?.id;
    const nodes = [personLink(c.user), el('span', { className: 'world-role', textContent: worldRole(c.role) })];
    if (ruler && !self) {
      const office = el('select', {}, ...worldRoles.map(([r, text]) => el('option', { value: r, textContent: text, selected: r === c.role })));
      office.addEventListener('change', () => act(office, () => api.appointCitizen(c.user.id, office.value), tr('Должность изменена.')));
      nodes.push(office);
    }
    if (!self && c.role !== 'ruler' && (ruler || (expels && !minister(c.role)))) {
      nodes.push(action(tr('Изгнать'), () => api.expelCitizen(c.user.id), tr('Гражданин изгнан.')));
    }

    return el('div', { className: 'world-person' }, ...nodes);
  };

  const nodes = [title(tr('Граждане')), el('div', { className: 'world-people' }, ...view.citizens.map(row))];
  if (ruler && view.citizens.length > 1) nodes.push(abdication());

  return nodes;
}

function abdication() {
  const heirs = view.citizens.filter((c) => c.user.id !== me?.id);
  const heir = el('select', {}, ...heirs.map((c) => el('option', { value: c.user.id, textContent: `${c.user.name} ${c.user.surname}`.trim() })));

  return el('div', { className: 'world-row' },
    el('span', { textContent: tr('Передать власть:') }), heir,
    action(tr('Передать'), () => api.abdicate(heir.value), tr('Власть передана.')));
}

function budget() {
  const hours = view.budget.filter((b) => b.income || b.donations || b.founding || b.colonization);
  if (!hours.length) return [title(tr('Бюджет за сутки')), hint(tr('За последние сутки в казну ничего не поступало.'))];

  const sum = (b) => b.income + b.donations + b.founding + b.colonization;
  const cell = (n) => el('td', { className: n < 0 ? 'stock-down' : '', textContent: n ? votes(n) : '—' });

  return [title(tr('Бюджет за сутки')), el('div', { className: 'world-budget-box' }, el('table', { className: 'world-budget' },
    el('tr', {}, ...[tr('Час'), tr('Доход провинций'), tr('Взносы'), tr('Основание'), tr('Колонии'), tr('Итого')].map((t) => el('th', { textContent: t }))),
    ...hours.reverse().map((b) => el('tr', {}, el('td', { textContent: formatDateTime(b.hour) }), cell(b.income), cell(b.donations), cell(b.founding), cell(b.colonization), cell(sum(b))))))];
}

function edit() {
  const v = view.viewer;
  if (v.stateId !== stateId || v.role !== 'ruler') return [];

  const s = view.state;
  const editor = flagEditor(s.flag, s.color);
  const adjective = el('input', { type: 'text', maxLength: 40, value: s.adjective });
  const motto = el('input', { type: 'text', maxLength: 80, value: s.motto });
  const field = (label, control) => el('tr', {}, el('td', { className: 'label', textContent: label }), el('td', {}, control));
  const save = action(tr('Сохранить'), () => {
    const look = editor.get();
    return api.editState({ adjective: adjective.value.trim(), motto: motto.value.trim(), color: look.color, flag: look.flag });
  }, tr('Изменения сохранены.'));

  return [title(tr('Флаг и девиз')), el('details', { className: 'world-edit' },
    el('summary', { textContent: tr('Изменить флаг, цвет и девиз') }),
    el('table', { className: 'world-form' }, field(tr('Прилагательное:'), adjective), field(tr('Девиз:'), motto)),
    editor.node, save)];
}

function newsView() {
  if (!news.length) return [];

  return [title(tr('Новости государства')), el('div', { className: 'world-events' }, ...news.slice(0, 20).map((e) => eventNode(e, world?.states ?? [view.state], provinces)))];
}

function show() {
  $('state').replaceChildren(header(), stats(), ...mine(), ...applications(), ...lands(), ...citizens(), ...budget(), ...edit(), ...newsView());
  document.title = `${view.state.name} | Freenet`;
  $('state-title').textContent = view.state.name;
}

async function reload() {
  [view, world] = await Promise.all([api.worldState(stateId), api.world()]);
  news = (await api.worldEvents(stateId)).events;
  me = await currentUser().catch(() => me);
  show();
}

me = await currentUser().catch(() => null);
renderChrome(me);

try {
  view = await answered;
  world = await worldAnswered.catch(() => null);
  news = (await newsAnswered.catch(() => ({ events: [] }))).events;
  $('state').hidden = false;
  show();

  // The names of the provinces come with the atlas, cached for good.
  if (world) {
    const atlas = await fetch(`/media/world/${encodeURIComponent(world.world.id)}/atlas`).then((r) => (r.ok ? r.json() : null)).catch(() => null);
    if (atlas) {
      provinces = atlas.provinces;
      show();
    }
  }
} catch (e) {
  showError($('error'), e);
}
