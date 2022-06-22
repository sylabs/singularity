// Copyright (c) 2022, Vanessa Sochat. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package parser

import (
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/sylabs/singularity/pkg/build/types"
	"github.com/sylabs/singularity/pkg/sylog"
)

// Layer is a layer (and metadata) in a Dockerfile
type Layer struct {
	Cmd   string   // lowercased command name (ex: `from`)
	Flags []string // Any flags such as `--from=...` for `COPY`.
	Value []string // The contents of the command (ex: `ubuntu:xenial`)
}

type Dockerfile struct {
	sections   map[string]*types.Script
	stages     []types.Definition
	cmd        string
	entrypoint string

	// Single entities being worked on
	appOrder []string
	stage    types.Definition
	f        types.Files
}

func NewDockerfile() *Dockerfile {
	d := Dockerfile{}
	d.initSections()
	d.newStage()
	return &d
}

// newSections populates empty sections for the dockerfile parser
func (d *Dockerfile) initSections() {
	sections := make(map[string]*types.Script)
	for section := range validSections {
		sections[section] = &types.Script{}
	}
	d.sections = sections
}

// finish closes the stage (adding to stages) and calls next
func (d *Dockerfile) finish() error {
	// Environment needs to be added before post
	d.sections["post"].Script = d.sections["environment"].Script + "\n" + d.sections["post"].Script
	d.sections["runscript"].Script = d.entrypoint + " " + d.cmd

	// Prepare files as list
	files := []types.Files{}
	if len(d.f.Files) > 0 {
		files = append(files, d.f)
	}
	err := populateDefinition(d.sections, &files, &d.appOrder, &d.stage)
	if err != nil {
		return err
	}
	d.stages = append(d.stages, d.stage)

	// Reset stage and assets for next part
	d.next()
	return nil
}

// next creates an empty docker bootstrap header, and files
func (d *Dockerfile) next() {
	// Create new stage
	d.newStage()
	d.f = types.Files{}
	d.entrypoint = ""
	d.cmd = ""
}

// Prepare new stage for parsing
func (d *Dockerfile) newStage() {
	d.stage = types.Definition{
		Header: map[string]string{
			"bootstrap": "docker",
			"from":      "",
		},
	}
}

// addFrom adds a docker FROM statement
func (d *Dockerfile) addFrom(value []string) {
	d.stage.Header["from"] = strings.Join(value, " ")
}

// addRun adds a docker RUN statement
func (d *Dockerfile) addRun(value []string) {
	d.sections["post"].Script = d.sections["post"].Script + strings.Join(value, "\n")
}

// addCopy adds a docker COPY statement
func (d *Dockerfile) addCopy(value []string) {
	// We require a source and destination
	if len(value) < 2 {
		return
	}
	src := value[0]
	for i, dst := range value {

		// Skip first (source)
		if i == 0 {
			continue
		}
		line := src + " " + dst
		if line = strings.TrimSpace(line); line == "" || strings.Index(line, "#") == 0 {
			continue
		}
		src, dst = parseFileLine(line)
		d.f.Files = append(d.f.Files, types.FileTransport{Src: src, Dst: dst})
	}
}

// addLabel adds a docker LABEL statement
func (d *Dockerfile) addLabel(value []string) {
	// Must have label and value
	if len(value) != 2 {
		return
	}
	d.sections["labels"].Script += value[0] + " " + value[1] + "\n"
}

// addEntrypoint sets the entrypoint
func (d *Dockerfile) addEntrypoint(value []string) {
	d.entrypoint = strings.Join(value, " ")
}

// addCmd sets the command
func (d *Dockerfile) addCmd(value []string) {
	d.cmd = strings.Join(value, " ")
}

// addEnvironment adds ENV statements.
func (d *Dockerfile) addEnvironment(value []string) {
	// If only on value set, assume exporting to be unset
	if len(value) == 1 {
		value = append(value, "")
	}
	d.sections["environment"].Script += "\nexport " + value[0] + "=" + value[1] + " || setenv " + value[0] + "=" + value[1]
}

// ParseDockerfile parses a Dockerfile buffer into Singularity sections
func ParseDockerfile(spec string, raw []byte) ([]types.Definition, error) {
	// A Dockerfile parser holds stages, definitions, and files
	d := NewDockerfile()

	file, err := os.Open(spec)
	if err != nil {
		return d.stages, err
	}
	defer file.Close()

	// Parse with docker official parser!
	parsed, err := parser.Parse(file)
	if err != nil {
		return d.stages, fmt.Errorf("unable to parse Dockerfile: %s", err)
	}

	// Show warnings to the user
	for _, w := range parsed.Warnings {
		sylog.Warningf(w.Short + "\n")
	}

	for i, child := range parsed.AST.Children {

		// If we make it up here, section can't be added yet
		layer := Layer{
			Cmd:   child.Value,
			Flags: child.Flags,
		}
		for n := child.Next; n != nil; n = n.Next {
			layer.Value = append(layer.Value, n.Value)
		}

		// Populate appropriate sections
		if layer.Cmd == "FROM" {
			d.addFrom(layer.Value)
		} else if layer.Cmd == "RUN" {
			d.addRun(layer.Value)
		} else if layer.Cmd == "COPY" || layer.Cmd == "ADD" {
			d.addCopy(layer.Value)
		} else if layer.Cmd == "ENV" {
			d.addEnvironment(layer.Value)
		} else if layer.Cmd == "LABEL" {
			d.addLabel(layer.Value)
		} else if layer.Cmd == "ENTRYPOINT" {
			d.addEntrypoint(layer.Value)
		} else if layer.Cmd == "CMD" {
			d.addCmd(layer.Value)
		} else {
			sylog.Warningf("Section %s is not supported to convert.", layer.Cmd)
		}

		// If we hit another FROM, close current (and start new stage)
		if i > 0 && layer.Cmd == "FROM" {
			err = d.finish()
			if err != nil {
				return d.stages, err
			}
		}
	}

	// Close final stage
	err = d.finish()
	if err != nil {
		return d.stages, err
	}

	if len(d.stages) > 0 {
		d.stages[len(d.stages)-1].Raw = raw
	}
	return d.stages, nil
}
