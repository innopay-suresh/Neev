package capture

import "image"

// Rect represents a changed region in a frame.
type Rect struct {
	X, Y, W, H int
}

// block is a grid coordinate for dirty rect detection.
type block struct{ gx, gy int }

// DetectDirtyRects compares two RGBA frames and returns the rectangles
// that have changed. It uses a block-based comparison with a threshold
// to handle minor compression artifacts and cursor blinking.
//
// Returns an empty slice if no changes are detected.
func DetectDirtyRects(current, previous *image.RGBA, blockSize int) []Rect {
	if current == nil || previous == nil {
		return nil
	}

	b := current.Bounds()
	if !b.Eq(previous.Bounds()) {
		// Resolution changed — return full frame as dirty
		return []Rect{{X: 0, Y: 0, W: b.Dx(), H: b.Dy()}}
	}

	if blockSize <= 0 {
		blockSize = 8 // pixels per block for comparison
	}

	// Build list of dirty blocks
	dirtyBlocks := make(map[block]struct{})

	w, h := b.Dx(), b.Dy()
	pix := current.Pix
	prevPix := previous.Pix
	stride := b.Dx() * 4 // RGBA stride

	// Compare in blocks
	for gy := 0; gy < h; gy += blockSize {
		for gx := 0; gx < w; gx += blockSize {
			// Check if block has changed
			if !blockEquals(pix, prevPix, stride, gx, gy, blockSize, w, h) {
				dirtyBlocks[block{gx, gy}] = struct{}{}
			}
		}
	}

	if len(dirtyBlocks) == 0 {
		return nil
	}

	// Merge adjacent blocks into rectangles using a greedy algorithm
	return mergeBlocksToRects(dirtyBlocks, blockSize, w, h)
}

// blockEquals compares a block of pixels between two frames.
// Returns true if the blocks are identical (within tolerance).
func blockEquals(pix, prevPix []byte, stride, bx, by, blockSize, w, h int) bool {
	endX := min(bx+blockSize, w)
	endY := min(by+blockSize, h)

	for y := by; y < endY; y++ {
		offset := y*stride + bx*4
		prevOffset := y*stride + bx*4
		for x := bx; x < endX; x++ {
			// Compare RGB (skip alpha)
			dr := int(pix[offset]) - int(prevPix[prevOffset])
			dg := int(pix[offset+1]) - int(prevPix[prevOffset+1])
			db := int(pix[offset+2]) - int(prevPix[prevOffset+2])

			// Threshold of 10 per channel to handle minor artifacts
			if dr < -10 || dr > 10 || dg < -10 || dg > 10 || db < -10 || db > 10 {
				return false
			}
			offset += 4
			prevOffset += 4
		}
	}
	return true
}

// mergeBlocksToRects converts a set of dirty blocks into minimal bounding rectangles.
func mergeBlocksToRects(blocks map[block]struct{}, blockSize, w, h int) []Rect {
	if len(blocks) == 0 {
		return nil
	}

	// Convert blocks to a 2D grid
	gridW := (w + blockSize - 1) / blockSize
	gridH := (h + blockSize - 1) / blockSize
	grid := make([]bool, gridW*gridH)

	for b := range blocks {
		gx := b.gx / blockSize
		gy := b.gy / blockSize
		if gx >= 0 && gx < gridW && gy >= 0 && gy < gridH {
			grid[gy*gridW+gx] = true
		}
	}

	// Find connected components (rectangles)
	visited := make([]bool, gridW*gridH)
	var rects []Rect

	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			idx := gy*gridW + gx
			if grid[idx] && !visited[idx] {
				// BFS to find connected region
				minX, minY := gx, gy
				maxX, maxY := gx, gy

				queue := []struct{ x, y int }{{gx, gy}}
				visited[idx] = true

				for len(queue) > 0 {
					curr := queue[0]
					queue = queue[1:]

					// Expand rectangle
					minX = min(minX, curr.x)
					maxX = max(maxX, curr.x)
					minY = min(minY, curr.y)
					maxY = max(maxY, curr.y)

					// Check 4 neighbors
					for _, d := range []struct{ dx, dy int }{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
						nx, ny := curr.x+d.dx, curr.y+d.dy
						if nx >= 0 && nx < gridW && ny >= 0 && ny < gridH {
							nidx := ny*gridW + nx
							if grid[nidx] && !visited[nidx] {
								visited[nidx] = true
								queue = append(queue, struct{ x, y int }{nx, ny})
							}
						}
					}
				}

				// Convert grid coords back to pixel coords
				rects = append(rects, Rect{
					X: minX * blockSize,
					Y: minY * blockSize,
					W: (maxX - minX + 1) * blockSize,
					H: (maxY - minY + 1) * blockSize,
				})
			}
		}
	}

	// Clamp rects to frame boundaries
	for i := range rects {
		rects[i].X = min(rects[i].X, w)
		rects[i].Y = min(rects[i].Y, h)
		rects[i].W = min(rects[i].W, w-rects[i].X)
		rects[i].H = min(rects[i].H, h-rects[i].Y)
	}

	return rects
}

// SubImage extracts a sub-rectangle from an RGBA image and returns it as a new RGBA image.
func SubImage(src *image.RGBA, r Rect) *image.RGBA {
	if r.X < 0 {
		r.X = 0
	}
	if r.Y < 0 {
		r.Y = 0
	}
	if r.X+r.W > src.Bounds().Dx() {
		r.W = src.Bounds().Dx() - r.X
	}
	if r.Y+r.H > src.Bounds().Dy() {
		r.H = src.Bounds().Dy() - r.Y
	}
	if r.W <= 0 || r.H <= 0 {
		return nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, r.W, r.H))
	srcStride := src.Bounds().Dx() * 4
	dstStride := r.W * 4

	for y := 0; y < r.H; y++ {
		srcRow := src.Pix[(r.Y+y)*srcStride+r.X*4:]
		dstRow := dst.Pix[y*dstStride:]
		copy(dstRow, srcRow)
	}

	return dst
}

// TotalPixels returns the total number of pixels in the given rectangles.
func TotalPixels(rects []Rect) int {
	total := 0
	for _, r := range rects {
		total += r.W * r.H
	}
	return total
}