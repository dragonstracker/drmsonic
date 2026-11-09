# drmsonic Roadmap

Status: planning only. No repository code changes until each phase is approved.
Basis: line-level read of `cmd/`, `internal/`, `ui/` at commit `36fc379`. Every finding names the function or file that proves it.

## 1. Vision

drmsonic is a single static binary that pulls DASH, HLS and fMP4 sources, decrypts ClearKey CENC, and republishes as HLS or UDP with no ffmpeg and no cgo.

Target: match the packaging, access-control and operations feature set of a hosted streaming service, on hardware Nono owns, with passthrough quality (no re-encode, no quality loss, near-zero CPU).

| Capability | Typical hosted service | drmsonic today | Target |
|------------|------------------------|----------------|--------|
| Full ABR ladder in one master playlist | yes | no, one rendition | yes (section 3) |
| Multiple audio languages and codecs | yes | one audio track | yes |
| Subtitles and captions | yes | ignored | WebVTT renditions |
| Pick best, worst, all, or explicit tracks | yes | `best` only in practice | selection grammar (3.1) |
| Parallel downloads, per-job quality | yes | no | job queue, CLI, panel (section 4) |
| Recording, DVR, timeshift | yes | stubs | disk-backed (4.6, 8) |
| CENC `cenc`/`cens` ClearKey | n/a | yes | yes |
| CENC `cbcs`, SAMPLE-AES | yes | rejected | supported (8) |
| Per-user access control and quotas | yes | stored, mostly unenforced | enforced (section 5) |
| Metrics, health, structured logs | yes | none | yes (section 9) |
| Output formats | HLS, DASH, SRT, RTMP | HLS (TS), UDP | adds CMAF HLS/DASH, SRT (8) |
| Transcoding | yes | out of scope | out of scope; optional `pipe` hand-off to an external encoder |

Differentiators to protect: one binary, zero runtime dependencies, passthrough fidelity, readable stdlib-only code.

## 2. Audit of the current code

### 2.1 Quality selection and output (Q)

| ID | Finding | Where |
|----|---------|-------|
| Q1 | Video `all` is treated as unset and resolves to the single highest-bandwidth track | `pipeline.chooseVideo` |
| Q2 | Audio accepts one id. The panel multi-select saves `id1,id2`, which matches nothing, so the pick silently becomes the first English track | `pipeline.chooseAudioTrack` |
| Q3 | Audio `all` picks the first English track, not the best bitrate | `pipeline.chooseAudioTrack` |
| Q4 | The master playlist advertises one variant; its audio codec comes from `AudioList[0]` whatever was selected | `web.handleMasterPlaylist` |
| Q5 | Output carries exactly one video PID and one audio PID | `mux.PIDVideo`, `mux.PIDAudio` |
| Q6 | HLS ingest always takes `BestVariant()`; the `Video` and `Audio` fields are ignored | `pipeline.runHLS` |
| Q7 | `EXT-X-MEDIA` is never parsed, so HLS sources with a separate audio group play silent | `manifest.ParseHLS` |
| Q8 | HLS track ids are variant URLs. Expiring query tokens change the id, so a saved selection stops matching | `pipeline.runHLS` variants |
| Q9 | `Subtitles`, `SubtitlesList` and `Manifest.Text` exist and are shown in the panel, but nothing consumes them | `config.Stream`, `manifest.Manifest` |
| Q10 | Server-side ABR only steps down (`lowerVideo`), never up, and drops buffered audio on a switch | `pipeline.runDASH` |
| Q11 | `applyTracks` writes the best track id into `st.Video` on first run, freezing "best" into a fixed id that dangles when the ladder changes | `web.applyTracks` |
| Q12 | A failed audio init is swallowed (`aSrc = nil`); the stream plays without audio and no status says so | `pipeline.runDASH` |
| Q13 | Audio fetch waits behind video fetch in one sequential loop, adding latency on every segment | `pipeline.runDASH` |
| Q14 | `EXT-X-DISCONTINUITY` is parsed and never acted on | `pipeline.runHLS` |

### 2.2 Engine and reliability (E)

