# Verification evidence

Verification record is updated after execution. Commands are reproducible from the repository root; hosted results are linked from the README CI badge.

## Local environment

- Windows, Python 3.11.9, portable Go 1.27.1 (official archive SHA256 verified).
- No Deepgram or OpenAI credentials used; no paid provider traffic.
- Docker CLI present; local engine unavailable. Compose configuration validation can run locally; container execution is delegated to CI.

## Scope of evidence

The browser demo transports generated PCM and uses fixed transcript text. Unit and integration tests establish software behavior, not speech accuracy or human coaching quality. Deepgram is verified with a mock provider only. No load benchmark, end-user study, or production deployment has been performed.

Executed outcomes will be recorded below before publication is finalized.
