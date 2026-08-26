package agent

// ServiceStart is what a system's service manager will do when something asks
// for a service — the half of the answer that says whether asking is worth
// anything at all.
//
// What a service is doing and whether anything may start it are two separate
// questions there, and only the first is answered by asking its state: a
// disabled service reports itself stopped, exactly as one merely waiting to be
// asked for does. Told apart, they call for opposite advice — one is started by
// the next session, the other by nobody until somebody enables it.
type ServiceStart int

const (
	// ServiceStartUnknown is what nobody asked, or what a service manager
	// answered in a word this vocabulary has none of.
	ServiceStartUnknown ServiceStart = iota
	// ServiceStartAutomatic — the system starts it by itself.
	ServiceStartAutomatic
	// ServiceStartOnDemand — nothing starts it until something asks, and
	// asking works.
	ServiceStartOnDemand
	// ServiceStartDisabled — nothing may start it until somebody enables it.
	ServiceStartDisabled
)

// String names the answer for a report to print.
func (s ServiceStart) String() string {
	switch s {
	case ServiceStartAutomatic:
		return "starts automatically"
	case ServiceStartOnDemand:
		return "starts on demand"
	case ServiceStartDisabled:
		return "disabled"
	default:
		return "start type unknown"
	}
}

// ServiceReading is what a system's service manager was asked about the service
// its agent is served from, read without starting anything.
//
// It is a reading rather than a verdict: what to say about it, and what may be
// done about it, are decided by callers that run on every system, so both
// answers stay checkable from either machine. Only the taking of the reading
// belongs to the system that has a service manager to take it from.
type ServiceReading struct {
	// Name is the service, as this system's service manager knows it. It is
	// empty on a system that serves its agent from no service at all, and it
	// is known without asking anybody — so a reading that failed still says
	// what it was about.
	Name string
	// Running says whether the service is serving at this moment.
	Running bool
	// Start says whether anything may start it, which is what tells a service
	// waiting to be asked for from one nothing can start.
	Start ServiceStart
	// Err is what the service manager said in place of an answer — a service
	// that is not installed, an account that may not ask — already phrased as
	// something to act on, with the command that puts it right where there is
	// one. Whatever was learned before the refusal is still reported alongside.
	Err error
}

// ServedByAService reports whether this system serves its agent from a service
// at all. The name is what says so: nothing else in a reading tells "there is
// none" from "there is one and the asking failed", and a report prints the
// section on the strength of this.
func (r ServiceReading) ServedByAService() bool { return r.Name != "" }
