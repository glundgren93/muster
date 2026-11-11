package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/layout"
	"github.com/glundgren93/muster/internal/state"
)

type startOutput struct {
	state.Run
	Created bool `json:"created"`
}

func newStartCommand() *cobra.Command {
	var request engine.StartRequest
	var promptFile string
	var output format
	command := &cobra.Command{
		Use:   "start [TEXT]",
		Short: "Create a run and dispatch it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := readPrompt(cmd.InOrStdin(), args, promptFile)
			if err != nil {
				return err
			}
			request.Prompt = prompt
			s, err := openSession()
			if err != nil {
				return err
			}
			result, startErr := s.engine.Start(cmd.Context(), request)
			if result.Run.ID == "" {
				return startErr
			}
			if !result.Created {
				fmt.Fprintf(cmd.ErrOrStderr(), "a run for source %s already exists\n", request.Source)
			}
			if output == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), startOutput{Run: result.Run, Created: result.Created}); err != nil {
					return err
				}
			} else if err := writeRunTable(cmd.OutOrStdout(), []state.Run{result.Run}, time.Now()); err != nil {
				return err
			}
			return startErr
		},
	}
	flags := command.Flags()
	flags.StringVarP(&promptFile, "file", "f", "", "read the prompt from `FILE` (- for stdin)")
	flags.StringVar(&request.Title, "title", "", "run title (default: first line of the prompt)")
	flags.StringVar(&request.ID, "id", "", "run id (default: a slug of the title)")
	flags.StringVar(&request.Source, "source", "", "opaque source reference; a second start with the same source changes nothing")
	flags.StringVar(&request.Model, "model", "", "model for the agent (skips pick_model)")
	flags.StringVar(&request.Workspace, "workspace", "", "existing `DIR` to work in (skips create_workspace)")
	flags.BoolVar(&request.Queue, "queue", false, "create the run only; leave dispatch to a later pass")
	addOutputFlag(command, &output)
	return command
}

func readPrompt(stdin io.Reader, args []string, file string) (string, error) {
	switch {
	case len(args) == 1 && file != "":
		return "", errors.New("give the prompt as an argument or with -f, not both")
	case len(args) == 1:
		return args[0], nil
	case file == "-":
		data, err := io.ReadAll(stdin)
		return string(data), err
	case file != "":
		data, err := os.ReadFile(file)
		return string(data), err
	}
	return "", errors.New("give the prompt as an argument or with -f FILE")
}

func newStatusCommand() *cobra.Command {
	var all, cached bool
	var output format
	command := &cobra.Command{
		Use:   "status [RUN]",
		Short: "Show runs, after checking their agents",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			if !cached {
				if _, err := s.engine.Reconcile(cmd.Context()); err != nil {
					return err
				}
			}
			if len(args) == 1 {
				run, err := s.engine.Find(args[0])
				if err != nil {
					return err
				}
				if output == formatJSON {
					return writeJSON(cmd.OutOrStdout(), run)
				}
				return writeRun(cmd.OutOrStdout(), run, time.Now())
			}
			runs, err := s.engine.Runs()
			if err != nil {
				return err
			}
			shown := []state.Run{}
			for _, run := range runs {
				if all || run.Status != state.Done {
					shown = append(shown, run)
				}
			}
			sortRuns(shown)
			if output == formatJSON {
				return writeJSON(cmd.OutOrStdout(), shown)
			}
			return writeRunTable(cmd.OutOrStdout(), shown, time.Now())
		},
	}
	command.Flags().BoolVarP(&all, "all", "a", false, "include done runs")
	command.Flags().BoolVar(&cached, "cached", false, "show stored state without asking agents")
	addOutputFlag(command, &output)
	return command
}

func newTickCommand() *cobra.Command {
	var output format
	command := &cobra.Command{
		Use:   "tick",
		Short: "Run one reconcile and dispatch pass",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			transitions, passErr := s.engine.Pass(cmd.Context())
			if transitions == nil {
				transitions = []engine.Transition{}
			}
			if output == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), transitions); err != nil {
					return err
				}
			} else if err := writeTransitions(cmd.OutOrStdout(), transitions); err != nil {
				return err
			}
			return passErr
		},
	}
	addOutputFlag(command, &output)
	return command
}

func newWatchCommand() *cobra.Command {
	var interval time.Duration
	command := &cobra.Command{
		Use:   "watch",
		Short: "Run passes until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			if interval <= 0 {
				interval = s.config.Interval
			}
			ctx := cmd.Context()
			fmt.Fprintf(cmd.ErrOrStderr(), "muster: watching %s every %s\n", s.dir.Root, interval)
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-timer.C:
				}
				transitions, err := s.engine.Pass(ctx)
				stamp := time.Now().Format(time.TimeOnly)
				for _, transition := range transitions {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", stamp, transition)
				}
				if err != nil && ctx.Err() == nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s muster: %v\n", stamp, err)
				}
				timer.Reset(interval)
			}
		},
	}
	command.Flags().DurationVar(&interval, "interval", 0, "time between passes (default: the interval setting)")
	return command
}

func newDoneCommand() *cobra.Command {
	return runCommand("done RUN", "Mark a run done", func(cmd *cobra.Command, s session, ref string) (state.Run, error) {
		return s.engine.Done(cmd.Context(), ref)
	})
}

