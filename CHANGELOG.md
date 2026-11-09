# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow the release tags on GitHub.

## [Unreleased] — branch `devloper`

### Added
- Login page at `/login` with session cookies and logout.
- Opaque playlist tokens `/t/<token>/...` (username/password never appear in stream URLs).
- Login rate limiting per IP.
- Rolling GitHub prerelease tag `dev` from every `devloper` push for VPS testing.
- `internal/selector` quality grammar package (Phase 2 start).
- Phase 1: role checks, provider scope, trusted proxies, CSRF on `/api/action`, SSRF private-fetch block, Docker `/app/data`, CI `gofmt` + `-race`.
- This changelog.

### Changed
- Panel static assets renamed from `o11.*` to `drmsonic.*`.
- Users password field no longer stores the literal `hidden` placeholder.
- Playlist links use stream tokens instead of `/{user}/{pass}/`.

### Fixed
- Auth ignored panel users unless `-user`/`-password` were set.
- `?user=` alone could grant access without a password.
- Empty-password users could authenticate.
- Admin vs panel-user capabilities were not enforced.
- Guessable username/password playlist paths removed.
- Selector comma filters (`h<=720,codec=avc1`) AND together instead of OR.
- SAVE USERS (`users_only`) and SAVE ALL PROVIDERS persistence.
- User rename ignored `NewUserName` from the panel.
- New users default to Web Access on.

### Added
- Default `admin` / `admin` on first install when no passwords exist (changeable under Users).
- Video track option **All qualities** alongside Best and per-rendition ids.
- Install prints binary version, verifies `dev-*` when `DRMSONIC_TAG=dev`, seeds admin on fresh installs.
- `dev` release `install.sh` defaults `DRMSONIC_TAG` to `dev` (avoids accidental `latest` / v1.1.x installs).
- Playlist path `/{username}/{password}/…` (app1.conf / m3u / streams) for IPTV clients.
- **Open panel without login** toggle on Users (`NoAuth`).
- **All qualities** remuxes each video track into an ABR master (`v/<id>/stream.m3u8`).

### Changed
- SAVE USERS is a single `saveusers` action (all rows + NoAuth in one write); `users_only` path removed.
- Installer defaults to the `dev` release; `DRMSONIC_TAG=latest|vX.Y.Z` pins another build.
- Session cookie satisfies CSRF for panel API posts (SAVE USERS works while logged in).
- app1.conf / playlists emit `/{user}/{pass}/stream/...` URLs when the user has a password.
- Dev releases pin `target_commitish` to the push SHA so tag `dev` tracks `devloper`.
- Config page first-load race fixed (provider select before DataTable / serialized `updateStreams`).
- `updateStreams` keeps a queued refresh's first-render flag, so a background tick can no longer discard the initial row build and leave cells blank until a manual reload.
- All-qualities master only advertises variants that already have segments; remux starts are staggered.
- The media buffer keeps six fragments beyond the announced window, so a client that fetches the oldest listed segment still gets it.

### Fixed
- Empty `app1.conf` / playlist responses were a blank page when a provider had no streams; they now carry a `no streams configured` marker.
- Users and Recordings pages shipped a JavaScript syntax error: `html/template` inserts spaces around actions inside `<script>`, so `deleteuser{{$n}}.onclick` rendered as `deleteuser 0 .onclick` and killed the whole inline script. ADD USER, SAVE USERS, DELETE, RENAME and the recording player never bound.
- Users row actions now use event delegation (`data-user-action`), so no Go value is ever spliced into a JS identifier.
- Users password / providers / NoAuth save crashed when the providers select returned null.
- Open panel without login never persisted when an earlier per-row update failed.
- Panel assets were served without cache headers, so a stale `drmsonic.js` could survive an upgrade; static files now send `Cache-Control: no-cache`.
- A segment request for a sequence already rotated out of the buffer used to block for five seconds before answering 404, stalling clients that fell behind; it now 404s immediately, while the live edge still waits for the next fragment.
- DASH streams emitted audio-only fragments whenever the video source briefly had no new segment, which churned the playlist and froze video with audio still running; the pipeline now waits for the next video segment.

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
