# Roadmap

The package was extracted from a parameter-search tool that needed exactly one thing: a
39-dimensional Halton sequence that actually fills its box at 600 points. Everything below
is wanted but not yet built, and none of it should land without a measurement that shows
it earning its place.

## Sobol

Sobol sequences have better high-dimensional behaviour than Halton and do not degrade with
dimension the way large-prime radical inverses do. Needs direction numbers — Joe & Kuo's
tables are the usual source, and they are large enough that generating or embedding them
is its own decision.

API shape is already reserved: `qmc.NewSobol(dims, opts...)` returning the same generator
surface, so a caller swaps one constructor.

- [ ] Direction numbers (embed Joe & Kuo, or generate from primitive polynomials)
- [ ] Gray-code recurrence for `Next`, direct evaluation for `At`
- [ ] Digital-shift randomization, then Owen scrambling

## Owen scrambling

Nested/Owen scrambling is stronger than the random-digit scrambling implemented here: it
permutes each digit conditionally on the digits above it, which removes correlations that a
per-position permutation leaves behind. It costs a hash per digit rather than a table
lookup.

- [ ] Owen scrambling as an option alongside `WithScrambling`
- [ ] Measure it against random-digit scrambling on the 39-dimension correlation test
      before recommending it — the cheaper scrambling already gets the worst pair from
      0.81 to 0.14, and the remaining headroom may not be worth the per-digit hash

## Leaping

Skipping every *L*-th point is sometimes used to decorrelate coordinates in place of
scrambling. It is easy to add (`WithLeap(n)`, one multiply in `fill`) and easy to get
wrong: a leap that shares a factor with a base makes that coordinate worse, not better.

- [ ] `WithLeap`, with the shared-factor trap documented and tested

## Discrepancy measurement

The correlation test is a proxy. Star discrepancy is the real quantity, and having it would
let every claim in the README be a number rather than an argument.

- [ ] `Discrepancy(points)` for low dimensions (exact star discrepancy is expensive above
      a handful of dimensions)
- [ ] An `L2` centred discrepancy, which is cheap in any dimension

## Downstream

- [ ] `mayfly.WithQMCInitialPopulation` — metaheuristics initialize their populations with
      uniform random draws (`mayfly/helpers.go`), and a low-discrepancy initial population
      is a well-documented cheap improvement. Wants measuring on mayfly's own benchmark
      suite, not asserting.
