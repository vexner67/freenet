// blurhash.js: draws a blurhash, the short string servers of the
// Fediverse send for a blurred picture to show until the picture itself
// is read (https://blurha.sh). The site draws it for the covers of the
// videos of their posts, with no request to anybody.

const digits = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~';

function decode83(text) {
  let value = 0;
  for (const c of text) {
    const digit = digits.indexOf(c);
    if (digit < 0) return -1;
    value = value * 83 + digit;
  }
  return value;
}

function toLinear(value) {
  const v = value / 255;
  return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
}

function toSRGB(value) {
  const v = Math.max(0, Math.min(1, value));
  return Math.round(v <= 0.0031308 ? v * 12.92 * 255 : (1.055 * v ** (1 / 2.4) - 0.055) * 255);
}

function signedSquare(value) {
  return Math.sign(value) * value * value;
}

// components are the colors of the cosine waves the picture is made of,
// in linear light; null for a string that is no blurhash.
function components(hash) {
  if (typeof hash !== 'string' || hash.length < 6) return null;
  if ([...hash].some((c) => !digits.includes(c))) return null;

  const size = decode83(hash[0]);
  const columns = (size % 9) + 1;
  const rows = Math.floor(size / 9) + 1;
  if (hash.length !== 4 + 2 * columns * rows) return null;

  const maximum = (decode83(hash[1]) + 1) / 166;
  const dc = decode83(hash.slice(2, 6));
  const colors = [[toLinear(dc >> 16), toLinear((dc >> 8) & 255), toLinear(dc & 255)]];

  for (let i = 1; i < columns * rows; i++) {
    const ac = decode83(hash.slice(4 + i * 2, 6 + i * 2));
    colors.push([
      signedSquare((Math.floor(ac / 361) - 9) / 9) * maximum,
      signedSquare(((Math.floor(ac / 19) % 19) - 9) / 9) * maximum,
      signedSquare(((ac % 19) - 9) / 9) * maximum,
    ]);
  }

  return { columns, rows, colors };
}

// decodeBlurhash is the RGBA pixels of a blurhash drawn width × height,
// or null for a string that is no blurhash.
export function decodeBlurhash(hash, width, height) {
  const parts = components(hash);
  if (parts === null) return null;

  const { columns, rows, colors } = parts;
  const pixels = new Uint8ClampedArray(width * height * 4);

  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      let r = 0;
      let g = 0;
      let b = 0;

      for (let j = 0; j < rows; j++) {
        for (let i = 0; i < columns; i++) {
          const basis = Math.cos((Math.PI * x * i) / width) * Math.cos((Math.PI * y * j) / height);
          const color = colors[i + j * columns];
          r += color[0] * basis;
          g += color[1] * basis;
          b += color[2] * basis;
        }
      }

      const at = 4 * (x + y * width);
      pixels[at] = toSRGB(r);
      pixels[at + 1] = toSRGB(g);
      pixels[at + 2] = toSRGB(b);
      pixels[at + 3] = 255;
    }
  }

  return pixels;
}

// drawBlurhash draws a blurhash on all of a canvas; false when it is no
// blurhash, and the canvas is left as it was.
export function drawBlurhash(canvas, hash) {
  const pixels = decodeBlurhash(hash, canvas.width, canvas.height);
  if (pixels === null) return false;

  const context = canvas.getContext('2d');
  if (!context) return false;

  context.putImageData(new ImageData(pixels, canvas.width, canvas.height), 0, 0);
  return true;
}