| ID | Finding | Where |
|----|---------|-------|
| E1 | Settings persisted and rendered in the panel but read by no engine code: `ManifestScript`, `SessionManifest`, `TimeRange`/`RangeStartTime`/`RangeEndTime`, `Start`/`End`, `RecordEvent`, `SpeedUp`, `MultiTS`, `PlaybackDelay`, `ContinuousPlayback`, `MaxConcurrentStreams`, `StallDetectTimeout`, the three autostart/autorestart periods, `UseDashDelay`, `IgnoreStaticDash`, `XForwardedFor`, `AlwaysResetSession`, `HlsFragmentsDuration`, `NoWaitFullPlaylist`, `NoRestartOnTrackChange`, `RestartFinished`, `UseCdm`, `PR*`, `VmxUniqueId`, `Script`, `PipeOutputCmdFormated` | `config.types`, `web.runtime` |
| E2 | Actions that return `"ok"` or an empty list without doing anything: `addrec`, `deleterec`, `deletereplay`, `stoprec`, `getrecordings`, `getreplays`, `getevents`, `channelsupdate*`, `eventsrefresh*`, `refreshkeys`, `flushkeys` | `web.handleAction` |
| E3 | Data races: `applyTracks` mutates shared `*Stream` values obtained from `Store.Provider` outside the store lock; `Channel.StatusLine`, `LastError`, `Info`, `BwIn` are written by pipeline goroutines and read by handlers without synchronisation; `ch.Readers++` runs from concurrent handlers | `web.runtime`, `state.Channel`, `web.handleSegment` |
| E4 | `Readers` counts segment requests and resets only on stop; `BwOut` is never written and `TotalBwOut` reports ingress | `web.acGetMonitoring`, `state` |
| E5 | PCR is written only on keyframe packets. UDP and IPTV receivers expect a PCR at least every 100 ms | `mux.writePES` |
| E6 | The ADTS header hard-codes the AAC-LC profile; HE-AAC is mislabeled. AC-3 and E-AC-3 sources are unsupported | `remux.ADTS`, `mux` stream types |
| E7 | `cbcs` returns an error; SAMPLE-AES is unsupported | `pipeline.openStream` |
| E8 | Unbounded reads: `fetch.Client.Get` uses `io.ReadAll` with no cap; `/api/action` decodes the body with no `MaxBytesReader` | `fetch`, `web.handleAction` |
| E9 | The HTTP server sets `ReadHeaderTimeout` only. No write or idle timeout, so a slow reader pins a goroutine and a segment buffer | `web.New` |
| E10 | No `Cache-Control`, `ETag`, CORS or compression on playlists and segments | `web.handle*` |
| E11 | `mediaStore.touch` takes the write lock on every request; `awaitSegments` polls every 200 ms; `find` scans linearly; `snapshot` copies the window per playlist hit | `web.media`, `web.pages` |
| E12 | RAM-only segment store with no global cap. Memory grows with streams x window x bitrate | `web.media` |
| E13 | Restart delay is fixed (`RestartDelay`, 5 s) with no backoff, jitter or circuit breaker | `web.supervise` |
| E14 | Per-PES allocations in the muxer and a prepend-based segment name builder add GC pressure | `mux.writePES`, `web.pad` |
| E15 | Test coverage: `cenc`, `manifest`, `mux`, `remux` only. None for `web`, `pipeline`, `fetch`, `config`, `state`, `mp4`, `out`. CI runs `go vet` and `go test` without `-race` or `gofmt` | `.github/workflows/build.yml` |

### 2.3 Security (S)

| ID | Finding |
|----|---------|
| S1 | CSRF on `/api/action`: browsers resend Basic credentials automatically, and the handler accepts any content type, so a cross-site `text/plain` POST can reach it |
| S2 | No SSRF guard. A manifest URL may point at loopback, RFC 1918 or `169.254.169.254` |
| S3 | Keys and credentials sit in provider JSON and the users file in plaintext with default file modes |
| S4 | No throttling on failed logins |
| S5 | No audit trail of admin actions |

## 3. Multi-quality output

The headline feature: choose best, all, or any subset of video and audio qualities, and publish them all from one stream.

### 3.1 Selection grammar

The existing `Video`, `Audio` and `Subtitles` strings stay the storage format, so saved provider files keep working. Each is a comma-separated token list; the result is the union of all tokens.

| Token | Video | Audio |
|-------|-------|-------|
| empty or `best` | highest bandwidth | highest bandwidth |
| `worst` | lowest bandwidth | lowest bandwidth |
| `all` | every track | every track |
| `none` | not valid | no audio |
| `<id>` | that track | that track |
| `top:N` | N highest | N highest |
| `h<=1080`, `h>=720` | height bound | not valid |
| `bw<=4000k`, `bw>=1m` | bandwidth bound | bandwidth bound |
| `codec=avc1`, `codec=hvc1`, `codec=mp4a` | codec prefix | codec prefix |
| `lang=en,es` | not valid | language filter |
| `default` | not valid | the manifest's default audio |

Rules:

