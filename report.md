<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test |
| --- | --- | --- | --- |
| linux | 73.0% | 7.7s | TestLoadKeysNoTerminalReturnsPromptly (1.29s) |
| macos | 66.4% | 15.0s | TestAddWithAskpassRealBinaryDarwin (5.54s) |

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.29 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.24 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.08 |
| TestClientCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 0.07 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.05 |
| TestClientItemsAttributesDelete | github.com/OrbintSoft/sshakku/internal/secretservice | 0.05 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.03 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.03 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.03 |
| TestClientCollection/an_existing_alias_is_returned_without_creating | github.com/OrbintSoft/sshakku/internal/secretservice | 0.02 |
| TestClientSearchCreateGetSecret | github.com/OrbintSoft/sshakku/internal/secretservice | 0.02 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 5.54 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.05 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.40 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.28 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.14 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.10 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.09 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestGivenUpExpiresAfterTTL | github.com/OrbintSoft/sshakku/internal/giveup | 0.02 |
| TestProcfsAncestryParent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.02 |
| TestRun/doctor_--user_unknown | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.01 |
| TestRun | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.01 |
| TestRecordAndGivenUp | github.com/OrbintSoft/sshakku/internal/giveup | 0.01 |
| TestExecRunnerRun/captures_stdout,_stderr,_and_exit_code | github.com/OrbintSoft/sshakku/internal/keys | 0.01 |

</details>
