// Charts of the stock exchange on canvas: a line of the index with its
// area, a sparkline of a company, candles of a share with its volume and a
// cross that follows the pointer. The colors are the variables of the
// palette, so the themes color them too; a chart is drawn again when the
// window changes its width.

import { el } from './layout.js';

const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

function palette() {
  return {
    up: css('--text-43a047'),
    down: css('--text-d94c4c'),
    line: css('--text-45688e'),
    area: css('--back-d9e1ea'),
    grid: css('--line-e8e8e8'),
    text: css('--text-626262'),
    back: css('--back-fff'),
    cross: css('--line-8b8b8b'),
    label: css('--back-595959'),
    labelText: css('--text-fff'),
  };
}

// fit sizes the canvas to its box for the pixels of the screen and gives
// its context and size in CSS pixels.
function fit(canvas, height) {
  const ratio = window.devicePixelRatio || 1;
  const width = canvas.clientWidth || canvas.parentElement?.clientWidth || 300;
  canvas.width = Math.round(width * ratio);
  canvas.height = Math.round(height * ratio);
  canvas.style.height = `${height}px`;
  const ctx = canvas.getContext('2d');
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  return { ctx, width, height };
}

const charts = new Set();
let resizing = 0;
window.addEventListener('resize', () => {
  clearTimeout(resizing);
  resizing = setTimeout(() => {
    for (const draw of charts) draw();
  }, 100);
});

// keep draws the chart now and whenever the window changes, while the
// canvas is on the page.
function keep(canvas, draw) {
  const redraw = () => {
    if (!canvas.isConnected) {
      charts.delete(redraw);
      return;
    }
    draw();
  };
  charts.add(redraw);
  requestAnimationFrame(redraw);
}

// scale maps values from lo to hi onto top to bottom pixels.
function scale(lo, hi, top, bottom) {
  const span = hi - lo || Math.abs(hi) || 1;
  return (v) => bottom - ((v - lo) / span) * (bottom - top);
}

// ticks are a few round values from lo to hi.
function ticks(lo, hi, count) {
  const raw = (hi - lo) / count || 1;
  const pow = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * pow).find((s) => s >= raw) ?? raw;
  const out = [];
  for (let v = Math.ceil(lo / step) * step; v <= hi; v += step) out.push(v);
  return out;
}

// sparkline is a small line of values, green going up and red going down.
export function sparkline(values, { width = 90, height = 26 } = {}) {
  const canvas = el('canvas', { className: 'spark' });
  canvas.style.width = `${width}px`;
  keep(canvas, () => {
    const { ctx } = fit(canvas, height);
    const c = palette();
    ctx.clearRect(0, 0, width, height);
    if (values.length < 2) return;
    const lo = Math.min(...values);
    const hi = Math.max(...values);
    const y = scale(lo, hi, 3, height - 3);
    const x = (i) => 1 + (i / (values.length - 1)) * (width - 2);
    ctx.strokeStyle = values.at(-1) >= values[0] ? c.up : c.down;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    values.forEach((v, i) => (i ? ctx.lineTo(x(i), y(v)) : ctx.moveTo(x(i), y(v))));
    ctx.stroke();
  });
  return canvas;
}

// lineChart is a line of points {time, value} with its area under it and
// the values on the right; format writes a value, when the hour of a point
// and tick the hour on the axis.
export function lineChart(points, { height = 170, format = String, when, tick }) {
  const canvas = el('canvas', { className: 'chart' });
  const tip = el('div', { className: 'chart-tip', hidden: true });
  const box = el('div', { className: 'chart-box' }, canvas, tip);
  let pointer = null;

  const draw = () => {
    const { ctx, width } = fit(canvas, height);
    const c = palette();
    const right = 56;
    const bottom = height - 18;
    ctx.clearRect(0, 0, width, height);
    if (points.length < 2) return;

    const values = points.map((p) => p.value);
    const lo = Math.min(...values);
    const hi = Math.max(...values);
    const pad = (hi - lo) * 0.1 || hi * 0.01 || 1;
    const y = scale(lo - pad, hi + pad, 6, bottom);
    const x = (i) => (i / (points.length - 1)) * (width - right);

    grid(ctx, c, ticks(lo - pad, hi + pad, 4), y, width, right, format);
    timeAxis(ctx, c, points.map((p) => p.time), x, bottom, width - right, tick);

    const up = values.at(-1) >= values[0];
    const gradient = ctx.createLinearGradient(0, 6, 0, bottom);
    gradient.addColorStop(0, c.area);
    gradient.addColorStop(1, c.back);
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.value)) : ctx.moveTo(x(i), y(p.value))));
    ctx.lineTo(x(points.length - 1), bottom);
    ctx.lineTo(0, bottom);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();

    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.value)) : ctx.moveTo(x(i), y(p.value))));
    ctx.strokeStyle = up ? c.up : c.down;
    ctx.lineWidth = 2;
    ctx.stroke();

    lastLabel(ctx, c, format(values.at(-1)), y(values.at(-1)), width, right, up);

    if (pointer !== null) {
      const i = Math.round((pointer / (width - right)) * (points.length - 1));
      const p = points[Math.min(Math.max(i, 0), points.length - 1)];
      crossAt(ctx, c, x(points.indexOf(p)), y(p.value), width - right, bottom);
      showTip(tip, x(points.indexOf(p)), width, [when(p.time), format(p.value)]);
    } else {
      tip.hidden = true;
    }
  };

  follow(canvas, (px) => {
    pointer = px;
    draw();
  });
  keep(canvas, draw);
  return box;
}