1. Unknown ids are dropped with one logged warning and a status note, never a silent fallback to a different track (fixes Q2, Q12).
2. An empty result after filtering falls back to the nearest lower rendition, with a status note.
3. `best` and `worst` are evaluated on every manifest refresh, not frozen into an id (fixes Q11).
4. The result is ordered by bandwidth descending and de-duplicated.

Implementation: `internal/selector`, a pure package taking `[]manifest.Track` plus the three strings and returning a `Plan`. Table-tested.

### 3.2 Track model

Both DASH and HLS parse into one `Track` shape: `Kind` (video, audio, text), `ID`, `Codecs`, `Width`, `Height`, `FrameRate`, `Bandwidth`, `Lang`, `Channels`, `Default`, `Kid`.

Stable ids:

- DASH: the `Representation@id`.
- HLS: `v<height>p<bandwidth>` for variants and `a<group>-<lang>-<name hash>` for audio. URLs never act as ids (fixes Q8).

### 3.3 Output layout

Existing URLs keep working: `master.m3u8`, `stream.m3u8` and `streamNNNNN.ts` under `/stream/{provider}/{stream}/`. `stream.m3u8` always serves the primary rendition, so single-quality players and `app1.conf` consumers see no change.

New relative paths under the same prefix, which inherit credential prefixes (section 5):

```
master.m3u8
v/{videoId}/stream.m3u8
v/{videoId}/streamNNNNN.ts
a/{audioId}/stream.m3u8
a/{audioId}/streamNNNNN.ts
s/{subId}/stream.m3u8
s/{subId}/subNNNNN.vtt
```

Routing: one pattern `GET /stream/{provider}/{stream}/{rest...}` that dispatches on the first segment of `rest`.

### 3.4 Master playlist

Layout `muxed` (default): every video variant carries the default audio inside its TS. Extra audio tracks appear as `EXT-X-MEDIA` renditions. The default audio entry has no `URI`, which HLS defines as "carried in the variant stream".

```
#EXTM3U
#EXT-X-VERSION:6
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="English",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Spanish",LANGUAGE="es",DEFAULT=NO,AUTOSELECT=YES,URI="a/a2/stream.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="sub",NAME="English",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=YES,URI="s/t1/stream.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=6200000,AVERAGE-BANDWIDTH=5600000,RESOLUTION=1920x1080,FRAME-RATE=25.000,CODECS="avc1.640028,mp4a.40.2",AUDIO="aud",SUBTITLES="sub"
v/1080p/stream.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=3600000,RESOLUTION=1280x720,FRAME-RATE=25.000,CODECS="avc1.64001f,mp4a.40.2",AUDIO="aud",SUBTITLES="sub"
v/720p/stream.m3u8
```

Layout `demuxed` (per-stream option): video variants carry video only; every audio track, including the default, is a separate rendition. This is the cleanest model for players with strong ABR logic and the only one that scales to many audio tracks without duplicating audio in every rung.

Values come from the parsed tracks: real `BANDWIDTH`, `RESOLUTION`, `FRAME-RATE` and `CODECS`. Placeholders such as the current `8133000` and `1920x1080` fallbacks are removed (fixes Q4).

Muxed-audio compatibility note: players that cannot switch audio inside a muxed variant simply play the default track. Players that can, use the `URI` renditions.

### 3.5 Pipeline architecture

Today one loop fetches, decrypts, muxes and publishes a single pair of tracks.

```
manifest watcher (1 per stream)
        │  Manifest{video[], audio[], text[]}
        ▼
selector ──► Plan{ renditions[], audioRenditions[], subtitleRenditions[] }
        │
        ▼
trackReader (1 goroutine per selected track)
   poll → fetch → decrypt → samples
   emits Chunk{track, segmentNumber, startPTS, duration, samples}
        │
        ▼
packager (1 per rendition)
   aligns video chunk n with audio window n → mux → Publish(renditionID, seq, dur, bytes)
        │
        ▼
mediaStore[provider/stream/renditionID] → ring buffer, HTTP handlers
```

Design rules:

1. Fetch each source track once. Two renditions that share an audio track share the reader and the decrypted samples.
2. Segment boundaries follow the source video segments. Every rendition uses the source segment number as `MEDIA-SEQUENCE`, so sequence numbers and boundaries match across the ladder. That alignment is what lets a player switch rungs without a glitch.
3. Audio chunks are cut to the video window with the `pending` logic that exists in `runDASH`, generalised to N audio tracks.
4. A failing rendition does not stop the stream. It is marked degraded, retried with backoff, and removed from the master after repeated failures. The stream fails only when no video and no audio rendition remains (fixes Q12).
5. Video and audio fetches run in parallel per segment window (fixes Q13).
6. Source-side ABR (`lowerVideo`) applies only to single-rendition mode. It gains a step-up path with hysteresis. In ladder mode switching belongs to the player.

