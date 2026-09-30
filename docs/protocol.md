# Streaming protocol

Connect to `/ws` on the gateway. For browser requests the Origin host must match the request host. Non-browser requests may omit Origin. There is no user authentication; run on loopback.

Within five seconds send a text JSON frame:

```json
{"type":"start","source":"demo","consent":true,"sample_rate":16000}
```

The alternative source is `microphone`, requiring a configured Deepgram key. Unsupported sources, missing consent, or another sample rate return `error` and close. No audio is forwarded before this handshake.

After `ready`, send binary **raw little-endian signed PCM16, mono, 16 kHz**. Normal browser frames are 3200 bytes / 100 ms. A frame must be nonempty, even-sized, and at most 32000 bytes. Headered WAV/MP3/WebM files are not supported. Bytes are not validated as intelligible speech.

| Server event | Fields / meaning |
|---|---|
| `ready` | source, synthetic flag, PCM format |
| `ack` | monotonic seq, cumulative audio_bytes, represented audio_seconds |
| `transcript` | text, final, synthetic; interim text must not enter final coaching input |
| `complete` | feedback object, telemetry, synthetic; terminal success |
| `error` | code, sanitized message; terminal failure |

To finish, send `{"type":"stop"}`. Stop microphone capture before sending it; further audio is an invalid state. The gateway sends Deepgram `CloseStream` and drains pending finalized events until provider metadata/normal closure or a five-second deadline. The demo closes its internal event stream immediately after flushing already generated segments. Early stops include only emitted segments. Zero audio is an error.

Lifecycle: `connecting → validating → streaming → draining → coaching → complete/closed`. Any failure ends the session; the browser stops all media tracks. Closing the tab closes the socket and releases server resources. There is no resume token or replay buffer.

## Backpressure and limits

- Gateway admission: 16 active sessions (including clients waiting on handshake).
- Input channel: 8 frames, each at most 32 KB; socket reads block when the channel is full.
- Provider channel: 16 events; cancellation unblocks pending emissions.
- Provider response maximum: 256 KB. Final transcript: 24 KB total (Go byte count).
- Audio limit: 3,840,000 bytes (120 seconds). Wall-clock input deadline: 120 seconds. Overall session context: 125 seconds after handshake.
- Browser: at most 20 outstanding 100 ms chunks and 64 KB of pending socket bytes; exceeding either stops capture visibly.
- Upstream/client writes: three-second deadline. Coaching request: three-second HTTP timeout.
- Transport errors end the stream. Partial feedback is not presented as completed.

This bounds application queues, not all OS/TCP/provider buffers. The session cap is per process, not distributed. A deliberate client can upload faster than real time up to the total byte limit; authentication and quota enforcement are outside this local lab.

## Telemetry semantics

`audio_seconds = audio_bytes / 32000` measures represented PCM duration. `session_elapsed_ms` is gateway wall time from ready through provider drain, before coaching. `coach_roundtrip_ms` includes the HTTP call and response decode. Neither is speech-to-text latency. There is no fabricated throughput, ASR latency, or percentile metric.

The demo's 80 frames represent eight seconds but its transcript is deliberately scripted; derived pace is not meaningful human performance data. UI displays word count/fillers and the actual coaching round trip. JSON exports keep the `synthetic` flag and raw telemetry.
