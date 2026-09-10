package recordstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Archive is a read-only inventory, not evidence of a complete remote sync.
// Valid records and backups remain available alongside individual issues.
type Archive struct {
	Exists  bool
	Records []Record
	Backups map[string][]byte
	Issues  []Issue
}

// Inspect checks the entire owned namespace, including exact backup hashes.
// Incomplete staging files are ignored; unexpected entries are preserved/reported.
func (s Store) Inspect() Archive {
	a := Archive{Records: []Record{}, Backups: map[string][]byte{}, Issues: []Issue{}}
	if err := s.validatePaths(); err != nil {
		a.Issues = append(a.Issues, Issue{s.Root, err})
		return a
	}
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return a
	}
	if err != nil {
		a.Issues = append(a.Issues, Issue{s.Root, err})
		return a
	}
	a.Exists = true
	for _, entry := range entries {
		path := filepath.Join(s.Root, entry.Name())
		if entry.Name() != "records" && entry.Name() != "legacy" && entry.Name() != ".staging" {
			a.Issues = append(a.Issues, Issue{path, fmt.Errorf("unexpected store entry")})
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = fmt.Errorf("not a plain directory")
			}
			a.Issues = append(a.Issues, Issue{path, err})
			continue
		}
		switch entry.Name() {
		case "records":
			report := scan(path)
			a.Records = report.Records
			a.Issues = append(a.Issues, report.Issues...)
		case "legacy":
			files, err := os.ReadDir(path)
			if err != nil {
				a.Issues = append(a.Issues, Issue{path, err})
				continue
			}
			for _, file := range files {
				p := filepath.Join(path, file.Name())
				id := strings.TrimSuffix(file.Name(), ".json")
				if !validID(id) || file.Name() != id+".json" {
					a.Issues = append(a.Issues, Issue{p, fmt.Errorf("unexpected backup filename")})
					continue
				}
				raw, err := readPlain(p)
				if err == nil {
					if digest(raw) != id {
						err = fmt.Errorf("backup hash mismatch")
					} else {
						_, err = NormalizeLegacy(raw)
					}
				}
				if err != nil {
					a.Issues = append(a.Issues, Issue{p, err})
				} else {
					a.Backups[id] = raw
				}
			}
		}
	}
	return a
}
