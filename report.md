<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 98.6% | 15.8s | TestNoCommandBlocksIndefinitely/GUI_detection_(xset) (10.01s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 98.8% | 48.4s | TestLoadKeysEmptyAnswerRealTerminal (12.15s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/config | 92.7% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 98.4% |
| github.com/OrbintSoft/sshakku/internal/keys | 98.9% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/config | 92.7% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 98.5% |
| github.com/OrbintSoft/sshakku/internal/keys | 99.8% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keepassxc | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestNoCommandBlocksIndefinitely/GUI_detection_(xset) | github.com/OrbintSoft/sshakku/internal/keys | 10.01 |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.26 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestPinentryPrompt | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestNoCommandBlocksIndefinitely/secret-tool_Store | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/graphical_passphrase_prompt_(kdialog) | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestNoCommandBlocksIndefinitely/secret-tool_Delete | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestPinentryPrompt/an_unanswered_dialog_does_not_strand_the_caller | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.24 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestAssociateWaitsLongerThanAnOrdinaryExchange | github.com/OrbintSoft/sshakku/internal/keepassxc | 0.16 |
| TestClientCallTimesOutWhenDaemonUnresponsive | github.com/OrbintSoft/sshakku/internal/secretservice | 0.16 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysEmptyAnswerRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 12.15 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 12.11 |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 11.89 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 4.46 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.14 |
| TestNoCommandBlocksIndefinitely/Bitwarden_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.60 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.49 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers | github.com/OrbintSoft/sshakku/internal/keys | 0.42 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.38 |
| TestNoCommandBlocksIndefinitely/1Password_Lookup | github.com/OrbintSoft/sshakku/internal/keys | 0.30 |
| TestTamperedEnvVarsHandledSafely | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.26 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.26 |
| TestAssociateWaitsLongerThanAnOrdinaryExchange | github.com/OrbintSoft/sshakku/internal/keepassxc | 0.16 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.15 |
| TestTamperedEnvVarsHandledSafely/SSH_AUTH_SOCK_points_at_a_dead_socket | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.15 |
| TestEnsureAgentRealGracefulStopRemovesSocket | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.13 |
| TestGatherReport | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.12 |
| TestKeychainGivesUpOnAKeychainThatNeverAnswers/List | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |

</details>
