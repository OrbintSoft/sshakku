<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test | HTML report |
| --- | --- | --- | --- | --- |
| linux | 73.2% | 6.7s | TestLoadKeysNoTerminalReturnsPromptly (1.32s) | [HTML](https://orbintsoft.github.io/sshakku/report-linux.html) |
| macos | 66.7% | 13.9s | TestAddWithAskpassRealBinaryDarwin (5.27s) | [HTML](https://orbintsoft.github.io/sshakku/report-macos.html) |

[Full coverage report (latest on `master`)](https://github.com/OrbintSoft/sshakku/blob/coverage-reports/report.md)

<details><summary>Coverage by package (linux)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 34.0% |
| github.com/OrbintSoft/sshakku/internal/keyring | 41.2% |
| github.com/OrbintSoft/sshakku/internal/paths | 57.4% |
| github.com/OrbintSoft/sshakku/tools/testreport | 63.2% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 73.7% |
| github.com/OrbintSoft/sshakku/internal/keys | 75.1% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 82.4% |
| github.com/OrbintSoft/sshakku/internal/keystate | 87.8% |
| github.com/OrbintSoft/sshakku/internal/agent | 90.7% |
| github.com/OrbintSoft/sshakku/internal/config | 91.2% |
| github.com/OrbintSoft/sshakku/internal/giveup | 92.1% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 94.2% |

</details>

<details><summary>Coverage by package (macos)</summary>

| Package | Coverage |
| --- | --- |
| github.com/OrbintSoft/sshakku/internal/keyring | 0.0% |
| github.com/OrbintSoft/sshakku/internal/secretservice | 0.0% |
| github.com/OrbintSoft/sshakku/cmd/sshakku | 34.4% |
| github.com/OrbintSoft/sshakku/tools/testreport | 63.2% |
| github.com/OrbintSoft/sshakku/internal/keys | 69.8% |
| github.com/OrbintSoft/sshakku/internal/paths | 70.1% |
| github.com/OrbintSoft/sshakku/internal/sessionlog | 73.7% |
| github.com/OrbintSoft/sshakku/internal/diagnose | 86.9% |
| github.com/OrbintSoft/sshakku/internal/keystate | 87.8% |
| github.com/OrbintSoft/sshakku/internal/agent | 91.0% |
| github.com/OrbintSoft/sshakku/internal/config | 91.2% |
| github.com/OrbintSoft/sshakku/internal/giveup | 92.1% |

</details>

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.32 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.24 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestClientCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 0.13 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.11 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.10 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.08 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestClientItemsAttributesDelete | github.com/OrbintSoft/sshakku/internal/secretservice | 0.06 |
| TestClientCollection/an_existing_alias_is_returned_without_creating | github.com/OrbintSoft/sshakku/internal/secretservice | 0.05 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestClientCollection/falls_back_to_a_completed_prompt_when_the_alias_is_not_supported | github.com/OrbintSoft/sshakku/internal/secretservice | 0.02 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 5.27 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.04 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.34 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.23 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.13 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.11 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.11 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestRun | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.01 |
| TestManagerReap | github.com/OrbintSoft/sshakku/internal/agent | 0.01 |
| TestExecRunnerRun/captures_stdout,_stderr,_and_exit_code | github.com/OrbintSoft/sshakku/internal/keys | 0.01 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/keys | 0.01 |
| TestKeyLifetime/empty_defaults | github.com/OrbintSoft/sshakku/internal/config | 0.00 |
| TestKeyLifetime/explicit_hours | github.com/OrbintSoft/sshakku/internal/config | 0.00 |

</details>
