# SSHakku — project rules

Rules for the rewrite of this project. Numbered, authoritative; add new rules by
appending. See `docs/THREAT-MODEL.md` for the threat model and the June 2026 incident.

## Rules

1. **Minimal changes.** Keep changes small, especially breaking ones. If a change
   touches many files or many lines, split it into sub-steps tracked in a
   gitignored `<activity>-steps.md` so work can resume even if the repo doesn't
   compile or is incomplete. Intermediate sub-steps may leave the repo broken, but
   each completed PLAN.md item must be committable (Rule 9). **Ask for
   authorization before each step.**

2. **Propose new rules** when one would help — don't add them silently.

3. **After a long task, before starting a new one**, decide whether to stay in the
   same session. To save tokens: ask for a compact, or write down what's done /
   what's left and start a new chat. Exception: keep context when it matters for
   the next task(s).

4. **Feel free to add skills** to the project (`.claude/skills/`).

5. **Before opening a PR or declaring done, verify quality** — run linters and
   tests if they aren't too heavy.

6. **Repo language is English** (code, comments, docs, log). Chat may be Italian.

7. **No sensitive data in committable files.** Never write personal data, anything
   about this machine/system, logs, or error output into a file that could be
   committed. That material goes only in gitignored scratch files.

8. **Step & scratch files.** A `<activity>-steps.md` holds the sub-steps of one
   PLAN.md activity. These — and any other scratch/working-memory files you create
   in the repo — are gitignored and free-form: store anything you need (logs,
   errors, notes), since they are never committed.

9. **Every PLAN.md item is committable when done.** Each plan item must leave the
   repo in a state that can be committed.

10. **Ask before committing.** Before each `git commit` (and before pushing), ask
    whether to commit — never commit or push unannounced. The user decides when.

11. **Check the current branch first.** At the start of a task and before every
    commit/push, run `git branch --show-current` and confirm you're on the intended
    feature branch (never `master`). The branch can change between turns — e.g. after
    a PR is merged — so never assume; verify.

12. **New file type → consider a linter.** Whenever a new kind or format of file
    first enters the repo (a new language, config/data format, etc.), evaluate
    whether a linter or validator exists for it and, if reasonable, add a
    `lint-<kind>` target wired into `make lint` (and CI). Record the decision —
    including a deliberate "no linter" — in PLAN.md.

13. **Don't embed one language inside another.** Never inline foreign-syntax
    content — a config-file body, another script, SQL, etc. — inside a heredoc or
    string literal of the host language. Author it as its own file of the proper
    type (so it can be read, diffed, and linted as that language) and have the host
    reference it: mount it, `source` it, or `sed`-substitute a committed template
    (`@TOKEN@` placeholders). Extends Rule 12. Writing a single literal value
    (`echo "x" > f`) is fine; a multi-line or structured fragment of another format
    is not.

14. **Least-privilege `GITHUB_TOKEN`.** Every GitHub Actions workflow declares an
    explicit top-level `permissions:` block granting only the scopes its steps use
    (default `permissions: contents: read`); grant any wider scope at the narrowest
    level (per-job) and only where a step needs it — never leave the token at the
    repository default. Re-audit whenever a workflow gains a step that writes (opens
    PRs, cuts releases, pushes packages/pages).

15. **Comments serve contributors and users, not the project's history.** A comment
    (and `--help`/`elog` text) states what the code does, how to use it, and any
    non-obvious *why* needed to maintain it safely (e.g. a workaround that must not be
    removed) — written for someone reading or using this project, who has no idea what
    our roadmap is. It does **not** narrate how the code came to be (no change log, no
    "we used to…", no PR storytelling) and contains **no references to PLAN.md, phase
    numbers, or rule numbers** — those are internal bookkeeping. The commit message and
    PLAN.md hold the history and the roadmap pointers; the code explains itself.

16. **License compliance (EUPL 1.2).** The project is licensed under the European
    Union Public Licence v. 1.2 (see `LICENSE` and `COPYRIGHT.md`), and the copyright
    holder must stay free to relicense it on request. Before adding any third-party
    dependency, runtime-invoked tool, or copied code, verify its licence is compatible
    with the EUPL 1.2 **and** does not obstruct relicensing; record any incompatibility
    and do not add it without explicit authorization. Keep `LICENSE`, `COPYRIGHT.md`,
    and `AUTHORS.md` accurate as the code, dependencies, and contributors change.

17. **Every commit carries a DCO sign-off.** Before each `git commit`, use `-s` (or
    an equivalent `Signed-off-by` trailer) and verify it landed with `git log -1`;
    before opening or updating a PR, confirm every commit on the branch is signed
    off. See `CONTRIBUTING.md` / `CLA.md` for the DCO 1.1 + CLA mechanism this
    supports.

