package command

import "fmt"

type ExitCode int

func (e ExitCode) Error() string {
	return fmt.Sprintf("exit code %d", e)
}
