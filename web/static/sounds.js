// The sounds of the casino, each game its own, made on the spot with Web
// Audio: no files to load. A browser lets a page sound only after a press,
// and each game starts from one; nothing sounds with reduced motion, as
// nothing moves then.

const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
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

// noise is a burst of hiss through a low filter, falling from freq to end.
function noise({ freq, end, seconds, volume = 1 }) {
  const a = context();
  if (!a) return;

  const buffer = a.createBuffer(1, Math.ceil(a.sampleRate * seconds), a.sampleRate);
  const samples = buffer.getChannelData(0);
  for (let i = 0; i < samples.length; i++) samples[i] = Math.random() * 2 - 1;

  const source = a.createBufferSource();
  const filter = a.createBiquadFilter();
  const gain = a.createGain();

  source.buffer = buffer;
  filter.type = 'lowpass';
  filter.frequency.setValueAtTime(freq, a.currentTime);
  filter.frequency.exponentialRampToValueAtTime(end, a.currentTime + seconds);
  gain.gain.setValueAtTime(loudness * volume, a.currentTime);
  gain.gain.exponentialRampToValueAtTime(0.0001, a.currentTime + seconds);
  source.connect(filter).connect(gain).connect(a.destination);
  source.start();
}

// wheelTick is a click of the light of "Огонёк" stepping to the next cell,
// a wooden one, as of a wheel of fortune.
export function wheelTick() {
  tone({ type: 'square', freq: 1400, end: 500, seconds: 0.03 });
}

export function wheelStop() {
  tone({ type: 'triangle', freq: 660, seconds: 0.14, volume: 1.4 });
  tone({ type: 'triangle', freq: 990, seconds: 0.2, volume: 1.4, delay: 0.09 });
}

// aimSweep is the marker of "Прицел" running over the strip: a whistle
// rising for as long as it runs.
export function aimSweep(seconds) {
  tone({ type: 'sine', freq: 240, end: 1800, seconds, volume: 0.9 });
}

export function aimStop() {
  tone({ type: 'sine', freq: 1500, end: 1200, seconds: 0.07, volume: 1.4 });
}

// rocketEngine is the roar of the flight of "Ракетка", rising as the
// rocket climbs; the returned function ends it.
export function rocketEngine() {
  const a = context();
  if (!a) return () => {};

  const oscillator = a.createOscillator();
  const gain = a.createGain();

  oscillator.type = 'sawtooth';
  oscillator.frequency.setValueAtTime(70, a.currentTime);
  oscillator.frequency.exponentialRampToValueAtTime(420, a.currentTime + 20);
  gain.gain.setValueAtTime(loudness * 0.5, a.currentTime);
  oscillator.connect(gain).connect(a.destination);
  oscillator.start();

  return () => {
    gain.gain.setTargetAtTime(0, a.currentTime, 0.03);
    oscillator.stop(a.currentTime + 0.2);
  };
}

export function rocketCash() {
  tone({ type: 'triangle', freq: 880, seconds: 0.1, volume: 1.4 });
  tone({ type: 'triangle', freq: 1320, seconds: 0.18, volume: 1.4, delay: 0.08 });
}

export function rocketBoom() {
  noise({ freq: 1800, end: 80, seconds: 0.7, volume: 3 });
}
