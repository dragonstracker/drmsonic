# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow the release tags on GitHub.

## [Unreleased] — branch `devloper`

### Added
- Phase 1 roadmap work: path-based stream credentials, role checks, provider scope, trusted proxies, CSRF guard on `/api/action`, SSRF block for private fetch targets, Docker `/app/data` volume, CI `gofmt` + `go test -race`.
- This changelog.

### Changed
- Panel static assets renamed from `o11.*` to `drmsonic.*`.
- Users password field no longer stores the literal `hidden` placeholder.
- Playlist and `app1.conf` links for users with passwords use `/{user}/{pass}/...` path credentials.

### Fixed
- Auth ignored panel users unless `-user`/`-password` were set.
- `?user=` alone could grant access without a password.
- Empty-password users could authenticate.
- Admin vs panel-user capabilities were not enforced.

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
