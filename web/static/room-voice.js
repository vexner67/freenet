// Only this module knows the media server SDK. No E2EE workers or policies.
let library;
const sdk = () => (library ??= import('/static/vendor/livekit-client.mjs'));

export async function joinVoice(media, { root, mutedIDs = [], memberIDs = [], onSpeaking = () => {}, onState = () => {}, onMuted = () => {} }) {
  const { Room, RoomEvent, Track } = await sdk();
  const blocked = new Set(mutedIDs);
  const silenced = new Set();
  const allowed = new Set(memberIDs);
  const room = new Room({ adaptiveStream: false, dynacast: false });
  let ended = false;
  const audible = (id) => allowed.has(id) && !blocked.has(id) && !silenced.has(id);
  const subscribe = (publication, participant) => {
    if (publication.kind === Track.Kind.Audio) publication.setSubscribed(audible(participant.identity));
  };
  room.on(RoomEvent.TrackPublished, subscribe);
  room.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
    if (track.kind !== Track.Kind.Audio || !audible(participant.identity)) { publication.setSubscribed(false); return; }
    const audio = track.attach();
    audio.dataset.roomPeer = participant.identity;
    root.append(audio);
    audio.play().catch(() => onState('audio_blocked'));
  });
  room.on(RoomEvent.TrackUnsubscribed, (track) => { for (const audio of track.detach()) audio.remove(); });
  room.on(RoomEvent.ActiveSpeakersChanged, (speakers) => onSpeaking(speakers.map((speaker) => speaker.identity)));
  room.on(RoomEvent.ConnectionStateChanged, (state) => { if (!ended) onState(state); });
  room.on(RoomEvent.TrackMuted, (publication, participant) => {
    if (participant === room.localParticipant && publication.kind === Track.Kind.Audio) onMuted(true);
  });
  room.on(RoomEvent.TrackUnmuted, (publication, participant) => {
    if (participant === room.localParticipant && publication.kind === Track.Kind.Audio) onMuted(false);
  });
  try {
    await room.connect(media.url, media.token, { autoSubscribe: false });
    for (const participant of room.remoteParticipants.values()) {
      for (const publication of participant.trackPublications.values()) subscribe(publication, participant);
    }
    room.startAudio().catch(() => { if (!ended) onState('audio_blocked'); });
    // A permission prompt can stay open indefinitely. Presence, chat and the
    // shared screen must be ready while the browser waits for the answer.
    room.localParticipant.setMicrophoneEnabled(true).then(() => { if (!ended) onMuted(false); })
      .catch(() => { if (!ended) { onMuted(true); onState('microphone_denied'); } });
  } catch (error) {
    ended = true;
    await room.disconnect();
    root.replaceChildren();
    throw error;
  }
  return {
    async leave() {
      ended = true;
      await room.disconnect();
      room.removeAllListeners();
      root.replaceChildren();
    },
    async mute(value) { await room.localParticipant.setMicrophoneEnabled(!value); onMuted(value); },
    async startAudio() { await room.startAudio(); },
    silence(id, value) {
      if (value) silenced.add(id); else silenced.delete(id);
      const participant = room.remoteParticipants.get(id);
      if (participant) for (const publication of participant.trackPublications.values()) subscribe(publication, participant);
    },
    members(ids, members) {
      blocked.clear(); for (const id of ids) blocked.add(id);
      allowed.clear(); for (const id of members) allowed.add(id);
      for (const participant of room.remoteParticipants.values()) for (const publication of participant.trackPublications.values()) subscribe(publication, participant);
    },
    blocked(id) { return blocked.has(id); },
  };
}
