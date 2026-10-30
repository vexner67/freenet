// The pictures of pets: round drawings, all on one ball of the same place
// and size. Clothes are drawings of their own, put over the pet at the
// cell (a sixteenth of the picture) of the head, eyes, neck or body.

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

// clothes are the drawings of clothes: the slot they go on and their size
// in cells of the pet.
const clothes = {
  cap: { src: '/static/pets/clothes/cap.svg', slot: 'head', w: 8, h: 4 },
  helmet: { src: '/static/pets/clothes/helmet.svg', slot: 'head', w: 8, h: 4 },
  crown: { src: '/static/pets/clothes/crown.svg', slot: 'head', w: 8, h: 4 },
  glasses: { src: '/static/pets/clothes/glasses.svg', slot: 'eyes', w: 10, h: 2 },
  shades: { src: '/static/pets/clothes/shades.svg', slot: 'eyes', w: 10, h: 2 },
  monocle: { src: '/static/pets/clothes/monocle.svg', slot: 'eyes', w: 10, h: 2 },
  bow: { src: '/static/pets/clothes/bow.svg', slot: 'neck', w: 8, h: 2 },
  scarf: { src: '/static/pets/clothes/scarf.svg', slot: 'neck', w: 12, h: 3 },
  medal: { src: '/static/pets/clothes/medal.svg', slot: 'neck', w: 8, h: 4 },
  sweater: { src: '/static/pets/clothes/sweater.svg', slot: 'body', w: 12, h: 4 },
  armor: { src: '/static/pets/clothes/armor.svg', slot: 'body', w: 12, h: 4 },
  cape: { src: '/static/pets/clothes/cape.svg', slot: 'body', w: 12, h: 4 },
};

// anchors are the top left cells of each slot: every pet is the same ball.
const anchors = { head: [4, 0.6], eyes: [3, 6.8], neck: [4, 11.2], body: [2, 10.9] };
// Things of a slot that lie around the ball and start further out.
const wide = { scarf: [2, 11] };

// Clothes are drawn body first, so that a scarf lies over a sweater and a
// hat over the eyes.
const layers = ['body', 'neck', 'eyes', 'head'];

export function petItemSlot(item) {
  return clothes[item]?.slot ?? '';
}

export function petItemImage(item, size) {
  const c = clothes[item];
  if (!c) return el('span');
  return el('img', { className: 'pet-picture', src: c.src, alt: '', width: size, height: Math.round(size * c.h / c.w) });
}

export function petCaseImage(id, size) {
  const src = { bronze: '/static/pets/cases/bronze.svg', silver: '/static/pets/cases/silver.svg', gold: '/static/pets/cases/gold.svg' }[id];
  return el('img', { className: 'pet-picture', src: src ?? '/static/pets/cases/bronze.svg', alt: '', width: size, height: size });
}

// petImage is a pet of kind at size pixels, in the clothes of outfit.
export function petImage(kind, size, outfit = []) {
  const picture = el('img', { className: 'pet-picture', src: pictures[kind] ?? pictures.cat, alt: '', width: size, height: size });
  const worn = (outfit ?? []).filter((item) => clothes[item]);
  if (!worn.length) return picture;

  const cell = size / 16;
  const box = el('span', { className: 'pet-dressed' }, picture);
  box.style.width = `${size}px`;
  box.style.height = `${size}px`;
  worn.sort((a, b) => layers.indexOf(clothes[a].slot) - layers.indexOf(clothes[b].slot));
  for (const item of worn) {
    const c = clothes[item];
    const [x, y] = wide[item] ?? anchors[c.slot];
    const img = el('img', { className: 'pet-picture pet-cloth', src: c.src, alt: '', width: c.w * cell, height: c.h * cell });
    img.style.left = `${x * cell}px`;
    img.style.top = `${y * cell}px`;
    box.append(img);
  }
  return box;
}
