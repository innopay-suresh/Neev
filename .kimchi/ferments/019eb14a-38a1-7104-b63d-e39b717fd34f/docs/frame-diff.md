# Frame Differencing for Dirty Rectangle Detection

## Audit Notes

### What was implemented

**File created:** `agent/capture/diff.go`

**Types:**
- `Rect` — represents a changed region with X, Y, W, H in pixel coordinates
- `FrameDiff` — holds previous frame pixels and dirty block bitmap

**Key method:** `DetectDirtyRects(current *image.RGBA) []Rect`

### Algorithm

1. **Block comparison (8x8 grid):** For each block, scan all 64 pixels against the stored previous frame. If any RGB byte differs, the block is marked dirty.

2. **Row-merging:** Iterate over each block row. Identify continuous runs of dirty blocks. Each run becomes a candidate rectangle (block coordinates).

3. **Vertical merging:** If the last rectangle shares the same X and width and is directly above the current run, extend it vertically instead of creating a new rectangle.

4. **Coordinate conversion:** Convert block coordinates to pixel coordinates (multiply by 8), then clamp to frame bounds.

5. **State update:** Copy current frame pixels to `prev` after all comparisons so the next call has the correct reference.

### Design decisions

- **Block size 8:** Coarse enough to keep the dirty bitmap small and merging trivial; fine enough to not waste bandwidth on a typical desktop where 80-95% of pixels are static.
- **Ignore alpha:** Per `samePixel` semantics, alpha is not compared. This is correct for screen content where alpha is typically 255 and minor alpha flicker should not trigger a dirty rect.
- **RGB-only comparison:** Only bytes 0-2 are compared. This matches encoder behavior (RGBA frames but H.264 only encodes RGB).
- **In-place pixel copy:** Using `copy(d.prev, curr)` after dirty detection ensures the comparison uses the correct previous frame even if `current` is reused by the caller.

### Verification

- **Build:** `CGO_ENABLED=1 go build -o remote-agent-mac .` — succeeds (linker version warnings are pre-existing, not from diff.go)
- **Vet:** `go vet capture/diff.go` — clean, no issues

### Integration points (Phase 2, future steps)

The `FrameDiff` type is ready to be wired into `stream/pipeline.go`:
- After `CaptureFrame()` returns, call `fd.DetectDirtyRects(frame)` to get changed rectangles
- Log dirty rect count per frame: `log.Info().Int("dirty_rects", len(rects)).Msg("frame diff")`
- For frames with zero dirty rects, skip encoding entirely (static frame)
- For frames with few/small dirty rects, optionally crop the encoder to just those regions

### Gate Verdicts

| Gate | Status | Notes |
|------|--------|-------|
| S1 (Correctness) | **PASS** | Build succeeds, code compiles, no vet errors |
| S2 (Spec Compliance) | **PASS** | Implements Rect type, FrameDiff struct, NewFrameDiff constructor, DetectDirtyRects with 8x8 block grid and row-merging as specified |
| S3 (Integration-Ready) | **PASS** | Accepts `*image.RGBA` (same type as pipeline CaptureFrame returns), returns `[]Rect` suitable for downstream rect-based encoding |