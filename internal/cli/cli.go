package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/glundgren93/muster/internal/config"
	"github.com/glundgren93/muster/internal/engine"
	"github.com/glundgren93/muster/internal/layout"
	"github.com/glundgren93/muster/internal/plugin"
)

var version = "dev"

func Main(args []string) int {
	root := newRootCommand()
	if len(args) > 0 && isPluginName(root, args[0]) {
		if err := runPlugin(args[0], args[1:]); !errors.Is(err, errNoPlugin) {
			fmt.Fprintf(os.Stderr, "muster: %v\n", err)
			return 1
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "muster: interrupted")
			return 130
		}
		fmt.Fprintf(os.Stderr, "muster: %v\n", err)
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "muster",
		Short:         "Hand tasks to coding agents and track them until they need a human",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.AddCommand(
		newStartCommand(),
		newStatusCommand(),
		newTickCommand(),
		newWatchCommand(),
		newDoneCommand(),
		newRetryCommand(),
		newRmCommand(),
		newDoctorCommand(),
		newVersionCommand(),
	)
	return root
}

var errNoPlugin = errors.New("no plugin found")

func isPluginName(root *cobra.Command, name string) bool {
	if name == "" || name[0] == '-' || name == "help" {
		return false
	}
	for _, command := range root.Commands() {
		if command.Name() == name || slices.Contains(command.Aliases, name) {
			return false
		}
	}
	return true
}

func runPlugin(name string, args []string) error {
	dir, err := layout.Resolve()
	if err != nil {
		return err
	}
	path, found := plugin.Find(dir, name)
	if !found {
		return errNoPlugin
	}
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate muster executable: %w", err)
	}
	return plugin.Exec(dir, path, args, bin)
}

type session struct {
	dir    layout.Dir
	config config.Config
	engine *engine.Engine
}

func openSession() (session, error) {
	dir, err := layout.Resolve()
	if err != nil {
		return session{}, err
	}
	cfg, err := config.Load(dir.ConfigFile())
	if err != nil {
		return session{}, err
	}
	bin, err := os.Executable()
	if err != nil {
		return session{}, fmt.Errorf("locate muster executable: %w", err)
	}
	eng, err := engine.New(engine.Options{Dir: dir, Config: cfg, Bin: bin, Now: time.Now})
	if err != nil {
		return session{}, err
	}
	return session{dir: dir, config: cfg, engine: eng}, nil
}

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
