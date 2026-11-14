package cli

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/state"
)

const (
	exitRunFailed   = 2
	exitRunBlocked  = 3
	exitWaitTimeout = 4
)

func newWaitCommand() *cobra.Command {
	request := engine.WaitRequest{}
	var output format
	command := &cobra.Command{
		Use:   "wait RUN...",
		Short: "Block until runs stop and need someone",
		Long: `Run passes until every given run is waiting, done or failed, then print them.
With --any, return as soon as one of them is.

Exit codes: 0 the agents finished; 2 a run failed, or its agent ended in error
or is gone; 3 an agent is blocked on a question or a permission prompt;
4 the timeout passed first.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if request.Timeout < 0 {
				return errors.New("--timeout must not be negative")
			}
			s, err := openSession()
			if err != nil {
				return err
			}
			if request.Interval <= 0 {
				request.Interval = s.config.Interval
			}
			request.Refs = args
			runs, waitErr := s.engine.Wait(cmd.Context(), request)
			if waitErr != nil && !errors.Is(waitErr, engine.ErrWaitTimeout) {
				return waitErr
			}
			if output == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), runs); err != nil {
					return err
				}
			} else if err := writeRunTable(cmd.OutOrStdout(), runs, time.Now()); err != nil {
				return err
			}
			if waitErr != nil {
				return &exitError{code: exitWaitTimeout, message: waitErr.Error()}
			}
			if code := waitExitCode(runs); code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
	flags := command.Flags()
	flags.BoolVar(&request.Any, "any", false, "return when the first run stops")
	flags.DurationVar(&request.Timeout, "timeout", 0, "give up after this long (default: wait forever)")
	flags.DurationVar(&request.Interval, "interval", 0, "time between passes (default: the interval setting)")
	addOutputFlag(command, &output)
	return command
}

func waitExitCode(runs []state.Run) int {
	code := 0
	for _, run := range runs {
		switch {
		case run.Status == state.Failed:
			return exitRunFailed
		case run.Status != state.Waiting:
		case run.AgentState == state.AgentError || run.AgentState == state.AgentGone:
			return exitRunFailed
		case run.AgentState == state.AgentBlocked:
			code = exitRunBlocked
		}
	}
	return code
}