### 3.6 Lazy renditions

Pulling every rung continuously multiplies ingress. Example ladder: 6.0 + 3.6 + 1.8 + 0.9 Mbps video plus three 128 kbps audio is about 12.7 Mbps in, against 6.1 Mbps for `best`.

- `Eager` rendition (default for the top rung and the default audio): always fetched while the stream runs.
- `OnRequest` rendition (default for the remaining rungs when `OnDemand` is set): its reader starts on the first playlist or segment request, waits for two segments, and stops after the idle limit already used by `sweepOnDemand`.
- The master playlist lists every selected rendition in both cases.
- Per-stream flag `EagerAll` forces every rendition eager.

### 3.7 Muxer changes

- `Muxer` takes a track list instead of two booleans: one video, N audio.
- Audio PIDs start at `0x0101` and increment. Each gets a PMT ISO 639 language descriptor.
- Stream types: AAC `0x0F`, AC-3 `0x81`, E-AC-3 `0x87`, H.264 `0x1B`, H.265 `0x24`.
- PCR on the video PID at most every 40 ms, independent of keyframes (fixes E5).
- The ADTS profile comes from the AudioSpecificConfig, not a constant (fixes E6).
- Audio frames are grouped per PES (about 2 KB) to cut header overhead.
- Reusable buffers replace per-PES `make` calls (fixes E14).

For UDP and IPTV outputs, a stream option `MultiTS` (an existing, unused field) selects one TS carrying all selected audio PIDs, because IPTV receivers expect that shape instead of HLS renditions.

### 3.8 HLS source ingest

- Parse `EXT-X-MEDIA` for AUDIO and SUBTITLES groups, and `AUDIO`/`SUBTITLES` on `EXT-X-STREAM-INF` (fixes Q7).
- Run the same selector, then run one `trackReader` per selected variant and audio rendition (fixes Q6).
- Act on `EXT-X-DISCONTINUITY`: reset timestamp continuity and emit a discontinuity marker downstream (fixes Q14).
- Support `EXT-X-BYTERANGE` and `EXT-X-MAP` per segment.

### 3.9 Subtitles

- DASH: `Manifest.Text` tracks with WebVTT segments pass through; `stpp`/TTML in fMP4 converts to WebVTT.
- HLS: `SUBTITLES` groups pass through as WebVTT.
- Output: `EXT-X-MEDIA TYPE=SUBTITLES` renditions with `subNNNNN.vtt` segments aligned to the video windows, with an `X-TIMESTAMP-MAP` header.
- CEA-608/708 captions inside the video: pass through untouched in H.264 SEI (already preserved by the Annex B path).

### 3.10 Memory and bandwidth budget

| Item | Figure | Source |
|------|--------|--------|
| Window per rendition | `NbAnnouncedFragments` (default 6) | `windowFor` |
| RAM per rendition | window x segment seconds x bitrate / 8 | 6 x 4 s x 6 Mbps = 18 MB |
| RAM, 4-rung ladder plus audio | about 36 MB per stream | sum of the example ladder |
| 100 streams, all qualities | about 3.6 GB | linear |

Controls:

1. `-max-ram` global cap on segment bytes, with eviction of the oldest segments of idle renditions first.
2. Per-stream `MaxRenditions` guard, default 8.
3. Panel shows projected ingress and RAM before saving a selection of `all`.
4. Optional disk spill for DVR windows (section 4.6).

### 3.11 Panel

- Quality picker becomes a table of parsed tracks with checkboxes: resolution, codec, bitrate, frame rate, language, key status (the existing KID hint stays).
- Presets above the table: Best, Worst, All, Top 3, Custom grammar field (3.1).
- Live summary: renditions count, projected ingress Mbps, projected RAM.
- Per-rendition status row: state, last segment age, bytes in, errors.
- Copy buttons for master URL and per-rendition URLs; built-in hls.js preview with a rendition selector (hls.js already ships in `ui/static`).
- A `probe` action (4.2) fills the table before the stream ever starts.

### 3.12 Tests

1. `selector`: table tests for every grammar token, conflicts, empty result fallback, dangling ids.
2. Fixture MPDs and master playlists: multi-language, multi-codec, separate audio groups, expiring-token URLs.
3. Alignment test: three renditions of one fixture emit identical sequence numbers and durations within one frame.
4. Master playlist golden files for `muxed` and `demuxed`.
5. Failure injection: 404 on one rung keeps the stream up and removes that rung after the retry budget.
6. `ffprobe` conformance in CI: every rendition parses, reports expected streams and monotonic timestamps.

