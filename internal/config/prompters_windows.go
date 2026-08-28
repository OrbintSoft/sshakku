//go:build windows

package config

// GUIPrompterPowerShell is the passphrase box a PowerShell host draws. It is
// declared here because it is a Windows mechanism: no other system has one to
// name.
const GUIPrompterPowerShell = "powershell"

// platformGUIPrompters are the values gui_prompter accepts on this system. A
// name outside this list cannot mean anything here — see resolveGUIPrompterFrom.
var platformGUIPrompters = []string{
	GUIPrompterAuto,
	GUIPrompterNone,
	GUIPrompterPowerShell,
}
