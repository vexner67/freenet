// The flags of the states of "Мир": a pattern of up to three colors of
// the palette of flags and an emblem, drawn as SVG from what the world
// service keeps — no pictures to upload. flagShapes is the drawing as
// data (tested without a browser), flagNode puts it on the page; the
// colors are classes of the stylesheet (.flag-cN), so the palette stays
// in palette.css.

const SVG = 'http://www.w3.org/2000/svg';

// A flag is 30 by 20: three to two, as most flags are.
export const FLAG_WIDTH = 30;
export const FLAG_HEIGHT = 20;

// The palettes of flags and of states have 24 colors each, as
// world.Colors.
export const COLORS = 24;

// flagPatterns are the patterns with how many colors each has, in the
// order the site offers them (world.patternColors).
export const flagPatterns = [
  ['plain', 1], ['bicolor_h', 2], ['bicolor_v', 2], ['tricolor_h', 3], ['tricolor_v', 3], ['cross', 2],
  ['nordic', 2], ['diagonal', 2], ['border', 2], ['triangle', 2], ['circle', 2],
];

export const patternColors = (pattern) => flagPatterns.find(([p]) => p === pattern)?.[1] ?? 1;

// The emblems drawn in a box of 24.
const emblemPaths = {
  star: 'M12 2l2.9 6.6 7.1.6-5.4 4.7 1.6 7L12 17.2 5.8 20.9l1.6-7L2 9.2l7.1-.6z',
  sun: 'M12 7a5 5 0 1 1 0 10a5 5 0 1 1 0-10zM11 1h2v4h-2zM11 19h2v4h-2zM1 11h4v2H1zM19 11h4v2h-4zM4.2 5.6l1.4-1.4 2.8 2.8-1.4 1.4zM15.6 17l1.4-1.4 2.8 2.8-1.4 1.4zM4.2 18.4l2.8-2.8 1.4 1.4-2.8 2.8zM15.6 7l2.8-2.8 1.4 1.4-2.8 2.8z',
  moon: 'M15 2a10 10 0 1 0 0 20a8 8 0 1 1 0-20z',
  crown: 'M3 19h18v2H3zM3 17l1.5-10 4.5 5 3-8 3 8 4.5-5L21 17z',
  anchor: 'M12 2a2.5 2.5 0 1 1 0 5a2.5 2.5 0 1 1 0-5zM11 7h2v3h4v2h-4v8a6 6 0 0 0 5-5h2a8 8 0 0 1-16 0h2a6 6 0 0 0 5 5v-8H7v-2h4z',
  tower: 'M5 22V10h1V5h3v3h2V5h2v3h2V5h3v5h1v12h-5v-5a2 2 0 0 0-4 0v5z',
  sword: 'M11 1h2l1 3v11h3v2h-4v3a1.5 1.5 0 1 1-2 0v-3H7v-2h3V4z',
  tree: 'M12 1l6 8h-3l5 7h-7v6h-2v-6H4l5-7H6z',
  wave: 'M1 9c2.75-3 5.5-3 8.25 0s5.5 3 8.25 0 3.75-1.5 5.5 0v3c-1.75-1.5-2.75-3-5.5 0s-5.5 3-8.25 0S3.75 9 1 12zM1 15c2.75-3 5.5-3 8.25 0s5.5 3 8.25 0 3.75-1.5 5.5 0v3c-1.75-1.5-2.75-3-5.5 0s-5.5 3-8.25 0S3.75 15 1 18z',
  wheat: 'M11 9h2v14h-2zM12 1c2 2 2 4.5 0 6.5c-2-2-2-4.5 0-6.5zM6 7c3.5 0 5.5 2 6 5c-3.5 0-5.5-2-6-5zM18 7c-3.5 0-5.5 2-6 5c.5-3 2.5-5 6-5zM6 12c3.5 0 5.5 2 6 5c-3.5 0-5.5-2-6-5zM18 12c-3.5 0-5.5 2-6 5c.5-3 2.5-5 6-5z',
  shield: 'M12 1l9 3.5V11c0 6.5-4 10-9 12c-5-2-9-5.5-9-12V4.5z',
  hammer: 'M3 3h12l3 3v3h-4V7h-2v15h-3V7H3z',
};

export const flagEmblems = ['', ...Object.keys(emblemPaths)];

// Where an emblem stands: a box of side size with its corner at x, y.
const emblemBoxes = {
  center: { x: 8, y: 3, size: 14 },
  canton: { x: 1.5, y: 1.5, size: 8 },
};

