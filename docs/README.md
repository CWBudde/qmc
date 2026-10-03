# qmc documentation

The [README](../README.md) introduces the library.
[CONTRIBUTING.md](../CONTRIBUTING.md) explains setup and checks.
[PLAN.md](../PLAN.md) is the authoritative remediation checklist: task status,
acceptance criteria, verification evidence, and documented deferrals live there.
Topic pages explain behavior and decisions; they do not maintain competing
open-work lists. New findings should receive a plan entry and a link from the
relevant topic.

| Page                                          | What it covers                                                                     | Related review tasks                |
| --------------------------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------- |
| [Choosing a sequence](choosing-a-sequence.md) | Dimensions, sample windows, Sobol alignment and projection quality                 | SCI-01, API-01                      |
| [Randomization](randomization.md)             | Fixed, nested, digital-shift and hash-based schemes; finite randomness assumptions | SCI-01, PERF-01                     |
| [Leaping](leaping.md)                         | Coprimality validation, deterministic windows and recurrence tradeoffs             | CORE-01, API-01                     |
| [Discrepancy](discrepancy.md)                 | Exact star limits, centered L2 formulas, RMS baseline and numerical precision      | CORE-05, CORE-06, CORE-08, DOC-01   |
| [Small budgets](small-sample-regime.md)       | Reproducible 40/160-point integration and discrepancy fixtures                     | TEST-01, TEST-02, DOC-01            |
| [API design](api-design.md)                   | The six-method interface, option ownership, panic boundaries and aligned blocks    | CORE-03, CORE-07, API-01            |
| [Testing methodology](testing-methodology.md) | Independent references, negative controls, uncertainty and verification budgets    | CORE-04, TEST-01 through TEST-03    |
| [Performance](performance.md)                 | Canonical current measurements, raw evidence and optimization decisions            | PERF-01                             |
| [Toolchain and CI](toolchain.md)              | Pinned setup, both modules, workflow security, artifacts and releases              | TOOL-01 through TOOL-04, SHIP-01/02 |
| [WebAssembly demo](wasm-demo.md)              | Worker ownership, accessibility, browser checks and rendering decisions            | DEMO-01 through DEMO-10             |

Mathematical identities and API limits are distinct from measured quality and
timing. Published comparisons identify their workload and reproduction method.
The canonical performance report contains environment metadata and raw data;
older changelog entries describe their historical release, rather than current
performance guarantees.
