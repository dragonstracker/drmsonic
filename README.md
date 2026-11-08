# drmsonic

Self-hosted OTT restreamer. It pulls DASH, HLS and fMP4 sources, decrypts CENC ClearKey
content, remuxes the elementary streams into MPEG-TS and republishes them as HLS from an
in-memory segment store.

Go 1.22, standard library only. No cgo, no ffmpeg, no external services.

```
source (DASH / HLS / fMP4)
        │
        ├─ manifest poll ── timeline, templates, playlists
        ├─ fragment fetch ─ subsamples, tfdt/trun, IVs
        ├─ CENC decrypt ─── AES-128-CTR
        ├─ remux ────────── Annex B (H.264/H.265) + ADTS (AAC)
        ├─ TS mux ───────── PAT, PMT, PES, PCR, 188-byte packets
        └─ publish ──────── in-memory HLS window  →  /stream/{provider}/{stream}/master.m3u8
```

## Features

| area | support |
|------|---------|
| DASH | dynamic and static MPD, `SegmentTemplate` with `$Number$` / `$Time$`, `SegmentTimeline`, `SegmentBase` init, per-period base URLs |
| HLS | master and media playlists, `EXT-X-MAP` fMP4, plain TS passthrough, `EXT-X-KEY` AES-128-CBC |
| DRM | CENC `cenc`/`cens` sample encryption, AES-128-CTR with subsample maps, ClearKey `kid:key` |
| Codecs | H.264, H.265 (VPS/SPS/PPS injection), AAC-LC (ADTS) |
| Output | `directhls` (HTTP HLS from RAM), `udppush` (MPEG-TS over UDP) |
| Panel | providers, stream editor, live status, mass update, users, playlists, per-stream start/stop |

