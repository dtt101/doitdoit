package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/taskclient"
)

const productionService = "https://doitdoit-service.davidt-91d.workers.dev"

func RunServiceCommand(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "Usage: doitdoit service login|status|logout|use|use-json|export|import|retry|history|restore [flags]")
		return 1
	}
	command := args[0]
	flags := flag.NewFlagSet("service "+command, flag.ContinueOnError)
	flags.SetOutput(out)
	id := flags.String("id", "", "Task ID for history or restore")
	revision := flags.Int64("revision", 0, "Reviewed task revision for restore")
	origin := flags.String("url", "", "Task service origin")
	source := flags.String("file", "", "Source JSON or portable export for import")
	apply := flags.Bool("apply", false, "Publish a validated import; otherwise dry-run only")
	dryRun := flags.Bool("dry-run", false, "Plan and preserve the source without contacting the service")
	reviewed := flags.Bool("reviewed-recovery", false, "Recovery artifacts have been explicitly reviewed")
	output := flags.String("out", "", "Private export destination")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return 1
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if *origin == "" {
		*origin = cfg.ServiceOrigin
	}
	if *origin == "" {
		*origin = productionService
	}
	*origin = strings.TrimSuffix(*origin, "/")
	path, err := taskclient.CredentialPath(*origin)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	switch command {
	case "login":
		c, e := taskclient.Login(ctx, *origin, out, func(location string) error {
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
		err = e
		if err == nil {
			fmt.Fprintln(out, "Signed in to", *origin, "— credentials saved privately. Run service use to select this backend.")
		}
	case "status":
		c, e := taskclient.LoadCredentials(path)
		err = e
		if e == nil {
			fmt.Fprintf(out, "Service: %s\nBackend: %s\nAccess expires: %s\nRefresh requires login: %t\n", c.Origin, cfg.Backend, c.ExpiresAt.Format(time.RFC3339), c.RefreshUncertain)
		}
	case "logout":
		c, e := taskclient.LoadCredentials(path)
		if e == nil {
			e = taskclient.Logout(ctx, c)
		}
		if e == nil {
			e = os.Remove(path)
		}
		err = e
		if err == nil {
			fmt.Fprintln(out, "Service login revoked and local credentials removed.")
		}
	case "use":
		if _, e := taskclient.LoadCredentials(path); e != nil {
			err = fmt.Errorf("sign in first with service login: %w", e)
			break
		}
		cfg.Backend = "service"
		cfg.ServiceOrigin = *origin
		err = config.SaveConfig(cfg)
		if err == nil {
			fmt.Fprintln(out, "Service backend selected. Existing JSON and recovery files are untouched.")
		}
	case "use-json":
		cfg.Backend = "json"
		err = config.SaveConfig(cfg)
		if err == nil {
			fmt.Fprintln(out, "JSON backend selected; service data has not been exported or merged.")
		}
	case "import":
		if *apply && *dryRun {
			err = fmt.Errorf("choose --apply or --dry-run")
			break
		}
		err = runServiceImport(ctx, *origin, path, *source, *apply, *reviewed, out)
	case "retry":
		if *source == "" {
			err = fmt.Errorf("retry requires --file retained-command.json")
			break
		}
		retained, e := taskclient.LoadCommand(*source, *origin)
		if e != nil {
			err = e
			break
		}
		client, e := taskclient.New(*origin, nil, taskclient.CredentialSource(*origin, path))
		if e != nil {
			err = e
			break
		}
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
		client, e := taskclient.New(*origin, nil, taskclient.CredentialSource(*origin, path))
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
			retained := taskclient.Command{Origin: *origin, Action: "restore", ID: *id, Mutation: &mutation}
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
			for {
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
		c, e := taskclient.New(*origin, nil, taskclient.CredentialSource(*origin, path))
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