## 4. Downloads and recording

### 4.1 Job model

A download job is a bounded run of the same engine used for live:

```
Job{ id, source{url | provider/stream}, selection{video, audio, subtitles},
     container, name, state, progress{segments, bytes, seconds}, error, created, finished }
```

States: `queued`, `running`, `paused`, `done`, `failed`, `cancelled`.

### 4.2 Probe

`probe` takes a manifest URL (plus optional headers, proxy, keys) and returns the track tables without starting anything. It feeds the panel quality picker and the CLI.

### 4.3 Multiple videos, different qualities

- `adddownload` accepts `Items[]`. Each item carries its own source and its own selection, so one request can queue: movie A at 1080p with English and Spanish audio, movie B at 720p with all audio, stream C at all video qualities.
- "All qualities" expands to one job per selected video track, named by height and codec, or to one multi-track file when the container supports it.
- Output name template: `{provider}/{stream}/{name}.{height}p.{vcodec}.{ext}`.

### 4.4 Containers

| Container | Content | Phase |
|-----------|---------|-------|
| `ts` | muxer from 3.7, multi-audio PIDs | 3 |
| `mp4` (fragmented) | decrypted samples rewritten into fMP4: sample entries restored from `encv`/`enca`, `senc`/`saiz`/`saio` dropped | 3 |
| `mkv` | multi-audio and multi-subtitle in one file | 5 |
| sidecars `.vtt` | subtitles as separate files | 3 |

### 4.5 Engine behaviour

- Worker pool: `-dl-workers` (default 2) jobs at once.
- Segments inside a job download with `-dl-conns` (default 4) parallel fetches and are written in order.
- Per-host connection cap, honouring provider `Bind`, `Proxy`, `Doh` and `Headers`.
- Output goes to `name.part` plus a small state file `{nextSegment, bytes}`; rename on success. A restart resumes from the stored segment.
- Free-space check before start and during the run; the job fails cleanly under a configured floor.
- Retry with exponential backoff and jitter; a missing key fails the job at once with a clear error.

### 4.6 Recording and DVR

The stubbed recording actions (E2) become thin wrappers over jobs:

- Recording: a `live` job on a running stream or manifest, with start and end time (the unused `TimeRange` fields), writing segments to disk.
- Event recording: `RecordEvent` starts a job when the event begins.
- DVR and timeshift: a disk-backed window per rendition, replacing the RAM-only ring when enabled, with a playlist that exposes `EXT-X-PROGRAM-DATE-TIME` and a seekable range.
- Replays: finished recordings served through `/stream/.../replay/{id}/master.m3u8`.

### 4.7 CLI

```
drmsonic probe <url> [-H "Name: value"] [-proxy ...]
drmsonic dl <url> -v best -a all -s all -o dir [-k kid:key] [-c mp4]
drmsonic dl -f list.txt
```

The CLI shares the pipeline package and runs with no server.

### 4.8 Tests

Fixture-based end-to-end runs that download a local HTTP-served DASH and HLS fixture with several selections, compare track counts through `ffprobe`, and kill and resume a job mid-run.

## 5. Authentication and access control

### 5.1 Confirmed defects

| ID | Defect | Effect |
|----|--------|--------|
| A1 | The Users page renders `value="hidden"` in the password field and the update action stores it | Editing any user field overwrites the real password with `hidden` |
| A2 | Auth is enforced only when static `-user` and `-password` flags are set | Users created in the panel are never checked |
| A3 | Playlist links use `?user=name` with no password | The link grants nothing, or everything when auth is off |
| A4 | `?user=` is not carried into `stream.m3u8` and segment URLs | Identity is lost after the master playlist |
| A5 | A user with an empty password can match an empty credential | Login bypass |
| A6 | `IsAdmin` and `HasWebAccess` are stored and never enforced | Every logged-in user acts as admin |
| A7 | `ProviderIds` is stored and never enforced | A user sees every provider |
| A8 | Credential comparison is not constant-time | Timing leak |
| A9 | The users file sits outside any Docker volume | Users vanish on redeploy |
| A10 | `-user` and `-password` can be passed separately | Silent open server |
| A11 | `User.Network` and `NetworkList()` exist and nothing calls them | Per-user IP limits do nothing |
| A12 | `remoteIP` reads `RemoteAddr` only, so behind a reverse proxy every client shares the proxy address | An `-allow` entry for the proxy grants anonymous admin to everyone |

### 5.2 Design: credentials in the path, no tokens

With auth on, every player URL carries the credentials as the first two path segments:

