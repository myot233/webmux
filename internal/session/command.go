package session

import "os/exec"

// RunCommand protects a service-owned child from the adopted-process reaper
// until exec.Cmd has collected its exit status and finished pipe handling.
func RunCommand(cmd *exec.Cmd) error {
	if err := startChild(cmd); err != nil {
		return err
	}
	defer releaseLeader(cmd.Process.Pid)
	return cmd.Wait()
}
