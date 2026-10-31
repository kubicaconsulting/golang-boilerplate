// Command dependencies verifies the explicitly reviewed module graph.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
)

type module struct {
	Path     string  `json:"Path"`
	Version  string  `json:"Version"`
	Main     bool    `json:"Main"`
	Indirect bool    `json:"Indirect"`
	Replace  *module `json:"Replace"`
}

type approval struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	write := flag.Bool("write", false, "record graph after reviewing docs/dependencies.md")
	flag.Parse()
	command := exec.CommandContext(context.Background(), "go", "list", "-m", "-json", "all")
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	current := []approval{}
	for {
		var m module
		if decodeErr := decoder.Decode(&m); decodeErr == io.EOF {
			break
		} else if decodeErr != nil {
			return decodeErr
		}
		if m.Main {
			continue
		}
		if m.Replace != nil {
			return fmt.Errorf("unreviewed module replacement: %s", m.Path)
		}
		current = append(current, approval{m.Path, m.Version})
	}
	sort.Slice(current, func(i, j int) bool { return current[i].Path < current[j].Path })
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	const path = "docs/dependencies.lock.json"
	if *write {
		if err = os.WriteFile(path, data, 0600); err != nil {
			return err
		}
		fmt.Printf("Recorded %d modules. Review this diff and update docs/dependencies.md before committing.\n", len(current))
		return nil
	}
	approved, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(approved, data) {
		return errors.New("dependency graph changed: review provenance/maintenance " +
			"and run make deps-inventory; see docs/dependencies.md")
	}
	fmt.Printf("Dependency review inventory matches: %d pinned modules.\n", len(current))
	return nil
}
