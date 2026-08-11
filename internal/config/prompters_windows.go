//go:build windows

package config

// platformGUIPrompters are the values gui_prompter accepts on this system. Only
// the two that name no program are here: this build draws no dialog of its own,
// so there is none for a user to ask for by name. "auto" therefore finds
// nothing and the passphrase is asked for wherever else it can be — see
// resolveGUIPrompterFrom.
var platformGUIPrompters = []string{
	GUIPrompterAuto,
	GUIPrompterNone,
}
