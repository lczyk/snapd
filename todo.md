# install-extended runtime failures

## k9s -- hangs on `snap run k9s version`

k9s is a terminal UI. `snap run k9s version` hangs for ~55s then times out. `k9s version` likely does not recognise `version` as a subcommand in the snap version and launches the interactive TUI instead, blocking indefinitely. test needs a non-blocking way to verify the binary works, or the subcommand name may differ.

## lazygit -- `--print-default-config` flag mismatch

`snap run lazygit --print-default-config` fails: "Expected a following arg for flag print-default-config, but it did not exist." the snap ships lazygit v0.x which uses `-c` / `--config` to print the default config, not `--print-default-config`. `--print-config-dir` works fine. fix: replace `--print-default-config` with `--config` in the test script, or drop the assertion if the flag version varies across snaps.

## yt-dlp -- `ModuleNotFoundError: No module named 'yt_dlp'`

the snap installs and sets up /snap/bin/yt-dlp, but at runtime python cannot find the bundled `yt_dlp` module. likely an arm64 packaging issue -- the snap's python site-packages tree is missing or mis-wired for this architecture. not a bug in snapd; the snap itself is broken on arm64.

---

# test gaps (squashfs + xz fork)

audit of what we don't cover after the round of perf + correctness work. ordered by real-world risk.

## xz / lzma layer

- **multi-block xz within a single stream.** legal per the format (`xz --block-size=N`) but upstream's default `Writer` emits one block per stream. our fork's `streamReader.Read` loop walks block-to-block; only the single-block path is exercised. needs a crafted fixture.
- **xz check types.** we test the default crc only. xz spec allows none / CRC32 / CRC64 / SHA-256. fork has `none-check.go` + crc paths; CRC64 and SHA-256 paths likely never run. easy to cover -- upstream `WriterConfig.CheckSum` exposes the knob.
- **non-LZMA2 filters.** xz format permits BCJ / delta filter chains. `verifyFilters` rejects anything other than LZMA2-last; no test confirms the error path. should fail cleanly, not panic.
- **`SingleStream` flag on `xz.Reader`.** unused / untested. dead-code unless we light it up.
- **golden vectors.** no canonical xz files from the xz-utils test suite committed. insurance vs upstream behaviour drift.

## squashfs layer

- **squashfs-level fuzzer.** we fuzz the xz decoder. we do _not_ fuzz `nativeReader` against arbitrary squashfs bytes -- malformed superblock / inode / dir entries. real attack surface.
- **fixture w/ non-xz comps for the native benches.** only xz fixture today; won't catch comp-specific regressions in `walkDir` / `extractAll`.
- **race detector on heavier paths.** `TestConcurrentDecode` (xz) under `-race` only. nothing exercises `extractFiles`' worker-pool path concurrently under `-race`.
- **bench correctness.** xz benches do `io.Copy(io.Discard, r)` -- timing only, never validates bytes. wouldn't catch a regression that produced corrupt output at full throughput.

## snap-level

end-to-end install correctness sits in spread tests outside what we've touched -- not in scope for this round.

## priority

1. squashfs-level fuzzer (highest real-world risk).
2. multi-block xz fixture + test (correctness on legal-but-uncommon shapes).
3. check-type matrix for xz (cheap, catches lurking bugs in CRC64 / SHA-256 / None paths).
4. golden vectors (drift insurance, cheap once gathered).
5. bench validation (cheap; skippable if differential / fuzz suite is trusted).
