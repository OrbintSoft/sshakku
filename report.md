<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 98.8% | 48.9s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 98.7% | 105.2s | TestLoadKeysDismissedOnRealTerminalIsNotAFailure (13.99s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |
| windows | 97.6% | 88.2s | TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys (26.82s) | [HTML](https://orbintsoft.github.io/sshakku/report-windows.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-windows.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/keys/protect | 89.6% |
| github.com/OrbintSoft/sshakku/internal/cli | 95.2% |
| github.com/OrbintSoft/sshakku/internal/config | 96.3% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 99.7% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/move | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/logline | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sshtools | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/keys/protect | 89.6% |
| github.com/OrbintSoft/sshakku/internal/cli | 95.2% |
| github.com/OrbintSoft/sshakku/internal/config | 96.3% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 99.7% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/move | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/logline | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sshtools | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (windows)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 74.3% |
| github.com/OrbintSoft/sshakku/internal/keys/protect | 93.2% |
| github.com/OrbintSoft/sshakku/internal/cli | 93.7% |
| github.com/OrbintSoft/sshakku/internal/config | 96.0% |
| github.com/OrbintSoft/sshakku/internal/install | 97.4% |
| github.com/OrbintSoft/sshakku/internal/keys/move | 97.9% |
| github.com/OrbintSoft/sshakku/internal/keystate | 98.3% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 98.3% |
| github.com/OrbintSoft/sshakku/internal/keys | 98.5% |
| github.com/OrbintSoft/sshakku/internal/sshtools | 98.6% |
| github.com/OrbintSoft/sshakku/internal/paths | 98.9% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 99.7% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/logline | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestBrokerAsksWhenWhatItHoldsNoLongerOpensTheKey | github.com/OrbintSoft/sshakku/internal/keys | 5.45 |
| TestPassphraseVerdict | github.com/OrbintSoft/sshakku/internal/keys | 5.08 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.35 |
| TestBrokerKeepsAPassphraseThatOpenedTheKey | github.com/OrbintSoft/sshakku/internal/keys | 3.87 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 3.62 |
| TestBrokerDoesNotKeepAPassphraseThatOpenedNothing | github.com/OrbintSoft/sshakku/internal/keys | 3.62 |
| TestLookForCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 2.10 |
| TestLookForCollection/a_wallet_that_stopped_answering | github.com/OrbintSoft/sshakku/internal/secretservice | 2.01 |
| TestPassphraseVerdict/the_passphrase_that_locked_it | github.com/OrbintSoft/sshakku/internal/keys | 1.69 |
| TestPassphraseVerdict/a_passphrase_that_does_not | github.com/OrbintSoft/sshakku/internal/keys | 1.69 |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.26 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.21 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.17 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.01 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysDismissedOnRealTerminalIsNotAFailure | github.com/OrbintSoft/sshakku/internal/keys | 13.99 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 13.42 |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 13.35 |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 10.91 |
| TestBrokerAsksWhenWhatItHoldsNoLongerOpensTheKey | github.com/OrbintSoft/sshakku/internal/keys | 6.64 |
| TestBrokerDoesNotKeepAPassphraseThatOpenedNothing | github.com/OrbintSoft/sshakku/internal/keys | 6.03 |
| TestBrokerKeepsAPassphraseThatOpenedTheKey | github.com/OrbintSoft/sshakku/internal/keys | 5.98 |
| TestPassphraseVerdict | github.com/OrbintSoft/sshakku/internal/keys | 5.52 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 5.37 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.78 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 2.36 |
| TestPassphraseVerdict/the_passphrase_that_locked_it | github.com/OrbintSoft/sshakku/internal/keys | 1.94 |
| TestPassphraseVerdict/a_passphrase_that_does_not | github.com/OrbintSoft/sshakku/internal/keys | 1.83 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.59 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.38 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.15 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.14 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.14 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.08 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.05 |

</details>

<details><summary>Slowest tests (windows)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys | github.com/OrbintSoft/sshakku/internal/install | 26.82 |
| TestBrokerAsksWhenWhatItHoldsNoLongerOpensTheKey | github.com/OrbintSoft/sshakku/internal/keys | 6.57 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal_that_hands_its_shell_a_command_of_its_own | github.com/OrbintSoft/sshakku/internal/install | 6.51 |
| TestPassphraseVerdict | github.com/OrbintSoft/sshakku/internal/keys | 5.94 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_build_step_handed_a_command_at_a_console | github.com/OrbintSoft/sshakku/internal/install | 5.26 |
| TestBrokerDoesNotKeepAPassphraseThatOpenedNothing | github.com/OrbintSoft/sshakku/internal/keys | 5.17 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_scheduled_job_handed_a_script_at_a_console | github.com/OrbintSoft/sshakku/internal/install | 5.10 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal's_own_shape,_driven_through_a_pipe | github.com/OrbintSoft/sshakku/internal/install | 4.98 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal's_own_shape,_told_that_nothing_may_ask | github.com/OrbintSoft/sshakku/internal/install | 4.97 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 4.93 |
| TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook | github.com/OrbintSoft/sshakku/internal/cli | 4.81 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.52 |
| TestBrokerKeepsAPassphraseThatOpenedTheKey | github.com/OrbintSoft/sshakku/internal/keys | 4.18 |
| TestPassphraseVerdict/the_passphrase_that_locked_it | github.com/OrbintSoft/sshakku/internal/keys | 1.99 |
| TestPassphraseVerdict/a_passphrase_that_does_not | github.com/OrbintSoft/sshakku/internal/keys | 1.84 |
| TestTheShellLibraryAgreesByteForByte/a_profile_ending_in_the_block | github.com/OrbintSoft/sshakku/internal/install | 1.76 |
| TestAnUninstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.75 |
| TestAnInstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.72 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.71 |
| TestTheShellLibraryAgreesByteForByte/a_profile_that_already_has_the_block | github.com/OrbintSoft/sshakku/internal/install | 1.62 |

</details>