// candleChart is the candles {hour, open, high, low, close, volume} of a
// share with the volume under them; format writes a price, when and tick
// the hours, texts are the names of the prices.
export function candleChart(candles, { height = 260, format = String, when, tick, texts }) {
  const canvas = el('canvas', { className: 'chart' });
  const tip = el('div', { className: 'chart-tip', hidden: true });
  const box = el('div', { className: 'chart-box' }, canvas, tip);
  let pointer = null;

  const draw = () => {
    const { ctx, width } = fit(canvas, height);
    const c = palette();
    const right = 56;
    const volumeHeight = 40;
    const bottom = height - 18 - volumeHeight - 6;
    ctx.clearRect(0, 0, width, height);
    if (!candles.length) return;

    const lo = Math.min(...candles.map((k) => k.low));
    const hi = Math.max(...candles.map((k) => k.high));
    const pad = (hi - lo) * 0.08 || hi * 0.02 || 1;
    const y = scale(lo - pad, hi + pad, 6, bottom);
    const slot = (width - right) / candles.length;
    const x = (i) => slot * i + slot / 2;
    const body = Math.max(1, Math.min(12, slot * 0.65));

    grid(ctx, c, ticks(lo - pad, hi + pad, 5), y, width, right, format);
    timeAxis(ctx, c, candles.map((k) => k.hour), x, height - 18, width - right, tick);

    const most = Math.max(1, ...candles.map((k) => k.volume));
    const volumeTop = bottom + 6;
    candles.forEach((k, i) => {
      const color = k.close >= k.open ? c.up : c.down;
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(Math.round(x(i)) + 0.5, y(k.high));
      ctx.lineTo(Math.round(x(i)) + 0.5, y(k.low));
      ctx.stroke();
      const top = y(Math.max(k.open, k.close));
      ctx.fillRect(Math.round(x(i) - body / 2), top, Math.round(body), Math.max(1, y(Math.min(k.open, k.close)) - top));

      const h = (k.volume / most) * volumeHeight;
      ctx.globalAlpha = 0.45;
      ctx.fillRect(Math.round(x(i) - body / 2), volumeTop + volumeHeight - h, Math.round(body), h);
      ctx.globalAlpha = 1;
    });

    const last = candles.at(-1);
    lastLabel(ctx, c, format(last.close), y(last.close), width, right, last.close >= candles[0].open);

    if (pointer !== null) {
      const i = Math.min(Math.max(Math.floor(pointer / slot), 0), candles.length - 1);
      const k = candles[i];
      crossAt(ctx, c, x(i), y(k.close), width - right, bottom);
      showTip(tip, x(i), width, [
        when(k.hour),
        `${texts.open} ${format(k.open)}  ${texts.high} ${format(k.high)}`,
        `${texts.low} ${format(k.low)}  ${texts.close} ${format(k.close)}`,
        `${texts.volume} ${k.volume}`,
      ]);
    } else {
      tip.hidden = true;
    }
  };

  follow(canvas, (px) => {
    pointer = px;
    draw();
  });
  keep(canvas, draw);
  return box;
}

function grid(ctx, c, values, y, width, right, format) {
  ctx.font = '10px Tahoma, Verdana, sans-serif';
  ctx.textBaseline = 'middle';
  ctx.textAlign = 'left';
  for (const v of values) {
    ctx.strokeStyle = c.grid;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, Math.round(y(v)) + 0.5);
    ctx.lineTo(width - right, Math.round(y(v)) + 0.5);
    ctx.stroke();
    ctx.fillStyle = c.text;
    ctx.fillText(format(v), width - right + 4, y(v));
  }
}

function timeAxis(ctx, c, times, x, base, width, tick) {
  ctx.fillStyle = c.text;
  ctx.textBaseline = 'top';
  ctx.textAlign = 'center';
  const every = Math.max(1, Math.ceil(times.length / Math.max(1, Math.floor(width / 70))));
  times.forEach((t, i) => {
    if ((times.length - 1 - i) % every) return;
    ctx.fillText(tick(t), Math.min(Math.max(x(i), 16), width - 16), base + 4);
  });
}

function lastLabel(ctx, c, text, at, width, right, up) {
  ctx.fillStyle = up ? c.up : c.down;
  ctx.fillRect(width - right + 1, at - 8, right - 1, 16);
  ctx.fillStyle = c.labelText;
  ctx.textAlign = 'left';
  ctx.textBaseline = 'middle';
  ctx.font = 'bold 10px Tahoma, Verdana, sans-serif';
  ctx.fillText(text, width - right + 4, at);
}

function crossAt(ctx, c, x, y, width, bottom) {
  ctx.strokeStyle = c.cross;
  ctx.setLineDash([3, 3]);
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(Math.round(x) + 0.5, 0);
  ctx.lineTo(Math.round(x) + 0.5, bottom);
  ctx.moveTo(0, Math.round(y) + 0.5);
  ctx.lineTo(width, Math.round(y) + 0.5);
  ctx.stroke();
  ctx.setLineDash([]);
}

function showTip(tip, x, width, lines) {
  tip.replaceChildren(...lines.map((text, i) => el(i ? 'div' : 'b', { textContent: text })));
  tip.hidden = false;
  const left = x > width / 2 ? x - tip.offsetWidth - 10 : x + 10;
  tip.style.left = `${Math.max(0, left)}px`;
}

// follow tells where the pointer is over the canvas, null when it left.
function follow(canvas, at) {
  const move = (event) => {
    const r = canvas.getBoundingClientRect();
    at(event.clientX - r.left);
  };
  canvas.addEventListener('pointermove', move);
  canvas.addEventListener('pointerdown', move);
  canvas.addEventListener('pointerleave', () => at(null));
}
