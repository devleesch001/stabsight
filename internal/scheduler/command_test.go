package scheduler_test

import (
	"testing"

	"github.com/devleesch001/stabsight/internal/scheduler"
)

func TestCommandString(t *testing.T) {
	tests := []struct {
		cmd      scheduler.Command
		expected string
	}{
		{scheduler.CmdPause, "CmdPause"},
		{scheduler.CmdResume, "CmdResume"},
		{scheduler.CmdStop, "CmdStop"},
		{scheduler.Command(999), "CmdUnknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.cmd.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
