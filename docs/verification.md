# Verification evidence

Recorded September 29, 2026 (America/New_York). Commands are reproducible from the repository root; hosted results are linked from the README CI badge.

## Local environment

- Windows, Python 3.11.9, portable Go 1.27.1 (official archive SHA256 verified).
- No Deepgram or OpenAI credentials used; no paid provider traffic.
- Docker CLI present; local engine unavailable. Compose configuration validation can run locally; container execution is delegated to CI.
- Python's 8 tests and JavaScript syntax checks passed locally. After the PC restart, the portable Windows Go build, all 7 top-level gateway/provider tests, and the real Go/Python smoke flow passed using the existing toolchain/cache with project-local temporary files outside the sandbox. Earlier sandboxed attempts stalled and were interrupted. Coverage and the race detector were verified on Linux CI.

## Scope of evidence

The browser demo transports generated PCM and uses fixed transcript text. Unit and integration tests establish software behavior, not speech accuracy or human coaching quality. Deepgram is verified with a mock provider only. No load benchmark, end-user study, or production deployment has been performed.

## Executed results

[GitHub Actions run 36650991316](https://github.com/dileepreddy27/voice-interview-lab/actions/runs/36650991316) passed on commit `5217b328e7be5315fdda2e156b95c78506d43523`:

| Check | Outcome |
|---|---|
| Go static checks and race-enabled test suite | Passed; internal gateway package has 80.0% statement coverage (entrypoint has no unit coverage; smoke tests exercise it) |
| Python evaluation/HTTP tests | 8 passed |
| JavaScript syntax | Passed for dashboard and audio worklet |
| Real Redis storage | Only word/pace/filler counts stored; 3590–3600 seconds remaining TTL; test deletes only its own key |
| Native Go + Python integration | 80 ordered acknowledgements, 256,000 audio bytes, 8 represented seconds, 4 final transcript segments, real coaching response |
| Chromium acceptance | Consent gate, full demo, JSON export, reset, early stop, no page JavaScript errors |
| Responsive layout | 390 px viewport has no horizontal overflow; desktop and mobile screenshots inspected |
| Docker Compose | Both images built; stack healthy; complete WebSocket smoke test passed |

The captured browser session produced **55 words**, four structure cues, and zero filler matches for the fixed script. Its displayed coaching HTTP round trip was **1 ms** on that CI run. This is one local service observation, not a latency distribution, production SLO, or speech-recognition benchmark. Generated demo audio represents eight seconds; the smoke client intentionally sends it faster than real time.

The same run logged a **8.14 ms native smoke-client wall time** and **73.7 ms Compose smoke-client wall time**; coaching round trips were 1 ms and 67 ms respectively. These are single-run, accelerated synthetic transport observations on a GitHub-hosted Ubuntu runner. They are recorded for reproducibility, not compared as a controlled benchmark.

The resumed Windows smoke run also verified 80 ordered acknowledgements, 256,000 bytes, 4 final segments, and 55 words. Its single-run smoke-client wall time was 47.28 ms, with an 11 ms coaching round trip. The local Chromium acceptance check also passed consent, full demo, export, reset, early stop, and mobile layout checks with no page JavaScript errors. Both temporary local service processes were stopped after the check. Redis was disabled for this local run; no paid API requests were made.

The [desktop](images/dashboard.png) and [mobile](images/mobile.png) screenshots come from that CI run, not a design mockup. They were inspected for visible transcript, completed feedback, consent disclosure, and readable layout.

## Not verified / not implemented

- No real Deepgram call, microphone permission/device path, speech accuracy study, provider latency study, or paid service access was tested.
- OpenAI Realtime is not implemented. Coaching is a local deterministic heuristic engine.
- Session admission/release is tested; 16-session real-time throughput is not benchmarked.
- No public application deployment, production authentication, distributed quotas, or production security review is claimed.

The final README/evidence commit has its own CI run, accessible through the badge; the fixed reference above preserves the provenance of these screenshots and measurements.
