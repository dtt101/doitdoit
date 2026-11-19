package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dtt101/doitdoit/taskclient"
	"github.com/dtt101/doitdoit/tasktransfer"
)

func runServiceImport(ctx context.Context, origin, credentialPath, source string, apply bool, out io.Writer) error {
	if source == "" {
		return fmt.Errorf("import requires --file SOURCE; defaults to dry-run, --apply publishes only after validation")
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
	file.Close()
	if err != nil {
		return err
	}
	plan, err := tasktransfer.Build(data)
	if err != nil {
		return err
	}
	work := filepath.Join(filepath.Dir(credentialPath), "imports", plan.Report.SourceSHA256)
	frozen := filepath.Join(work, "source.snapshot.json")
	if previous, e := os.ReadFile(frozen); e == nil && string(previous) != string(data) {
		return fmt.Errorf("frozen snapshot differs; preserve and review %s", frozen)
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	if err := taskclient.WritePrivate(frozen, data); err != nil {
		return err
	}
	report, err := json.MarshalIndent(plan.Report, "", "  ")
	if err != nil {
		return err
	}
	if err := taskclient.WritePrivate(filepath.Join(work, "report.json"), report); err != nil {
		return err
	}
	fmt.Fprintf(out, "Source frozen at %s\n", frozen)
	fmt.Fprintln(out, string(report))
	if len(plan.Report.Issues) > 0 {
		return fmt.Errorf("import blocked by %d source issues; review a copy of the source without changing the original", len(plan.Report.Issues))
	}
	if !apply {
		fmt.Fprintln(out, "Dry-run only. No requests sent and no configuration files changed.")
		return nil
	}
	client, err := taskclient.New(origin, nil, taskclient.CredentialSource(origin, credentialPath))
	if err != nil {
		return err
	}
	if plan.Timezone != "" {
		board, e := client.Board(ctx)
		if e != nil {
			return e
		}
		if board.Timezone != plan.Timezone {
			return fmt.Errorf("export timezone %s differs from owner timezone %s", plan.Timezone, board.Timezone)
		}
	}
	run, err := client.BeginImport(ctx, plan.Report.SourceSHA256, len(plan.Tasks))
	if err != nil {
		return err
	}
	if run.CommittedAt == nil {
		batch := []taskclient.Task{}
		size := 0
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			err := client.StageImport(ctx, run.ID, batch)
			batch = nil
			size = 0
			return err
		}
		for _, task := range plan.Tasks {
			raw, _ := json.Marshal(task)
			if len(batch) >= 25 || size+len(raw) > 200000 {
				if err := flush(); err != nil {
					return err
				}
			}
			batch = append(batch, task)
			size += len(raw) + 1
		}
		if err := flush(); err != nil {
			return err
		}
		if err := client.CommitImport(ctx, run.ID); err != nil {
			return err
		}
	}
	exported, err := client.Export(ctx)
	if err != nil {
		return fmt.Errorf("publication may have succeeded; retry this same source to verify: %w", err)
	}
	expected := map[string]bool{}
	for _, task := range plan.Tasks {
		expected[task.ID] = true
	}
	actual := []taskclient.Task{}
	for _, task := range exported.Tasks {
		if expected[task.ID] {
			actual = append(actual, task)
		}
	}
	if err := tasktransfer.Verify(plan.Tasks, actual); err != nil {
		return fmt.Errorf("published import needs review: %w", err)
	}
	result, _ := json.MarshalIndent(map[string]any{"run_id": run.ID, "source_sha256": plan.Report.SourceSHA256, "verified": true, "task_count": len(actual)}, "", "  ")
	if err := taskclient.WritePrivate(filepath.Join(work, "verified.json"), result); err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported and field-verified %d tasks.\n", len(actual))
	return nil
}