```
hls://127.0.0.1:7779/username/password/stream/channels/channel1/master.m3u8
```

With no password anywhere, URLs stay as they are today:

```
hls://127.0.0.1:7779/stream/channels/channel1/master.m3u8
```

`app1.conf` with credentials:

```
stream channels_channel1 {
	url hls://127.0.0.1:7779/username/password/stream/channels/channel1/master.m3u8;
	title "[channels] channel1";
}
```

`playlist.m3u`, `live.m3u`, `event.m3u`, `replay.m3u` follow the same prefix rule.

### 5.3 Routing

| Pattern | Segments | Meaning |
|---------|----------|---------|
| `/stream/{p}/{s}/{rest...}` | 4 or more, first is `stream` | Anonymous form |
| `/{user}/{pass}/stream/{p}/{s}/{rest...}` | 6 or more, third is `stream` | Credentialed stream |
| `/{user}/{pass}/{playlist,live,event,replay}.m3u`, `/{user}/{pass}/app1.conf` | 3 | Credentialed playlist |

- Split on `URL.EscapedPath()`, then `url.PathUnescape` each credential, so `/` and `%` inside a password survive.
- Generate the prefix with `url.PathEscape`.
- Strip the prefix and hand the inner path to the existing handlers. Relative URLs in HLS playlists inherit the prefix, which also covers every new rendition path in 3.3 (fixes A4).
- Basic auth stays valid for the panel and API. Path credentials win when both are present.

### 5.4 Rules

- Auth is required when static `-user` and `-password` are both set, or any stored user has a non-empty password.
- A user with an empty password cannot authenticate (A5). A store holding only passwordless users keeps the panel open, so an unfinished setup never locks Nono out.
- Compare user and password with `crypto/subtle.ConstantTimeCompare` (A8).
- Static `-user`/`-password` is always admin and is the recovery path.
- Exactly one of the two flags is a startup error (A10).
- `Network` restricts a user to listed addresses and CIDRs (A11).
- `-trusted-proxies` lists the proxies whose `X-Forwarded-For` is honoured. Without it, `RemoteAddr` is the only source (A12). `-allow` matches the resolved client address.

### 5.5 Roles and quotas

| Capability | Player | Panel user | Admin |
|------------|--------|------------|-------|
| Playlists and streams | granted providers | granted providers | all |
| Panel pages | no | yes | yes |
| Viewer actions: status, config read, monitoring, recordings, replays, events, start, stop, restart, probe | no | yes | yes |
| `/users`, `/config`, `/debug`, write actions, downloads | no | no | yes |

- Provider scope: empty `ProviderIds` means no providers for non-admins. Out-of-scope providers disappear from playlists and answer 404 on stream routes.
- Per-user quota fields, all optional: `MaxConnections` (concurrent segment fetches per user), `MaxStreams`, `AllowedRenditions` (cap on height or bandwidth), `Expires` (date). Enforced in the auth wrapper and the stream handlers.
- Quotas give the hosted-service plan model (tiered subscriber access) without tokens or sessions.

### 5.6 Users page

- Password input starts empty with the placeholder `new password`.
- The update action ignores an empty password and keeps the stored one (A1).
- Playlist links render as `/{user}/{pass}/playlist.m3u` and friends, visible to admins for users that have a password.

### 5.7 Hardening

- CSRF: `/api/action` requires `Content-Type: application/json` and a same-origin `Origin` or `Referer` (S1).
- SSRF: block loopback, link-local, metadata and RFC 1918 targets for manifest and segment fetches by default, with `-allow-private-fetch` to opt in (S2).
- Secrets: write provider and users files with mode `0600` (S3).
- Login throttling: per-address exponential delay after failed attempts (S4).
- Audit log: admin actions with user, address and action name (S5).
- Request logging redacts the first two segments of credentialed paths.
- Credentials appear in URLs and therefore in player logs and referrers. Serve over TLS through a reverse proxy outside a trusted network. The README says so.

### 5.8 Persistence

- Dockerfile gets `/app/data`, declares it a volume, and starts with `-c data/drmsonic.json`.
- `drmsonic.json` must not live inside the providers directory, because every `*.json` there loads as a provider.
- `docker-compose.yml` mounts `./data:/app/data`.

### 5.9 Tests

`internal/web/auth_test.go`:

