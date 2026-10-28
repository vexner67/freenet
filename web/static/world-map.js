// The map of "Мир" on a canvas, without libraries: the picture of the
// land in tiles the world service drew (the closest zoom there is, and a
// coarser one stretched while it loads, so the map is never empty), and
// over it what changes every hour — the states — painted in the browser
// from the raster of the provinces: every cell of the map knows its
// province, so a state is the cells of its provinces, and the province
// under a finger is one look into the raster. The map is dragged with a
// mouse or a finger, zoomed with the wheel, two fingers or a double
// click; a click chooses a province.

import { el } from './layout.js';
import { worldResources } from './labels.js';

// A tile is 256 pixels; at zoom 0 the world is two tiles wide.
export const TILE = 256;
export const BASE_WIDTH = 2 * TILE;

// The closest zoom any map has; the one a world has is learned when its
// tiles run out.
const MAX_ZOOM = 6;

// The canvas of a phone is drawn no finer than twice its pixels: finer is
// slow and not seen.
const MAX_RATIO = 2;

// tileZoom is the zoom of the tiles for scale (screen pixels a cell) of a
// map width cells wide: the coarsest whose pixels are not larger than
// those of the screen.
export function tileZoom(scale, width, maxZoom = MAX_ZOOM) {
  const wanted = (width * scale) / BASE_WIDTH;
  let z = 0;
  while (z < maxZoom && 2 ** z < wanted) z++;

  return z;
}

// visibleTiles are the tiles of zoom z a view shows: a view is the cell at
// the left top corner (x, y), the scale and the size of the screen.
export function visibleTiles(view, z, width, height) {
  const pixels = (BASE_WIDTH * 2 ** z) / width; // tile pixels a cell
  const columns = 2 ** (z + 1);
  const rows = 2 ** z;
  const x0 = Math.max(0, Math.floor((view.x * pixels) / TILE));
  const y0 = Math.max(0, Math.floor((view.y * pixels) / TILE));
  const x1 = Math.min(columns - 1, Math.floor(((view.x + view.width / view.scale) * pixels) / TILE));
  const y1 = Math.min(rows - 1, Math.floor(((view.y + view.height / view.scale) * pixels) / TILE));
  const tiles = [];
  for (let ty = y0; ty <= y1 && ty * TILE < height * pixels; ty++) {
    for (let tx = x0; tx <= x1; tx++) tiles.push([tx, ty]);
  }

  return tiles;
}

// cellAt is the cell under a point of the screen, null off the map.
export function cellAt(view, sx, sy, width, height) {
  const x = Math.floor(view.x + sx / view.scale);
  const y = Math.floor(view.y + sy / view.scale);
  if (x < 0 || y < 0 || x >= width || y >= height) return null;

  return { x, y };
}

// stateCenters are where the names of the states stand: the middle of
// their provinces weighed by area, and how wide the state is (cells).
export function stateCenters(owners, provinces) {
  const sums = new Map();
  for (const p of provinces) {
    const owner = owners[p.id] ?? 0;
    if (!owner) continue;
    const s = sums.get(owner) ?? { x: 0, y: 0, area: 0, minX: Infinity, maxX: -Infinity };
    s.x += p.x * p.area;
    s.y += p.y * p.area;
    s.area += p.area;
    s.minX = Math.min(s.minX, p.x);
    s.maxX = Math.max(s.maxX, p.x);
    sums.set(owner, s);
  }

  const centers = new Map();
  for (const [owner, s] of sums) centers.set(owner, { x: s.x / s.area, y: s.y / s.area, span: Math.max(s.maxX - s.minX, Math.sqrt(s.area)) });

  return centers;
}

// parseRGB reads "rgb(1, 2, 3)" as [1, 2, 3].
export function parseRGB(text) {
  const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(text ?? '');

  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : [128, 128, 128];
}

// clampView keeps the map on the screen: it never leaves more than half
// the screen empty, and is never smaller than the screen.
export function clampView(view, width, height) {
  const minScale = Math.min(view.width / width, view.height / height);
  const scale = Math.max(view.scale, minScale);
  const shownW = view.width / scale;
  const shownH = view.height / scale;
  const clamp = (v, shown, size) => (shown >= size ? (size - shown) / 2 : Math.min(Math.max(v, 0), size - shown));

  return { ...view, scale, x: clamp(view.x, shownW, width), y: clamp(view.y, shownH, height) };
}

