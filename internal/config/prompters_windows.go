//go:build windows

package config

// GUIPrompterNative is the passphrase box SSHakku draws itself, with this
// system's own window calls. It is declared here because it is a Windows
// mechanism: no other system has one to name.
const GUIPrompterNative = "native"

// platformGUIPrompters are the values gui_prompter accepts on this system. A
// name outside this list cannot mean anything here — see resolveGUIPrompterFrom.
var platformGUIPrompters = []string{
	GUIPrompterAuto,
	GUIPrompterNone,
	GUIPrompterNative,
}
