<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 100.0% | 17.5s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 100.0% | 46.1s | TestLoadKeysDismissedOnRealTerminalIsNotAFailure (9.77s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |
| windows | 94.6% | 31.7s | TestExecRunnerRun (4.52s) | [HTML](https://orbintsoft.github.io/sshakku/report-windows.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-windows.html) |

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
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
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
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (windows)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 0.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 0.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 36.6% |
| github.com/OrbintSoft/sshakku/internal/keyring | 58.3% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 61.2% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 80.7% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 87.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 92.2% |
| github.com/OrbintSoft/sshakku/internal/install | 94.1% |
| github.com/OrbintSoft/sshakku/internal/agent | 94.2% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 94.3% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 94.8% |
| github.com/OrbintSoft/sshakku/internal/cli | 94.8% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 95.8% |
| github.com/OrbintSoft/sshakku/internal/keys | 98.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 98.3% |
| github.com/OrbintSoft/sshakku/internal/config | 98.7% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.35 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 3.02 |
| TestLookForCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 2.09 |
| TestLookForCollection/a_wallet_that_stopped_answering | github.com/OrbintSoft/sshakku/internal/secretservice | 2.01 |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.28 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.21 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.01 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestPinentryPrompt | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestPinentryAvailable | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestNoCommandBlocksIndefinitely/secret-tool_Delete | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/which_pinentry_is_installed | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Store | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/graphical_passphrase_prompt_(kdialog) | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysDismissedOnRealTerminalIsNotAFailure | github.com/OrbintSoft/sshakku/internal/keys | 9.77 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 9.33 |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 9.21 |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 8.80 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.58 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 1.26 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.22 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.16 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.08 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.06 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.04 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.02 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 0.76 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers | github.com/OrbintSoft/sshakku/internal/keys/wallet | 0.41 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestWaitingForAServiceEndsAtTheBoundItWasGiven | github.com/OrbintSoft/sshakku/internal/agent | 0.25 |
| TestAServiceComingUpIsWaitedForRatherThanStartedAgain | github.com/OrbintSoft/sshakku/internal/agent | 0.20 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 0.18 |
| TestAssociateWaitsLongerThanAnOrdinaryExchange | github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 0.17 |

</details>

<details><summary>Slowest tests (windows)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.52 |
| TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook | github.com/OrbintSoft/sshakku/internal/cli | 4.50 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 2.07 |
| TestAnInstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.91 |
| TestAnUninstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.87 |
| TestWithNoFileNamedTheShellIsAskedWhereItLooks | github.com/OrbintSoft/sshakku/internal/install | 1.54 |
| TestWiringAFileAndUnwiringItLeavesItAsItWasFound | github.com/OrbintSoft/sshakku/internal/install | 1.27 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.25 |
| TestUninstallingLeavesTheFileAsItWasFound | github.com/OrbintSoft/sshakku/internal/cli | 1.24 |
| TestTheSearchListStepIsTakenUnlessItIsDeclined | github.com/OrbintSoft/sshakku/internal/install | 1.21 |
| TestAFileThatHeldNothingButTheWiringIsNotLeftBehind | github.com/OrbintSoft/sshakku/internal/cli | 1.14 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.08 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.04 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.04 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.03 |
| TestInstallingTwiceLeavesOneWiring | github.com/OrbintSoft/sshakku/internal/install | 1.01 |
| TestTheTwoEditionsDoNotShareTheirProfiles | github.com/OrbintSoft/sshakku/internal/install | 0.91 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 0.81 |
| TestARealShellReadsOneLoginFileAndTheChoiceFollowsIt | github.com/OrbintSoft/sshakku/internal/install | 0.69 |
| TestOneMachineWiresAPowerShellAndAGitBashWithoutSwappingTheirFiles | github.com/OrbintSoft/sshakku/internal/cli | 0.64 |

</details>
