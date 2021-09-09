// Copyright (c) 2019-2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package singularity

import (
	"fmt"
	"regexp"
	"strings"
)

// BindOption represents a bind option with its associated
// value if any.
type BindOption struct {
	Value string `json:"value,omitempty"`
}

const (
	flagOption  = true
	valueOption = false
)

// bindOptions is a map of option strings valid in bind specifications.
// If true, the option is a flag. If false, the option takes a value.
var bindOptions = map[string]bool{
	"ro":        flagOption,
	"rw":        flagOption,
	"image-src": valueOption,
	"id":        valueOption,
}

// BindPath stores a parsed bind path specification. Source and Destination
// paths are required.
type BindPath struct {
	Source      string                 `json:"source"`
	Destination string                 `json:"destination"`
	Options     map[string]*BindOption `json:"options"`
}

// ImageSrc returns the value of the option image-src for a BindPath, or an
// empty string if the option wasn't set.
func (b *BindPath) ImageSrc() string {
	if b.Options != nil && b.Options["image-src"] != nil {
		src := b.Options["image-src"].Value
		if src == "" {
			return "/"
		}
		return src
	}
	return ""
}

// ID returns the value of the option id for a BindPath, or an empty string if
// the option wasn't set.
func (b *BindPath) ID() string {
	if b.Options != nil && b.Options["id"] != nil {
		return b.Options["id"].Value
	}
	return ""
}

// Readonly returns true if the ro option was set for a BindPath.
func (b *BindPath) Readonly() bool {
	return b.Options != nil && b.Options["ro"] != nil
}

// ParseBindPath parses a string specifying one or more (comma separated) bind
// paths in src[:dst[:options]] format, and returns all encountered bind paths
// as a slice. Options may be simple flags, e.g. 'rw', or take a value, e.g.
// 'id=2'. Multiple options are separated with commas. Note that multiple binds
// are also separated with commas, so the logic must distinguish.
func ParseBindPath(bindpaths string) ([]BindPath, error) {
	var bind string
	var binds []BindPath
	var elem int

	// there is a better regular expression to handle
	// that directly without all the logic below ...
	// we need to parse various syntax:
	// source1
	// source1:destination1
	// source1:destination1:option1
	// source1:destination1:option1,option2
	// source1,source2
	// source1:destination1:option1,source2
	re := regexp.MustCompile(`([^,^:]+:?)`)

	// with the regex above we get string array:
	// - source1 -> [source1]
	// - source1:destination1 -> [source1:, destination1]
	// - source1:destination1:option1 -> [source1:, destination1:, option1]
	// - source1:destination1:option1,option2 -> [source1:, destination1:, option1, option2]
	for _, m := range re.FindAllString(bindpaths, -1) {
		s := strings.TrimSpace(m)
		isColon := bind != "" && bind[len(bind)-1] == ':'

		// options are taken only if the bind has a source
		// and a destination
		if elem == 2 {
			isOption := false

			for option, flag := range bindOptions {
				if flag {
					if s == option {
						isOption = true
						break
					}
				} else {
					if strings.HasPrefix(s, option+"=") {
						isOption = true
						break
					}
				}
			}
			if isOption {
				if !isColon {
					bind += ","
				}
				bind += s
				continue
			}
		} else if elem > 2 {
			return nil, fmt.Errorf("wrong bind syntax: %s", bind)
		}

		elem++

		if bind != "" {
			if isColon {
				bind += s
				continue
			}
			bp, err := newBindPath(bind)
			if err != nil {
				return nil, fmt.Errorf("while getting bind path: %s", err)
			}
			binds = append(binds, bp)
			elem = 1
		}
		// new bind path
		bind = s
	}

	if bind != "" {
		bp, err := newBindPath(bind)
		if err != nil {
			return nil, fmt.Errorf("while getting bind path: %s", err)
		}
		binds = append(binds, bp)
	}

	return binds, nil
}

// newBindPath returns BindPath record based on the provided bind
// string argument and ensures that the options are valid.
func newBindPath(bind string) (BindPath, error) {
	var bp BindPath

	splitted := strings.SplitN(bind, ":", 3)

	bp.Source = splitted[0]
	if bp.Source == "" {
		return bp, fmt.Errorf("empty bind source for bind path %q", bind)
	}

	bp.Destination = bp.Source

	if len(splitted) > 1 {
		bp.Destination = splitted[1]
	}

	if len(splitted) > 2 {
		bp.Options = make(map[string]*BindOption)

		for _, value := range strings.Split(splitted[2], ",") {
			valid := false
			for optName, isFlag := range bindOptions {
				if isFlag && optName == value {
					bp.Options[optName] = &BindOption{}
					valid = true
					break
				} else if strings.HasPrefix(value, optName+"=") {
					bp.Options[optName] = &BindOption{Value: value[len(optName+"="):]}
					valid = true
					break
				}
			}
			if !valid {
				return bp, fmt.Errorf("%s is not a valid bind option", value)
			}
		}
	}

	return bp, nil
}
