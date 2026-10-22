package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
	"github.com/dtt101/doitdoit/styles"
	"github.com/dtt101/doitdoit/taskclient"
)

func runServiceTUI(origin string, cfg *config.Config) error {
	origin = strings.TrimSuffix(origin, "/")
	path, err := taskclient.CredentialPath(origin)
	if err != nil {
		return err
	}
	if _, err := taskclient.LoadCredentials(path); err != nil {
		return fmt.Errorf("run doitdoit service login --url %s first: %w", origin, err)
	}
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(path), "terminal.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another service TUI is using this recovery directory: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	client, err := taskclient.New(origin, nil, taskclient.CredentialSource(origin, path))
	if err != nil {
		return err
	}
	theme, err := styles.ResolveTheme(cfg.Theme)
	if err != nil {
		theme = styles.DefaultTheme()
	}
	styles.Apply(theme)
	m, err := model.NewServiceModel(client, origin, filepath.Join(filepath.Dir(path), "pending-command.json"))
	if err != nil {
		return err
	}
	program := tea.NewProgram(m)
	watchThemeReload(program)
	_, err = program.Run()
	return err
}
