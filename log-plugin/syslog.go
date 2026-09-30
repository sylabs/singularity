// Copyright (c) 2020-2026, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package main

import (
	"fmt"
	"log"
	"log/syslog"
	"path"
	"text/template"
)

// TypeSyslog is the identification string for this type of logger.
const TypeSyslog = "syslog"

// SyslogOutput holds configuration and state for a single output of the logger.
// The plugin may be configured to use multiple outputs, each with their own configuration.
type SyslogOutput struct {
	config     LogOutputConfig
	configPath string
	template   *template.Template
}

// NewSysLogOutput creates a SyslogOutput configured for a particular destination.
func NewSyslogOutput(c LogOutputConfig, cPath string) (*SyslogOutput, error) {
	s := SyslogOutput{
		config:     c,
		configPath: cPath,
	}

	if c.Type != TypeSyslog {
		return nil, fmt.Errorf("invalid type %q != %q", c.Type, TypeSyslog)
	}

	if s.config.Template == "" {
		return nil, fmt.Errorf("a template file is required")
	}

	if path.IsAbs(s.config.Template) {
		return nil, fmt.Errorf("template file must be a relative path inside plugin directory")
	}

	tplFile := path.Join(s.configPath, s.config.Template)
	tpl, err := template.ParseFiles(tplFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing template file: %v", err)
	}

	s.template = tpl
	return &s, nil
}

// Log writes an syslog entry through this configured output and its template.
func (s SyslogOutput) Log(data LogData) error {
	logger, err := syslog.NewLogger(syslog.LOG_NOTICE, log.LstdFlags)
	if err != nil {
		return fmt.Errorf("could not create syslog logger: %v", err)
	}

	if err := s.template.Execute(logger.Writer(), data); err != nil {
		return fmt.Errorf("could not process log message: %v", err)
	}

	return nil
}
