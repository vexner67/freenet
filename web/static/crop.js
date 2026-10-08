// The arithmetic of choosing the square of a photo (avatar.js), apart
// from the page so that it is tested.

// fitInside is the size of a width×height picture scaled down, keeping
// its proportions, to fit maxWidth×maxHeight; with grow, a smaller one is
// scaled up to fit too.
export function fitInside(width, height, maxWidth, maxHeight, { grow = false } = {}) {
  let k = Math.min(maxWidth / width, maxHeight / height);
  if (!grow) k = Math.min(k, 1);

  return { width: Math.max(1, Math.round(width * k)), height: Math.max(1, Math.round(height * k)) };
}

// cropSquare is the square first offered: in the middle, four fifths of
// the shorter side, as openvk's cropper starts.
export function cropSquare(width, height) {
  const side = Math.round(Math.min(width, height) * 0.8);

  return { x: Math.round((width - side) / 2), y: Math.round((height - side) / 2), side };
}

// movedSquare is square moved by dx, dy and kept inside bounds.
export function movedSquare(square, dx, dy, bounds) {
  return {
    x: clamp(square.x + dx, 0, bounds.width - square.side),
    y: clamp(square.y + dy, 0, bounds.height - square.side),
    side: square.side,
  };
}

// resizedSquare grows square by delta from its top left corner, no
// smaller than minSide (or the picture, if smaller) and inside bounds.
export function resizedSquare(square, delta, bounds, minSide) {
  const largest = Math.min(bounds.width - square.x, bounds.height - square.y);
  const smallest = Math.min(minSide, largest);

  return { x: square.x, y: square.y, side: clamp(square.side + delta, smallest, largest) };
}

function clamp(value, low, high) {
  return Math.min(Math.max(value, low), high);
}
