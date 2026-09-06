<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 99.9% | 17.2s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 99.9% | 72.0s | TestLoadKeysFirstTimePromptRealTerminal (16.33s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |
| windows | 96.3% | 30.1s | TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook (8.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-windows.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-windows.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/sshtools | 97.5% |
| github.com/OrbintSoft/sshakku/internal/keys | 99.7% |
| github.com/OrbintSoft/sshakku/internal/cli | 99.8% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
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
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/sshtools | 97.5% |
| github.com/OrbintSoft/sshakku/internal/keys | 99.7% |
| github.com/OrbintSoft/sshakku/internal/cli | 99.8% |
| github.com/OrbintSoft/sshakku/internal/install | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
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
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (windows)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 81.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 89.7% |
| github.com/OrbintSoft/sshakku/internal/agent | 92.1% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 92.2% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 92.3% |
| github.com/OrbintSoft/sshakku/internal/install | 94.2% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet | 94.7% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 94.8% |
| github.com/OrbintSoft/sshakku/internal/cli | 94.9% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 95.8% |
| github.com/OrbintSoft/sshakku/internal/sshtools | 97.7% |
| github.com/OrbintSoft/sshakku/internal/keys | 98.1% |
| github.com/OrbintSoft/sshakku/internal/keystate | 98.3% |
| github.com/OrbintSoft/sshakku/internal/paths | 98.8% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/logline | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/platform | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run | 100.0% |
| github.com/OrbintSoft/sshakku/internal/run/runtest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testproc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/testtmp | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.36 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 3.18 |
| TestLookForCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 2.09 |
| TestLookForCollection/a_wallet_that_stopped_answering | github.com/OrbintSoft/sshakku/internal/secretservice | 2.01 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 2.01 |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.26 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.21 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.02 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.01 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.01 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.32 |
| TestPinentryPrompt | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestPinentryAvailable | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.31 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Delete | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Store | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 16.33 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 15.57 |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 15.26 |
| TestLoadKeysDismissedOnRealTerminalIsNotAFailure | github.com/OrbintSoft/sshakku/internal/keys | 13.56 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.69 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 2.49 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.36 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 1.31 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 1.20 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.14 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.13 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.05 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.03 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.03 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.61 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers | github.com/OrbintSoft/sshakku/internal/keys/wallet | 0.42 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 0.29 |
| TestWaitingForAServiceEndsAtTheBoundItWasGiven | github.com/OrbintSoft/sshakku/internal/agent | 0.26 |
| TestAServiceComingUpIsWaitedForRatherThanStartedAgain | github.com/OrbintSoft/sshakku/internal/agent | 0.20 |

</details>

<details><summary>Slowest tests (windows)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestTheShellYouNameIsTheOneWiredAndTheReportSaysWhereToLook | github.com/OrbintSoft/sshakku/internal/cli | 8.01 |
| TestWiringAFileAndUnwiringItLeavesItAsItWasFound | github.com/OrbintSoft/sshakku/internal/install | 6.18 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.61 |
| TestSSHAddCanStartWithWhatThisSystemSaysToGiveIt | github.com/OrbintSoft/sshakku/internal/keys | 2.35 |
| TestLogKeepsEveryLineWrittenConcurrently | github.com/OrbintSoft/sshakku/internal/sessionlog | 2.29 |
| TestARealShellReadsOneLoginFileAndTheChoiceFollowsIt | github.com/OrbintSoft/sshakku/internal/install | 1.67 |
| TestTheShellLibraryAgreesByteForByte | github.com/OrbintSoft/sshakku/internal/install | 1.47 |
| TestAnUninstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.32 |
| TestAnInstallThatCannotFinishSaysWhichStepStoppedIt | github.com/OrbintSoft/sshakku/internal/install | 1.30 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.27 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.08 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.08 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.05 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.04 |
| TestWithNoFileNamedTheShellIsAskedWhereItLooks | github.com/OrbintSoft/sshakku/internal/install | 0.94 |
| TestTheTwoEditionsDoNotShareTheirProfiles | github.com/OrbintSoft/sshakku/internal/install | 0.85 |
| TestTheSearchListStepIsTakenUnlessItIsDeclined | github.com/OrbintSoft/sshakku/internal/install | 0.80 |
| TestTheShellLibraryAgreesByteForByte/a_profile_with_lines_of_its_own | github.com/OrbintSoft/sshakku/internal/install | 0.61 |
| TestUninstallingLeavesTheFileAsItWasFound | github.com/OrbintSoft/sshakku/internal/cli | 0.60 |
| TestInstallingTwiceLeavesOneWiring | github.com/OrbintSoft/sshakku/internal/install | 0.60 |

</details>
