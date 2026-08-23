//go:build js && wasm

package main

import "syscall/js"

// leapExamples is how many admissible leaps jsLeaps offers as a starting
// point. Enough to show that they are sparse and roughly where they begin;
// short enough to sit on one line of the page.
const leapExamples = 5

// jsLeaps answers the one question the leap control cannot answer for itself:
// whether the number now in the box is a leap this generator will accept, and
// if not, which nearby number is.
//
// It exists because leaping is the only control on this page whose legal
// values depend on the other controls. A burn-in of 500 is as valid at two
// dimensions as at sixty; a leap of 4 is refused at every dimension count
// because 2 is always a base, and a leap of 169 is fine at six dimensions and
// refused at seven, where 13 joins the base list. Without this export the page
// could only find out by asking for points and getting an error back, so the
// scatter would blank on most of the slider — the control would look broken
// rather than sparse.
//
// Admissibility is decided by BUILDING a generator and reading the error, not
// by re-deriving the coprimality rule here. That is deliberate and it is the
// same principle info.go states for randomizations: the library is the only
// place that says which options a constructor accepts, and a second copy in
// the demo is the copy that goes stale after a release. Here it is cheaper
// than it looks — an unrandomized constructor sieves primes and allocates
// nothing else — and it means this export cannot disagree with the points call
// it is predicting.
func jsLeaps(opts js.Value) any {
	source := readString(opts, "source", defaultSource)

	spec, ok := sources[source]
	if !ok || spec.construct == nil {
		return errorResult("leaps: unknown sequence %q", source)
	}

	var (
		dims = clampInt(readInt(opts, "dims", defaultDims), 1, spec.maxDims)
		leap = clampInt(readInt(opts, "leap", defaultLeap), 1, maxLeap)
	)

	admissible, reason := leapAdmissible(source, dims, leap)

	// The examples start at 2, not at the requested leap, because their job is
	// to show where the admissible leaps for this configuration begin. At 39
	// Halton dimensions the answer is 173 — every smaller number has a prime
	// factor among the first 39 primes — and seeing that gap is most of
	// understanding why the control refuses so much.
	examples := admissibleLeaps(source, dims, 2, leapExamples)

	// suggested is the nearest usable value at or above what the user asked
	// for, so the page can offer it rather than make them hunt. It is only
	// offered, never applied: silently rounding a leap up would mean the
	// number on screen was not the number the sequence used.
	var suggested any

	if found := admissibleLeaps(source, dims, max(leap, 2), 1); len(found) > 0 {
		suggested = found[0]
	}

	result := map[string]any{
		"source":     source,
		"dims":       dims,
		"leap":       leap,
		"admissible": admissible,
		"suggested":  suggested,
		"examples":   intsToJS(examples),
	}

	// null rather than "" when there is nothing to report, so the page tests
	// the field itself instead of comparing against an empty string.
	if reason == "" {
		result["reason"] = nil
	} else {
		result["reason"] = reason
	}

	return result
}

// leapAdmissible reports whether the library accepts this leap here, and the
// refusal it gave if not.
//
// Leap 1 short-circuits: it is the neutral value, WithLeap(1) is bit-identical
// to no option at all, and it is what every control starts on — so the common
// case never builds anything.
func leapAdmissible(source string, dims, leap int) (bool, string) {
	if leap <= 1 {
		return true, ""
	}

	if _, err := newGenerator(source, dims, 0, leap, randomizationNone, 0); err != nil {
		return false, err.Error()
	}

	return true, ""
}

// admissibleLeaps collects up to want leaps at or above from, in order.
//
// The scan stops at maxLeap rather than running until it has want of them: on
// a source and dimension count where admissible leaps are sparse near the top
// of the range the loop would otherwise be bounded only by the clamp, and this
// runs on the browser's only thread. A short list is a fine answer; a stalled
// tab is not.
func admissibleLeaps(source string, dims, from, want int) []int {
	out := make([]int, 0, want)

	for candidate := max(from, 2); candidate <= maxLeap && len(out) < want; candidate++ {
		if ok, _ := leapAdmissible(source, dims, candidate); ok {
			out = append(out, candidate)
		}
	}

	return out
}
