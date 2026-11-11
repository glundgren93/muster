package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type Store struct {
	statePath string
	lockPath  string
}

func NewStore(statePath, lockPath string) Store {
	return Store{statePath: statePath, lockPath: lockPath}
}

func (s Store) Load() (File, error) {
	data, err := os.ReadFile(s.statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Version: FileVersion, Runs: []Run{}}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("read state: %w", err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("parse state %s: %w", s.statePath, err)
	}
	if file.Version != FileVersion {
		return File{}, fmt.Errorf("state %s has version %d, expected %d", s.statePath, file.Version, FileVersion)
	}
	if file.Runs == nil {
		file.Runs = []Run{}
	}
	for i := range file.Runs {
		if file.Runs[i].FollowUps == nil {
			file.Runs[i].FollowUps = []FollowUp{}
		}
	}
	return file, nil
}

func (s Store) Update(mutate func(*File) error) error {
	lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open state lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	file, err := s.Load()
	if err != nil {
		return err
	}
	if err := mutate(&file); err != nil {
		return err
	}
	return s.save(file)
}

func (s Store) save(file File) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(s.statePath), ".state-*.json")
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := writeAndClose(temp, data); err != nil {
		os.Remove(temp.Name())
		return fmt.Errorf("save state: %w", err)
	}
	if err := os.Rename(temp.Name(), s.statePath); err != nil {
		os.Remove(temp.Name())
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

func writeAndClose(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
