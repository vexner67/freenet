import { Room, RoomEvent, LocalAudioTrack, version } from '/static/vendor/livekit-client.mjs';

const log = document.getElementById('log');
const status = document.getElementById('status');
const say = (message) => { log.textContent += `${message}\n`; };
let room, context, oscillator;
let violations = 0;
window.addEventListener('securitypolicyviolation', (event) => {
  violations++;
  say(`CSP VIOLATION: ${event.violatedDirective}: ${event.blockedURI}; source=${event.sourceFile}:${event.lineNumber}; sample=${event.sample}`);
});
window.addEventListener('error', (event) => say(`ERROR: ${event.message}`));
window.addEventListener('unhandledrejection', (event) => say(`REJECTION: ${event.reason}`));
status.textContent = `Library ${version} loaded. Ready.`;
say(`Trusted Types supported: ${Boolean(window.trustedTypes)}`);

async function join(microphone) {
  document.getElementById('join').disabled = true;
  document.getElementById('microphone').disabled = true;
  try {
    const response = await fetch('/probe-token', { method: 'POST' });
    if (!response.ok) throw new Error(`Token: ${response.status}`);
    const { token } = await response.json();
    room = new Room({ adaptiveStream: false, dynacast: false });
    room.on(RoomEvent.ConnectionStateChanged, (state) => { status.textContent = state; say(`Connection: ${state}`); });
    room.on(RoomEvent.ParticipantConnected, (participant) => say(`Participant: ${participant.identity}`));
    room.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
      const audio = track.attach();
      document.getElementById('audio').append(audio);
      audio.play().catch((error) => say(`Autoplay: ${error.message}`));
      say(`Subscribed audio: ${participant.identity}, ${publication.trackSid}`);
      const analyser = context.createAnalyser();
      context.createMediaStreamSource(new MediaStream([track.mediaStreamTrack])).connect(analyser);
      const samples = new Float32Array(analyser.fftSize);
      const timer = setInterval(() => {
        if (!room || room.state === 'disconnected') { clearInterval(timer); return; }
        analyser.getFloatTimeDomainData(samples);
        const peak = Math.max(...samples.map(Math.abs));
        if (peak > 0.001) { say(`RECEIVED AUDIO peak=${peak.toFixed(4)}; CSP violations=${violations}`); clearInterval(timer); }
      }, 500);
    });
    room.on(RoomEvent.TrackUnsubscribed, (track) => { for (const node of track.detach()) node.remove(); });
    context = new AudioContext();
    await context.resume();
    await room.connect(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}`, token);
    await room.startAudio();
    if (microphone) {
      await room.localParticipant.setMicrophoneEnabled(true);
    } else {
      oscillator = context.createOscillator();
      const gain = context.createGain();
      gain.gain.value = 0.02;
      const destination = context.createMediaStreamDestination();
      oscillator.connect(gain).connect(destination);
      oscillator.frequency.value = 440;
      oscillator.start();
      await room.localParticipant.publishTrack(new LocalAudioTrack(destination.stream.getAudioTracks()[0]));
    }
    say(`Published audio; CSP violations=${violations}`);
    document.getElementById('leave').disabled = false;
  } catch (error) {
    say(`FAILED: ${error.stack || error}`);
    await leave();
  }
}

async function leave() {
  if (oscillator) { oscillator.stop(); oscillator = null; }
  if (room) { await room.disconnect(); room = null; }
  if (context) { await context.close(); context = null; }
  document.getElementById('audio').replaceChildren();
  document.getElementById('join').disabled = false;
  document.getElementById('microphone').disabled = false;
  document.getElementById('leave').disabled = true;
}

document.getElementById('join').addEventListener('click', () => join(false));
document.getElementById('microphone').addEventListener('click', () => join(true));
document.getElementById('leave').addEventListener('click', () => leave().catch((error) => say(error.message)));
window.addEventListener('pagehide', () => { if (room) room.disconnect(); });
