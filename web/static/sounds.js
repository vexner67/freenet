// The sounds of calls, made on the spot with Web Audio: no files to load. A
// browser lets a page sound only after a press, and a call starts from one;
// nothing sounds with reduced motion.

// Outside a browser (the tests) there is nothing to sound.
const still = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? true;
const loudness = 0.08;

let audio = null;

function context() {
  if (still) return null;
  audio ??= new AudioContext();
  if (audio.state === 'suspended') audio.resume().catch(() => {});

  return audio;
}

// tone is one note: from freq to end hertz in seconds, fading out.
function tone({ type, freq, end = freq, seconds, volume = 1, delay = 0 }) {
  const a = context();
  if (!a) return;

  const at = a.currentTime + delay;
  const oscillator = a.createOscillator();
  const gain = a.createGain();

  oscillator.type = type;
  oscillator.frequency.setValueAtTime(freq, at);
  oscillator.frequency.exponentialRampToValueAtTime(end, at + seconds);
  gain.gain.setValueAtTime(loudness * volume, at);
  gain.gain.exponentialRampToValueAtTime(0.0001, at + seconds);
  oscillator.connect(gain).connect(a.destination);
  oscillator.start(at);
  oscillator.stop(at + seconds);
}

// callUp is a call connected: two bright notes up, as an old messenger.
export function callUp() {
  tone({ type: 'sine', freq: 784, seconds: 0.16, volume: 1.4 });
  tone({ type: 'sine', freq: 1175, seconds: 0.26, volume: 1.4, delay: 0.12 });
}

// callDown is a call over: the same notes falling.
export function callDown() {
  tone({ type: 'sine', freq: 1175, seconds: 0.16, volume: 1.2 });
  tone({ type: 'sine', freq: 587, seconds: 0.3, volume: 1.2, delay: 0.12 });
}

// callClick is a switch of the microphone or the camera.
export function callClick(on = true) {
  tone({ type: 'square', freq: on ? 1500 : 900, end: on ? 1900 : 600, seconds: 0.04, volume: 0.6 });
}
