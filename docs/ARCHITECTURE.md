# Architecture

How a source segment becomes a servable MPEG-TS segment.

## 1. Manifest

`internal/manifest/mpd.go` parses the MPD into `Manifest{Video, Audio []Track}`. A `Track`
holds the resolved init URL, the segment template, the timescale, the presentation time
offset and a materialised list of `SegmentRef{Number, Time, Duration, URL}`.

`SegmentTemplate` supports:

- `$RepresentationID$`, `$Number$`, `$Time$`, `$Bandwidth$`
- `$Number%05d$` style formatting
- `startNumber`, `duration`, `timescale`
- `SegmentTimeline` with `t`, `d`, `r` (including `r = -1`, repeat to period end)

`TailSegments(n)` returns the last `n` references, which is what a live runner needs: the
newest `n` segments are the only ones that can still be fetched.

`internal/manifest/hls.go` parses master playlists (variant attributes, `EXT-X-MEDIA`),
media playlists (`EXTINF`, `EXT-X-MAP`, `EXT-X-KEY`, `EXT-X-MEDIA-SEQUENCE`, `EXT-X-ENDLIST`)
and resolves relative URLs against the playlist URL.

## 2. Init segment

`mp4.ParseInit` walks `moov/trak/mdia/minf/stbl/stsd` and extracts:

- handler (`vide` / `soun`), timescale, codec fourcc
- `avcC`, `hvcC`, `esds` payloads
- sample entry dimensions, frame rate, channel count
- `sinf/schm` scheme type and `sinf/schi/tenc` default KID, IV size, pattern

Encrypted tracks are detected from `schm == cenc`/`cens`. `cbcs` is rejected at startup
instead of producing garbage samples.

## 3. Fragment

`mp4.ParseFragment` reads a `moof` + `mdat` pair:

- `tfhd` — default sample duration, size, flags, base data offset
- `tfdt` — base media decode time
- `trun` — sample count, per sample duration, size, flags, composition offset
- `senc` — per sample IV and subsample map

Samples come back as `{Size, Duration, CTS, Flags, IV, Subsamples, offset}`, with `end`
computed after the muxer decides the output rate.

## 4. Decryption

`cenc.DecryptSample` keeps a single AES-CTR stream per sample:

```
pos = 0
for each subsample:
    skip clear bytes
    decrypt protected bytes, incrementing the CTR block for every 16 bytes
```

The counter is never reset inside a sample, so a protected range that ends mid-block is
continued in the next range. This is the part that breaks naive `AES-CTR-per-range`
implementations: the key stream has to follow the sample, not the range.

## 5. Elementary streams

Video samples are length-prefixed NAL units. `remux.ToAnnexB` replaces every length prefix
with a 4-byte start code and prepends:

- an access unit delimiter (`09 F0` for H.264, `46 01 50` for H.265)
- VPS/SPS/PPS on keyframes, when the packager did not inline them

AAC samples become ADTS frames: 7-byte header, profile from `AudioSpecificConfig`, sample
rate index and channel configuration from the init segment.

## 6. Timestamps

Manifest ticks are converted to 90 kHz:

```
dts = (baseMediaDecodeTime + sampleOffset - presentationTimeOffset) * 90000 / timescale
pts = dts + compositionOffset * 90000 / timescale
```

`compositionOffset` can be negative when a packager uses `ctts` version 1. MPEG-TS requires
`PTS >= DTS`, so `pipeline.shiftNegativeCTS` moves the decode timestamps of the video samples
earlier by the smallest offset when one is negative. Audio is untouched, which keeps the
relative A/V offset intact.

Output timestamps are continuous for the life of a stream: each published segment is anchored
so its first sample lands exactly where the previous segment ended (`outDTS`). Restarts,
rendition switches and skipped segments therefore never rewind the timeline. The muxer also
clamps `DTS` to `PTS` as a last resort.

## 7. TS muxer

`internal/mux` writes:

- PAT on PID `0x0000`, PMT on PID `0x1000`
- video PES on PID `0x0100` (stream id `0xE0`), PCR in the adaptation field of keyframes
- audio PES on PID `0x0101` (stream id `0xC0`)

`appendPTS` writes the 33-bit 90 kHz field with the `001` / `011` prefix nibbles. The first
packet of a PES subtracts the PES header and any PCR bytes from the payload budget, so every
packet is exactly 188 bytes, padded with `0xFF` stuffing. Continuity counters are per PID.

Segments are assembled per publish call: `Begin()`, a series of `Video`/`Audio`, then
`Reset()` returns the bytes and clears the buffer.

## 8. Publish

`internal/web/media.go` keeps a ring of the last `NbAnnouncedFragments` segments per channel.
A segment name is `stream%05d.ts` and its number is the media sequence, so the playlist
`EXT-X-MEDIA-SEQUENCE` is the sequence of the oldest announced segment.

`GET /stream/{p}/{s}/stream.m3u8` renders the window, `GET .../streamNNNNN.ts` serves bytes
from RAM. The master playlist reports the selected video track's resolution, frame rate,
bandwidth and codecs, so players select the variant the panel is actually producing.

`udppush` sends the same bytes as 1316-byte datagrams (7 TS packets) to the configured target.

## 9. Supervision

`internal/web/runtime.go` runs one goroutine per started channel:

```
start → context → pipeline.Run (DASH or HLS)
                 ↘ on error: status, log, sleep RestartDelay, retry
stop  → cancel context → pipeline returns → media buffer dropped
```

Live channels poll the manifest on the MPD `minimumUpdatePeriod` (clamped to 1..10s) and
fetch the next segment after the last consumed one. When the consumer falls more than two
segments behind the live edge, the runner skips ahead to the newest segment instead of
drifting further behind.
