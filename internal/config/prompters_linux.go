//go:build linux

package config

// The dialogs a Linux session can be asked in. They are declared here because
// each is a Linux program: a build for another operating system has none of
// them to run, and a user there cannot choose one.
const (
	// GUIPrompterPinentry is GnuPG's own dialog, which draws with whichever
	// toolkit the distribution built it for.
	GUIPrompterPinentry = "pinentry"
	// GUIPrompterKDialog is KDE's dialog.
	GUIPrompterKDialog = "kdialog"
	// GUIPrompterZenity is GNOME's dialog.
	GUIPrompterZenity = "zenity"
)

// platformGUIPrompters are the values gui_prompter accepts on this system. A
// name outside this list cannot mean anything here — see resolveGUIPrompterFrom.
var platformGUIPrompters = []string{
	GUIPrompterAuto,
	GUIPrompterNone,
	GUIPrompterPinentry,
	GUIPrompterKDialog,
	GUIPrompterZenity,
}
