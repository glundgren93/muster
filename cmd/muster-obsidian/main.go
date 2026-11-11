package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/obsidian"
)

const dirEnv = "MUSTER_OBSIDIAN_DIR"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)
	root := &cobra.Command{
		Use:           "muster obsidian",
		Short:         "Use a folder of Obsidian notes as a muster task source",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.AddCommand(newSyncCommand(), newWatchCommand(), newImportBoardCommand())
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "muster obsidian: %v\n", err)
		os.Exit(1)
	}
}

func addDirFlag(command *cobra.Command, dir *string) {
	command.Flags().StringVarP(dir, "dir", "d", os.Getenv(dirEnv), "folder of task notes (default $"+dirEnv+")")
}

func syncer(dir string, cmd *cobra.Command) (obsidian.Syncer, error) {
	if dir == "" {
		return obsidian.Syncer{}, fmt.Errorf("give the folder of task notes with --dir or $%s", dirEnv)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return obsidian.Syncer{}, err
	}
	if !info.IsDir() {
		return obsidian.Syncer{}, fmt.Errorf("%s is not a directory", dir)
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return obsidian.Syncer{}, err
	}
	return obsidian.Syncer{Dir: absolute, Muster: obsidian.NewCommandMuster(), Log: cmd.OutOrStdout()}, nil
}

func reportErrors(cmd *cobra.Command, result obsidian.Result) error {
	for _, err := range result.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "muster obsidian: %v\n", err)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("%d note(s) could not be synced", len(result.Errors))
	}
	return nil
}

func newSyncCommand() *cobra.Command {
	var dir string
	command := &cobra.Command{
		Use:   "sync",
		Short: "Start, follow up and close runs from notes, and write run status back",
		Long: `Scan the folder for notes with a status property.

  status: now          start a run for the note; for a note that already has a run,
                       send its follow_up property as a follow-up, or retry a failed run
  status: done         mark the note's run done

Every note with a run gets status (the run's status) and run (the run id) written back.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := syncer(dir, cmd)
			if err != nil {
				return err
			}
			result, err := s.Sync()
			if err != nil {
				return err
			}
			return reportErrors(cmd, result)
		},
	}
	addDirFlag(command, &dir)
	return command
}

func newWatchCommand() *cobra.Command {
	var dir string
	var interval time.Duration
	command := &cobra.Command{
		Use:   "watch",
		Short: "Run muster tick and sync every interval until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := syncer(dir, cmd)
			if err != nil {
				return err
			}
			muster := obsidian.NewCommandMuster()
			fmt.Fprintf(cmd.ErrOrStderr(), "muster obsidian: watching %s every %s\n", s.Dir, interval)
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-timer.C:
				}
				stamp := time.Now().Format(time.TimeOnly)
				if err := muster.Tick(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s muster obsidian: %v\n", stamp, err)
				}
				result, err := s.Sync()
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s muster obsidian: %v\n", stamp, err)
				}
				reportErrors(cmd, result)
				timer.Reset(interval)
			}
		},
	}
	addDirFlag(command, &dir)
	command.Flags().DurationVar(&interval, "interval", 15*time.Second, "time between passes")
	return command
}

func newImportBoardCommand() *cobra.Command {
	var options obsidian.ConvertOptions
	command := &cobra.Command{
		Use:   "import-board BOARD.md --out DIR",
		Short: "Convert an Obsidian Kanban board into one task note per card",
		Long: `Convert a board made with the Kanban community plugin into task notes, one per card,
written to a new folder. The board itself is only read.

Each note gets status from its lane: the lane name in lower case with other characters
turned into -, done for a checked card. Cards in a lane called Now get status ready unless
--keep-now is given, so the first sync does not start them all. A card that is only a
[[link]] to a note in --vault gets that note's text as its body.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Out == "" {
				return errors.New("--out is required")
			}
			if entries, err := os.ReadDir(options.Out); err == nil && len(entries) > 0 {
				return fmt.Errorf("%s is not empty; give a new folder", options.Out)
			}
			content, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			cards, err := obsidian.ParseBoard(string(content))
			if err != nil {
				return err
			}
			converted, err := obsidian.ConvertBoard(cards, options)
			counts := map[string]int{}
			for _, note := range converted {
				counts[note.Status]++
			}
			for _, status := range slices.Sorted(maps.Keys(counts)) {
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s %d\n", status, counts[status])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %d note(s) to %s\n", len(converted), options.Out)
			return err
		},
	}
	command.Flags().StringVar(&options.Out, "out", "", "new folder for the task notes")
	command.Flags().StringVar(&options.Vault, "vault", "", "vault folder used to resolve [[links]] in cards")
	command.Flags().BoolVar(&options.KeepNow, "keep-now", false, "keep status now for cards in the Now lane")
	return command
}