// flagShapes is a flag as what to draw: { tag, attrs, color }, from the
// back to the front. A flag the server could not give is a plain one of
// the first color.
export function flagShapes(flag) {
  const pattern = flagPatterns.some(([p]) => p === flag?.pattern) ? flag.pattern : 'plain';
  const colors = flag?.colors?.length ? flag.colors : [0];
  const c = (i) => colors[Math.min(i, colors.length - 1)];
  const rect = (x, y, width, height, color) => ({ tag: 'rect', attrs: { x, y, width, height }, color });
  const polygon = (points, color) => ({ tag: 'polygon', attrs: { points }, color });
  const W = FLAG_WIDTH;
  const H = FLAG_HEIGHT;

  const shapes = {
    plain: () => [rect(0, 0, W, H, c(0))],
    bicolor_h: () => [rect(0, 0, W, H / 2, c(0)), rect(0, H / 2, W, H / 2, c(1))],
    bicolor_v: () => [rect(0, 0, W / 2, H, c(0)), rect(W / 2, 0, W / 2, H, c(1))],
    tricolor_h: () => [0, 1, 2].map((i) => rect(0, (H / 3) * i, W, H / 3, c(i))),
    tricolor_v: () => [0, 1, 2].map((i) => rect((W / 3) * i, 0, W / 3, H, c(i))),
    cross: () => [rect(0, 0, W, H, c(0)), rect(13, 0, 4, H, c(1)), rect(0, 8, W, 4, c(1))],
    nordic: () => [rect(0, 0, W, H, c(0)), rect(8, 0, 4, H, c(1)), rect(0, 8, W, 4, c(1))],
    diagonal: () => [rect(0, 0, W, H, c(0)), polygon(`0,${H} ${W},0 ${W},${H}`, c(1))],
    border: () => [rect(0, 0, W, H, c(1)), rect(2, 2, W - 4, H - 4, c(0))],
    triangle: () => [rect(0, 0, W, H, c(0)), polygon(`0,0 12,${H / 2} 0,${H}`, c(1))],
    circle: () => [rect(0, 0, W, H, c(0)), { tag: 'circle', attrs: { cx: W / 2, cy: H / 2, r: 6 }, color: c(1) }],
  }[pattern]();

  const emblem = emblemPaths[flag?.emblem];
  if (emblem) {
    const box = emblemBoxes[flag.emblemAt] ?? emblemBoxes.center;
    shapes.push({
      tag: 'path',
      attrs: { d: emblem, transform: `translate(${box.x} ${box.y}) scale(${box.size / 24})` },
      color: flag.emblemColor ?? 0,
    });
  }

  return shapes;
}

// flagNode is a flag as an SVG width pixels wide, with a thin frame as
// flags on the maps of atlases have.
export function flagNode(flag, width = 30, title = '') {
  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('class', 'flag');
  svg.setAttribute('viewBox', `0 0 ${FLAG_WIDTH} ${FLAG_HEIGHT}`);
  svg.setAttribute('width', String(width));
  svg.setAttribute('height', String(Math.round((width * FLAG_HEIGHT) / FLAG_WIDTH)));
  svg.setAttribute('role', 'img');
  if (title) {
    const t = document.createElementNS(SVG, 'title');
    t.textContent = title;
    svg.append(t);
  }

  for (const { tag, attrs, color } of flagShapes(flag)) {
    const node = document.createElementNS(SVG, tag);
    for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, String(value));
    node.setAttribute('class', `flag-c${color}`);
    svg.append(node);
  }

  return svg;
}

// randomFlag is a flag to start from: a pattern and colors that differ.
export function randomFlag(random = Math.random) {
  const pick = (n) => Math.floor(random() * n);
  const [pattern, count] = flagPatterns[pick(flagPatterns.length)];
  const colors = [];
  while (colors.length < count) {
    const c = pick(COLORS);
    if (!colors.includes(c)) colors.push(c);
  }

  return { pattern, colors, emblem: '', emblemColor: 0, emblemAt: 'center' };
}

// fitFlag gives a flag as many colors as its pattern has: those it had,
// then new ones.
export function fitFlag(flag) {
  const count = patternColors(flag.pattern);
  const colors = flag.colors.slice(0, count);
  for (let c = 0; colors.length < count; c++) {
    if (!colors.includes(c)) colors.push(c);
  }

  return { ...flag, colors };
}
