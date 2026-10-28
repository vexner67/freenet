// What the world service allows, told ahead on the map of "Мир": where a
// state may be founded and where its colonists may go. The service checks
// it all again (world.CheckFound, world.Colonize); this only marks the
// provinces, so it is written as the domain is.

// landNeighbours are the provinces of land a province borders.
function landNeighbours(provinces, id) {
  return (provinces[id]?.links ?? []).map((l) => l.to).filter((to) => provinces[to] && !provinces[to].sea);
}

// distances are the steps over land from id to every province within
// limit steps (world.Atlas.distances).
export function distances(provinces, id, limit) {
  const dist = new Map([[id, 0]]);
  const queue = [id];
  for (let k = 0; k < queue.length; k++) {
    const c = queue[k];
    if (dist.get(c) === limit) continue;
    for (const n of landNeighbours(provinces, c)) {
      if (!dist.has(n)) {
        dist.set(n, dist.get(c) + 1);
        queue.push(n);
      }
    }
  }

  return dist;
}

// isSettleable: land nobody holds, settles, nor keeps as a free city.
export function isSettleable(provinces, world, id) {
  const p = provinces[id];
  if (!p || p.sea || p.freeCity) return false;

  return !(world.owners[id] ?? 0) && !world.colonies.some((c) => c.province === id);
}

// foundable are the provinces a capital may be founded in: settleable,
// and fewer than capitalDistance steps from no capital.
export function foundable(provinces, world) {
  const limit = world.rules.capitalDistance - 1;
  const near = new Set();
  for (const s of world.states) {
    for (const id of distances(provinces, s.capital, limit).keys()) near.add(id);
  }

  return provinces.filter((p) => !near.has(p.id) && isSettleable(provinces, world, p.id)).map((p) => p.id);
}

// colonizable are the settleable provinces bordering a land of the state.
export function colonizable(provinces, world, stateId) {
  const owner = world.states.findIndex((s) => s.id === stateId) + 1;
  if (!owner) return [];

  const out = new Set();
  provinces.forEach((p) => {
    if ((world.owners[p.id] ?? 0) !== owner) return;
    for (const n of landNeighbours(provinces, p.id)) {
      if (isSettleable(provinces, world, n)) out.add(n);
    }
  });

  return [...out];
}

// mayColonize and mayAnswer are the offices that send colonists and that
// answer the applications (world.mayColonize, world.mayAnswer).
export const mayColonize = (role) => role === 'ruler' || role === 'minister_economy';
export const mayAnswer = (role) => role === 'ruler' || role === 'minister_interior';

// ownerOf is the state of a province, null for none.
export function ownerOf(world, id) {
  return world.states[(world.owners[id] ?? 0) - 1] ?? null;
}
