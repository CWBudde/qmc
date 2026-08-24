# API design

Why the surface looks the way it does, and which parts of it are still under review. The
code carries most of this reasoning already — `sequence.go`'s doc comment and `options.go`'s
`randomization` type are the primary sources; this page collects what is spread across them
and records what has not been settled.

## `Sequence` is deliberately small

Six methods: `Dims`, `Next`, `NextInto`, `Reset`, `At`, `AtInto`.

It excludes everything specific to one construction. Halton exposes `Bases` and
`Permutation`; Sobol has neither, because it works in base 2 in every dimension. Putting
either on the interface would force one generator to answer a question that does not apply to
it, and the honest answer to an inapplicable question is not a zero value — it is that the
caller is holding the wrong type.

The stateful/stateless split matters more than it looks. `At(i)` depends only on `i` and the
configuration, so it is the reproducible entry point and the one safe to call concurrently;
`Next` carries a cursor and is not.

The interface was settled _before_ the second generator landed. Adding one afterwards to a
package with two concrete types would have been a breaking change.

`sequence.go` closes with a compile-time assertion per generator. That is what keeps the
interface honest: a method renamed on one generator and not the other stops the build there
rather than at some caller's type switch.

## Options are fixed at construction

`Option` applies at construction time and the resulting configuration is fixed for the
generator's life. A sequence whose parameters could change mid-run would not be reproducible,
which is the whole point of using a quasi-random sequence.

Randomization is **one field, not four bools**, because the schemes are mutually exclusive.
Modelling it as independent bools would make "scrambled and digitally shifted" a representable
state that no constructor knows what to do with, and the usual outcome of a
representable-but-meaningless state is that some branch silently picks a winner. Here the last
option applied wins — what functional-options ordering already promises — and a scheme that
does not apply to the generator being built is an error at construction rather than something
quietly ignored.

## The panic policy

The package panics on some correctly-typed arguments: `fill` on index overflow,
`scrambledRadicalInverse` on an unreversible index, and any `dst` shorter than `Dims()`.

The `dst` case is the clearest: a short write leaves the tail coordinates holding stale values
that look like plausible positions. Returning silently would produce a wrong answer that
passes every downstream check.

The overflow cases have the same shape. `fill` once computed `skip + 1 + i` unchecked; on
overflow the wrapped index hit the `index < 0` guard in `radicalInverse` and returned the
all-zeros origin — the one point the package documents it never returns. Refusing is strictly
better than that.

## Still open

- **`Describe()` on `Sequence`.** A caller holding a `Sequence` cannot ask what randomization
  is in effect, how many dimensions the concrete generator can reach, or whether it has prime
  bases. The WebAssembly demo needs all three and gets them from a hand-maintained table on
  the side. A small `Describe()` returning a value type would remove that duplication without
  putting Halton-specific methods on the interface.
- **`type Option func(*settings)` renders as a leak.** Exporting a function type over an
  unexported struct is a deliberate and common way to stop third parties writing options, but
  godoc shows `func(*settings)`. Either document the intent on the type or move to an
  interface with an unexported method.
- **Panicking is a hard sell for a library.** The reasoning above is sound and written down,
  but it is worth revisiting whether the stateless entry points should return `(point, error)`.
  If they ever do, `NextInto`'s unchecked `h.cursor++` needs its own guard — today it is
  unreachable only because `fill` panics one index earlier.
- **Unreachable `base < 2` guards.** `radicalInverse` and `scrambledRadicalInverse` still
  carry them, and no in-package call site can trip them: bases are always primes. They are
  cheap, but they are also the mechanism that turned the index overflow into silent zeros.
  Decide whether they are a precondition worth keeping or dead weight worth deleting.
