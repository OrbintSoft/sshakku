//go:build darwin

package config

// GUIPrompterOsascript is the dialog macOS draws through its own scripting
// interpreter. It is declared here because it is a macOS mechanism: no other
// system has one to name.
const GUIPrompterOsascript = "osascript"

// platformGUIPrompters are the values gui_prompter accepts on this system. A
// name outside this list cannot mean anything here — see resolveGUIPrompterFrom.
var platformGUIPrompters = []string{
	GUIPrompterAuto,
	GUIPrompterNone,
	GUIPrompterOsascript,
}
