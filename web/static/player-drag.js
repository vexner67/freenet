// Keep a floating player inside the viewport, including after a resize.
export function playerPosition(x, y, width, height, viewportWidth, viewportHeight) {
  return { x: Math.max(0, Math.min(x, viewportWidth - width)), y: Math.max(0, Math.min(y, viewportHeight - height)) };
}

export function dragPlayer(box) {
  let drag = null;
  let moved = false;
  const place = (x, y) => {
    const rect = box.getBoundingClientRect();
    const point = playerPosition(x, y, rect.width, rect.height, window.innerWidth, window.innerHeight);
    box.style.left = point.x + 'px';
    box.style.top = point.y + 'px';
    box.style.right = 'auto';
    box.style.bottom = 'auto';
    const below = point.y < (window.innerHeight - rect.height) / 2;
    box.classList.toggle('queue-below', below);
    box.style.setProperty('--player-queue-room', Math.max(0, (below ? window.innerHeight - point.y - rect.height : point.y) - 4) + 'px');
  };
  box.addEventListener('pointerdown', (event) => {
    if (event.button !== 0 || event.target.closest('button, input, a[href], .player-time, .player-queue')) return;
    const rect = box.getBoundingClientRect();
    drag = { id: event.pointerId, x: event.clientX - rect.left, y: event.clientY - rect.top };
    box.setPointerCapture(event.pointerId);
    box.classList.add('player-dragging');
    event.preventDefault();
  });
  box.addEventListener('pointermove', (event) => {
    if (!drag || drag.id !== event.pointerId) return;
    moved = true;
    box.classList.add('player-moved');
    place(event.clientX - drag.x, event.clientY - drag.y);
  });
  const end = (event) => {
    if (!drag || drag.id !== event.pointerId) return;
    drag = null;
    box.classList.remove('player-dragging');
    if (box.hasPointerCapture(event.pointerId)) box.releasePointerCapture(event.pointerId);
  };
  box.addEventListener('pointerup', end);
  box.addEventListener('pointercancel', end);
  box.addEventListener('lostpointercapture', end);
  const keepInside = () => {
    if (!moved || box.hidden) return;
    const rect = box.getBoundingClientRect();
    place(rect.left, rect.top);
  };
  window.addEventListener('resize', keepInside);
  return keepInside;
}
