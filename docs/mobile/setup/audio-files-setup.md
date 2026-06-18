# Audio Files Setup

The mobile client currently includes call audio assets under:

```text
mobile/src/assets/sounds/
```

Expected files:

- `incoming_call.mp3`: incoming call ringtone.
- `ringback.mp3`: outbound ringback tone.

Recommended format:

- MP3.
- 44.1 kHz or 48 kHz.
- Mono.
- 64-128 kbps.
- Less than 500 KB per file.

Verification:

```bash
cd mobile
bash scripts/verify-alarm-setup.sh
```

If either file is missing, the app can still run, but the corresponding call sound will not play. Keep file names stable unless the audio service import paths are updated at the same time.
