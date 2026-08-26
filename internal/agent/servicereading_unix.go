//go:build unix

package agent

import "context"

// ReadAgentService reports that this system serves its agent from no service.
//
// The agent here is a process this program starts on a socket it chose, so
// there is no service manager to ask and nothing for a report to print. That is
// an answer rather than a piece of work left undone, and it is returned as an
// empty reading with no error: failing to find a service that does not exist is
// not a failure.
func ReadAgentService(context.Context) ServiceReading { return ServiceReading{} }
