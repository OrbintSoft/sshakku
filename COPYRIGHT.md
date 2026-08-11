# Copyright

Copyright © 2026 Stefano Balzarotti (OrbintSoft) and contributors.

    Licensed under the EUPL

## Licence

This project is released under the **European Union Public Licence v. 1.2**
(`EUPL-1.2`). The full, authoritative text is in [LICENSE](LICENSE). The list of
contributors is in [AUTHORS.md](AUTHORS.md).

## Contributions

Contributors keep the copyright in their contributions. By contributing they
license their work to the public under the EUPL 1.2 and grant the copyright holder
a *non-exclusive* licence under the [Contributor License Agreement](CLA.md) — there
is no copyright assignment. See [CONTRIBUTING.md](CONTRIBUTING.md) for how this is
accepted (a `Signed-off-by` trailer certifying the [DCO](DCO.txt) and the CLA).

## Third-party components

Compiled into the distributed binary:

- **Go standard library** — BSD-3-Clause.
- **golang.org/x/sys** — BSD-3-Clause (Linux kernel keyring and syscall access).
- **github.com/BurntSushi/toml** — MIT (parsing the TOML config file).
- **github.com/godbus/dbus/v5** — BSD-2-Clause (native D-Bus Secret Service
  client, replacing the `secret-tool` shell-out for scoped collection
  lock/unlock).
- **golang.org/x/crypto** — BSD-3-Clause (NaCl box: the X25519 and
  XSalsa20-Poly1305 primitives KeePassXC's local protocol encrypts every
  message with).
- **github.com/ebitengine/purego** — Apache-2.0 (calling macOS's
  Security.framework and CoreFoundation, loaded at run time, so building for
  macOS needs no C toolchain).

Imported only by tests, never linked into the distributed binary:

- **go.uber.org/goleak** — MIT (goroutine-leak detection in the test suite).
- **github.com/stretchr/testify** — MIT (the `require` and `assert` assertions
  the test suite is written in), which brings with it
  **github.com/davecgh/go-spew** (ISC) and **github.com/pmezard/go-difflib**
  (BSD-3-Clause), used to render the difference between what a failing assertion
  expected and what it got, and **gopkg.in/yaml.v3** (MIT for the files ported
  from libyaml, Apache-2.0 for the rest).

`github.com/kr/text` (MIT) also appears in `go.mod` as an indirect requirement.
It is a test dependency of one of the modules above and is compiled into nothing
here — neither the binary nor this project's own tests.

These permissive licences (Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC and MIT) are
compatible with the EUPL 1.2 and with offering the project under additional
licences, so they do not obstruct relicensing. Apache-2.0 carries two conditions
the others do not — its notice and attribution requirements, met by this file,
and a patent grant that terminates for anyone who starts patent litigation over
the covered work. Neither restricts how this project may be licensed onward. It
is worth knowing, rather than discovering later, that Apache-2.0 cannot be
combined with GPLv2, one of the licences the EUPL's appendix would otherwise
allow this work to be relicensed to; GPLv3 and every other listed option are
unaffected.

Build- and CI-only tools (the Go toolchain and the linters) run as separate
processes, are neither bundled nor distributed, and impose no terms on the
software.

## Relicensing

The project's public release is, and will remain, under the EUPL 1.2. In addition,
the copyright holder may distribute the project under other licences — for example
a proprietary or OEM licence — alongside the public EUPL 1.2 release. The CLA's
non-exclusive grant covers contributors' work for this purpose, so no contribution
has to be removed or re-negotiated.

Preserving that freedom is a project rule: before any third-party code or
dependency is introduced, its licence is checked for compatibility with the
EUPL 1.2 and with the ability to offer the project under additional licences.
