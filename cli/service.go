package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/taskclient"
)

func RunServiceCommand(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "Usage: doitdoit service login|status|logout|export|import|retry|history|restore [flags]")
		return 1
	}
	command := args[0]
	flags := flag.NewFlagSet("service "+command, flag.ContinueOnError)
	flags.SetOutput(out)
	id := flags.String("id", "", "Task ID for history or restore")
	revision := flags.Int64("revision", 0, "Reviewed task revision for restore")
	origin := flags.String("url", "", "Task service origin")
	source := flags.String("file", "", "Portable service export file for import or retained command for retry")
	apply := flags.Bool("apply", false, "Publish a validated import; otherwise dry-run only")
	dryRun := flags.Bool("dry-run", false, "Plan and preserve the source without contacting the service")
	output := flags.String("out", "", "Private export destination")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(out, "Unexpected arguments: %v\n", flags.Args())
		return 1
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	resolvedOrigin := cfg.ResolvedServiceOrigin(*origin)
	path, err := taskclient.CredentialPath(resolvedOrigin)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	switch command {
	case "login":
		c, e := taskclient.Login(ctx, resolvedOrigin, out, func(location string) error {
			name := "xdg-open"
			if runtime.GOOS == "darwin" {
				name = "open"
			}
			cmd := exec.Command(name, location)
			if e := cmd.Start(); e != nil {
				return e
			}
			go cmd.Wait()
			return nil
		})
		if e == nil {
			e = taskclient.SaveCredentials(path, c)
		}
		if e == nil && cfg.ServiceOrigin != resolvedOrigin {
			cfg.ServiceOrigin = resolvedOrigin
			e = config.SaveConfig(cfg)
		}
		err = e
		if err == nil {
			fmt.Fprintln(out, "Signed in to", resolvedOrigin, "— credentials saved privately.")
		}
	case "status":
		c, e := taskclient.LoadCredentials(path)
		err = e
		if e == nil {
			fmt.Fprintf(out, "Service: %s\nAccess expires: %s\nRefresh requires login: %t\n", c.Origin, c.ExpiresAt.Format(time.RFC3339), c.RefreshUncertain)
		}
	case "logout":
		c, e := taskclient.LoadCredentials(path)
		if e == nil {
			e = taskclient.Logout(ctx, c)
		}
		if e == nil {
			e = taskclient.RemoveCredentials(path)
		}
		err = e
		if err == nil {
			fmt.Fprintln(out, "Service login revoked and local credentials removed.")
		}
	case "import":
		if *apply && *dryRun {
			err = fmt.Errorf("choose --apply or --dry-run")
			break
		}
		err = runServiceImport(ctx, resolvedOrigin, path, *source, *apply, out)
	case "retry":
		if *source == "" {
			err = fmt.Errorf("retry requires --file retained-command.json")
			break
		}
		retained, e := taskclient.LoadCommand(*source, resolvedOrigin)
		if e != nil {
			err = e
			break
		}
		client, e := taskclient.New(resolvedOrigin, nil, taskclient.CredentialSource(resolvedOrigin, path))
		if e != nil {
			err = e
			break
		}
		if retained.RelativeDate != "" {
			retained, err = client.ResolveCommand(ctx, retained)
			if err == nil {
				var data []byte
				data, err = retained.JSON()
				if err == nil {
					err = taskclient.WritePrivate(*source, data)
				}
			}
			if err != nil {
				break
			}
		}
		_, err = client.Execute(ctx, retained)
		if err == nil {
			err = os.Remove(*source)
			if err == nil {
				fmt.Fprintln(out, "Original operation acknowledged; retained command cleared.")
			} else {
				err = fmt.Errorf("original operation acknowledged; recovery cleanup failed: %w", err)
			}
		}
	case "history", "restore":
		if *id == "" {
			err = fmt.Errorf("--id is required")
			break
		}
		client, e := taskclient.New(resolvedOrigin, nil, taskclient.CredentialSource(resolvedOrigin, path))
		if e != nil {
			err = e
			break
		}
		if command == "restore" {
			if *revision < 1 {
				err = fmt.Errorf("restore requires the reviewed --revision")
				break
			}
			mutation := taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: *revision}
			retained := taskclient.Command{Origin: resolvedOrigin, Action: "restore", ID: *id, Mutation: &mutation}
			draft := filepath.Join(filepath.Dir(path), "restore-"+mutation.OperationID+".json")
			data, e := retained.JSON()
			if e == nil {
				e = taskclient.WritePrivate(draft, data)
			}
			if e != nil {
				err = fmt.Errorf("request not sent; cannot retain command: %w", e)
				break
			}
			_, err = client.Execute(ctx, retained)
			if err != nil {
				err = fmt.Errorf("not acknowledged; original command retained at %s: %w", draft, err)
			} else if e := os.Remove(draft); e != nil {
				err = fmt.Errorf("restored; recovery cleanup failed: %w", e)
			}
			if err == nil {
				fmt.Fprintln(out, "Task restored with its existing date and notes.")
			}
		} else {
			cursor := int64(0)
			const maxPages = 10000
			for pageNum := 0; ; pageNum++ {
				if pageNum >= maxPages {
					err = fmt.Errorf("task service: history page limit exceeded")
					break
				}
				page, e := client.History(ctx, *id, cursor)
				if e != nil {
					err = e
					break
				}
				encoded, _ := json.MarshalIndent(page, "", "  ")
				fmt.Fprintln(out, string(encoded))
				if page.NextCursor == nil {
					break
				}
				cursor = *page.NextCursor
			}
		}
	case "export":
		if *output == "" {
			err = fmt.Errorf("export requires --out FILE")
			break
		}
		c, e := taskclient.New(resolvedOrigin, nil, taskclient.CredentialSource(resolvedOrigin, path))
		if e != nil {
			err = e
			break
		}
		data, e := c.Export(ctx)
		if e != nil {
			err = e
			break
		}
		encoded, e := json.MarshalIndent(data, "", "  ")
		if e != nil {
			err = e
			break
		}
		err = taskclient.WritePrivateExclusive(*output, encoded)
		if err == nil {
			fmt.Fprintf(out, "Exported %d tasks (including deleted) at cursor %d.\n", len(data.Tasks), data.SnapshotCursor)
		}
	default:
		err = fmt.Errorf("unknown service command %q", command)
	}
	if err != nil {
		fmt.Fprintln(out, "Service command failed:", err)
		return 1
	}
	return 0
}
