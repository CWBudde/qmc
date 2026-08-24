# qmc documentation

The [README](../README.md) is the overview: what the package is, how to call it, and which
sequence to reach for. These pages carry the long form — the measurements behind each claim,
the caveats that only matter once you are relying on a number, and the reasoning behind the
choices that are not obvious from the code.

Everything here was measured rather than asserted. Where a figure appears, the test that
produces it is named, so it can be re-run rather than trusted.

| page                                              | what it answers                                                                                                                                             |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Choosing a sequence](choosing-a-sequence.md)     | Sobol or Halton, the dimension ceilings, and the two Sobol balance properties that make a correct sequence look broken                                      |
| [Randomization](randomization.md)                 | The four options, what each costs, the high-dimensional Halton defect they cure, and how far the hash-based Owen scramble is from an exact one              |
| [Leaping](leaping.md)                             | `WithLeap`, why a shared factor is refused rather than documented, and why Sobol's version of the trap is not the base-2 restatement it looks like          |
| [Discrepancy](discrepancy.md)                     | `StarDiscrepancy`'s exactness and its two-gate refusal; `CenteredL2Discrepancy`'s closed form and the saturation that makes it useless above ~20 dimensions |
| [The small-sample regime](small-sample-regime.md) | What happens at 40 points — the regime a seeded population actually uses, and the one the rest of the suite never touches                                   |
| [Testing methodology](testing-methodology.md)     | Why the gates assert ratios rather than constants, how many seeds a statistic needs, and what a stratification test cannot police                           |
| [Performance](performance.md)                     | Benchmark figures and the caveats attached to them, construction cost, and the WebAssembly demo's cost model                                                |
