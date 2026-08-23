//go:build js && wasm

package main

import (
	"math/rand"
	"syscall/js"
)

const maxPoints = 20000

// jsPoints projects the sequence onto two chosen coordinates: the scatter plot
// that is the demo's front page.
//
// The comparison set is generated here too, with math/rand, rather than by
// calling Math.random() in the page. That is not pedantry about where the code
// lives — a JavaScript comparison set could not be reproduced from the seed on
// screen, and reproducibility is the axis on which the two samplers are being
// compared. Both sides of the picture come out of the same call, with the same
// seed, drawn the same way.
func jsPoints(opts js.Value) any {
	source := readString(opts, "source", "halton")
	if source != "halton" && source != "random" {
		return errorResult("points: unknown source %q", source)
	}

	var (
		dims     = clampInt(readInt(opts, "dims", defaultDims), 1, maxDims)
		count    = clampInt(readInt(opts, "count", defaultCount), 1, maxPoints)
		skip     = clampInt(readInt(opts, "skip", defaultSkip), 0, maxSkip)
		scramble = readBool(opts, "scramble", true)
		seed     = readUint64(opts, "seed", defaultSeed)
	)

	// The axis defaults come from the info table, so the page opens on the
	// interesting pair (37/38). They are then clamped into range, which is
	// what makes lowering the dimension count safe: at dims = 2 the request
	// for coordinate 37 becomes coordinate 1 rather than an out-of-range read.
	axisX := clampInt(readInt(opts, "axisX", defaultAxisX), 0, dims-1)
	axisY := clampInt(readInt(opts, "axisY", defaultAxisY), 0, dims-1)

	xy := make([]float32, 0, count*2)

	var bases []int

	// One buffer for every point. AtInto and the rand loop both write into it
	// in place, so a 20,000-point redraw allocates exactly the output array
	// and nothing else — which matters on a heap the browser may refuse to
	// grow.
	point := make([]float64, dims)

	if source == "random" {
		rng := rand.New(rand.NewSource(int64(seed))) //nolint:gosec // not cryptography; reproducibility is the requirement

		for range count {
			for d := range point {
				point[d] = rng.Float64()
			}

			xy = append(xy, float32(point[axisX]), float32(point[axisY]))
		}
	} else {
		generator, err := newGenerator(dims, skip, scramble, seed)
		if err != nil {
			return errorResult("points: %v", err)
		}

		bases = generator.Bases()

		// At/AtInto rather than Next: the point drawn for index i must not
		// depend on how many points were drawn before it, or a redraw at a
		// different count would shift the whole picture.
		for i := range count {
			generator.AtInto(i, point)
			xy = append(xy, float32(point[axisX]), float32(point[axisY]))
		}
	}

	out := opts.Get("out")

	response := map[string]any{
		"count":    count,
		"dims":     dims,
		"axisX":    axisX,
		"axisY":    axisY,
		"skip":     skip,
		"scramble": scramble,
		"seed":     float64(seed),
		"source":   source,

		// The prime base is what explains the shape on screen — dimension 38's
		// base 167 is why its first 167 unscrambled points march up a ramp in
		// steps of 1/167 — so the page can label the axes with it. A
		// pseudo-random draw has no bases at all, and reporting a plausible
		// number there would invite exactly the wrong reading, so both are
		// null.
		"baseX": axisBase(bases, axisX),
		"baseY": axisBase(bases, axisY),
	}

	// x and y interleaved, so the page walks xy[2*i], xy[2*i+1] and hands the
	// same buffer to the canvas without a second pass to de-interleave.
	putFloats(response, out, "xy", xy)

	return response
}

func axisBase(bases []int, axis int) any {
	if axis < 0 || axis >= len(bases) {
		return nil
	}

	return bases[axis]
}
