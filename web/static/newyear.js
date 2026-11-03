// The New Year decoration of the site, as the sites of the 2000s wore it:
// snow falling over the page, a garland of running lights under the
// header, a hat on the "f" of the logo, a fir tree under the menu that
// lights up at a press, a countdown to midnight and Santa's sleigh flying
// over the page now and then with a gift to catch. The
// administrators turn it on, off or leave it to the calendar (/api/v1/decor);
// the browser remembers the answer for a while, and theme.js puts the
// garland and the hat on before the page is painted. Whoever finds the
// snow too much turns it off under the tree, on this device.

import { api } from './api.js';
import { newYearGreeting, untilNewYear } from './labels.js';
import { fanfare, jingle } from './sounds.js';
import { tr } from './i18n.js';

const KEY = 'freenet.decor';
const SNOW_KEY = 'freenet.snow';
const LIGHTS_KEY = 'freenet.lights';
const GIFTS_KEY = 'freenet.gifts';
// How long the answer of the server lasts in the browser.
const FRESH = 10 * 60 * 1000;

const root = globalThis.document?.documentElement;
const still = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');
// The sleigh and the sparks follow a mouse, not a finger, on a big screen.
const mouse = globalThis.matchMedia?.('(pointer: fine)');

function read(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key, value) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // The choice lasts until the page is left.
  }
}

function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
}

// remember keeps what the server said, for theme.js and the next pages;
// the admin panel calls it when it changes the decoration.
export function remember(festive) {
  write(KEY, JSON.stringify({ festive, at: Date.now() }));
}

const snowOff = () => read(SNOW_KEY) === 'off';
const counted = (key) => Number.parseInt(read(key), 10) || 0;

let tree = null;

// decorate puts the decoration on or takes it off.
function decorate(festive) {
  root.classList.toggle('festive', festive);

  if (!festive) {
    tree?.remove();
    tree = null;
    stopSnow();
    stopSleighs();
    return;
  }

  if (!tree) {
    tree = treeNode();
    if (tree) document.querySelector('.sidebar')?.append(tree);
  }
  if (!snowOff()) startSnow();
  startSleighs();
}

// treeNode is the fir tree under the menu, with the greeting of the day,
// the countdown to midnight, what was lit and caught, and the switch of
// the snow. A press on the tree lights it: it jumps, throws sparks and
// rings, and every tenth press rings louder.
function treeNode() {
  if (!document.querySelector('.sidebar')) return null;

  const lights = el('b', { textContent: counted(LIGHTS_KEY) });
  const gifts = el('b', { textContent: counted(GIFTS_KEY) });
  const fir = el('button', { type: 'button', className: 'newyear-tree-button', title: tr('Зажечь ёлку') },
    el('img', { className: 'newyear-tree', src: '/static/newyear/tree.svg', alt: tr('Зажечь ёлку') }));
  fir.addEventListener('click', () => light(fir, lights));

  const node = el('div', { className: 'newyear' },
    fir,
    el('div', { className: 'newyear-greeting', textContent: newYearGreeting(new Date()) }),
    countdown(),
    el('div', { className: 'newyear-score' },
      el('span', { className: 'newyear-lights', title: tr('Огоньков зажжено') }, lights),
      el('span', { className: 'newyear-gifts', title: tr('Подарков поймано') }, gifts)));
  giftsShown = gifts;

  if (!still.matches) {
    const toggle = el('a', { className: 'newyear-snow' });
    toggle.setAttribute('role', 'button');
    const label = () => {
      toggle.textContent = snowOff() ? tr('Включить снег') : tr('Выключить снег');
    };
    toggle.addEventListener('click', () => {
      const off = !snowOff();
      write(SNOW_KEY, off ? 'off' : null);
      if (off) stopSnow();
      else startSnow();
      label();
    });
    label();
    node.append(toggle);
  }

  return node;
}

// countdown is the plate of the days, hours, minutes and seconds left to
// midnight, ticking each second; in January it wishes a happy New Year.
function countdown() {
  if (untilNewYear(new Date()) === 0) {
    return el('div', { className: 'newyear-count newyear-came', textContent: tr('С Новым годом!') });
  }

  const digits = [tr('дн'), tr('ч'), tr('мин'), tr('с')].map((name) => {
    const digit = el('b', { className: 'newyear-digit' });
    return { digit, cell: el('span', { className: 'newyear-cell' }, digit, el('span', { textContent: name })) };
  });
  const plate = el('div', { className: 'newyear-count' },
    el('div', { className: 'newyear-count-title', textContent: tr('До Нового года:') }),
    el('div', { className: 'newyear-cells' }, ...digits.map((d) => d.cell)));

  const tick = () => {
    const left = untilNewYear(new Date());
    if (left === 0 || !root.classList.contains('festive')) {
      clearInterval(timer);
      if (left === 0) plate.replaceWith(el('div', { className: 'newyear-count newyear-came', textContent: tr('С Новым годом!') }));
      return;
    }
    const s = Math.floor(left / 1000);
    const parts = [Math.floor(s / 86400), Math.floor(s / 3600) % 24, Math.floor(s / 60) % 60, s % 60];
    parts.forEach((v, i) => {
      const text = i === 0 ? String(v) : String(v).padStart(2, '0');
      if (digits[i].digit.textContent !== text) digits[i].digit.textContent = text;
    });
  };
  const timer = setInterval(tick, 1000);
  tick();
  return plate;
}

