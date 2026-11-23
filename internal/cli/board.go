package cli

import (
	"bytes"
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/board"
	"github.com/glundgren93/muster/internal/state"
)

func newBoardCommand() *cobra.Command {
	var watch bool
	var interval time.Duration
	command := &cobra.Command{
		Use:   "board",
		Short: "Show runs as a kanban board you can drag them across",
		Long: `Show runs in columns by status: queued, running, waiting, done and failed.

Drag a card with the mouse, or select it and press H or L, to run the command
that moves it: Done marks it done, Running or Queued follows up on a waiting or
done run (asking for the text) or retries a failed one, and Running starts a
queued run. Waiting and Failed are reached only on their own.

The board asks the agents for their state every interval, like muster status.
With --watch it runs full passes, like muster watch, so queued runs start too.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			if interval == 0 {
				interval = s.config.Interval
			}
			if interval <= 0 {
				return errors.New("--interval must be positive")
			}
			return board.Run(cmd.Context(), s.engine, board.Options{
				Interval: interval,
				Watch:    watch,
				Describe: func(run state.Run) string {
					var b bytes.Buffer
					writeRun(&b, run, time.Now())
					return b.String()
				},
				Age: age,
				Now: time.Now,
			})
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "also start queued runs, like muster watch")
	command.Flags().DurationVar(&interval, "interval", 0, "time between agent checks (default: the interval setting)")
	return command
}
