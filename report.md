<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 100.0% | 29.9s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 100.0% | 62.8s | TestLoadKeysFirstTimePromptRealTerminal (12.17s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |
| windows | 97.3% | 78.2s | TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys (26.70s) | [HTML](https://orbintsoft.github.io/sshakku/report-windows.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-windows.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
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
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
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
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 89.7% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 90.1% |
| github.com/OrbintSoft/sshakku/internal/agent | 92.1% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 92.2% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 92.3% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 94.7% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 95.8% |
| github.com/OrbintSoft/sshakku/internal/install | 97.4% |
| github.com/OrbintSoft/sshakku/internal/sshtools | 97.7% |
| github.com/OrbintSoft/sshakku/internal/cli | 97.9% |
| github.com/OrbintSoft/sshakku/internal/keystate | 98.3% |
| github.com/OrbintSoft/sshakku/internal/keys | 98.4% |
| github.com/OrbintSoft/sshakku/internal/paths | 98.8% |
| github.com/OrbintSoft/sshakku/internal/config | 99.5% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/internal/logline | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.35 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 3.63 |
| TestLookForCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 2.08 |
| TestLookForCollection/a_wallet_that_stopped_answering | github.com/OrbintSoft/sshakku/internal/secretservice | 2.01 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.29 |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.26 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.21 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.01 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestPinentryPrompt | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestPinentryAvailable | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Store | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/which_pinentry_is_installed | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Delete | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 12.17 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 9.96 |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 9.56 |
| TestLoadKeysDismissedOnRealTerminalIsNotAFailure | github.com/OrbintSoft/sshakku/internal/keys | 8.36 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.51 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 3.32 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.40 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.23 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.09 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.07 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.05 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.03 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.03 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 0.80 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.61 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers | github.com/OrbintSoft/sshakku/internal/keys/wallet | 0.41 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestWaitingForAServiceEndsAtTheBoundItWasGiven | github.com/OrbintSoft/sshakku/internal/agent | 0.25 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 0.24 |
| TestAServiceComingUpIsWaitedForRatherThanStartedAgain | github.com/OrbintSoft/sshakku/internal/agent | 0.21 |

</details>

<details><summary>Slowest tests (windows)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys | github.com/OrbintSoft/sshakku/internal/install | 26.70 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal_that_hands_its_shell_a_command_of_its_own | github.com/OrbintSoft/sshakku/internal/install | 6.68 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_build_step_handed_a_command_at_a_console | github.com/OrbintSoft/sshakku/internal/install | 5.16 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_scheduled_job_handed_a_script_at_a_console | github.com/OrbintSoft/sshakku/internal/install | 4.99 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal's_own_shape,_told_that_nothing_may_ask | github.com/OrbintSoft/sshakku/internal/install | 4.95 |
| TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys/a_terminal's_own_shape,_driven_through_a_pipe | github.com/OrbintSoft/sshakku/internal/install | 4.92 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.51 |
| TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook | github.com/OrbintSoft/sshakku/internal/cli | 4.41 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.98 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 1.80 |
| TestAnUninstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.74 |
| TestAnInstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.73 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.25 |
| TestWithNoFileNamedTheShellIsAskedWhereItLooks | github.com/OrbintSoft/sshakku/internal/install | 1.20 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.07 |
| TestTheAgentsEndpointIsOpenedSoItsServerCannotActAsUs | github.com/OrbintSoft/sshakku/internal/agent/reach | 1.06 |
| TestAFileThatHeldNothingButTheWiringIsNotLeftBehind | github.com/OrbintSoft/sshakku/internal/cli | 1.04 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.04 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.04 |
| TestTheSearchListStepIsTakenUnlessItIsDeclined | github.com/OrbintSoft/sshakku/internal/install | 1.04 |

</details>
