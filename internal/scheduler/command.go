package scheduler

// Command represents a control signal sent by the Scheduler to ProbeWorkers.
type Command int

const (
	// CmdPause instructs the worker to finish its in-flight probe, send an Ack,
	// and pause executions until CmdResume is received.
	CmdPause Command = iota + 1

	// CmdResume instructs a paused worker to resume its regular scheduling loop.
	CmdResume

	// CmdStop instructs the worker to terminate its execution loop permanently.
	CmdStop
)

// String returns a human-readable representation of the Command.
func (c Command) String() string {
	switch c {
	case CmdPause:
		return "CmdPause"
	case CmdResume:
		return "CmdResume"
	case CmdStop:
		return "CmdStop"
	default:
		return "CmdUnknown"
	}
}
