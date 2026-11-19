package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/taskclient"
)

func RunAddCommand(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(out)
	service := flags.String("service", "", "task service origin (overrides configured service)")
	when := flags.String("when", "today", "today, tomorrow, future, or YYYY-MM-DD")
	notes := flags.String("notes", "", "plain-text notes for the task")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: doitdoit add [--when <target>] [--service <origin>] [--notes <text>] <title>")
		fmt.Fprintln(out, "\nAdd a task without opening the TUI. Put flags before the title.")
		fmt.Fprintln(out, "Titles can be quoted or supplied as multiple arguments. Notes preserve whitespace and line breaks.")
		fmt.Fprintln(out, "\nOptions:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
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
	return addServiceTask(cfg.ResolvedServiceOrigin(*service), title, *notes, *when, out)
}

func addServiceTask(origin, title, notes, when string, out io.Writer) int {
	origin = strings.TrimSuffix(origin, "/")
	path, err := taskclient.CredentialPath(origin)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	client, err := taskclient.New(origin, nil, taskclient.CredentialSource(origin, path))
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var date *string
	switch when {
	case "future":
	case "today", "tomorrow":
	default:
		parsed, e := time.Parse("2006-01-02", when)
		if e != nil || parsed.Format("2006-01-02") != when {
			fmt.Fprintln(out, "Use today, tomorrow, future or YYYY-MM-DD")
			return 2
		}
		date = &when
	}
	command := taskclient.Command{Origin: origin, Action: "create", Create: &taskclient.CreateRequest{OperationID: taskclient.OperationID(), Title: title, Notes: notes, ScheduledDate: date}}
	if when == "today" || when == "tomorrow" {
		now := time.Now()
		command.RelativeDate, command.CapturedAt = when, &now
	}
	draft := filepath.Join(filepath.Dir(path), "capture-"+command.Create.OperationID+".json")
	data, err := command.JSON()
	if err == nil {
		err = taskclient.WritePrivate(draft, data)
	}
	if err != nil {
		fmt.Fprintln(out, "Request not sent; cannot retain command:", err)
		return 1
	}
	command, err = client.ResolveCommand(ctx, command)
	if err == nil {
		data, err = command.JSON()
		if err == nil {
			err = taskclient.WritePrivate(draft, data)
		}
	}
	if err != nil {
		fmt.Fprintf(out, "Request not sent: %v. Capture retained at %s; use service retry --file to resume.\n", err, draft)
		return 1
	}
	task, err := client.Execute(ctx, command)
	if err != nil {
		fmt.Fprintf(out, "Not acknowledged: %v. Original command retained at %s\n", err, draft)
		return 1
	}
	if err := os.Remove(draft); err != nil {
		fmt.Fprintln(out, "Saved; draft cleanup failed:", err)
	}
	fmt.Fprintln(out, "Added to service:", task.Title)
	return 0
}
