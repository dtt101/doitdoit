package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dtt101/doitdoit/taskclient"
	"github.com/dtt101/doitdoit/taskstore"
	"github.com/dtt101/doitdoit/tasktransfer"
)

func runServiceImport(ctx context.Context, origin, credentialPath, source string, apply, reviewed bool, out io.Writer) error {
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
	artifacts := []string{}
	if _, e := os.Stat(source + ".bak"); e == nil {
		artifacts = append(artifacts, source+".bak")
	}
	conflicts, e := taskstore.ConflictPaths(source)
	if e != nil {
		return e
	}
	artifacts = append(artifacts, conflicts...)
	if directory, e := taskstore.RecoveryDirectory(source); e == nil {
		entries, _ := os.ReadDir(directory)
		for _, entry := range entries {
			if !entry.IsDir() {
				artifacts = append(artifacts, filepath.Join(directory, entry.Name()))
			}
		}
	}
	needsReview := len(artifacts) > 0
	for i, path := range artifacts {
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		raw, e := io.ReadAll(io.LimitReader(f, 64<<20+1))
		f.Close()
		if e != nil {
			return e
		}
		if len(raw) > 64<<20 {
			return fmt.Errorf("recovery artifact too large: %s", path)
		}
		destination := filepath.Join(work, fmt.Sprintf("recovery-%03d-%s", i, filepath.Base(path)))
		if e := taskclient.WritePrivate(destination, raw); e != nil {
			return e
		}
		plan.Report.RecoveryPaths = append(plan.Report.RecoveryPaths, path)
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
		fmt.Fprintln(out, "Dry-run only. No requests sent and no task/configuration files changed.")
		return nil
	}
	if needsReview && !reviewed {
		return fmt.Errorf("recovery material was preserved; review it and pass --reviewed-recovery only after resolving which tasks to import")
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
	fmt.Fprintf(out, "Imported and field-verified %d tasks. Source and recovery artifacts retained; backend selection unchanged.\n", len(actual))
	return nil
}
