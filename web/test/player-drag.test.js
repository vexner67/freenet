import test from 'node:test';
import assert from 'node:assert/strict';
import { playerPosition, dragPlayer } from '../static/player-drag.js';

test('the player stays reachable at every viewport edge and after resizing', () => {
  assert.deepEqual(playerPosition(50, 80, 500, 40, 1200, 800), { x: 50, y: 80 });
  assert.deepEqual(playerPosition(-30, -20, 500, 40, 1200, 800), { x: 0, y: 0 });
  assert.deepEqual(playerPosition(1100, 790, 500, 40, 1200, 800), { x: 700, y: 760 });
  assert.deepEqual(playerPosition(700, 760, 500, 40, 320, 600), { x: 0, y: 560 });
});

test('dragging ignores controls, captures a free area and stops on cancellation', () => {
  const handlers = new Map();
  const classes = new Set();
  let captured = false;
  const previous = globalThis.window;
  globalThis.window = { innerWidth: 1000, innerHeight: 600, addEventListener() {} };
  try {
    const box = {
      hidden: false,
      style: { setProperty() {} },
      classList: { add: (c) => classes.add(c), remove: (c) => classes.delete(c), toggle: (c, on) => on ? classes.add(c) : classes.delete(c) },
      getBoundingClientRect: () => ({ left: 100, top: 200, width: 500, height: 40 }),
      addEventListener: (type, fn) => handlers.set(type, fn),
      setPointerCapture: () => { captured = true; },
      hasPointerCapture: () => captured,
      releasePointerCapture: () => { captured = false; },
    };
    dragPlayer(box);
    const down = { button: 0, pointerId: 1, clientX: 110, clientY: 210, preventDefault() {}, target: { closest: () => true } };
    handlers.get('pointerdown')(down);
    assert.equal(captured, false);
    handlers.get('pointerdown')({ ...down, target: { closest: () => null } });
    assert.equal(captured, true);
    handlers.get('pointermove')({ pointerId: 1, clientX: -40, clientY: 30 });
    assert.equal(box.style.left, '0px');
    assert.equal(box.style.top, '20px');
    assert(classes.has('queue-below'));
    handlers.get('pointercancel')({ pointerId: 1 });
    assert.equal(captured, false);
    handlers.get('pointermove')({ pointerId: 1, clientX: 900, clientY: 500 });
    assert.equal(box.style.left, '0px');
  } finally {
    globalThis.window = previous;
  }
});