// light is a press on the tree: one more light, a jump, sparks and a bell.
function light(fir, shown) {
  const n = counted(LIGHTS_KEY) + 1;
  write(LIGHTS_KEY, String(n));
  shown.textContent = n;
  bump(shown);
  bump(fir);
  sparks(fir, n % 10 === 0 ? 14 : 6);
  if (n % 10 === 0) {
    fanfare();
    pop(fir, tr('Ёлка сияет! Огоньков: {0}', n), 'gold');
  } else {
    jingle(n % 5);
    pop(fir, '+1', 'good');
  }
}

// bump makes node jump once; a new press starts the jump over.
function bump(node) {
  if (still.matches) return;
  node.classList.remove('newyear-bump');
  void node.offsetWidth;
  node.classList.add('newyear-bump');
}

// pop is a word flying up from node, as the rewards of the games.
function pop(node, text, tone) {
  if (still.matches || !node.isConnected) return;
  const r = node.getBoundingClientRect();
  const fly = el('div', { className: `game-float ${tone}`, textContent: text });
  fly.style.left = `${r.left + scrollX + r.width / 2}px`;
  fly.style.top = `${r.top + scrollY + r.height / 4}px`;
  document.body.append(fly);
  fly.addEventListener('animationend', () => fly.remove());
}

// sparks throws count golden stars out of node in all directions.
function sparks(node, count) {
  if (still.matches || !node.isConnected) return;
  const r = node.getBoundingClientRect();
  for (let i = 0; i < count; i++) {
    const angle = (i / count) * Math.PI * 2 + Math.random() * 0.5;
    const reach = 30 + Math.random() * 40;
    spark(r.left + r.width / 2, r.top + r.height / 2, Math.cos(angle) * reach, Math.sin(angle) * reach);
  }
}

// spark is one star at x, y of the window, flying dx, dy and fading.
function spark(x, y, dx, dy) {
  const star = el('img', { className: 'newyear-spark', src: '/static/newyear/spark.svg', alt: '' });
  star.style.left = `${x}px`;
  star.style.top = `${y}px`;
  star.style.setProperty('--dx', `${dx}px`);
  star.style.setProperty('--dy', `${dy}px`);
  document.body.append(star);
  star.addEventListener('animationend', () => star.remove());
}

let giftsShown = null;
let sleighTimer = 0;
let trail = null;

// startSleighs sends the sleigh over the page every minute or so, and
// lets the mouse leave a trail of sparks: on a big screen, with a mouse,
// when the person does not ask for less motion.
function startSleighs() {
  if (sleighTimer || still.matches || !mouse?.matches || root.classList.contains('m')) return;

  const next = () => {
    sleighTimer = setTimeout(() => {
      if (!document.hidden) flySleigh();
      next();
    }, 40_000 + Math.random() * 50_000);
  };
  sleighTimer = setTimeout(() => {
    flySleigh();
    next();
  }, 8_000);

  let last = 0;
  let alive = 0;
  trail = (e) => {
    if (e.timeStamp - last < 45 || alive >= 10) return;
    last = e.timeStamp;
    alive++;
    const star = el('img', { className: 'newyear-trail', src: '/static/newyear/spark.svg', alt: '' });
    star.style.left = `${e.clientX}px`;
    star.style.top = `${e.clientY}px`;
    document.body.append(star);
    star.addEventListener('animationend', () => {
      alive--;
      star.remove();
    });
  };
  addEventListener('pointermove', trail, { passive: true });
}

function stopSleighs() {
  clearTimeout(sleighTimer);
  sleighTimer = 0;
  if (trail) removeEventListener('pointermove', trail);
  trail = null;
  document.querySelector('.newyear-sleigh')?.remove();
}