1. Open server without credentials serves plain URLs.
2. Static credentials: none, wrong, basic, path.
3. Empty-password user cannot log in.
4. A passwordless user does not turn auth on.
5. Path credentials reach stream, rendition and segment routes; unknown and ungranted providers give 404.
6. Playlists embed credentials and omit ungranted providers.
7. `app1.conf` matches the format in 5.2 byte for byte.
8. Credentials containing `/`, `@`, space and `%`.
9. Panel and `/users` access per role.
10. Viewer versus admin API actions.
11. Update with an empty password keeps the old one.
12. `Network` allow and deny; `X-Forwarded-For` honoured only from trusted proxies.
13. Quota enforcement: connection cap, rendition cap, expiry.
14. Cross-origin `text/plain` POST to `/api/action` is rejected.

Run with `go vet ./...` and `go test -race ./...` in `golang:1.22`. Go is not on the host PATH.

## 6. Naming

| Current | New |
|---------|-----|
| `ui/static/o11.js` | `ui/static/drmsonic.js` |
| `ui/static/o11.ico` | `ui/static/drmsonic.ico` |
| `ui/static/o11.css` | merged into `ui/static/drmsonic.css`, original removed |
| three references in `ui/templates/headers.html` | point at the new files |
| two mentions in `README.md` | reworded |

`research/o11/` is source material and stays untouched.

## 7. Engine hardening

| Work | Fixes |
|------|-------|
| Replace shared mutable `Channel` fields with a mutex-guarded struct or atomics and hand handlers immutable snapshots; copy-on-write `Stream` updates under the store lock | E3 |
| Count readers by active connections with a decrement on close; write real `BwOut` per rendition | E4 |
| `http.Server` gets `WriteTimeout` sized to the segment length, `IdleTimeout`, and `MaxHeaderBytes`; segment writes use a deadline | E9 |
| `MaxBytesReader` on `/api/action`; response size cap on `fetch.Client.Get` | E8 |
| `Cache-Control: no-cache` on live playlists, `public, max-age` plus `ETag` on segments, CORS with a configurable origin, gzip for playlists | E10 |
| Per-stream atomic last-access timestamp instead of a map under a write lock; a per-stream notify channel instead of 200 ms polling; map by segment name | E11 |
| Global `-max-ram` cap with eviction; optional disk spill | E12 |
| Exponential restart backoff with jitter and a circuit breaker that parks a stream after repeated failures, visible in the panel | E13 |
| Implement or remove every setting in E1: each field gets a reader and a test, or leaves the panel and the JSON schema | E1 |
| Replace stubbed actions with implementations from section 4, or return `501` | E2 |
| Surface warnings (missing audio, dropped rendition, dangling selection) as status notes | Q12 |

Settings in E1 that map onto planned work: `TimeRange`, `RangeStartTime`, `RangeEndTime`, `Start`, `End`, `RecordEvent` (section 4.6); `MultiTS` (3.7); `MaxConcurrentStreams` (global and per-provider limit in the supervisor); `StallDetectTimeout` (restart when no segment arrives for N seconds); `PlaybackDelay` and `UseDashDelay` (live-edge offset); `NoRestartOnTrackChange` (hold the pipeline through manifest ladder changes); `XForwardedFor` (header on upstream fetches). `UseCdm`, `PR*`, `VmxUniqueId` and `PipeOutputCmdFormated` wait for section 8 decisions.

## 8. Feature gaps

| Feature | Value | Notes |
|---------|-------|-------|
| `cbcs` pattern decryption (AES-CBC, 1:9) | Covers most current commercial streams | Stdlib `crypto/cipher`; constant-IV and per-sample-IV variants |
| SAMPLE-AES for HLS | Covers FairPlay-style HLS with a known key | Same cipher core as `cbcs` |
| AC-3 and E-AC-3 passthrough | Premium audio | Muxer stream types in 3.7 |
| CMAF output (fMP4 HLS and DASH) | One segment set serves both protocols; clean multi-audio, subtitles, HEVC, AV1 | Rewrites moof/sample entries; TS stays for UDP and legacy players |
| LL-HLS | Low-latency live | Needs CMAF parts and blocking playlist reload |
| SRT output and pull | Contribution and distribution links | UDP sender already exists as the template |
| Pipe output | Hand-off to an external encoder for transcoding | Finishes `OutPipe`; keeps the core transcode-free |
| Playlist outputs by tag and by provider, XMLTV EPG | Player apps, IPTV | `epg.html` and events stubs exist |
| Webhooks | Stream start, stop, error, recording done | Small signed POST, HMAC secret |
| Remote CDM hook for Widevine and PlayReady | Key acquisition for licensed content | Defined as an external HTTP service; core stays CDM-free. Decide scope before building |
| Thumbnails and preview sprites | Panel monitoring | Keyframe extraction without decode where possible |
| Multi-instance coordination | Horizontal scale | Shared config plus consistent assignment of streams to nodes; later phase |

