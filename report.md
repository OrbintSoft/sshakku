<!-- sshakku:test-health-report -->
## Test health

| OS | Coverage | Wall time | Slowest test |
| --- | --- | --- | --- |
| linux | 73.0% | 6.6s | TestLoadKeysNoTerminalReturnsPromptly (1.31s) |
| macos | 66.4% | 14.7s | TestAddWithAskpassRealBinaryDarwin (5.17s) |

<details><summary>Slowest tests (linux)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestLoadKeysNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.31 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.01 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.31 |
| TestClientUnlockLock | github.com/OrbintSoft/sshakku/internal/secretservice | 0.24 |
| TestClientUnlockLock/a_hung_prompt_times_out_and_is_dismissed | github.com/OrbintSoft/sshakku/internal/secretservice | 0.21 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.20 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.12 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.09 |
| TestClientCollection | github.com/OrbintSoft/sshakku/internal/secretservice | 0.09 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.07 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestClientItemsAttributesDelete | github.com/OrbintSoft/sshakku/internal/secretservice | 0.06 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.04 |
| TestClientCollection/an_existing_alias_is_returned_without_creating | github.com/OrbintSoft/sshakku/internal/secretservice | 0.02 |
| TestClientItemsAttributesDelete/Items_and_ItemAttributes_reflect_what_was_created | github.com/OrbintSoft/sshakku/internal/secretservice | 0.02 |

</details>

<details><summary>Slowest tests (macos)</summary>

| Test | Package | Seconds |
| --- | --- | --- |
| TestAddWithAskpassRealBinaryDarwin | github.com/OrbintSoft/sshakku/internal/keys | 5.17 |
| TestReadTTYLineNoTerminalReturnsPromptly | github.com/OrbintSoft/sshakku/internal/keys | 1.16 |
| TestExecRunnerRun | github.com/OrbintSoft/sshakku/internal/keys | 0.41 |
| TestExecRunnerRun/zero_Timeout_does_not_bound_the_command | github.com/OrbintSoft/sshakku/internal/keys | 0.29 |
| TestFlockLockerSerialises | github.com/OrbintSoft/sshakku/internal/agent | 0.13 |
| TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID | github.com/OrbintSoft/sshakku/internal/agent | 0.12 |
| TestSocketHandoffExpiresUnclaimed | github.com/OrbintSoft/sshakku/internal/keys | 0.11 |
| TestEnsureAgentRealZombie | github.com/OrbintSoft/sshakku/internal/agent | 0.10 |
| TestDoctorDetectsAndFixesDeadOursAgent | github.com/OrbintSoft/sshakku/internal/diagnose | 0.10 |
| TestExecRunnerRun/a_positive_Timeout_kills_a_command_that_outlives_it | github.com/OrbintSoft/sshakku/internal/keys | 0.10 |
| TestEnsureAgentRealClean | github.com/OrbintSoft/sshakku/internal/agent | 0.07 |
| TestEnsureAgentRealReachableButEmptyIsHealthy | github.com/OrbintSoft/sshakku/internal/agent | 0.06 |
| TestEnsureAgentRealHealthyReuse | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestEnsureAgentRealForeignAdopted | github.com/OrbintSoft/sshakku/internal/agent | 0.05 |
| TestRun/doctor_--user_unknown | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.01 |
| TestRun | github.com/OrbintSoft/sshakku/cmd/sshakku | 0.01 |
| TestExecRunnerRun/captures_stdout,_stderr,_and_exit_code | github.com/OrbintSoft/sshakku/internal/keys | 0.01 |
| TestExecRunnerRun/a_command_that_finishes_within_its_Timeout_completes_normally | github.com/OrbintSoft/sshakku/internal/keys | 0.01 |
| TestKeyLifetime/empty_defaults | github.com/OrbintSoft/sshakku/internal/config | 0.00 |
| TestKeyLifetime/explicit_hours | github.com/OrbintSoft/sshakku/internal/config | 0.00 |

</details>
