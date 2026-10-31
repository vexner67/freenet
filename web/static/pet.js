// The pictures of pets: round drawings, all on one ball of the same place
// and size, with the eyes in one place. Clothes are drawings of that very
// size, made to the ball, put right over the pet.

import { el } from './layout.js';

const pictures = {
  cat: '/static/pets/cat.svg',
  dog: '/static/pets/dog.svg',
  hamster: '/static/pets/hamster.svg',
  rabbit: '/static/pets/rabbit.svg',
  turtle: '/static/pets/turtle.svg',
  fox: '/static/pets/fox.svg',
  owl: '/static/pets/owl.svg',
  dragon: '/static/pets/dragon.svg',
  wolf: '/static/pets/wolf.svg',
  panda: '/static/pets/panda.svg',
  unicorn: '/static/pets/unicorn.svg',
  phoenix: '/static/pets/phoenix.svg',
  big2z: '/static/pets/big2z.svg',
};

// clothes are the drawings of clothes, each of the size of a pet and made
// to its ball: the slot they go on and the part of the drawing to show the
// thing alone, x, y, width and height.
const clothes = {
  cap: { src: '/static/pets/clothes/cap.svg', slot: 'head', view: [30, 12, 124, 62] },
  helmet: { src: '/static/pets/clothes/helmet.svg', slot: 'head', view: [18, 6, 124, 70] },
  crown: { src: '/static/pets/clothes/crown.svg', slot: 'head', view: [30, 0, 100, 60] },
  glasses: { src: '/static/pets/clothes/glasses.svg', slot: 'eyes', view: [16, 50, 128, 56] },
  shades: { src: '/static/pets/clothes/shades.svg', slot: 'eyes', view: [16, 50, 128, 56] },
  monocle: { src: '/static/pets/clothes/monocle.svg', slot: 'eyes', view: [68, 50, 68, 90] },
  bow: { src: '/static/pets/clothes/bow.svg', slot: 'neck', view: [28, 98, 104, 60] },
  scarf: { src: '/static/pets/clothes/scarf.svg', slot: 'neck', view: [18, 104, 124, 56] },
  medal: { src: '/static/pets/clothes/medal.svg', slot: 'neck', view: [46, 98, 68, 56] },
  sweater: { src: '/static/pets/clothes/sweater.svg', slot: 'body', view: [8, 84, 144, 70] },
  armor: { src: '/static/pets/clothes/armor.svg', slot: 'body', view: [6, 80, 148, 74] },
  // A cape hangs behind the pet and shows in front by its collar.
  cape: { src: '/static/pets/clothes/cape.svg', slot: 'body', view: [0, 96, 160, 64], front: '/static/pets/clothes/cape-clasp.svg' },
};

// Clothes are drawn body first, so that a scarf lies over a sweater and a
// hat over the eyes.
const layers = ['body', 'neck', 'eyes', 'head'];

// The owl has bigger eyes than the others: glasses grow to them.
const bigEyes = { owl: 1.18 };

export function petItemSlot(item) {
  return clothes[item]?.slot ?? '';
}

// petItemImage is the thing alone, as big as fits a square of size pixels.
export function petItemImage(item, size) {
  const c = clothes[item];
  if (!c) return el('span');
  const [x, y, w, h] = c.view;
  const scale = size / Math.max(w, h);

  return el('img', { className: 'pet-picture', src: `${c.src}#svgView(viewBox(${x},${y},${w},${h}))`, alt: '', width: Math.round(w * scale), height: Math.round(h * scale) });
}

export function petCaseImage(id, size) {
  const src = { bronze: '/static/pets/cases/bronze.svg', silver: '/static/pets/cases/silver.svg', gold: '/static/pets/cases/gold.svg' }[id];
  return el('img', { className: 'pet-picture', src: src ?? '/static/pets/cases/bronze.svg', alt: '', width: size, height: size });
}

function layer(src, size) {
  return el('img', { className: 'pet-picture pet-cloth', src, alt: '', width: size, height: size });
}

// petImage is a pet of kind at size pixels, in the clothes of outfit: each
// is a drawing of the same size put right over it.
export function petImage(kind, size, outfit = []) {
  const picture = el('img', { className: 'pet-picture', src: pictures[kind] ?? pictures.cat, alt: '', width: size, height: size });
  const worn = (outfit ?? []).filter((item) => clothes[item]);
  if (!worn.length) return picture;

  const box = el('span', { className: 'pet-dressed' }, picture);
  box.style.width = `${size}px`;
  box.style.height = `${size}px`;
  worn.sort((a, b) => layers.indexOf(clothes[a].slot) - layers.indexOf(clothes[b].slot));
  for (const item of worn) {
    const c = clothes[item];
    const front = layer(c.front ?? c.src, size);

    if (c.front) picture.before(layer(c.src, size));
    if (c.slot === 'eyes' && bigEyes[kind]) front.style.transform = `scale(${bigEyes[kind]})`;
    box.append(front);
  }
  return box;
}