// flySleigh lets Santa cross the window high up; a press on the sleigh
// catches the gift it carries.
function flySleigh() {
  if (document.querySelector('.newyear-sleigh')) return;

  const sleigh = el('button', { type: 'button', className: 'newyear-sleigh', title: tr('Поймать подарок') },
    el('img', { src: '/static/newyear/sleigh.svg', alt: tr('Поймать подарок') }));
  sleigh.style.top = `${60 + Math.random() * Math.max(40, innerHeight * 0.35)}px`;
  sleigh.addEventListener('click', () => {
    const r = sleigh.getBoundingClientRect();
    const n = counted(GIFTS_KEY) + 1;
    write(GIFTS_KEY, String(n));
    if (giftsShown) {
      giftsShown.textContent = n;
      bump(giftsShown);
    }
    const gift = el('img', { className: 'newyear-gift', src: '/static/newyear/gift.svg', alt: '' });
    gift.style.left = `${r.left + r.width / 3}px`;
    gift.style.top = `${r.top}px`;
    document.body.append(gift);
    gift.addEventListener('animationend', () => gift.remove());
    for (let i = 0; i < 10; i++) {
      const angle = (i / 10) * Math.PI * 2;
      spark(r.left + r.width / 3, r.top + r.height / 2, Math.cos(angle) * 50, Math.sin(angle) * 50);
    }
    pop(sleigh, tr('Подарок пойман! Всего: {0}', n), 'gold');
    fanfare();
    jingle(7);
    sleigh.remove();
  }, { once: true });
  sleigh.addEventListener('animationend', () => sleigh.remove());
  document.body.append(sleigh);
  jingle();
}

let snow = null;

// startSnow lets a few flakes fall over the page: a canvas nothing can
// click, drawn only while the tab is seen and the person does not ask
// the system for less motion.
function startSnow() {
  if (snow || still.matches) return;

  const canvas = el('canvas', { className: 'snow' });
  canvas.setAttribute('aria-hidden', 'true');
  document.body.append(canvas);
  const ctx = canvas.getContext('2d');
  const flakes = [];
  let width = 0;
  let height = 0;
  let color = '';

  const fit = () => {
    const ratio = window.devicePixelRatio || 1;
    width = innerWidth;
    height = innerHeight;
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    color = getComputedStyle(canvas).color;

    // A real fall: one flake to 18px of width, no more than 80.
    const count = Math.min(80, Math.round(width / 18));
    while (flakes.length < count) flakes.push(flake(Math.random() * height));
    flakes.length = count;
  };

  const flake = (y) => ({
    x: Math.random() * width,
    y,
    size: 1 + Math.random() * 2.6,
    fall: 14 + Math.random() * 26,
    sway: 6 + Math.random() * 14,
    phase: Math.random() * Math.PI * 2,
    alpha: 0.55 + Math.random() * 0.45,
  });

  let last = performance.now();
  let frame = 0;

  const draw = (now) => {
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    ctx.clearRect(0, 0, width, height);
    ctx.strokeStyle = color;
    ctx.fillStyle = color;

    for (const f of flakes) {
      f.y += f.fall * dt;
      f.phase += dt;
      if (f.y > height + 6) Object.assign(f, flake(-6));
      const x = f.x + Math.sin(f.phase) * f.sway;
      ctx.globalAlpha = f.alpha;
      if (f.size < 2.2) {
        ctx.beginPath();
        ctx.arc(x, f.y, f.size, 0, Math.PI * 2);
        ctx.fill();
      } else {
        star(ctx, x, f.y, f.size * 1.6, f.phase / 3);
      }
    }
    frame = requestAnimationFrame(draw);
  };

  // A hidden tab is not drawn: requestAnimationFrame waits for it, and
  // the time it was away does not throw the flakes down at once.
  const seen = () => {
    if (document.hidden) return;
    last = performance.now();
  };

  fit();
  addEventListener('resize', fit);
  document.addEventListener('visibilitychange', seen);
  frame = requestAnimationFrame(draw);

  snow = () => {
    cancelAnimationFrame(frame);
    removeEventListener('resize', fit);
    document.removeEventListener('visibilitychange', seen);
    canvas.remove();
  };
}

// star is a flake of six spokes, as the snow of the old sites.
function star(ctx, x, y, r, turn) {
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let i = 0; i < 3; i++) {
    const a = turn + (i * Math.PI) / 3;
    ctx.moveTo(x - Math.cos(a) * r, y - Math.sin(a) * r);
    ctx.lineTo(x + Math.cos(a) * r, y + Math.sin(a) * r);
  }
  ctx.stroke();
}

function stopSnow() {
  snow?.();
  snow = null;
}

// The page wears what the browser remembers at once and asks the server
// again when that is old.
function cached() {
  try {
    const { festive, at } = JSON.parse(read(KEY)) ?? {};
    return typeof festive === 'boolean' ? { festive, fresh: Date.now() - at < FRESH } : null;
  } catch {
    return null;
  }
}

// The admin panel wears none of it: it only tells the decoration.
if (root && !root.classList.contains('aui')) {
  const known = cached();
  if (known) decorate(known.festive);
  if (!known?.fresh) {
    api.decor().then(({ festive }) => {
      remember(festive);
      decorate(festive);
    }).catch(() => {});
  }
}
