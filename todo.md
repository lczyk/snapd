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

- **`SingleStream` flag on `xz.Reader`.** unused / untested. dead-code unless we light it up.

## snap-level

end-to-end install correctness sits in spread tests outside what we've touched -- not in scope for this round.

## priority

1. SingleStream flag (low value -- decide whether to keep or strip the dead code first).
