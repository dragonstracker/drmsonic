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
- Panel status showed raw Go errors (`Get "https://…": dial tcp …`). Statuses now report short o11-style text (`Access denied (403)`, `Connection timeout`, `TLS handshake failed`, `Not found (404)`) with the full detail on the second line behind **show more**.
- Finished presentations (HLS `EXT-X-ENDLIST`, exhausted DASH MPD, 404/410 manifest) looped forever as `Running`. They now end with `Broadcast ended`, stop the pipeline, and follow **Restart finished broadcast** when it is enabled.
- HTTP 4xx manifest errors were retried as if they were transient.
- Selector comma filters (`h<=720,codec=avc1`) AND together instead of OR.
- SAVE USERS (`users_only`) and SAVE ALL PROVIDERS persistence.
- User rename ignored `NewUserName` from the panel.
- New users default to Web Access on.
- Config / Channels built the full editor only for the page that was open on first load, so any channel reached by paging, **Show All**, or search showed an empty row. Every channel now builds its editor when it is drawn.
- Config / Channels column widths summed past 100% and pushed the options column out of the window.
- Panel assets were served uncompressed over ten requests, so a remote panel spent seconds loading before the first row appeared; JS and CSS are now gzipped, content-hashed with `ETag` revalidation, and delivered as three bundles (`app.js`, `app.css`, `tz.js`).
- Panel HTML and `/api/action` JSON were uncompressed; both are gzipped now.
- `hls.min.js` blocked every page even though only the player needs it; it loads on first play.
- Config / Channels rows grew from 55px to 170px once their editors arrived, shifting the page while it filled; the row height is reserved up front.
- The first channel build waited for the status poll before requesting config; the config request now starts in parallel.
- Bundling the CSS turned every panel page white: `drmsonic.css` begins with a UTF-8 BOM, and inside a bundle that byte swallowed the `body { background: black }` rule. Leading BOMs are stripped when assets are read, and the file itself no longer carries one.
- Streams and Events ignored the provider in the URL: every page rendered one table per configured provider, so `/status/example` listed all 45 streams of both providers under a 45-stream total. Only the selected provider renders now; **All Running** still spans every provider, and the provider picker keeps listing all of them.
- Filter changes threw on pages where no provider was preselected, because the filter and delete handlers read a provider from an unselected dropdown; they now fall back to the provider in the URL.
- `handleSegment` counted a reader per request and never released it; the counter is only ever read by the monitoring page, so it no longer drifts.
- Media handlers blocked the player: an unknown segment name waited 5s before 404, a cold playlist waited 10s for two segments, and a cold master waited 8s. Segments now answer immediately unless the request is exactly the next sequence, playlists return whatever is buffered, and the cold-start wait is one playlist length.
- Playlists and segments sent no `Access-Control-Allow-Origin`, so browser players refused them. Media responses now allow any origin, and playlists are `no-store` so no proxy serves a stale window.
- The segment buffer kept `window+6` fragments and advertised six by default; it keeps `window+8` and advertises seven, matching the reference layout.
- DASH live playlists advanced in bursts of two or three every five seconds, because the manifest was only re-read once per `minimumUpdatePeriod` (`PT5S` on most CDNs). The refresh interval is now capped by the segment duration, so a 2s channel yields one segment every 2s.
- On-demand streams were stopped after a 20s pause, dropping the buffer while the viewer was still attached. The idle window is now at least 60s.

### Added
- Default `admin` / `admin` on first install when no passwords exist (changeable under Users).
- Video track option **All qualities** alongside Best and per-rendition ids.
- Install prints binary version, verifies `dev-*` when `DRMSONIC_TAG=dev`, seeds admin on fresh installs.
- `dev` release `install.sh` defaults `DRMSONIC_TAG` to `dev` (avoids accidental `latest` / v1.1.x installs).
- Playlist path `/{username}/{password}/…` (app1.conf / m3u / streams) for IPTV clients.
- **Open panel without login** toggle on Users (`NoAuth`).
- Per-user **Playlist** toggle (`NoPlaylist`): a row can keep playlist access without panel login, or lose playlists while keeping Web UI access.
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
- A user without **Web Access** could still get a panel session from `/login` and then hit raw `forbidden` on every page. Login is refused with *This account is for playlist access only*, while their `/{user}/{pass}/…` playlists keep working.
- Playlist and stream URLs answered unauthenticated clients with the HTML login page; they now return `401`, so players stop instead of chasing a redirect.
- A session whose user lost **Web Access** stayed on `forbidden`; the panel now sends it through `/logout`.
- SAVE USERS applied **Open panel without login** before validating the rows, so a rejected save could flip the toggle in memory.
- Panel assets were served without cache headers, so a stale `drmsonic.js` could survive an upgrade; static files now send `Cache-Control: no-cache`.
- A segment request for a sequence already rotated out of the buffer used to block for five seconds before answering 404, stalling clients that fell behind; it now 404s immediately, while the live edge still waits for the next fragment.
- DASH streams emitted audio-only fragments whenever the video source briefly had no new segment, which churned the playlist and froze video with audio still running; the pipeline now waits for the next video segment.
- Provider import rejected every file with `invalid import body`: the panel posts the file text as `ProviderImportConfig`, but only a `Body` object was read. Import now accepts the panel payload, an exported provider object, and o11-style exports (`Channels`, `ManifestUrl`, `Headers` as manifest/media maps, `XForwardedFor` as a string), and the typed provider name wins when one is given.
- Provider import was capped at 1 MB, so large channel lists were refused; panel API bodies allow 16 MB.

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
