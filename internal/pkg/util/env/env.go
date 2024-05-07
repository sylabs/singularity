// Copyright (c) 2018-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package env

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sylabs/singularity/v4/internal/pkg/util/shell/interpreter"
)

var readonlyVars = map[string]bool{
	"EUID":   true,
	"GID":    true,
	"HOME":   true,
	"IFS":    true,
	"OPTIND": true,
	"PWD":    true,
	"UID":    true,
}

// SetFromList sets environment variables from environ argument list.
func SetFromList(environ []string) error {
	for _, env := range environ {
		splitted := strings.SplitN(env, "=", 2)
		if len(splitted) != 2 {
			return fmt.Errorf("can't process environment variable %s", env)
		}
		if err := os.Setenv(splitted[0], splitted[1]); err != nil {
			return err
		}
	}
	return nil
}

// SingularityEnvMap returns a map of SINGULARITYENV_ prefixed env vars to their values.
func SingularityEnvMap(hostEnv []string) map[string]string {
	singularityEnv := map[string]string{}

	for _, envVar := range hostEnv {
		if !strings.HasPrefix(envVar, SingularityEnvPrefix) {
			continue
		}
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], SingularityEnvPrefix)
		singularityEnv[key] = parts[1]
	}

	return singularityEnv
}

// FileMap returns a map of KEY=VAL env vars from an environment file f. The env
// file is shell evaluated using mvdan/sh with arguments and environment set
// from args and hostEnv.
func FileMap(ctx context.Context, f string, args []string, hostEnv []string) (map[string]string, error) {
	envMap := map[string]string{}

	content, err := os.ReadFile(f)
	if err != nil {
		return envMap, fmt.Errorf("could not read environment file %q: %w", f, err)
	}

	// Use the embedded shell interpreter to evaluate the env file, with an empty starting environment.
	// Shell takes care of comments, quoting etc. for us and keeps compatibility with native runtime.
	env, err := interpreter.EvaluateEnv(ctx, content, args, hostEnv)
	if err != nil {
		return envMap, fmt.Errorf("while processing %s: %w", f, err)
	}

	for _, envVar := range env {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 2 {
			continue
		}
		// Strip out the runtime env vars set by the shell interpreter so we
		// don't attempt to overwrite bash builtin readonly vars.
		// https://github.com/sylabs/singularity/issues/1263
		if _, ok := readonlyVars[parts[0]]; ok {
			continue
		}

		envMap[parts[0]] = parts[1]
	}

	return envMap, nil
}

// MergeMap merges two maps of environment variables, with values in b replacing
// values also set in a.
func MergeMap(a map[string]string, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}
