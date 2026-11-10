# LiveKit CSP probe

Run `bash scripts/rooms-probe/run.sh` from the repository root; open
http://localhost:8090 in two browsers. The standalone harness uses the actual
`rest.NewServer` page handler, CSP, Trusted Types and asset versioning. Its
synthetic audio button publishes a quiet 440 Hz tone, and the peer checks
received PCM, so connection alone cannot be mistaken for successful audio.
The Microphone button is for the remaining manual voice check.

The harness binds loopback only, uses LiveKit's public development credentials,
issues short-lived test tokens server-side and never runs on production.
Ctrl+C shuts down the harness and removes the container and temporary web root.
No CSP exceptions, workers, E2EE or Trusted Types policies are enabled.

2026-11-10: LiveKit server 1.13.11, client 2.23.0. Two Chromium in-app tabs
connected, published, subscribed and received PCM in both directions (peak
0.0201). Trusted Types supported; no Trusted Types failures. The browser injected
an inline style (`style-src-elem`, source `:2`); the identical violation occurs
on a control page without the library. No additional violation on connection
or publication. The production CSP is unchanged. Chrome/Firefox with real
microphones and Safari on a physical iPhone remain required before deployment.
The owner authorized continuing implementation before those checks.

Client source: https://registry.npmjs.org/livekit-client/-/livekit-client-2.23.0.tgz
SHA-256 of `web/static/vendor/livekit-client.mjs`:
`053820ce316a0d9881e54d4745eeb45b96575c05d3591a214f318168336e1367`.
The npm archive's SHA-512 integrity was verified before extraction; the vendor
file is unchanged. License is stored beside it. `web/test/vendor.test.js` checks
the hash, syntax, absence of external imports and dangerous code/HTML sinks.
