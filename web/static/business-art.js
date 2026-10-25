// The pictures of "Бизнес": each business is its premises seen from
// above, in pixels as the games of the 2000s drew them — the hall with the
// counter, the bays or the workshop, the storeroom with its racks, the
// street with the billboard. Customers walk in (drive in at the carwash,
// come by lorry to the factory), workers stand at their places, and every
// improvement shows: the rug and the plants of the decor, the sign over the
// door, the machines, the racks, the floor of the finish, the uniforms of
// the trained staff. Worn premises crack, a ruined business is boarded up.
// Everything is drawn here with rectangles on a small canvas, scaled up by
// a whole number so the pixels stay square.

import { businessCondition, businessStations, businessWalkers } from './labels.js';
import { tr } from './i18n.js';

export const W = 224;
export const H = 144;

// The plan of the premises: the wall along the top, the floor, the wall
// along the bottom, the sidewalk and the road; the hall on the left, the
// storeroom behind the inner wall on the right.
const FACE = 6;
const FLOOR = 22;
const BOTTOM = 110;
const WALK = 116;
const ROAD = 128;
const HALL = [4, 152];
const STORE = [156, 220];
const PASS = [70, 90];

const doors = {
  kiosk: [60, 76],
  cafe: [60, 76],
  carwash: [52, 108],
  factory: [96, 128],
};

const C = {
  outline: '#2b2b2b',
  wallTop: '#3d4250',
  wallTopLight: '#555b6b',
  baseboard: '#6b5233',
  skin: ['#f1c27d', '#e0ac69', '#c68642', '#8d5524'],
  hair: ['#3b2a1a', '#6a4e2d', '#c9a15b', '#1d1d1d', '#a0522d'],
  shirts: ['#c0392b', '#2e86c1', '#27ae60', '#8e44ad', '#d68910', '#16a085', '#7f8c8d'],
  pants: ['#2c3e50', '#34495e', '#5d4037', '#1b2631'],
  shoes: '#1c1c1c',
  uniform: ['#8a8f96', '#2f5ea8', '#2e8b57', '#b03a2e'],
  gold: '#f2c230',
  goldDark: '#b8860b',
  walk: '#bdbdbd',
  walkLine: '#a5a5a5',
  road: '#4b4d52',
  roadLine: '#e8e8e8',
  wood: '#8b5a2b',
  woodLight: '#a8713c',
  woodDark: '#5e3b1a',
  metal: '#9ea7b0',
  metalDark: '#6b7480',
  metalLight: '#cfd6dd',
  glass: '#9fd3e6',
  glassDark: '#5aa7c4',
  leaf: '#3c9a3c',
  leafDark: '#2a6e2a',
  pot: '#b5562d',
  rug: '#3d5aa8',
  rugDark: '#2a4080',
  rugLight: '#7a96d6',
  sofa: '#a0392f',
  sofaDark: '#6e231c',
  box: '#c8955a',
  boxDark: '#9a6c3a',
  red: '#d23a32',
  white: '#ffffff',
  black: '#111111',
  water: '#7fc8f0',
  foam: '#f4f8fb',
  yellow: '#f0c419',
  crack: '#4a3a2a',
};

// The floors and walls of the finish: worn boards, parquet, tiles,
// marble; the carwash and the factory have concrete, cleaner each tier.
const finishes = [
  { face: '#b3a27f', trim: '#7d6a4a', floor: ['#9e6d43', '#946540'], seam: '#7a5232' },
  { face: '#d8c59c', trim: '#8b6b3e', floor: ['#b57d47', '#a26d3c'], seam: '#80562e' },
  { face: '#cfe1e6', trim: '#5f8a96', floor: ['#e8ecee', '#c9d3d8'], seam: '#a9b4ba' },
  { face: '#f0e4c6', trim: '#c9a227', floor: ['#ece8df', '#e2ddd2'], seam: '#c8c1b4' },
];
const concrete = [
  { face: '#a9aaa2', trim: '#6e6f69', floor: ['#8e908c', '#878985'], seam: '#73756f' },
  { face: '#c3c4bc', trim: '#7d7e78', floor: ['#9c9e9a', '#959793'], seam: '#80827d' },
  { face: '#d5dde2', trim: '#6c8794', floor: ['#a9b2b7', '#a2abb0'], seam: '#8a959b' },
  { face: '#e6e9ec', trim: '#c9a227', floor: ['#bcc4c9', '#b4bcc1'], seam: '#9aa4aa' },
];

