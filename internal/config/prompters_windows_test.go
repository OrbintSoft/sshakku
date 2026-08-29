//go:build windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rule is checked from every platform against tables written out by hand
// (TestResolveGUIPrompterFrom). This checks the other half, which only this
// platform can: that the table this build actually carries is the one holding
// what this platform can draw.
func TestThisPlatformOffersTheBoxItCanDraw(t *testing.T) {
	got, err := resolveGUIPrompterFrom(new(GUIPrompterNative), platformGUIPrompters)
	require.NoError(t, err, "the box this platform draws must be one the user is allowed to name")
	assert.Equal(t, GUIPrompterNative, got, "and naming it must leave it in force")
}

func TestThisPlatformRefusesADialogItCannotDraw(t *testing.T) {
	got, err := resolveGUIPrompterFrom(new("osascript"), platformGUIPrompters)
	assert.Error(t, err, "a dialog belonging to another system can never come true here, so it is a mistake")
	assert.Equal(t, GUIPrompterAuto, got, "and what applies is auto, which leaves the user asked rather than unasked")
}
