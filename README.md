# coverage-reports

This branch is written by CI only — see `.github/workflows/test.yml`'s
`publish-coverage-report` job on `master`. Do not edit or merge it by hand;
its history is unrelated to `master`.

It holds, refreshed after every merge to `master`:

- `coverage-linux.json`, `coverage-macos.json` — shields.io endpoint badge
  data (rendered into the badges on `master`'s `README.md`).
- `report.md` — the latest per-OS coverage, wall-clock time, and slowest-test
  report.