// random is a little generator of the same numbers for the same seed, so
// cracks and seams stay where they were from frame to frame.
function random(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const tierOf = (b, perk) => b.perks?.find((p) => p.perk === perk)?.tier ?? 0;

// Sprites: rows of letters, each a colour of the palette given.
const person = [
  '..hhhh..',
  '.hhhhhhh',
  '.hsssshh',
  '..ssss..',
  '.bbbbbb.',
  'sbbbbbbs',
  'sbbbbbbs',
  '.bbbbbb.',
];
const legs = [
  ['.pppppp.', '.pp..pp.', '.pp..pp.', '.kk..kk.'],
  ['.pppppp.', '.pp..pp.', '.kk..pp.', '.....kk.'],
  ['.pppppp.', '.pp..pp.', '.pp..kk.', '.kk.....'],
];
const car = [
  '..bbbbbb..',
  '.bbbbbbbb.',
  '.bwwwwwwb.',
  '.bwwwwwwb.',
  'kbbbbbbbbk',
  'kbbllllbbk',
  '.bbllllbb.',
  '.bbllllbb.',
  '.bbllllbb.',
  '.bbbbbbbb.',
  'kbbbbbbbbk',
  'kbwwwwwwbk',
  '.bwwwwwwb.',
  '.bbbbbbbb.',
  '..bbbbbb..',
];

function sprite(ctx, rows, x, y, colors, turned = false) {
  for (let r = 0; r < rows.length; r++) {
    const row = rows[r];
    for (let c = 0; c < row.length; c++) {
      const color = colors[row[c]];
      if (!color) continue;
      ctx.fillStyle = color;
      if (turned) ctx.fillRect(x + r, y + c, 1, 1);
      else ctx.fillRect(x + c, y + r, 1, 1);
    }
  }
}

function rect(ctx, x, y, w, h, color) {
  ctx.fillStyle = color;
  ctx.fillRect(Math.round(x), Math.round(y), Math.round(w), Math.round(h));
}

function frame(ctx, x, y, w, h, color) {
  rect(ctx, x, y, w, 1, color);
  rect(ctx, x, y + h - 1, w, 1, color);
  rect(ctx, x, y, 1, h, color);
  rect(ctx, x + w - 1, y, 1, h, color);
}

// drawPerson draws a person standing with their feet at x, y.
function drawPerson(ctx, p, x, y, step) {
  const colors = { h: p.hair, s: p.skin, b: p.shirt, p: p.pants, k: C.shoes };
  const left = Math.round(x) - 4;
  const top = Math.round(y) - 12;
  rect(ctx, left + 1, top + 11, 6, 2, 'rgba(0,0,0,0.18)');
  sprite(ctx, person, left, top, colors);
  sprite(ctx, legs[step], left, top + 8, colors);
  if (p.cap) {
    rect(ctx, left + 1, top, 6, 2, p.cap);
    rect(ctx, left + 1, top + 2, 7, 1, p.capBrim ?? p.cap);
  }
  if (p.apron) rect(ctx, left + 2, top + 5, 4, 3, p.apron);
  if (p.badge) rect(ctx, left + 5, top + 5, 1, 1, p.badge);
  if (p.box) {
    rect(ctx, left + 1, top + 3, 6, 4, C.box);
    rect(ctx, left + 1, top + 5, 6, 1, C.boxDark);
  }
}

function drawCar(ctx, a, x, y, turned) {
  const colors = { b: a.shirt, w: '#2b3a4a', k: C.black, l: a.roof };
  if (turned) sprite(ctx, car, Math.round(x) - 7, Math.round(y) - 5, colors, true);
  else sprite(ctx, car, Math.round(x) - 5, Math.round(y) - 7, colors);
}

function drawTruck(ctx, a, x, y, flip) {
  const left = Math.round(x) - 14;
  const top = Math.round(y) - 6;
  rect(ctx, left + 1, top + 11, 28, 2, 'rgba(0,0,0,0.2)');
  const cab = flip ? left : left + 20;
  const box = flip ? left + 9 : left;
  rect(ctx, box, top, 20, 12, a.roof);
  frame(ctx, box, top, 20, 12, C.outline);
  rect(ctx, box + 2, top + 5, 16, 1, '#00000022');
  rect(ctx, cab, top + 1, 9, 10, a.shirt);
  frame(ctx, cab, top + 1, 9, 10, C.outline);
  rect(ctx, flip ? cab + 1 : cab + 5, top + 3, 3, 6, '#2b3a4a');
}

// Premises keeps the people of a picture and draws it frame by frame.
export class Premises {
  constructor(kind) {
    this.kind = kind;
    this.agents = [];
    this.seed = [...kind].reduce((n, c) => n * 31 + c.charCodeAt(0), 7);
    this.hover = null;
    this.parts = [];
    this.time = 0;
  }

  // set gives the picture the business as it is now; the people walking
  // stay, only as many come and go as the business draws.
  set(b, plan) {
    this.b = b;
    this.plan = plan;
    let { served, passing } = businessWalkers(b, plan);
    // Cars and lorries do not queue on top of each other: as many come in
    // as there are bays, and one lorry loads at the gate at a time.
    if (this.kind === 'carwash') served = Math.min(served, this.stations());
    if (this.kind === 'factory') served = Math.min(served, 1);
    const want = [...Array(served).fill(true), ...Array(passing).fill(false)];

    this.agents = this.agents.filter((a, i) => i < want.length);
    want.forEach((going, i) => {
      const a = this.agents[i];
      if (!a || a.going !== going) this.agents[i] = this.newAgent(going, i);
    });
    this.parts = this.layout();
  }

  newAgent(going, i) {
    const rnd = random(this.seed + i * 977 + Math.floor(this.time * 10));
    const pick = (list) => list[Math.floor(rnd() * list.length)];
    const a = {
      going,
      skin: pick(C.skin),
      hair: pick(C.hair),
      shirt: pick(C.shirts),
      roof: pick(['#e6e6e6', '#f3d34a', '#9fb7c9', '#d9d9d9']),
      pants: pick(C.pants),
      slot: i,
      wait: 0,
      leg: 0,
      delay: i * 1.7,
    };
    a.path = this.route(a, rnd);
    a.x = a.path[0].x;
    a.y = a.path[0].y;
    a.at = 0;
    return a;
  }

  stations() {
    return businessStations(this.b?.level ?? 1);
  }

  // places are where the people of the hall are served, a point each.
  places() {
    const n = this.stations();
    switch (this.kind) {
      case 'carwash':
        return Array.from({ length: n }, (_, i) => ({ x: 22 + i * 34, y: 58 }));
      case 'factory':
        return [];
      default:
        return Array.from({ length: n }, (_, i) => ({ x: 34 + i * 30, y: 54 }));
    }
  }

  tables() {
    if (this.kind !== 'cafe') return [];
    const n = Math.min(8, 2 + (this.b?.level ?? 1));
    return Array.from({ length: n }, (_, i) => ({ x: 22 + (i % 4) * 32, y: 72 + Math.floor(i / 4) * 22 }));
  }

  // route is the way of a person (a car, a lorry): along the street, or
  // in through the door to be served and back out.
  route(a, rnd) {
    const fromLeft = rnd() < 0.5;
    const [d0, d1] = doors[this.kind];
    const door = (d0 + d1) / 2;
    const off = fromLeft ? -16 : W + 16;
    const away = fromLeft ? W + 16 : -16;

    if (this.kind === 'carwash' || this.kind === 'factory') {
      const lane = fromLeft ? ROAD + 11 : ROAD + 5;
      a.vehicle = this.kind === 'carwash' ? 'car' : 'truck';
      if (!a.going) return [{ x: off, y: lane }, { x: away, y: lane }];
      if (this.kind === 'factory') {
        return [{ x: off, y: lane }, { x: door, y: lane, wait: 3.5, load: true }, { x: away, y: lane }];
      }
      const bays = this.places();
      const bay = bays[a.slot % bays.length];
      return [
        { x: off, y: lane }, { x: door, y: lane }, { x: door, y: BOTTOM + 4 },
        { x: door, y: 92 }, { x: bay.x, y: 92 }, { x: bay.x, y: bay.y + 6, wait: 3, wash: true },
        { x: bay.x, y: 92 }, { x: door, y: 92 }, { x: door, y: lane }, { x: away, y: lane },
      ];
    }

    const lane = WALK + 4 + Math.floor(rnd() * 6);
    if (!a.going) return [{ x: off, y: lane }, { x: away, y: lane }];

    const places = this.places();
    const place = places[a.slot % places.length];
    const queue = Math.floor(a.slot / places.length);
    const path = [
      { x: off, y: lane }, { x: door, y: lane }, { x: door, y: BOTTOM - 2 },
      { x: place.x, y: place.y + 8 + queue * 13, wait: 2.5, buy: true },
    ];
    const tables = this.tables();
    if (tables.length) {
      const t = tables[(a.slot * 3 + Math.floor(rnd() * tables.length)) % tables.length];
      path.push({ x: t.x - 9, y: t.y + 6, wait: 4, sit: true });
    }
    path.push({ x: door, y: BOTTOM - 2 }, { x: door, y: lane }, { x: away, y: lane });
    return path;
  }

  step(dt, still) {
    this.time += dt;
    for (let i = 0; i < this.agents.length; i++) {
      const a = this.agents[i];
      if (still) continue;
      if (a.delay > 0) {
        a.delay -= dt;
        continue;
      }
      if (a.wait > 0) {
        a.wait -= dt;
        continue;
      }
      const next = a.path[a.at + 1];
      if (!next) {
        this.agents[i] = this.newAgent(a.going, a.slot);
        this.agents[i].delay = 0.5 + (i % 3);
        continue;
      }
      const speed = a.vehicle ? 34 : 18;
      const dx = next.x - a.x;
      const dy = next.y - a.y;
      const dist = Math.hypot(dx, dy);
      const move = speed * dt;
      a.turned = Math.abs(dx) > Math.abs(dy);
      a.flip = dx < 0;
      if (dist <= move) {
        a.x = next.x;
        a.y = next.y;
        a.at++;
        a.wait = next.wait ?? 0;
        a.doing = next;
      } else {
        a.x += (dx / dist) * move;
        a.y += (dy / dist) * move;
        a.leg += move;
        a.doing = null;
      }
    }
  }

  // layout is where the parts of the picture are, for the clicks.
  layout() {
    const parts = [];
    const n = this.stations();
    if (this.kind === 'carwash') {
      parts.push({ part: 'till', x: 6, y: 92, w: 14, h: 16, label: tr('Касса') });
      parts.push({ part: 'staff', x: 6, y: FLOOR, w: 34 * n + 8, h: 18, label: tr('Работники') });
    } else if (this.kind === 'factory') {
      parts.push({ part: 'till', x: 8, y: FLOOR + 2, w: 22, h: 16, label: tr('Касса') });
      parts.push({ part: 'staff', x: 34, y: FLOOR + 6, w: 116, h: 46, label: tr('Работники') });
    } else {
      parts.push({ part: 'till', x: 10, y: 34, w: 14, h: 14, label: tr('Касса') });
      parts.push({ part: 'staff', x: 24, y: FLOOR, w: 30 * n + 4, h: 26, label: tr('Работники') });
    }
    parts.push({ part: 'storage', x: STORE[0], y: FLOOR, w: STORE[1] - STORE[0], h: BOTTOM - FLOOR, label: tr('Склад') });
    parts.push({ part: 'ad', x: 182, y: WALK, w: 36, h: 12, label: tr('Рекламный щит') });
    parts.push({ part: 'perks', x: doors[this.kind][1] + 2, y: BOTTOM - 1, w: 30, h: 8, label: tr('Улучшения') });
    parts.push({ part: 'building', x: HALL[0], y: FACE, w: HALL[1] - HALL[0], h: BOTTOM - FACE + 6, label: tr('Здание') });
    return parts;
  }

  partAt(x, y) {
    return this.parts.find((p) => x >= p.x && x < p.x + p.w && y >= p.y && y < p.y + p.h) ?? null;
  }

  draw(ctx) {
    const b = this.b;
    if (!b) return;
    const rnd = random(this.seed);
    const finish = tierOf(b, 'finish');
    const style = (this.kind === 'carwash' || this.kind === 'factory' ? concrete : finishes)[finish];

    this.drawStreet(ctx);
    this.drawFloor(ctx, style, finish, rnd);
    this.drawWalls(ctx, style, finish);
    this.drawFixtures(ctx);
    this.drawDecor(ctx, tierOf(b, 'decor'));
    this.drawStore(ctx, tierOf(b, 'shelves'));
    if (this.kind === 'carwash') this.drawBays(ctx);
    else if (this.kind === 'factory') this.drawWorkshop(ctx);
    else this.drawCounter(ctx);
    this.drawTables(ctx);
    this.drawWorkers(ctx);
    this.drawSign(ctx, tierOf(b, 'brand'));
    this.drawBillboard(ctx);
    this.drawWear(ctx, rnd);

    const people = [...this.agents].filter((a) => a.delay <= 0).sort((p, q) => p.y - q.y);
    for (const a of people) this.drawAgent(ctx, a);

    if (b.bankrupt) this.drawRuin(ctx);
    else this.drawAlerts(ctx);

    if (this.hover) {
      const p = this.hover;
      frame(ctx, p.x, p.y, p.w, p.h, C.yellow);
      frame(ctx, p.x + 1, p.y + 1, p.w - 2, p.h - 2, 'rgba(255,255,255,0.5)');
    }
  }

  drawStreet(ctx) {
    rect(ctx, 0, WALK, W, ROAD - WALK, C.walk);
    for (let x = 0; x < W; x += 12) rect(ctx, x, WALK, 1, ROAD - WALK, C.walkLine);
    rect(ctx, 0, WALK + 6, W, 1, C.walkLine);
    rect(ctx, 0, ROAD, W, H - ROAD, C.road);
    rect(ctx, 0, ROAD, W, 1, '#3a3c40');
    for (let x = 0; x < W; x += 16) rect(ctx, x, ROAD + 8, 8, 1, C.roadLine);
    // Trees along the sidewalk.
    for (const x of [26, 134]) {
      rect(ctx, x - 5, WALK + 1, 10, 9, C.leafDark);
      rect(ctx, x - 4, WALK + 1, 8, 7, C.leaf);
      rect(ctx, x - 2, WALK + 2, 3, 2, '#5cbf5c');
    }
  }

  drawFloor(ctx, style, finish, rnd) {
    const [x0, x1] = HALL;
    const [a, c] = style.floor;
    const industrial = this.kind === 'carwash' || this.kind === 'factory';
    for (let y = FLOOR; y < BOTTOM; y += 8) {
      for (let x = x0; x < x1; x += 8) {
        const odd = ((x - x0) / 8 + (y - FLOOR) / 8) % 2;
        let color = a;
        if (industrial) color = odd ? a : c;
        else if (finish === 0) color = ((y - FLOOR) / 4) % 2 ? a : c;
        else color = odd ? a : c;
        rect(ctx, x, y, Math.min(8, x1 - x), 8, color);
      }
    }
    if (!industrial && finish === 0) {
      for (let y = FLOOR; y < BOTTOM; y += 4) {
        rect(ctx, x0, y, x1 - x0, 1, style.seam);
        for (let x = x0 + Math.floor(rnd() * 24); x < x1; x += 18 + Math.floor(rnd() * 20)) rect(ctx, x, y, 1, 4, style.seam);
      }
    } else if (!industrial && finish === 1) {
      for (let y = FLOOR; y < BOTTOM; y += 8) for (let x = x0; x < x1; x += 8) {
        rect(ctx, x, y + 3, 8, 1, style.seam);
        rect(ctx, x + 3, y, 1, 8, style.seam);
      }
    } else {
      for (let y = FLOOR; y < BOTTOM; y += finish === 3 ? 16 : 8) rect(ctx, x0, y, x1 - x0, 1, style.seam);
      for (let x = x0; x < x1; x += finish === 3 ? 16 : 8) rect(ctx, x, FLOOR, 1, BOTTOM - FLOOR, style.seam);
      if (finish === 3) {
        for (let i = 0; i < 40; i++) rect(ctx, x0 + rnd() * (x1 - x0), FLOOR + rnd() * (BOTTOM - FLOOR), 2, 1, '#d6cfc0');
      }
    }
    // The storeroom has plain boards whatever the finish.
    for (let y = FLOOR; y < BOTTOM; y += 4) {
      rect(ctx, STORE[0], y, STORE[1] - STORE[0], 4, ((y - FLOOR) / 4) % 2 ? '#a58560' : '#9a7a55');
    }
    if (this.kind === 'factory') {
      rect(ctx, x0, 62, x1 - x0, 1, C.yellow);
      rect(ctx, x0, 78, x1 - x0, 1, C.yellow);
    }
  }

  drawWalls(ctx, style, finish) {
    // The top wall: its top seen from above, then its face.
    rect(ctx, 0, 0, W, FACE, C.wallTop);
    rect(ctx, 0, FACE - 1, W, 1, C.wallTopLight);
    rect(ctx, 0, FACE, W, FLOOR - FACE, style.face);
    rect(ctx, 0, FLOOR - 2, W, 2, style.trim);
    if (finish >= 2) for (let x = 6; x < W; x += 12) rect(ctx, x, FACE + 2, 1, FLOOR - FACE - 5, '#00000010');
    // The side walls, the inner one with its passage.
    rect(ctx, 0, 0, HALL[0], BOTTOM, C.wallTop);
    rect(ctx, STORE[1], 0, W - STORE[1], BOTTOM, C.wallTop);
    rect(ctx, HALL[1], 0, STORE[0] - HALL[1], PASS[0], C.wallTop);
    rect(ctx, HALL[1], PASS[1], STORE[0] - HALL[1], BOTTOM - PASS[1], C.wallTop);
    // The bottom wall with the door (the gate).
    const [d0, d1] = doors[this.kind];
    rect(ctx, 0, BOTTOM, d0, WALK - BOTTOM, C.wallTop);
    rect(ctx, d1, BOTTOM, W - d1, WALK - BOTTOM, C.wallTop);
    rect(ctx, 0, BOTTOM, d0, 1, C.wallTopLight);
    rect(ctx, d1, BOTTOM, W - d1, 1, C.wallTopLight);
    rect(ctx, d0, BOTTOM + 2, d1 - d0, 2, this.kind === 'carwash' || this.kind === 'factory' ? C.yellow : '#7a6a55');
    // Windows on the bottom wall.
    for (const x of [16, 132, 172]) {
      if (x + 14 > d0 && x < d1) continue;
      rect(ctx, x, BOTTOM + 1, 14, 3, C.glass);
      rect(ctx, x, BOTTOM + 1, 14, 1, C.glassDark);
    }
  }

  // drawFixtures is what each kind of premises has from the start: the
  // gondolas of the kiosk, the tyres and the vacuum of the carwash, the
  // pallets of the factory.
  drawFixtures(ctx) {
    if (this.kind === 'kiosk') {
      for (const x of [132, 143]) {
        rect(ctx, x, 54, 8, 48, C.metalDark);
        rect(ctx, x + 1, 55, 6, 46, C.metalLight);
        rect(ctx, x + 3, 55, 1, 46, C.metal);
        for (let y = 56, i = 0; y < 99; y += 4, i++) {
          rect(ctx, x + 1, y, 2, 3, C.shirts[i % C.shirts.length]);
          rect(ctx, x + 4, y, 2, 3, C.shirts[(i + 3) % C.shirts.length]);
        }
      }
    } else if (this.kind === 'cafe') {
      // A menu board on the wall.
      rect(ctx, 76, FACE + 2, 18, 13, C.black);
      for (let k = 0; k < 4; k++) rect(ctx, 78, FACE + 4 + k * 3, 10 - (k % 2) * 3, 1, C.white);
    } else if (this.kind === 'carwash') {
      for (let i = 0; i < 3; i++) {
        rect(ctx, 140, 92 + i * 5, 10, 5, C.black);
        rect(ctx, 142, 93 + i * 5, 6, 3, '#3a3a3a');
      }
      rect(ctx, 141, 78, 8, 12, C.yellow);
      rect(ctx, 142, 79, 6, 4, C.black);
      rect(ctx, 144, 85, 2, 5, C.metalDark);
    } else if (this.kind === 'factory') {
      for (const [x, y] of [[118, 86], [134, 86], [118, 98]]) {
        rect(ctx, x, y + 8, 14, 3, C.woodDark);
        rect(ctx, x + 1, y, 12, 8, C.box);
        rect(ctx, x + 1, y + 3, 12, 1, C.boxDark);
        rect(ctx, x + 6, y, 1, 8, C.boxDark);
      }
      // A forklift by the pallets.
      rect(ctx, 138, 98, 10, 9, C.yellow);
      rect(ctx, 140, 99, 6, 4, '#2b3a4a');
      rect(ctx, 136, 104, 2, 4, C.metalDark);
    }
  }

  drawCounter(ctx) {
    const n = this.stations();
    const equip = tierOf(this.b, 'equipment');
    const x = 24;
    const w = n * 30 + 4;
    // The back wall: fridges at the kiosk, coffee machines at the cafe.
    for (let i = 0; i <= equip; i++) {
      const ex = 28 + i * 26;
      if (this.kind === 'kiosk') {
        rect(ctx, ex, FACE + 3, 16, FLOOR - FACE - 1, C.metalDark);
        rect(ctx, ex + 1, FACE + 4, 14, FLOOR - FACE - 4, C.glass);
        for (let s = 0; s < 3; s++) {
          for (let k = 0; k < 4; k++) rect(ctx, ex + 2 + k * 3, FACE + 5 + s * 5, 2, 3, C.shirts[(s + k + i) % C.shirts.length]);
        }
      } else if (i > 0 || equip === 0) {
        rect(ctx, ex, FACE + 6, 14, 12, C.metalDark);
        rect(ctx, ex + 1, FACE + 7, 12, 6, C.metalLight);
        rect(ctx, ex + 4, FACE + 13, 6, 3, C.black);
        rect(ctx, ex + 2, FACE + 8, 2, 2, equip >= 2 ? C.red : C.metal);
        if (equip >= 3) rect(ctx, ex + 9, FACE + 8, 3, 2, C.gold);
      }
    }
    // The counter itself.
    rect(ctx, x, 40, w, 8, C.wood);
    rect(ctx, x, 40, w, 2, C.woodLight);
    rect(ctx, x, 47, w, 1, C.woodDark);
    if (this.kind === 'cafe') {
      for (let i = 0; i < n; i++) {
        rect(ctx, x + 8 + i * 30, 41, 4, 3, C.white);
        rect(ctx, x + 14 + i * 30, 42, 6, 2, '#e8b04a');
      }
    } else {
      for (let i = 0; i < n * 3; i++) rect(ctx, x + 6 + i * 9, 41, 4, 3, C.shirts[i % C.shirts.length]);
    }
    // The cash register.
    rect(ctx, 10, 36, 14, 12, C.wood);
    rect(ctx, 12, 37, 10, 6, C.metalDark);
    rect(ctx, 13, 38, 8, 3, '#79e07a');
    rect(ctx, 12, 44, 10, 3, C.metal);
  }

  drawTables(ctx) {
    for (const t of this.tables()) {
      rect(ctx, t.x - 6, t.y - 2, 12, 9, C.woodDark);
      rect(ctx, t.x - 5, t.y - 1, 10, 7, C.woodLight);
      rect(ctx, t.x - 1, t.y + 1, 3, 2, C.white);
      rect(ctx, t.x - 10, t.y, 3, 5, C.wood);
      rect(ctx, t.x + 7, t.y, 3, 5, C.wood);
    }
  }

  drawBays(ctx) {
    const equip = tierOf(this.b, 'equipment');
    this.places().forEach((p) => {
      rect(ctx, p.x - 15, FLOOR + 6, 1, 70, C.yellow);
      rect(ctx, p.x + 15, FLOOR + 6, 1, 70, C.yellow);
      rect(ctx, p.x - 4, p.y + 14, 8, 3, '#5d6166');
      for (let k = 0; k < 4; k++) rect(ctx, p.x - 3 + k * 2, p.y + 15, 1, 1, '#3c3f43');
      // The washer on the wall, bigger each tier.
      rect(ctx, p.x - 6, FACE + 4, 12, 10 + equip, C.metalDark);
      rect(ctx, p.x - 5, FACE + 5, 10, 4, equip >= 2 ? '#3d7fd1' : C.metal);
      rect(ctx, p.x - 1, FACE + 10, 2, 4 + equip, C.black);
      if (equip >= 3) rect(ctx, p.x + 2, FACE + 6, 2, 2, C.gold);
      if (equip >= 1) rect(ctx, p.x - 14, FACE + 8, 4, 8, C.red);
    });
    // The terminal of the till by the gate.
    rect(ctx, 6, 92, 14, 16, C.metalDark);
    rect(ctx, 8, 94, 10, 6, '#79e07a');
    rect(ctx, 8, 102, 10, 4, C.metal);
  }

  drawWorkshop(ctx) {
    const equip = tierOf(this.b, 'equipment');
    const working = !this.b.bankrupt && this.b.served > 0;
    // The office desk with the till.
    rect(ctx, 8, FLOOR + 2, 22, 12, C.wood);
    rect(ctx, 10, FLOOR + 3, 8, 6, C.black);
    rect(ctx, 11, FLOOR + 4, 6, 4, '#79e07a');
    rect(ctx, 20, FLOOR + 4, 8, 6, C.white);
    // The conveyor into the storeroom.
    rect(ctx, 8, 66, STORE[0] + 10 - 8, 8, '#3b3d42');
    const shift = working ? Math.floor(this.time * 12) % 6 : 0;
    for (let x = 8 + shift; x < STORE[0] + 10; x += 6) rect(ctx, x, 67, 1, 6, '#5b5e64');
    rect(ctx, 8, 66, STORE[0] + 2, 1, C.metal);
    rect(ctx, 8, 73, STORE[0] + 2, 1, C.metal);
    if (working) {
      for (let i = 0; i < 6; i++) {
        const x = 12 + ((this.time * 12 + i * 28) % (STORE[0] - 4));
        rect(ctx, x, 67, 6, 6, C.box);
        rect(ctx, x, 69, 6, 1, C.boxDark);
      }
    }
    // The machines along the belt.
    for (let i = 0; i <= equip; i++) {
      const mx = 40 + i * 28;
      rect(ctx, mx, 42, 20, 22, C.metalDark);
      rect(ctx, mx + 1, 43, 18, 6, C.metal);
      rect(ctx, mx + 3, 51, 14, 8, '#4a4f57');
      rect(ctx, mx + 6, 59, 8, 7, C.metal);
      const lamp = working && Math.floor(this.time * 2 + i) % 2 ? '#7be37b' : '#2f6b2f';
      rect(ctx, mx + 15, 44, 3, 3, lamp);
      if (equip >= 3) rect(ctx, mx + 2, 44, 3, 3, C.gold);
    }
  }

  drawWorkers(ctx) {
    const b = this.b;
    const training = tierOf(b, 'training');
    const look = {
      skin: C.skin[1], hair: C.hair[0], shirt: C.uniform[training], pants: C.pants[0],
      apron: training >= 2 ? C.white : null, badge: training >= 1 ? C.yellow : null,
      cap: training >= 3 ? C.gold : null, capBrim: C.goldDark,
    };
    let spots;
    if (this.kind === 'carwash') spots = this.places().map((p) => ({ x: p.x - 10, y: p.y + 4 }));
    else if (this.kind === 'factory') spots = Array.from({ length: tierOf(b, 'equipment') + 1 }, (_, i) => ({ x: 50 + i * 28, y: 82 }));
    else spots = this.places().map((p) => ({ x: p.x, y: 38 }));

    const shown = Math.min(b.staff, spots.length);
    for (let i = 0; i < shown; i++) {
      const s = spots[i];
      const bob = !b.bankrupt && Math.floor(this.time * 2 + i) % 2 ? 1 : 0;
      drawPerson(ctx, { ...look, skin: C.skin[i % C.skin.length], hair: C.hair[(i + 1) % C.hair.length] }, s.x, s.y - bob, 0);
    }
    // The rest carry boxes about the storeroom.
    const extra = Math.min(b.staff - shown, 3);
    for (let i = 0; i < extra; i++) {
      const span = 40;
      const t = (this.time * 10 + i * 17) % (span * 2);
      const y = FLOOR + 22 + (t < span ? t : span * 2 - t) + i * 6;
      drawPerson(ctx, { ...look, box: true }, STORE[0] + 10 + i * 16, Math.min(y, BOTTOM - 2), Math.floor(this.time * 6) % 2 + 1);
    }
  }

  drawStore(ctx, shelves) {
    const racks = 1 + shelves;
    const storage = Math.max(this.b.storage ?? this.plan.storage * this.b.level, 1);
    const fill = Math.max(0, Math.min(1, this.b.stock / storage));
    const slots = racks * 7;
    let boxes = Math.round(fill * slots);
    if (this.b.stock > 0 && boxes === 0) boxes = 1;
    for (let r = 0; r < racks; r++) {
      const x = STORE[0] + 50 - r * 14;
      rect(ctx, x, FLOOR + 2, 10, 70, C.metalDark);
      for (let s = 0; s < 7; s++) {
        const y = FLOOR + 4 + s * 10;
        rect(ctx, x, y + 8, 10, 1, C.metal);
        if (boxes > 0) {
          rect(ctx, x + 1, y + 1, 8, 7, C.box);
          rect(ctx, x + 1, y + 4, 8, 1, C.boxDark);
          boxes--;
        }
      }
    }
    // The pallet by the passage.
    rect(ctx, STORE[0] + 4, BOTTOM - 14, 14, 12, C.woodLight);
    for (let k = 0; k < 3; k++) rect(ctx, STORE[0] + 4, BOTTOM - 13 + k * 4, 14, 1, C.woodDark);
  }

  drawDecor(ctx, decor) {
    if (decor >= 1) {
      const [x, y, w, h] = this.kind === 'carwash' ? [92, 96, 44, 13] : this.kind === 'factory' ? [8, 84, 40, 22] : [44, 84, 60, 22];
      rect(ctx, x, y, w, h, C.rugDark);
      rect(ctx, x + 1, y + 1, w - 2, h - 2, C.rug);
      frame(ctx, x + 3, y + 3, w - 6, h - 6, C.rugLight);
      for (let k = x + 6; k < x + w - 6; k += 6) rect(ctx, k, y + h / 2, 2, 1, C.rugLight);
      if (this.kind === 'kiosk') {
        // The awning over the door outside.
        const [d0, d1] = doors.kiosk;
        for (let k = d0 - 8; k < d1 + 8; k += 4) rect(ctx, k, WALK, 4, 3, (k / 4) % 2 ? C.red : C.white);
      }
    }
    if (decor >= 2) {
      const pots = {
        kiosk: [[8, FLOOR + 30], [8, BOTTOM - 10], [118, BOTTOM - 10]],
        cafe: [[8, FLOOR + 30], [HALL[1] - 10, FLOOR + 30], [HALL[1] - 10, BOTTOM - 10]],
        carwash: [[HALL[1] - 10, FLOOR + 30], [HALL[1] - 10, FLOOR + 44]],
        factory: [[8, FLOOR + 30], [HALL[1] - 10, FLOOR + 30]],
      }[this.kind];
      for (const [x, y] of pots) {
        rect(ctx, x, y + 4, 6, 5, C.pot);
        rect(ctx, x - 1, y - 2, 8, 7, C.leafDark);
        rect(ctx, x, y - 1, 6, 5, C.leaf);
      }
      // A picture on the wall.
      rect(ctx, 118, FACE + 3, 18, 11, C.goldDark);
      rect(ctx, 119, FACE + 4, 16, 9, '#7ab6e0');
      rect(ctx, 119, FACE + 9, 16, 4, '#5aa65a');
    }
    if (decor >= 3) {
      // A sofa, a television and a clock.
      const [x, y] = { factory: [56, 92], carwash: [97, 98] }[this.kind] ?? [108, 94];
      rect(ctx, x, y, 34, 12, C.sofaDark);
      rect(ctx, x + 2, y + 3, 30, 8, C.sofa);
      rect(ctx, x + 2, y, 30, 3, C.sofaDark);
      rect(ctx, 140, FACE + 3, 9, 9, C.white);
      frame(ctx, 140, FACE + 3, 9, 9, C.black);
      rect(ctx, 144, FACE + 5, 1, 3, C.black);
      rect(ctx, 144, FACE + 7, 3, 1, C.black);
      rect(ctx, 98, FACE + 3, 16, 11, C.black);
      const glow = Math.floor(this.time * 3) % 3;
      rect(ctx, 99, FACE + 4, 14, 9, ['#4a8fd8', '#58a0e0', '#3f80c8'][glow]);
    }
  }

  drawSign(ctx, brand) {
    const [, d1] = doors[this.kind];
    const x = d1 + 2;
    const y = BOTTOM - 1;
    const w = 30;
    const colors = [
      ['#8b6b3e', '#d9c39a'],
      ['#2f5ea8', '#ffffff'],
      ['#2b1b3d', Math.floor(this.time * 2) % 2 ? '#ff4fd8' : '#ff9cea'],
      [C.goldDark, C.gold],
    ][brand];
    rect(ctx, x, y, w, 8, colors[0]);
    rect(ctx, x + 2, y + 2, w - 4, 4, colors[1]);
    if (brand >= 2) for (let k = x + 1; k < x + w; k += 3) rect(ctx, k, y, 1, 1, Math.floor(this.time * 4 + k) % 2 ? C.yellow : C.white);
  }

  drawBillboard(ctx) {
    const b = this.b;
    const on = b.adUntil && new Date(b.adUntil) > new Date();
    const x = 182;
    const y = WALK + 1;
    rect(ctx, x + 6, y + 9, 2, 3, C.metalDark);
    rect(ctx, x + 28, y + 9, 2, 3, C.metalDark);
    rect(ctx, x, y, 36, 10, C.metalDark);
    if (on) {
      rect(ctx, x + 1, y + 1, 34, 8, C.yellow);
      rect(ctx, x + 3, y + 3, 12, 4, C.red);
      rect(ctx, x + 17, y + 3, 16, 1, C.black);
      rect(ctx, x + 17, y + 5, 12, 1, C.black);
      if (Math.floor(this.time * 3) % 2) for (let k = x + 1; k < x + 36; k += 4) rect(ctx, k, y, 1, 1, C.white);
    } else {
      rect(ctx, x + 1, y + 1, 34, 8, '#e9e9e9');
      rect(ctx, x + 4, y + 4, 28, 1, '#c4c4c4');
    }
  }

  drawWear(ctx, rnd) {
    const worn = businessCondition(this.b.condition);
    if (!worn) return;
    const cracks = worn === 2 ? 10 : 4;
    for (let i = 0; i < cracks; i++) {
      let x = HALL[0] + 4 + rnd() * (HALL[1] - HALL[0] - 8);
      let y = i % 2 ? FACE + 2 + rnd() * 10 : FLOOR + 4 + rnd() * (BOTTOM - FLOOR - 8);
      for (let k = 0; k < 6; k++) {
        rect(ctx, x, y, 1, 1, C.crack);
        x += rnd() < 0.5 ? 1 : -1;
        y += 1;
      }
    }
    if (worn === 2) {
      for (let i = 0; i < 5; i++) rect(ctx, HALL[0] + 8 + rnd() * 130, FLOOR + 8 + rnd() * 70, 8, 8, '#00000030');
    }
  }

  drawAgent(ctx, a) {
    const moving = !a.doing;
    if (a.vehicle === 'car') {
      drawCar(ctx, a, a.x, a.y, a.turned);
      if (a.doing?.wash && a.wait > 0) {
        for (let i = 0; i < 6; i++) {
          const t = (this.time * 20 + i * 7) % 14;
          rect(ctx, a.x - 7 + ((i * 5) % 14), a.y - 8 + t, 1, 2, C.water);
          rect(ctx, a.x - 5 + ((i * 3) % 10), a.y - 4 + ((i * 4) % 9), 2, 2, C.foam);
        }
      }
      return;
    }
    if (a.vehicle === 'truck') {
      drawTruck(ctx, a, a.x, a.y, a.flip);
      if (a.doing?.load && a.wait > 0) {
        const t = (this.time * 8) % 10;
        rect(ctx, a.x - 2, BOTTOM + 6 + t, 5, 4, C.box);
      }
      return;
    }
    const step = moving ? 1 + (Math.floor(a.leg / 3) % 2) : 0;
    drawPerson(ctx, { ...a, box: a.doing?.buy && a.wait < 1 }, a.x, a.y, step);
  }

  drawAlerts(ctx) {
    const b = this.b;
    if (Math.floor(this.time * 2) % 2) return;
    const alerts = [];
    if (b.stock <= 0) alerts.push('storage');
    if (b.inDebt) alerts.push('till');
    if (b.staff === 0) alerts.push('staff');
    if (businessCondition(b.condition) === 2) alerts.push('building');
    for (const part of alerts) {
      const p = this.parts.find((q) => q.part === part);
      if (!p) continue;
      const x = part === 'building' ? p.x + 4 : p.x + p.w - 8;
      const y = part === 'building' ? p.y + 20 : p.y + 2;
      rect(ctx, x, y, 7, 9, C.red);
      frame(ctx, x, y, 7, 9, '#7a1010');
      rect(ctx, x + 3, y + 2, 1, 4, C.white);
      rect(ctx, x + 3, y + 7, 1, 1, C.white);
    }
  }

  drawRuin(ctx) {
    rect(ctx, 0, 0, W, WALK, 'rgba(40,40,40,0.45)');
    const [d0, d1] = doors[this.kind];
    for (let k = 0; k < 3; k++) {
      rect(ctx, d0 - 2, BOTTOM - 1 + k * 2, d1 - d0 + 4, 2, k % 2 ? C.woodDark : C.woodLight);
    }
    ctx.save();
    ctx.translate(W / 2 - 20, 60);
    ctx.rotate(-0.15);
    rect(ctx, -30, -9, 100, 18, C.red);
    rect(ctx, -28, -7, 96, 14, '#f6e9e8');
    ctx.fillStyle = C.red;
    ctx.font = 'bold 10px Tahoma, Verdana, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(tr('Разорён').toUpperCase(), 20, 1);
    ctx.restore();
  }
}
