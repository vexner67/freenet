// The New Year decoration of the site, as the sites of the 2000s wore it:
// snow falling over the page, a garland under the header, a hat on the
// "f" of the logo and a fir tree with a greeting under the menu. The
// administrators turn it on, off or leave it to the calendar (/api/v1/decor);
// the browser remembers the answer for a while, and theme.js puts the
// garland and the hat on before the page is painted. Whoever finds the
// snow too much turns it off under the tree, on this device.

import { api } from './api.js';
import { newYearGreeting } from './labels.js';
import { tr } from './i18n.js';

const KEY = 'freenet.decor';
const SNOW_KEY = 'freenet.snow';
// How long the answer of the server lasts in the browser.
const FRESH = 10 * 60 * 1000;

const root = globalThis.document?.documentElement;
const still = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');

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

let tree = null;

// decorate puts the decoration on or takes it off.
function decorate(festive) {
  root.classList.toggle('festive', festive);

  if (!festive) {
    tree?.remove();
    tree = null;
    stopSnow();
    return;
  }

  if (!tree) {
    tree = treeNode();
    if (tree) document.querySelector('.sidebar')?.append(tree);
  }
  if (!snowOff()) startSnow();
}

// treeNode is the fir tree under the menu, with the greeting of the day
// and the switch of the snow.
function treeNode() {
  if (!document.querySelector('.sidebar')) return null;

  const node = el('div', { className: 'newyear' },
    el('img', { className: 'newyear-tree', src: '/static/newyear/tree.svg', alt: '' }),
    el('div', { className: 'newyear-greeting', textContent: newYearGreeting(new Date()) }));

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

    // Few: one flake to 32px of width, no more than 40.
    const count = Math.min(40, Math.round(width / 32));
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
