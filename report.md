<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 100.0% | 5.4s | TestLoadKeysNoTerminalReturnsPromptly (1.21s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 100.0% | 40.0s | TestLoadKeysFirstTimePromptRealTerminal (16.41s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 100.0% |
| github.com/OrbintSoft/sshakku/internal/agent | 100.0% |
| github.com/OrbintSoft/sshakku/internal/config | 100.0% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 100.0% |
| github.com/OrbintSoft/sshakku/internal/giveup | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keys | 100.0% |
| github.com/OrbintSoft/sshakku/internal/keystate | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.21 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.23 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestClientCallTimesOutWhenDaemonUnresponsive | github.com/OrbintSoft/sshakku/internal/secretservice | 0.18 |
| TestLogTrims | github.com/OrbintSoft/sshakku/internal/sessionlog | 0.15 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealGracefulStopRemovesSocket | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestClientCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 0.06 |
| TestGatherReport | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.05 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.05 |
| TestClientCallErrors | github.com/OrbintSoft/sshakku/internal/secretservice | 0.05 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysFirstTimePromptRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 16.41 |
| TestLoadKeysWrongPassphraseRealTerminal | github.com/OrbintSoft/sshakku/internal/keys | 14.70 |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 5.72 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.03 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.38 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.24 |
| TestTamperedEnvVarsHandledSafely | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.15 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.14 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestEnsureAgentRealGracefulStopRemovesSocket | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestDoctorCrossUser/successful_read_reports_the_target_session | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.12 |
| TestDoctorCrossUser | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.12 |
| TestGatherReport | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.11 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.11 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.09 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.09 |
| TestTamperedEnvVarsHandledSafely/SSH_AUTH_SOCK_points_at_a_dead_socket | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.09 |

</details>
