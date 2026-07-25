<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |
| --- | --- | --- | --- | --- | --- |
| linux | 94.5% | 4.8s | TestLoadKeysNoTerminalReturnsPromptly (1.29s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-linux.html) |
| macos | 84.3% | 8.9s | TestAddWithAskpassRealBinaryDarwin (4.44s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) | [HTML](https://orbintsoft.github.io/sshakku/coverage-macos.html) |

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 83.9% |
| github.com/OrbintSoft/sshakku/internal/keystate | 87.8% |
| github.com/OrbintSoft/sshakku/internal/agent | 91.1% |
| github.com/OrbintSoft/sshakku/internal/config | 91.2% |
| github.com/OrbintSoft/sshakku/internal/giveup | 92.1% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 94.2% |
| github.com/OrbintSoft/sshakku/internal/keys | 99.7% |
| github.com/OrbintSoft/sshakku/internal/keyring | 100.0% |
| github.com/OrbintSoft/sshakku/internal/paths | 100.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 100.0% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/secretservice | 2.3% |
| github.com/OrbintSoft/sshakku/internal/keyring | 58.3% |
| github.com/OrbintSoft/sshakku/internal/keys | 85.4% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 85.6% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 86.9% |
| github.com/OrbintSoft/sshakku/internal/keystate | 87.8% |
| github.com/OrbintSoft/sshakku/internal/agent | 91.0% |
| github.com/OrbintSoft/sshakku/internal/config | 91.2% |
| github.com/OrbintSoft/sshakku/internal/giveup | 92.1% |
| github.com/OrbintSoft/sshakku/internal/paths | 97.4% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 100.0% |
| github.com/OrbintSoft/sshakku/tools/testreport | 100.0% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.29 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.25 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.09 |
| TestClientCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 0.09 |
| TestClientCallErrors | github.com/OrbintSoft/sshakku/internal/secretservice | 0.08 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.06 |
| TestClientItemsAttributesDelete | github.com/OrbintSoft/sshakku/internal/secretservice | 0.06 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestGatherReport | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.04 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 4.44 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.13 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.35 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.23 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.15 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.13 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.11 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.10 |
| TestDoctorCrossUser/successful_read_reports_the_target_session | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.08 |
| TestDoctorCrossUser | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.08 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestGatherReport | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.06 |
| TestLogTrims | github.com/OrbintSoft/sshakku/internal/sessionlog | 0.06 |
| TestManagerReap | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestRunRender | github.com/OrbintSoft/sshakku/tools/testreport | 0.03 |

</details>