18. **Comments stay local to what the file itself controls.** Don't describe, in
    one file, behaviour that another file actually decides (e.g. a Dockerfile
    shouldn't say how often CI triggers it — that's the workflow's job; a config
    file shouldn't restate a default owned by the code that reads it). Such a
    comment drifts out of sync the moment the owning file changes, and nothing
    forces the two edits to happen together. State only what this file itself does
    or decides; extends Rule 15.

19. **Keep the test matrix current.** Whenever a new OS/target, integration,
    environment, configuration, or installation method is implemented, add or
    update its row in `docs/TEST-MATRIX.md` in the same change — see PLAN.md
    Phase 6 / open decision 24. Every ✅ names the feature (Rule 21) the test
    verifies, so a reader can check the test against the promise rather than
    against the code. A `—` or a documented ❌ must be justified by something
    that **cannot exist** ("the Keychain has no daemon to freeze"), never by how
    the product behaves today: "has no observable effect" is what a defect looks
    like from the inside, and writing it into the matrix turns the defect into
    its own excuse.

20. **Exclude only genuinely untestable code from coverage.** Tag a block
    `//coverage:ignore` (with a plain-comment reason on the line above it,
    Rule 15) only when it cannot be unit-tested in principle — the process
    entry point's lone `os.Exit`, a platform stub with no observable
    behaviour. Never use it to hide code that is testable but not yet tested;
    when tempted, refactor the untestable part into a thin injectable seam and
    test the rest instead. `go-ignore-cov` (wired into `make test-json`) then
    counts the tagged block as covered so the reported percentage stays honest.

21. **Every user-visible behaviour is a documented feature.** `docs/FEATURES.md`
    is the authoritative catalogue of what SSHakku promises its users, each entry
    carrying a stable id and stated as an outcome the user can observe — not as a
    description of the implementation. Before implementing or changing behaviour,
    read the feature it belongs to and confirm the implementation still satisfies
    it; if the behaviour isn't in the catalogue yet, add it there **first**, in
    the same change. A promise that lives only in prose (a README paragraph, a
    commit message, someone's memory) cannot be referenced by a test, and what no
    test references is what silently stops working.

22. **Test the promise, not the implementation.** Unit tests verify that the code
    does what it says: they are written from the code and that is their job.
    Integration, acceptance and end-to-end tests verify that the *product does
    what it promises*: they must be derived from a feature in `docs/FEATURES.md`
    and from nothing else. Never write one by reading the implementation first — a
    test derived from the code can only ever agree with the code, including where
    the code is wrong, which is exactly how a broken feature keeps a green suite.
    State in the test which feature it verifies.

23. **Red before green.** A test must be observed **failing** before it is
    committed passing, and the commit message says how it was made to fail. For a
    bug: reproduce it first, then diagnose — never the reverse. For a feature:
    write the assertion against the promise, watch it fail, then implement. A test
    that has never been red proves only that it agrees with today's code. When a
    fix and its test land together, commit the failing test first so the pairing
    is visible in the history.

24. **Never stub the decision under test.** A test may replace dependencies
    *upstream* of what it asserts; it must never stub the very thing it exists to
    judge. Forcing `guiAvailable = true` inside a test of the askpass wiring, or
    substituting a fake secret backend in a test whose subject is the wallet, does
    not test that seam — it asserts the seam away. The same defect wearing a
    different hat: discarding an output stream whose content *is* the behaviour
    (`io.Discard` over a command whose job is to print). When the honest version
    of a test is hard to write, that difficulty is information about the product,
    not an obstacle to route around.

25. **"Verified" means the product ran.** Linters, unit tests and coverage verify
    that the code is internally consistent; they are never evidence that a feature
    works, and must not be reported as such. For any change to user-visible
    behaviour, verification means driving the **real binary** through the user's
    scenario in a disposable environment and observing the user-visible outcome —
    see the `verify-e2e` skill. Report what was actually exercised and what was
    not: "unit tests pass, the feature was not run end to end" is a useful,
    honest statement; "verified" without a run is not.

26. **Never claim a capability by negation.** A build tag that *provides* a
    platform's feature is named after the platform that has it (`_darwin.go`,
    `_linux.go`) — never after the absence of another (`_other.go`,
    `//go:build !linux`), because a negation silently claims every platform it
    does not exclude. `//go:build !linux` on the wallet table offered the macOS
    Keychain to FreeBSD and to Windows, which have none, so the wrong default
    arrived quietly instead of failing to build.

    The mirror image is correct and stays: a tag that reports a capability
    **absent** may be a negation, because the absence really is what every other
    platform has in common — `//go:build !linux` on a stub whose every method
    answers "no kernel keyring here" is true wherever it compiles.

    So: negation for "this is missing", the platform's own name for "this is
    how it works here". If no tag then covers the remainder, that is the right
    outcome — this project targets Linux and macOS, and a third platform failing
    to build says so out loud, where a wrong default would not. Applies to test
    files too.

    The cost is real: a tagged branch cannot be exercised from the other
    platform (see Rule 20). Contain it by putting only the *data* behind the tag
    — the table of what this platform has — and keeping the logic that reads it
    platform-neutral, taking that table as an argument, so both answers stay
    checkable from either machine.