func newRetryCommand() *cobra.Command {
	return runCommand("retry RUN", "Queue a failed run and dispatch", func(cmd *cobra.Command, s session, ref string) (state.Run, error) {
		return s.engine.Retry(cmd.Context(), ref)
	})
}

func runCommand(use, short string, act func(*cobra.Command, session, string) (state.Run, error)) *cobra.Command {
	var output format
	command := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			run, err := act(cmd, s, args[0])
			if err != nil {
				return err
			}
			return printRun(cmd, output, run)
		},
	}
	addOutputFlag(command, &output)
	return command
}

func printRun(cmd *cobra.Command, output format, run state.Run) error {
	if output == formatJSON {
		return writeJSON(cmd.OutOrStdout(), run)
	}
	return writeRunTable(cmd.OutOrStdout(), []state.Run{run}, time.Now())
}

func newRmCommand() *cobra.Command {
	var force bool
	var output format
	command := &cobra.Command{
		Use:   "rm RUN",
		Short: "Delete a run record and its prompt files",
		Long:  "Delete a run record and runs/<id>. The workspace and the agent are left alone.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			removed, err := s.engine.Remove(args[0], force)
			if removed.ID == "" {
				return err
			}
			if output == formatJSON {
				if err := writeJSON(cmd.OutOrStdout(), removed); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", removed.ID)
			}
			return err
		},
	}
	command.Flags().BoolVar(&force, "force", false, "remove a starting or running run")
	addOutputFlag(command, &output)
	return command
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the configuration, hooks and state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			problems := doctor(cmd.OutOrStdout())
			if problems > 0 {
				return fmt.Errorf("%d problem(s) found", problems)
			}
			return nil
		},
	}
}

func doctor(w io.Writer) int {
	problems := 0
	report := func(ok bool, format string, args ...any) {
		mark := "ok     "
		if !ok {
			mark = "problem"
			problems++
		}
		fmt.Fprintf(w, "%s  %s\n", mark, fmt.Sprintf(format, args...))
	}
	note := func(format string, args ...any) {
		fmt.Fprintf(w, "note     %s\n", fmt.Sprintf(format, args...))
	}
	dir, err := layout.Resolve()
	if err != nil {
		report(false, "%v", err)
		return problems
	}
	report(true, "muster directory %s", dir.Root)
	cfg, err := config.Load(dir.ConfigFile())
	if err != nil {
		report(false, "%v", err)
		return problems
	}
	if _, statErr := os.Stat(dir.ConfigFile()); statErr != nil {
		note("no %s; using defaults", dir.ConfigFile())
	} else {
		report(true, "config %s", dir.ConfigFile())
	}
	for _, name := range config.RequiredHooks {
		if _, configured := cfg.Hook(name); configured {
			report(true, "hook %s configured", name)
		} else {
			report(false, "hook %s is required but not configured", name)
		}
	}
	for _, optional := range []struct {
		hook    config.HookName
		command string
	}{{config.ReadAgent, "read"}, {config.SendAgent, "send"}} {
		if _, configured := cfg.Hook(optional.hook); !configured {
			note("hook %s is not configured, so muster %s will not work", optional.hook, optional.command)
		}
	}
	if cfg.MaxRunning == 0 {
		note("max_running is 0; no run will be dispatched")
	}
	file, err := state.NewStore(dir.StateFile(), dir.LockFile()).Load()
	if err != nil {
		report(false, "%v", err)
		return problems
	}
	report(true, "state %s with %d run(s)", dir.StateFile(), len(file.Runs))
	for _, run := range engine.StuckStarting(cfg, file.Runs, time.Now()) {
		report(false, "run %s has been starting since %s; the next pass marks it failed", run.ID, run.StartingAt.Local().Format(time.DateTime))
	}
	return problems
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), versionString())
		},
	}
}

func newReadCommand() *cobra.Command {
	var lines int
	command := &cobra.Command{
		Use:   "read RUN",
		Short: "Print the agent's recent output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lines <= 0 {
				return errors.New("--lines must be positive")
			}
			s, err := openSession()
			if err != nil {
				return err
			}
			output, err := s.engine.Read(cmd.Context(), args[0], lines)
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), output)
			return err
		},
	}
	command.Flags().IntVarP(&lines, "lines", "n", 80, "number of lines")
	return command
}

func newSendCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "send RUN MESSAGE",
		Short: "Send text to the live agent",
		Long:  "Send text to the live agent through send_agent. Refuses when the agent reports blocked, unless --force.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			return s.engine.Send(cmd.Context(), args[0], args[1], force)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "send even when the agent is blocked")
	return command
}

func newFollowCommand() *cobra.Command {
	var queue bool
	var output format
	command := &cobra.Command{
		Use:   "follow RUN MESSAGE",
		Short: "Add a follow-up and start a new agent session for it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openSession()
			if err != nil {
				return err
			}
			run, err := s.engine.Follow(cmd.Context(), args[0], args[1], queue)
			if err != nil {
				return err
			}
			return printRun(cmd, output, run)
		},
	}
	command.Flags().BoolVar(&queue, "queue", false, "queue the run only; leave dispatch to a later pass")
	addOutputFlag(command, &output)
	return command
}
