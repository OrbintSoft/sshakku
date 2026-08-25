package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// F55: the report names what the service manager said, so each answer is a
// word somebody reading the report can act on. The unknown one says it is
// unknown rather than borrowing the name of a state that was never reported —
// a report that prints "starts on demand" for an answer nobody read is worse
// than one that admits it did not find out.
func TestHowAServiceStartsIsNamedForAReportToPrint(t *testing.T) {
	for _, tc := range []struct {
		start ServiceStart
		named string
	}{
		{ServiceStartAutomatic, "starts automatically"},
		{ServiceStartOnDemand, "starts on demand"},
		{ServiceStartDisabled, "disabled"},
		{ServiceStartUnknown, "start type unknown"},
		{ServiceStart(42), "start type unknown"},
	} {
		assert.Equal(t, tc.named, tc.start.String(),
			"the answer a report prints for %d", int(tc.start))
	}
}

// A system that serves its agent from no service at all is the ordinary case
// on two of the three platforms, and the report must be able to tell it from a
// service it failed to read. The name is what separates them: nothing else in
// the reading distinguishes "there is none" from "there is one and nobody
// asked", and the report prints the section on the strength of it.
func TestAReadingWithNoServiceNamedIsNotAServiceAtAll(t *testing.T) {
	assert.False(t, ServiceReading{}.ServedByAService(),
		"a reading that names no service describes no service")
	assert.True(t, ServiceReading{Name: "ssh-agent"}.ServedByAService(),
		"a reading that names one describes one, whatever else it holds")
}
