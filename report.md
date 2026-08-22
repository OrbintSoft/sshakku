<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 99.9% | 32.8s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 99.9% | 58.2s | TestLoadKeysFirstTimePromptRealTerminal (10.30s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/install | 99.6% |
| github.com/OrbintSoft/sshakku/internal/cli | 99.8% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
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
| github.com/OrbintSoft/sshakku/internal/install | 99.5% |
| github.com/OrbintSoft/sshakku/internal/cli | 99.8% |
| github.com/OrbintSoft/sshakku/internal/cli/dialog | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/reach | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/backend | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/crossuser | 100.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/shell | 100.0% |
| github.com/OrbintSoft/sshakku/internal/cli/walletcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose/launcher | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/prompt | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys/handoff | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
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

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.36 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 4.05 |
| TestLookForCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 2.10 |
| TestLookForCollection/a_wallet_that_stopped_answering | github.com/OrbintSoft/sshakku/internal/secretservice | 2.01 |
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
| TestNoCommandBlocksIndefinitely/secret-tool_Store | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/graphical_passphrase_prompt_(zenity) | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Delete | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestPinentryPrompt/an_unanswered_dialog_does_not_strand_the_caller | github.com/OrbintSoft/sshakku/internal/keys/prompt | 0.30 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 10.30 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 9.51 |
| TestLoadKeysDismissedOnRealTerminalIsNotAFailure | github.com/OrbintSoft/sshakku/internal/keys | 9.01 |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 8.50 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/run | 4.81 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 2.39 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/run | 1.37 |
| TestExecRunnerRun/Stdin_is_fed_to_the_program | github.com/OrbintSoft/sshakku/internal/run | 1.17 |
| TestExecRunnerRun/Env_is_added_to_the_inherited_environment,_not_put_in_its_place | github.com/OrbintSoft/sshakku/internal/run | 1.12 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys/prompt | 1.09 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/run | 1.03 |
| TestExecRunnerRunStdinEnvAndStartFailure | github.com/OrbintSoft/sshakku/internal/keys | 1.02 |
| TestARealPowerShellAnswersAboutItself | github.com/OrbintSoft/sshakku/internal/install | 0.61 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers | github.com/OrbintSoft/sshakku/internal/keys/wallet | 0.41 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestWaitingForAServiceEndsAtTheBoundItWasGiven | github.com/OrbintSoft/sshakku/internal/agent | 0.26 |
| TestAServiceComingUpIsWaitedForRatherThanStartedAgain | github.com/OrbintSoft/sshakku/internal/agent | 0.20 |
| TestAssociateWaitsLongerThanAnOrdinaryExchange | github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire | 0.16 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |

</details>
