// The pictures of pets: pixel drawings of 16×16, shown in whole steps so
// that their pixels stay sharp.

import { el } from './layout.js';

const pictures = {
  cat: '/static/pets/cat.svg',
  dog: '/static/pets/dog.svg',
  hamster: '/static/pets/hamster.svg',
  dragon: '/static/pets/dragon.svg',
};

export function petImage(kind, size) {
  return el('img', { className: 'pet-picture', src: pictures[kind] ?? pictures.cat, alt: '', width: size, height: size });
}
