// Copyright (c) 2022, Vanessa Sochat. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package parser

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sylabs/singularity/internal/pkg/test"
)

func TestDockerDefinitionFile(t *testing.T) {
	empty := make(map[string]string)
	definedLabels := map[string]string{
		"dinosaur": "vanessa",
		"peanut":   "butter",
	}
	longCommand := "command1command2 && command3command4 &&     command5"
	withEnv := "export pasta=macaroni || setenv pasta=macaroni\nexport topping=sauce || setenv topping=sauce"
	postWithEnv := withEnv + "\ncommand"

	// These tests are for non-multistage builds
	tests := []struct {
		name      string
		defPath   string
		success   bool
		files     int
		from      string
		post      string
		labels    map[string]string
		env       string
		runscript string
	}{
		{"Basic", "dockerfile_test/testdata_good/basic/Dockerfile", true, 0, "alpine", "apk update", empty, "", ""},
		{"Files", "dockerfile_test/testdata_good/files/Dockerfile", true, 2, "alpine", "", empty, "", ""},
		{"Labels", "dockerfile_test/testdata_good/labels/Dockerfile", true, 0, "alpine", "", definedLabels, "", ""},
		{"Runs", "dockerfile_test/testdata_good/runs/Dockerfile", true, 0, "alpine", longCommand, empty, "", ""},
		{"SingleFrom", "dockerfile_test/testdata_good/single_from/Dockerfile", true, 0, "scratch", "", empty, "", ""},
		{"Environment", "dockerfile_test/testdata_good/envs/Dockerfile", true, 0, "alpine", postWithEnv, empty, withEnv, ""},
		{"Entrypoint", "dockerfile_test/testdata_good/entrypoint/Dockerfile", true, 0, "alpine", "", empty, "", "/bin/bash"},
		{"Cmd", "dockerfile_test/testdata_good/cmd/Dockerfile", true, 0, "alpine", "", empty, "", "hello"},
		{"EntrypointCmd", "dockerfile_test/testdata_good/entrypoint_cmd/Dockerfile", true, 0, "alpine", "", empty, "", "special-executable do-all-the-things"},
	}
	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			defFile, err := os.Open(tt.defPath)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer defFile.Close()
			stages, err := All(defFile, tt.defPath)
			if err != nil && tt.success {
				t.Fatal("failed to parse definition file:", err)
			} else if err == nil && !tt.success {
				t.Fatal("parsing should have failed but error is nil")
			}
			if !tt.success {
				return
			}

			// The number of stages should be 1 for these tests
			if len(stages) != 1 {
				t.Fatalf("number of stages mismatch, expected 1 and found %d", len(stages))
			}

			stage := stages[0]

			// All of these should be docker bootstrap
			if stage.Header["bootstrap"] != "docker" {
				t.Fatalf("incorrect stage, expected docker and found %s", stage.Header["from"])
			}
			if stage.Header["from"] != tt.from {
				t.Fatalf("incorrect stage name, expected %s and found %s", tt.from, stage.Header["from"])
			}

			if len(stage.AppOrder) > 0 {
				t.Fatalf("found SCIF apps (but docker does support it)")
			}
			if tt.files != 0 && len(stage.BuildData.Files[0].Files) != tt.files {
				t.Fatalf("incorrect file count, found %d and expected %d", len(stage.BuildData.Files), tt.files)
			}

			// We only care about the build script
			post := strings.Trim(stage.BuildData.Scripts.Post.Script, "\n")
			if post != tt.post {
				t.Fatalf("stage 1 post script mismatch, found '%s' and expected '%s'", stage.BuildData.Scripts.Post.Script, tt.post)
			}
			labels := stage.ImageData.Labels
			if !reflect.DeepEqual(labels, tt.labels) {
				t.Fatalf("incorrect labels found %s", labels)
			}
			env := strings.Trim(stage.ImageData.ImageScripts.Environment.Script, "\n")
			if env != tt.env {
				t.Fatalf("stage 1 environment script mismatch, found '%s' and expected '%s'", env, tt.env)
			}
			runscript := strings.Trim(stage.ImageData.ImageScripts.Runscript.Script, " ")
			if runscript != tt.runscript {
				t.Fatalf("stage 1 runscript script mismatch, found '%s' and expected '%s'", runscript, tt.runscript)
			}
		}))
	}
}
