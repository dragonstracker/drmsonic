# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow the release tags on GitHub.

## [Unreleased] — branch `devloper`

### Added
- Video selector `max` (highest bandwidth) and audio selectors `lang=en` / `desc:en`.

### Fixed
- Start applies current Manifest/Keys in the same request (no blur/save race).
- Streams page status/start/stop ids mismatched (`_` vs `~`); running state updates again.
- Events refresh no longer downgrades a stream that already had Manifest/Keys when the API returns the same id blank.
- Events auto-remove no longer deletes streams that still have Manifest/Keys (manual paste / managed rows survive refresh).
- Quality/track dropdown appears after MPD parse: track discovery no longer blocked by IgnoreUpdate, UI refreshes on track list change.

## [1.1.4] — 2026-11-11

### Added
- Panel login at `/login` with session cookies, logout, and per-IP login rate limiting.
- Opaque playlist tokens `/t/<token>/...` and optional `/{username}/{password}/...` playlist paths.
- Provider script runner (`scripts/<name>.sh|.py`) for `channels`, `events`, `manifest`, and `cdm` (o11 contract).
- UPDATE CHANNELS / REFRESH EVENTS request+apply against the provider Script field.
- Events auto refresh: when enabled, runs `action=events` every Events refresh period and applies changes.
- Demo script downloads: `static/example.py` and `static/DRMSONIC.py`.
- Default `admin` / `admin` on first install when no passwords exist.
- Video track option **All qualities** (ABR master with per-rendition variants).
- **Open panel without login** (`NoAuth`) and per-user **Playlist** toggle.
- Rolling GitHub prerelease tag `dev` from `devloper` for VPS testing.
- Phase 1 security baseline: role checks, provider scope, trusted proxies, CSRF on `/api/action`, SSRF private-fetch block, Docker `/app/data`, CI `gofmt` + `-race`.

### Changed
- Login page restyled (split brand/operator layout).
- Panel static assets renamed from `o11.*` to `drmsonic.*`.
- SAVE USERS is a single `saveusers` action (all rows + NoAuth).
- Installer defaults to the `dev` release; pin with `DRMSONIC_TAG=latest|vX.Y.Z`.
- HLS/DASH delivery tuned toward o11 parity: measured bandwidth, standard playlist MIME, deeper segment window, DASH refresh at segment cadence, on-demand idle ≥60s with Autostart exemption.
- Panel assets gzipped and bundled (`app.js` / `app.css` / `tz.js`) with `ETag` revalidation; HTML and `/api/action` gzipped; `hls.min.js` loads on first play.

### Fixed
- Users SAVE showed a red Error/OK dialog after success; success now reloads quietly.
- ADD USER allowed empty passwords; create now requires a password.
- NEW PROVIDER / RENAME navigated to `/config/"ok"`; API returns the provider id and the UI parses JSON.
- Config editors stayed empty for stream ids with underscores (`tnt_sports_2`); DOM ids use `~`.
- Wrong-password lockouts blocked correct logins and grew without bound; correct login always succeeds, failures decay, delay capped at 30s.
- Auth, playlist, and panel capability gaps (empty passwords, `?user=` alone, Web Access vs playlist-only, CSRF/session for SAVE USERS).
- Stream status text, broadcast-ended handling, selector AND filters, provider import shapes/size, segment 404 stalls, audio-only DASH bursts, config first-load races, and panel JS template quoting inside `<script>`.

## [1.1.3] — 2026-11-09

### Changed
- `install.sh` upgrades in place on re-run (stop, replace binary, keep providers, restart).

## [1.1.2] — 2026-11-09

### Fixed
- `app1.conf` HLS URLs use the request host instead of `127.0.0.1`.

## [1.1.1] — 2026-11-09

### Added
- `uninstall.sh` full VPS wipe (service, unit, ufw, install dir).

## [1.1.0] — 2026-11-09

### Added
- GitHub Releases binary publish (linux amd64/arm64, windows amd64).
- Binary VPS installer on port `7779`.
- Default HTTP port `7779`.
