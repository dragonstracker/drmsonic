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