## 9. Operations and observability

| ID | Work |
|----|------|
| D1 | `/healthz` (process up) and `/readyz` (config loaded, running streams healthy) |
| D2 | `/metrics`, Prometheus text format, stdlib only: per-stream and per-rendition ingress bytes, segment age, restarts, errors, viewers, RAM, goroutines |
| D3 | Structured JSON logs with levels, a stream id field and redaction of credentialed paths |
| D4 | CI: `gofmt -l`, `go vet`, `go test -race`, fuzz smoke, fixture end-to-end, build matrix |
| D5 | Graceful shutdown with a drain timeout; stop pipelines through context cancellation only |
| D6 | Release binaries for linux/amd64, linux/arm64, windows/amd64 (exists), plus a checksum file and SBOM |
| D7 | Config validation on load with line-level errors; atomic writes with a backup of the previous file |
| D8 | Soak test: 50 streams x 4 renditions for 24 hours, flat memory and no goroutine growth |
| D9 | Fuzz targets: MPD, M3U8, `mp4.ParseInit`, `mp4.ParseFragment`, selector grammar |

## 10. Performance targets

| Metric | Target |
|--------|--------|
| CPU, one 6 Mbps stream, remux only | under 2 percent of one core |
| Added latency from the pipeline | under one source segment |
| Time to first segment, on-demand start | under 3 s after the first request |
| RAM per rendition | window x segment x bitrate, under the `-max-ram` cap |
| Playlist response | under 1 ms at 1000 requests per second |
| Allocation rate in steady state | no per-sample heap allocations on the mux path |

Measured with `go test -bench`, `pprof` and the soak test in D8. Optimisations that change code structure wait for a benchmark that shows the cost.

## 11. Code standards

- Standard library only.
- Delete before adding: no unused flags, helpers or fallbacks.
- Comments explain a reason, never restate the line.
- `gofmt`, `go vet`, `-race` clean.
- One change per commit, imperative subject.
- Public behaviour changes update the README in the same commit.

## 12. Phases

| Phase | Scope | Exit check |
|-------|-------|------------|
| 1 | Auth and naming: sections 5.2 to 5.8 core, section 6, Docker data volume, CI gains `-race` and `gofmt` | Tests in 5.9 pass; playlists correct with and without credentials |
| 2 | Selector and model: 3.1, 3.2, stable ids, honest master playlist for single rendition, status warnings, E3 race fixes | `selector` tests green; `go test -race ./...` clean; `Video`/`Audio` lists with several ids no longer fall back silently |
| 3 | Ladder and multi-audio output: 3.3 to 3.8, muxer changes, lazy renditions, HLS ingest rewrite | Golden master files match; alignment test passes; `ffprobe` accepts every rendition; hls.js switches rungs |
| 4 | Panel quality picker and probe: 3.11, 4.2 | Probe fills the table before start; projected ingress shown |
| 5 | Downloads and recording: section 4 | Two parallel jobs with different qualities finish and play; kill and resume works |
| 6 | Subtitles and `cbcs`/SAMPLE-AES: 3.9, section 8 first three rows | WebVTT renditions play; a `cbcs` fixture decrypts bit-exact |
| 7 | Hardening, quotas and operations: 5.5, 5.7, sections 7 and 9 | Soak test flat; metrics scraped; CSRF and SSRF tests green |
| 8 | CMAF output, LL-HLS, SRT, webhooks, DVR on disk | Same segments serve HLS and DASH; LL-HLS latency under 3 s |

Rough size: phases 1 and 2 are days each; phase 3 is the largest at one to two weeks; phases 4 to 7 are about a week each; phase 8 is open-ended and ordered by demand.

## 13. Decisions and open questions

| Question | Decision |
|----------|----------|
| Tokens or sessions | No. Credentials in the path |
| Default layout for ladders | `muxed` for compatibility with IPTV apps; `demuxed` as an option |
| `all` quality default for lazy fetch | `OnRequest` for non-top rungs when `OnDemand` is set, `Eager` otherwise |
| Empty `ProviderIds` | No providers for non-admins |
| Config location in Docker | `/app/data/drmsonic.json` |
| Transcoding | Out of scope; optional pipe hand-off |
| Hash stored passwords | Open. Hashing blocks the ready-made playlist links in 5.6; the choice is hashed storage with no link display, or plaintext storage with links |
| CDM support | Open. Decide between an external CDM service hook and staying ClearKey-only |
| CMAF as the primary segment format | Open. Phase 8 adds it beside TS; promote it to default once HLS-TS consumers are confirmed to be gone |
