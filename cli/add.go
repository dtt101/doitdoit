package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
)

func RunAddCommand(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(out)
	when := flags.String("when", "today", "today, tomorrow, future, or YYYY-MM-DD")
	filePath := flags.String("file", "", "task JSON file (overrides config)")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: doitdoit add [--when <target>] [--file <path>] <title>")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	title := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if title == "" {
		flags.Usage()
		return 2
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(out, "Error loading config: %v\n", err)
		return 1
	}
	path := *filePath
	if path == "" {
		path = cfg.StoragePath
	}
	if path == "" {
		fmt.Fprintln(out, "No storage path configured; launch doitdoit once or pass --file")
		return 1
	}
	path, err = config.ExpandPath(path)
	if err != nil {
		fmt.Fprintf(out, "Error expanding path: %v\n", err)
		return 1
	}
	retention, _ := cfg.Retention()
	_, key, err := model.CaptureTask(path, title, *when, retention)
	if err != nil {
		fmt.Fprintf(out, "Error adding task: %v\n", err)
		return 1
	}
	label := key
	if key == "Future" {
		label = "Future"
	}
	fmt.Fprintf(out, "Added to %s: %s\n", label, title)
	return 0
}
