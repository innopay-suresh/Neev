# ABR Implementation — Step 3 Audit Notes

## What was done

### 1. Rewrote `/agent/capture/abr.go`

Replaced the old AIMD-style ABR controller with a synchronous, ladder-based implementation.

**Key design decisions:**
- `bitrateLadder = []int{2048, 1024, 512, 256}` kbps — 4-step fixed ladder
- `step int` tracks position in ladder (0 = highest 2048 kbps)
- `GetTargetBitrate()` is the public API: feed stats, read bitrate — no background goroutine needed
- Step-down: `lossRate > 5%` OR `rtt > 200ms` for 3+ consecutive samples → move down one rung
- Step-up: `lossRate < 1%` AND `rtt < 100ms` for 10+ consecutive samples → move up one rung
- On change, logs: `log.Info().Int("bitrate_kbps", bitrateLadder[step]).Msg("abr")`

The controller is now stateless between calls (no `Run()` goroutine needed) — the pipeline calls `GetTargetBitrate()` synchronously in `collectStats`.

### 2. Updated `/agent/stream/pipeline.go`

**NewPipeline init change:**
```
- abr := capture.NewABRController(enc, minBitrateKbps, maxBitrateKbps)
+ abr := capture.NewABRController()
```

**Removed** the `go p.abr.Run(2 * time.Second)` / `defer p.abr.Stop()` goroutine pair from `Start()` since ABR is now synchronous.

**Added SetBitrate wiring in collectStats:**
```
// Feed ABR and apply recommended bitrate
rtt := time.Duration(rttMs) * time.Millisecond
p.abr.UpdateStats(rtt, lossRate)
p.encoder.SetBitrate(p.abr.GetTargetBitrate())
```

**Resolution-change path** also updated to use `NewABRController()` (no encoder param).

### 3. Pre-existing bug fixed (unrelated to ABR)

`pipeline.go` line 296: `currentFPS :=` was redeclared where `currentFPS` was already in scope from line 244 (same function). Fixed by removing the redundant short-declaration (variable already assigned earlier in the block).

## Verification

```
cd /agent && CGO_ENABLED=1 go build -o remote-agent-mac .
```
Build succeeded. Only linker warnings about macOS version mismatch on .dylib files (26.0 built vs 14.0 target) — not errors, binary produced.

## Expected behavior during session

With simulated congestion (loss > 5% or rtt > 200ms for 3+ seconds):
```
abr bitrate_kbps=1024
abr bitrate_kbps=512
```

With recovery (loss < 1% AND rtt < 100ms for 10+ seconds):
```
abr bitrate_kbps=1024
abr bitrate_kbps=2048
```

## Gate Verdicts

- **S1 (abr.go completeness)**: S — 4-step ladder, step-down (3+ consecutive bad), step-up (10+ consecutive good), `GetTargetBitrate()`, log on change all present
- **S2 (pipeline.go wiring)**: S — `NewPipeline` initializes `abr := NewABRController()`, `collectStats` calls `p.encoder.SetBitrate(p.abr.GetTargetBitrate())`, resolution-change path also wired
- **S3 (build success)**: S — `CGO_ENABLED=1 go build -o remote-agent-mac .` succeeds (warnings only, no errors)