Not supported: `cbcs` (pattern encryption), SAMPLE-AES, Widevine/PlayReady license acquisition,
SRT, pipe output, recording. See [Roadmap](#roadmap).

## Requirements

- Go 1.22 or newer to build
- Any OS for the panel; Linux amd64/arm64 and Windows amd64 binaries are produced by `build.sh`
- Outbound HTTPS to the origin, and UDP if you use `udppush`

## Build

```sh
sh build.sh
```

Produces:

```
bin/drmsonic-linux-amd64
bin/drmsonic-linux-arm64
bin/drmsonic-windows-amd64.exe
```

Local build for the current platform:

```sh
go build -trimpath -ldflags '-s -w' -o bin/drmsonic ./cmd/drmsonic
```

Windows:

```powershell
.\build.ps1
```

Docker:

```sh
docker run --rm -v "$PWD:/src" -w /src golang:1.22-alpine sh build.sh
```

## Run

```sh
./bin/drmsonic -p 1234 -b 0.0.0.0 -providers providers -c drmsonic.json
```

Open `http://127.0.0.1:1234/`.

| flag | default | meaning |
|------|---------|---------|
| `-p` | `1234` | HTTP port |
| `-b` | `0.0.0.0` | bind address |
| `-baseurl` | request host | public URL written into playlists |
| `-providers` | `providers` | providers directory |
| `-c` | `drmsonic.json` | users/config file |
| `-user` / `-password` | empty | static admin credentials |
| `-allow` | empty | comma separated IPs or CIDRs that skip auth |
| `-l` | empty | log file |
| `-v` | `0` | verbosity 1..3 |
| `-noautostart` | off | do not start autostart streams |
| `-accesslog` | off | log every request |
| `-path` | empty | chdir before start |
| `-version` | | print version and exit |

`run.sh` is a thin wrapper around the release binary.

## Quick start

1. Open the panel, go to **Providers** and create a provider (or edit `providers/example.json`).
2. Open **Config**, add a stream, paste the manifest URL.
3. For encrypted sources paste `kid:key` pairs into **Keys**, one per line.
4. **SAVE ALL**, then **START** on the **Streams** page.
5. Play `http://<host>:1234/stream/<provider>/<stream>/master.m3u8`.

A stream is keyed by `ProviderId/StreamId`; both appear in the playback URL.

## ClearKey keys

```
kid:key
```

Both parts are hex. A KID may be written with dashes or without:

```
0123456789abcdef0123456789abcdef:f0e1d2c3b4a5968778695a4b3c2d1e0f
```

A bare key (no KID) is used for every sample. Keys are matched against the `default_KID`
in the init segment, so a provider with several tracks can carry several lines.

IV size is read from `tenc`. Turn on **Fix IV size** when a packager writes a 16-byte IV
but encrypts with an 8-byte one.

## Testing the output

Every check below works against a running channel.

### Browser

The **Streams** page has an inline player. It uses hls.js and plays the panel's own playlist,
so if the player starts, the whole pipeline works.

### VLC

```sh
vlc http://127.0.0.1:1234/stream/channels/channel1/master.m3u8
```

### ffplay

```sh
ffplay -fflags nobuffer -flags low_delay \
  http://127.0.0.1:1234/stream/channels/channel1/master.m3u8
```

### mpv

```sh
mpv --no-cache http://127.0.0.1:1234/stream/channels/channel1/master.m3u8
```

### curl and ffprobe

Fetch the playlists:

```sh
curl -s http://127.0.0.1:1234/stream/channels/channel1/master.m3u8
curl -s http://127.0.0.1:1234/stream/channels/channel1/stream.m3u8
```

Grab the last announced segment and inspect it:

```sh
SEG=$(curl -s http://127.0.0.1:1234/stream/channels/channel1/stream.m3u8 |
      grep -o 'stream[0-9]*\.ts' | tail -1)
curl -s -o seg.ts http://127.0.0.1:1234/stream/channels/channel1/$SEG
ffprobe -hide_banner seg.ts
```

Expected: `mpegts`, one H.264 or H.265 video stream at the selected resolution, one AAC
stream, duration equal to the segment length. A clean decode:

```sh
ffmpeg -v error -i seg.ts -f null -
```

No output and exit code 0 means the segment decodes without errors. For a continuity check
across segments, concatenate three and decode them together:

```sh
ffmpeg -v warning -i a.ts -i b.ts -i c.ts -f null -
```

### UDP output

Set **Output mode** to `udppush` and **Output params** to `udp://239.0.0.1:1234` (or a
unicast `udp://127.0.0.1:1234`), then:

```sh
ffplay udp://@239.0.0.1:1234
```

Packets carry 7 TS packets each (1316 bytes), the usual multicast layout.

### External playlists

```
/playlist.m3u   every running stream
/live.m3u       live streams
/event.m3u      events
/app1.conf      App1 style config
```

These are plain text lists for IPTV clients and players that take an M3U tuner.

## API

Everything the panel does is a `POST /api/action` with a JSON body whose `Action` field
selects the verb.

```
getstatus, getconf, getmonitoring, getrecordings, getreplays, getevents,
start, stop, restart,
save, updatestream, addchannel, deletestream, renamechannel,
addprovider, deleteprovider, renameprovider, refresh, rescanproviders,
updateglobal, applyglobalnetwork, massupdate*, adduser, updateuser,
deleteuser, renameuser, export, import, shutdown
```

Stream selection uses `Streams: [ { "Id": "channel1" } ]`, `"all"`, or a comma separated list.

```sh
curl -s -X POST http://127.0.0.1:1234/api/action \
  -d '{"Action":"start","ProviderId":"channels","Streams":[{"Id":"channel1"}]}'
```

## HTTP endpoints

```
/                       providers
/providers
/status/{provider}      live table
/config/{provider}      stream editor
/events/{provider}
/monitoring
/users
/rec
/help
/debug                  runtime dump

/api/action             JSON RPC

/playlist.m3u
/live.m3u
/event.m3u
/replay.m3u
/app1.conf

/stream/{provider}/{stream}/master.m3u8
/stream/{provider}/{stream}/stream.m3u8
/stream/{provider}/{stream}/streamNNNNN.ts
```

## Provider file

One file per provider: `providers/<id>.json`. A stream carries its manifest, selected tracks,
keys, running mode, output mode, network overrides and restart policy. Saves are atomic
(temp file, then rename) and happen off the streaming path.

`providers/*.json` is ignored by git; `providers/example.json` is the template that ships
with the repository.

## Network overrides

Per provider, and per stream when **Network override** is on:

| field | meaning |
|-------|---------|
| `Bind` | source address for outbound sockets |
| `Proxy` | `http://`, `https://` or `socks5://` |
| `Doh` | DNS-over-HTTPS endpoint (RFC 8484) |
| `NetworkScope` | which phases the override applies to: `script,manifest,media` |
| `Headers` | extra request headers, `Name: value` per line |

## Layout

```
cmd/drmsonic/          entry point, flags, signals
internal/config/       provider/stream model, atomic JSON store
internal/state/        runtime channel state, status projection
internal/manifest/     MPD and M3U8 parsers
internal/mp4/          ISO-BMFF init and fragment parsing
internal/cenc/         subsample aware AES-128-CTR
internal/remux/        Annex B and ADTS conversion
internal/mux/          MPEG-TS packetiser
internal/fetch/        HTTP client with bind, proxy, DoH
internal/pipeline/     DASH and HLS stream runners
internal/out/          UDP output
internal/web/          router, action dispatch, playlists, templates
ui/static/             front end assets
ui/templates/          server templates
providers/             one JSON file per provider
```

`docs/ARCHITECTURE.md` covers the packet level details: subsample decryption, timestamp
normalisation, the TS muxer and the segment window.

## Roadmap

- `cbcs` pattern decryption
- SAMPLE-AES HLS
- SRT and pipe output
- fMP4/CMAF output
- recording and replay

## License

MIT. See [LICENSE](LICENSE).