// colorOf reads a color of the stylesheet off a probe element: the
// palette stays in palette.css and follows the theme.
function colorOf(box, className, property = 'backgroundColor') {
  const probe = el('span', { className: `world-probe ${className}` });
  box.append(probe);
  const color = parseRGB(getComputedStyle(probe)[property]);
  probe.remove();

  return color;
}

// createWorldMap draws the map of a world in box. options: worldId, atlas,
// raster (the province of every cell), onSelect(province or null),
// onHover(province or null). The map it returns is told what changes.
export function createWorldMap(box, { worldId, atlas, raster, onSelect = () => {}, onHover = () => {} }) {
  const canvas = el('canvas', { className: 'world-canvas' });
  box.append(canvas);
  const ctx = canvas.getContext('2d');
  const W = atlas.width;
  const H = atlas.height;
  const provinces = atlas.provinces;

  let view = { x: 0, y: 0, scale: 1, width: 1, height: 1 };
  let ratio = 1;
  let maxZoom = MAX_ZOOM;
  let mode = 'political';
  let owners = [];
  let states = [];
  let colonies = [];
  let selected = null;
  let marked = new Set();
  let frame = 0;
  const tiles = new Map();

  const overlay = document.createElement('canvas');
  overlay.width = W;
  overlay.height = H;
  const octx = overlay.getContext('2d');

  // --- the tiles ---

  function tile(z, x, y) {
    const key = `${z}/${x}/${y}`;
    let t = tiles.get(key);
    if (t) return t;

    const image = new Image();
    t = { image, ready: false };
    tiles.set(key, t);
    image.onload = () => {
      t.ready = true;
      redraw();
    };
    image.onerror = () => {
      // A map has fewer zooms than asked: the closer ones are not tried.
      if (z > 0 && z <= maxZoom) {
        maxZoom = z - 1;
        redraw();
      }
    };
    image.src = `/media/world/${encodeURIComponent(worldId)}/tiles/${z}/${x}/${y}`;

    return t;
  }

  // drawTile draws tile x, y of zoom z, or the part of a coarser tile it
  // is in while it loads.
  function drawTile(z, x, y) {
    const size = (TILE * W) / (BASE_WIDTH * 2 ** z); // cells a tile
    const dx = (x * size - view.x) * view.scale;
    const dy = (y * size - view.y) * view.scale;
    const ds = size * view.scale;

    for (let up = 0; up <= z; up++) {
      const t = up === 0 ? tile(z, x, y) : tiles.get(`${z - up}/${x >> up}/${y >> up}`);
      if (!t?.ready) continue;
      const part = TILE >> up;
      ctx.drawImage(t.image, (x % (1 << up)) * part, (y % (1 << up)) * part, part, part, dx, dy, ds + 0.5, ds + 0.5);
      return;
    }
  }

  // --- the states over the land ---

  // paint draws the states, the marked and the chosen provinces into the
  // overlay, a pixel a cell.
  function paint() {
    const image = octx.createImageData(W, H);
    const px = image.data;
    const palette = states.map((s) => colorOf(box, `state-b${s.color}`));
    const markColor = colorOf(box, 'world-mark');
    const pickColor = colorOf(box, 'world-pick');
    const colonyOf = new Map(colonies.map((c) => [c.province, c.stateId]));
    const stateIndex = new Map(states.map((s, i) => [s.id, i]));
    const fill = mode === 'political' ? 110 : 0;
    const ownerOf = (p) => owners[p] ?? 0;

    for (let y = 0; y < H; y++) {
      for (let x = 0; x < W; x++) {
        const i = y * W + x;
        const p = raster[i];
        const owner = ownerOf(p);
        const o = i * 4;
        let color = null;
        let alpha = 0;

        if (owner && mode !== 'relief') {
          // The edge of a state is drawn strong, its inside faint.
          const edge = (x > 0 && ownerOf(raster[i - 1]) !== owner) || (x < W - 1 && ownerOf(raster[i + 1]) !== owner) ||
            (y > 0 && ownerOf(raster[i - W]) !== owner) || (y < H - 1 && ownerOf(raster[i + W]) !== owner);
          color = palette[owner - 1];
          alpha = edge ? 230 : fill;
        } else if (colonyOf.has(p) && mode !== 'relief') {
          // A colony is striped in the color of its state.
          color = palette[stateIndex.get(colonyOf.get(p))];
          alpha = (x + y) % 4 < 2 ? 150 : 0;
        }
        if (marked.has(p)) {
          color = markColor;
          alpha = (x + y) % 3 === 0 ? 200 : 90;
        }
        if (p === selected) {
          const edge = raster[i - 1] !== p || raster[i + 1] !== p || raster[i - W] !== p || raster[i + W] !== p;
          color = pickColor;
          alpha = edge ? 255 : 70;
        }
        if (!color || !alpha) continue;

        px[o] = color[0];
        px[o + 1] = color[1];
        px[o + 2] = color[2];
        px[o + 3] = alpha;
      }
    }

    octx.putImageData(image, 0, 0);
    redraw();
  }

  // --- the words over the map ---

  function drawLabels() {
    const text = colorOf(box, 'world-label', 'color');
    const halo = colorOf(box, 'world-halo', 'color');
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.lineJoin = 'round';
    ctx.strokeStyle = `rgba(${halo.join(',')},0.85)`;
    ctx.fillStyle = `rgb(${text.join(',')})`;
    const at = (x, y) => [(x - view.x) * view.scale * ratio, (y - view.y) * view.scale * ratio];
    const taken = [];
    const free = (x, y, w, h) => {
      if (taken.some((r) => x < r.x + r.w && r.x < x + w && y < r.y + r.h && r.y < y + h)) return false;
      taken.push({ x, y, w, h });
      return true;
    };
    const word = (s, x, y, size, bold = false) => {
      ctx.font = `${bold ? 'bold ' : ''}${Math.round(size * ratio)}px Tahoma, Verdana, sans-serif`;
      const w = ctx.measureText(s).width;
      if (!free(x - w / 2, y - size * ratio / 2, w, size * ratio)) return;
      ctx.lineWidth = 3 * ratio;
      ctx.strokeText(s, x, y);
      ctx.fillText(s, x, y);
    };

    // The states by their names, the larger first; the provinces once
    // they are large enough to hold a name.
    if (mode !== 'relief') {
      const centers = stateCenters(owners, provinces);
      const shown = [...centers].sort((a, b) => b[1].span - a[1].span);
      for (const [owner, c] of shown) {
        const s = states[owner - 1];
        if (!s) continue;
        const [x, y] = at(c.x, c.y);
        word(s.name.toUpperCase(), x, y, Math.min(18, Math.max(10, c.span * view.scale * 0.15)), true);
      }
    }

    for (const p of provinces) {
      if (p.sea || !p.name) continue;
      const side = Math.sqrt(p.area) * view.scale;
      const [x, y] = at(p.x, p.y);
      if (x < 0 || y < 0 || x > canvas.width || y > canvas.height) continue;
      if (mode === 'resources' && p.resources?.length && side > 14) {
        word(p.resources.map((r) => worldResources[r]?.[1] ?? '').join(''), x, y + 12 * ratio, 12);
      }
      if (side > 60 || (p.freeCity && side > 30)) word(p.name, x, y, 10);
    }
  }

  // --- drawing ---

  function draw() {
    frame = 0;
    const width = canvas.width;
    const height = canvas.height;
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.fillStyle = `rgb(${colorOf(box, 'world-sea').join(',')})`;
    ctx.fillRect(0, 0, width, height);

    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    const z = tileZoom(view.scale * ratio, W, maxZoom);
    for (const [x, y] of visibleTiles(view, z, W, H)) drawTile(z, x, y);

    ctx.imageSmoothingEnabled = view.scale < 2;
    ctx.drawImage(overlay, -view.x * view.scale, -view.y * view.scale, W * view.scale, H * view.scale);
    ctx.imageSmoothingEnabled = true;

    ctx.setTransform(1, 0, 0, 1, 0, 0);
    drawLabels();
  }

  function redraw() {
    if (!frame) frame = requestAnimationFrame(draw);
  }

  function setView(next) {
    view = clampView({ ...view, ...next }, W, H);
    redraw();
  }

  function resize() {
    const rect = box.getBoundingClientRect();
    ratio = Math.min(window.devicePixelRatio || 1, MAX_RATIO);
    canvas.width = Math.max(1, Math.round(rect.width * ratio));
    canvas.height = Math.max(1, Math.round(rect.height * ratio));
    const first = view.width === 1;
    // The middle of the screen stays where it was.
    const cx = view.x + view.width / view.scale / 2;
    const cy = view.y + view.height / view.scale / 2;
    view = { ...view, width: rect.width, height: rect.height };
    if (first) {
      setView({ scale: 0 });
      return;
    }
    setView({ x: cx - rect.width / view.scale / 2, y: cy - rect.height / view.scale / 2 });
  }

  // zoomAt scales the map by factor keeping the point sx, sy of the
  // screen over the same cell.
  function zoomAt(factor, sx, sy) {
    const scale = Math.min(Math.max(view.scale * factor, 0.1), 16);
    const cx = view.x + sx / view.scale;
    const cy = view.y + sy / view.scale;
    setView({ scale, x: cx - sx / scale, y: cy - sy / scale });
  }

  function provinceAtPoint(sx, sy) {
    const cell = cellAt(view, sx, sy, W, H);
    if (!cell) return null;
    const p = provinces[raster[cell.y * W + cell.x]];

    return p && !p.sea ? p : null;
  }

  // --- the mouse and the fingers ---

  const pointers = new Map();
  let moved = 0;
  let pinch = 0;
  let lastTap = 0;

  const point = (event) => {
    const r = canvas.getBoundingClientRect();
    return { x: event.clientX - r.left, y: event.clientY - r.top };
  };

  canvas.addEventListener('pointerdown', (event) => {
    canvas.setPointerCapture(event.pointerId);
    pointers.set(event.pointerId, point(event));
    moved = 0;
    if (pointers.size === 2) {
      const [a, b] = [...pointers.values()];
      pinch = Math.hypot(a.x - b.x, a.y - b.y);
    }
  });

  canvas.addEventListener('pointermove', (event) => {
    const now = point(event);
    const before = pointers.get(event.pointerId);
    if (!before) {
      onHover(provinceAtPoint(now.x, now.y));
      return;
    }

    if (pointers.size === 1) {
      moved += Math.abs(now.x - before.x) + Math.abs(now.y - before.y);
      setView({ x: view.x - (now.x - before.x) / view.scale, y: view.y - (now.y - before.y) / view.scale });
    } else if (pointers.size === 2) {
      pointers.set(event.pointerId, now);
      const [a, b] = [...pointers.values()];
      const distance = Math.hypot(a.x - b.x, a.y - b.y);
      if (pinch) zoomAt(distance / pinch, (a.x + b.x) / 2, (a.y + b.y) / 2);
      pinch = distance;
      moved += 10;
    }
    pointers.set(event.pointerId, now);
  });

  const release = (event) => {
    const at = pointers.get(event.pointerId);
    pointers.delete(event.pointerId);
    if (pointers.size < 2) pinch = 0;
    if (!at || moved > 6 || event.type === 'pointercancel') return;

    // A second tap soon after the first zooms in, as maps do.
    const now = Date.now();
    if (event.pointerType !== 'mouse' && now - lastTap < 300) {
      zoomAt(2, at.x, at.y);
      lastTap = 0;
      return;
    }
    lastTap = now;
    onSelect(provinceAtPoint(at.x, at.y));
  };
  canvas.addEventListener('pointerup', release);
  canvas.addEventListener('pointercancel', release);
  canvas.addEventListener('pointerleave', () => onHover(null));

  canvas.addEventListener('wheel', (event) => {
    event.preventDefault();
    const at = point(event);
    zoomAt(event.deltaY < 0 ? 1.25 : 0.8, at.x, at.y);
  }, { passive: false });

  canvas.addEventListener('dblclick', (event) => {
    const at = point(event);
    zoomAt(2, at.x, at.y);
  });

  new ResizeObserver(resize).observe(box);
  resize();

  return {
    // setWorld gives the states and who owns what (the owner of province
    // p is states[owners[p] - 1]).
    setWorld(next) {
      ({ owners, states, colonies } = next);
      paint();
    },
    setMode(next) {
      mode = next;
      paint();
    },
    select(id) {
      selected = id;
      paint();
    },
    mark(ids) {
      marked = new Set(ids);
      paint();
    },
    // show brings province id to the middle, closer if the map is far.
    show(id) {
      const p = provinces[id];
      if (!p) return;
      const scale = Math.max(view.scale, 2);
      setView({ scale, x: p.x - view.width / scale / 2, y: p.y - view.height / scale / 2 });
    },
    zoom(factor) {
      zoomAt(factor, view.width / 2, view.height / 2);
    },
    resize,
  };
}

// loadMap reads the atlas and the raster of a world, cached for good by
// the browser.
export async function loadMap(worldId) {
  const base = `/media/world/${encodeURIComponent(worldId)}`;
  const [atlas, raster] = await Promise.all([
    fetch(`${base}/atlas`).then((r) => (r.ok ? r.json() : Promise.reject(new Error('map')))),
    fetch(`${base}/raster`).then((r) => (r.ok ? r.arrayBuffer() : Promise.reject(new Error('map')))),
  ]);

  return { atlas, raster: new Uint16Array(raster) };
}
