# API design

The public surface remains small and compatible. API-01's decisions are recorded
below; [PLAN.md](../PLAN.md) carries implementation and verification status.
Package documentation lives in sequence.go, with constructor and option
contracts beside their implementations.

## Sequence and capability metadata

Keep Sequence's six methods: Dims, Next, NextInto, Reset, At, AtInto. Concrete
Halton exposes Bases and Permutation; these do not belong on Sobol or on an
arbitrary third-party Sequence. Compile-time assertions check both built-in
implementations against the interface.

Do not add Describe to Sequence or an optional descriptor interface at present.
Requested instance dimensions, available constructor/table capacity, selected
options, and a product's workload limits are different kinds of information.
Dims already describes the instance. Applications choose and can retain their
configuration; constructors validate supported options and tables. A generic
maximum would be ambiguous for Halton's memory-limited sieve and Sobol's
caller-supplied tables, and new descriptor fields would need their own stable
meaning and compatibility contract. Existing evidence does not establish that
additional public surface is needed.

The demo's source/randomization menus are product choices, including a
pseudo-random comparison that is not a Sequence. Its 64-dimension offer is a
workload cap. The copied 1024-dimension library ceiling has been removed.
The Go browser fixture checks every offered sequence/randomization at both
dimension endpoints against real constructors. This catches unsupported menu
drift without teaching JavaScript or a duplicate table how the library works.
Prime/digit inspection uses concrete Halton methods; leap and metric validation
remain Go-side. A future library-wide capability facility should be driven by
multiple callers and separate instance metadata from constructor/product policy.

## Options and reader ownership

Keep `type Option func(*settings)`. settings is private deliberately: external
callers compose supported With functions rather than bypassing validated
configuration invariants. Private fields can evolve without becoming a public
configuration struct. Switching to a sealed interface would change an existing
exported type while giving these callers little additional capability.
Nil options are unsupported.

Options apply in order at construction, with the last randomization option
winning. The resulting generator configuration is fixed. One randomization
field prevents mutually incompatible boolean combinations; constructors reject
schemes that do not apply to that generator.

Value options capture normalized immutable values and can be shared by
concurrent constructors. WithDirectionNumbers captures a consumable reader
instead: NewSobol consumes it and does not close it. Supply a fresh reader/option
per constructor, for example
`WithDirectionNumbers(bytes.NewReader(tableBytes))`. Rewinding/reuse is
sequential only; sharing a reader concurrently is unsupported. The reader's
lifecycle is distinct from the immutable table/state of a constructed generator.

## Indexed access and panic boundaries

Keep existing signatures; do not add generic checked indexed-access helpers.
They would need a new index-capability contract for arbitrary Sequence
implementations, and a universal recover wrapper would risk converting
unrelated implementation failures into apparently ordinary request errors.
Replacing signatures would break callers. Concrete checked variants could be
considered if a demonstrated consumer needs them, but the current demo already
validates/clamps its own bounded requests and distinguishes recoverable requests
from actual runtime failure.

The present contract is explicit:

- Negative point indices are treated as zero; constructors clamp negative skip
  and nonpositive leap values to their neutral values.
- Construction returns errors for unsupported configurations, invalid tables,
  or a skip that leaves no first representable point.
- Halton's raw index is `skip + 1 + i*leap` and must fit int. Sobol's raw ceiling
  is `min(MaxInt, 2^32-1)`. Multiplication is checked before it can wrap.
- A raw index beyond that ceiling panics. Fixed digit-scrambled Halton can also
  panic at extremely large representable indices when the permuted digit
  reversal would overflow uint64. Representable raw arithmetic does not remove
  this separate reversal limit.
- Destination buffers shorter than Dims panic. A failing call may have written
  earlier coordinates; Into calls are not transactional error-returning writes.
- Next and NextInto return the final admissible point normally. A failed draw
  does not advance the cursor; reset restores point zero. Indexed methods leave
  the cursor unchanged and can run concurrently with separate output buffers.
  Stateful methods require caller serialization.

Boundary, leap, short-buffer, reversal-overflow, state-preservation, and
concurrency regressions cover these contracts. Executable 386 checks exercise
the lower int ceiling independently of Sobol's word bound.

## Aligned Sobol blocks and the raw origin

For a block of `N = 2^m` points with leap 1, choose a representable integer
`q >= 1` and `WithSkip(q*N - 1)`. Indexed points 0..N-1 then visit raw indices
q*N..(q+1)*N-1. The whole block must fit the raw ceiling; check the arithmetic
before converting q\*N to int. For example:

```go
const n = 256
g, err := qmc.NewSobol(2, qmc.WithSkip(2*n-1), qmc.WithDigitalShift(7))
if err != nil {
    return err
}
points := qmc.Draw(g, n) // raw indices 512..767; Draw preserves the cursor
```

Do not add a raw-origin or aligned-block API now. Existing skip/index access
already expresses later aligned blocks. Default skip 0 starts at raw index 1;
raw-origin block 0..N-1 is deliberately unavailable, and negative skip is
clamped, so WithSkip(-1) cannot request it. This differs from implementations
that expose the origin; reproducing their exact first block needs an explicitly
different interface/configuration, not a silent change to existing outputs.

Later-block regressions check q=1,2,3,17 and the final complete representable
block at N=16/256, with plain, digital-shift, and hash-based Owen sampling on
amd64 and executable 386. They compare indexed/stateful methods and every
dyadic aspect ratio of the known t=0 first-two-dimensional projection. A
general (t,m,s) net has occupancy 2^t, so these tests do not imply one-point
occupancy for every projection. Leap above one is not a complete raw block.
Gray ordering permutes low bits within a block while mapping its fixed high
bits to another fixed high-bit pattern; it need not map every later block onto
itself for aligned-block occupancy to hold.

## Internal helper preconditions

Keep the cheap invalid-base/negative-index guards on private radical-inverse
helpers. Constructors supply prime bases at least 2, and checked raw arithmetic
prevents wrapped negative indices from reaching them. Direct package tests and
fuzzing also use these helpers: returning zero for invalid direct inputs is
their explicit defensive behavior, verified by the guard regression. The guard
is not an overflow policy and must not replace validation in fill.

Nested root-cache slices are constructor-owned immutable permutations matching
their bases. rootPermutation receives a valid constructor dimension; the digit
loop uses local scratch. Its absence outside the cached prefix is an ordinary
fallback. These are internal invariants, not public caller configuration.

## Bulk and workspace decisions

Keep Into plus Draw's contiguous matrix, without another batch or workspace
surface. PERF-01's reused-buffer plain bulk prototypes have medians within 2%
and overlapping timing ranges. CORE-07's scratch threshold remains explicit:
nested Halton allocates per coordinate above base 512 (0/1/3 allocations at
97/98/100 dimensions). A caller-owned workspace could address that workload,
but would need clear dimension/configuration ownership and concurrent-buffer
rules. The demo caps dimensions below the threshold; a representative consumer
need and end-to-end benefit are missing. Shared mutable scratch would violate
indexed concurrency. Revisit one coordinated allocation design when that
evidence exists, rather than adding overlapping APIs now.
