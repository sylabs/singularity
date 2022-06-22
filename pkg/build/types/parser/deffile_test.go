// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sylabs/singularity/internal/pkg/test"
	"github.com/sylabs/singularity/pkg/build/types"
)

func TestScanDefinitionFile(t *testing.T) {
	tests := []struct {
		name     string
		defPath  string
		sections string
	}{
		{"Arch", "deffile_test/testdata_good/arch/arch", "deffile_test/testdata_good/arch/arch_sections.json"},
		{"Apps", "deffile_test/testdata_good/apps/apps", "deffile_test/testdata_good/apps/apps_sections.json"},
		{"BusyBox", "deffile_test/testdata_good/busybox/busybox", "deffile_test/testdata_good/busybox/busybox_sections.json"},
		{"Debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap_sections.json"},
		{"Docker", "deffile_test/testdata_good/docker/docker", "deffile_test/testdata_good/docker/docker_sections.json"},
		{"Fingerprint", "deffile_test/testdata_good/fingerprint/fingerprint", "deffile_test/testdata_good/fingerprint/fingerprint_sections.json"},
		{"LocalImage", "deffile_test/testdata_good/localimage/localimage", "deffile_test/testdata_good/localimage/localimage_sections.json"},
		{"Scratch", "deffile_test/testdata_good/scratch/scratch", "deffile_test/testdata_good/scratch/scratch_sections.json"},
		// TODO(mem): reenable this; disabled while shub is down
		// {"Shub", "deffile_test/testdata_good/shub/shub", "deffile_test/testdata_good/shub/shub_sections.json"},
		{"Yum", "deffile_test/testdata_good/yum/yum", "deffile_test/testdata_good/yum/yum_sections.json"},
		{"Zypper", "deffile_test/testdata_good/zypper/zypper", "deffile_test/testdata_good/zypper/zypper_sections.json"},
		{"Zypper_SLE", "deffile_test/testdata_good/zypper_sle/zypper", "deffile_test/testdata_good/zypper_sle/zypper_sections.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			deffile := tt.defPath
			r, err := os.Open(deffile)
			if err != nil {
				t.Fatal("failed to read deffile:", err)
			}
			defer r.Close()

			s := bufio.NewScanner(r)
			s.Split(scanDefinitionFile)
			for s.Scan() && s.Text() == "" && s.Err() == nil {
			}

			b, err := ioutil.ReadFile(tt.sections)
			if err != nil {
				t.Fatal("failed to read JSON:", err)
			}

			type DefFileSections struct {
				Header string
			}
			var d []DefFileSections
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal("failed to unmarshal JSON:", err)
			}

			// Right now this only does the header, but the json files are
			// written with all of the sections in mind so that could be added.
			if s.Text() != d[0].Header {
				t.Fatal("scanDefinitionFile does not produce same header as reference")
			}
		}))
	}
}

// Specific test to cover some corner cases of splitToken()
//func TestSplitToken(t *testing.T) {
//	ident_str := "test test1"
//	content_str := "content1 content2"
//	str := "%%%%" + ident_str + "\n" + content_str
//	ident, content := splitToken(str)
//	if ident != ident_str || content != content_str {
//		t.Fatal("splitToken returned bad values")
//	}
//
//	str = "%%" + ident_str
//	ident, content = splitToken(str)
//	if ident != ident_str || content != "" {
//		t.Fatal("splitToken returned bad values")
//	}
//}

// Specific tests to cover some corner cases of parseTokenSection()
func TestParseTokenSection(t *testing.T) {
	// Fake map
	testMap := make(map[string]*types.Script)
	testMap["fakeKey1"] = &types.Script{Script: "%content1 content2 content3"}
	testMap["fakeKey2"] = &types.Script{Script: ""}

	// Incorrect token; map not used
	str := "test test1"
	myerr := parseTokenSection(str, nil, nil, nil)
	if myerr == nil {
		t.Fatal("test expected to fail but succeeded")
	}

	// Another incorrect token case; map not used
	myerr = parseTokenSection("apptest\ntest", nil, nil, nil)
	if myerr == nil {
		t.Fatal("test expected to fail but succeeded")
	}

	// Correct token
	appOrder := []string{}
	myerr = parseTokenSection("appenv apptest apptest2\ntest", testMap, nil, &appOrder)
	if myerr != nil {
		t.Fatal("error while parsing sections")
	}
	if testMap["appenv apptest"].Script != "test" {
		t.Fatal("returned map is invalid", testMap["appenv"].Script)
	}
}

// Specific tests to cover some corner cases of doSections()
func TestDoSections(t *testing.T) {
	// This is an string representing an invalid section, we make sure it is not identified as a header
	invalidStr := "%apptest\ntesttext"

	// This is a fake data structure
	myData := new(types.Definition)
	myData.Labels = make(map[string]string)

	s1 := bufio.NewScanner(strings.NewReader(invalidStr))
	s1.Split(scanDefinitionFile)

	// advance scanner until it returns a useful token
	for s1.Scan() && s1.Text() == "" {
		// Nothing to do
	}

	myerr := doSections(s1, myData)
	if myerr == nil {
		t.Fatal("Test passed while expected to fail")
	}

	// Now we define a valid first section but an invalid second section
	invalidStr = "%appenv apptest apptest2\ntest\n%appenv\ntest"
	s2 := bufio.NewScanner(strings.NewReader(invalidStr))
	s2.Split(scanDefinitionFile)

	// Advance the scanner until it returns a useful token
	for s2.Scan() && s2.Text() == "" {
		// Nothing to do
	}

	myerr = doSections(s2, myData)
	if myerr == nil {
		t.Fatal("Test passed while expected to fail")
	}
}

func TestParseDefinitionFile(t *testing.T) {
	tests := []struct {
		name     string
		defPath  string
		jsonPath string
	}{
		{"Arch", "deffile_test/testdata_good/arch/arch", "deffile_test/testdata_good/arch/arch.json"},
		{"Apps", "deffile_test/testdata_good/apps/apps", "deffile_test/testdata_good/apps/apps.json"},
		{"BusyBox", "deffile_test/testdata_good/busybox/busybox", "deffile_test/testdata_good/busybox/busybox.json"},
		{"Debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap.json"},
		{"Docker", "deffile_test/testdata_good/docker/docker", "deffile_test/testdata_good/docker/docker.json"},
		{"Fingerprint", "deffile_test/testdata_good/fingerprint/fingerprint", "deffile_test/testdata_good/fingerprint/fingerprint.json"},
		{"LocalImage", "deffile_test/testdata_good/localimage/localimage", "deffile_test/testdata_good/localimage/localimage.json"},
		{"Scratch", "deffile_test/testdata_good/scratch/scratch", "deffile_test/testdata_good/scratch/scratch.json"},
		// TODO(mem): reenable this; disabled while shub is down
		// {"Shub", "deffile_test/testdata_good/shub/shub", "deffile_test/testdata_good/shub/shub.json"},
		{"Yum", "deffile_test/testdata_good/yum/yum", "deffile_test/testdata_good/yum/yum.json"},
		{"Zypper", "deffile_test/testdata_good/zypper/zypper", "deffile_test/testdata_good/zypper/zypper.json"},
		{"Zypper_SLE", "deffile_test/testdata_good/zypper_sle/zypper", "deffile_test/testdata_good/zypper_sle/zypper.json"},
		{"NoHeader", "deffile_test/testdata_good/noheader/noheader", "deffile_test/testdata_good/noheader/noheader.json"},
		{"NoHeaderComments", "deffile_test/testdata_good/noheadercomments/noheadercomments", "deffile_test/testdata_good/noheadercomments/noheadercomments.json"},
		{"NoHeaderWhiteSpace", "deffile_test/testdata_good/noheaderwhitespace/noheaderwhitespace", "deffile_test/testdata_good/noheaderwhitespace/noheaderwhitespace.json"},
		{"MultipleScripts", "deffile_test/testdata_good/multiplescripts/multiplescripts", "deffile_test/testdata_good/multiplescripts/multiplescripts.json"},
		{"SectionArgs", "deffile_test/testdata_good/sectionargs/sectionargs", "deffile_test/testdata_good/sectionargs/sectionargs.json"},
		{"MultipleFiles", "deffile_test/testdata_good/multiplefiles/multiplefiles", "deffile_test/testdata_good/multiplefiles/multiplefiles.json"},
		{"QuotedFiles", "deffile_test/testdata_good/quotedfiles/quotedfiles", "deffile_test/testdata_good/quotedfiles/quotedfiles.json"},
		{"Shebang", "deffile_test/testdata_good/shebang/shebang", "deffile_test/testdata_good/shebang/shebang.json"},
		{"ShebangTest", "deffile_test/testdata_good/shebang_test/shebang_test", "deffile_test/testdata_good/shebang_test/shebang_test.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			defFile, err := os.Open(tt.defPath)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer defFile.Close()

			jsonFile, err := os.OpenFile(tt.jsonPath, os.O_RDWR, 0o644)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer jsonFile.Close()

			defTest, err := ParseDefinitionFile(defFile)
			if err != nil {
				t.Fatal("failed to parse definition file:", err)
			}

			var defCorrect types.Definition
			if err := json.NewDecoder(jsonFile).Decode(&defCorrect); err != nil {
				t.Fatal("failed to parse JSON:", err)
			}

			if !reflect.DeepEqual(defTest, defCorrect) {
				b, _ := json.MarshalIndent(defCorrect, "", "  ")
				t.Logf("Expected:\n%s", string(b))
				b, _ = json.MarshalIndent(defTest, "", "  ")
				t.Logf("Got:\n%s", string(b))
				t.Fatal("parsed definition did not match reference")
			}
		}))
	}
}

func TestParseDefinitionFileFailure(t *testing.T) {
	tests := []struct {
		name    string
		defPath string
	}{
		{"BadSection", "deffile_test/testdata_bad/bad_section"},
		{"JSONInput1", "deffile_test/testdata_bad/json_input_1"},
		{"JSONInput2", "deffile_test/testdata_bad/json_input_2"},
		{"Empty", "deffile_test/testdata_bad/empty"},
		{"EmptyComments", "deffile_test/testdata_bad/emptycomments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			defFile, err := os.Open(tt.defPath)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer defFile.Close()

			if _, err = ParseDefinitionFile(defFile); err == nil {
				t.Fatal("unexpected success parsing definition file")
			}
		}))
	}
}

// Specific tests to cover some corner cases of IsInvalidSectionError()
func TestIsInvalidSectionErrors(t *testing.T) {
	// Test of IsInvalidSectionError()
	dummyKeys := []string{"dummy_key1", "dummy_key2"}
	myValidErr1 := &InvalidSectionError{dummyKeys, errInvalidSection}
	myValidErr2 := &InvalidSectionError{dummyKeys, errEmptyDefinition}
	myInvalidErr := errors.New("My dummy error")
	if IsInvalidSectionError(myValidErr1) == false ||
		IsInvalidSectionError(myValidErr2) == false ||
		IsInvalidSectionError(myInvalidErr) == true {
		t.Fatal("unexpecter return value for IsInvalidSectionError()")
	}

	// Test of Error()
	expectedStr1 := "invalid section(s) specified: " + strings.Join(dummyKeys, ", ")
	expectedStr2 := "Empty definition file: " + strings.Join(dummyKeys, ", ")
	if myValidErr1.Error() != expectedStr1 || myValidErr2.Error() != expectedStr2 {
		t.Fatal("unexpecter result from Error()", myValidErr1.Error())
	}
}

// Specific tests to cover some corner cases of PopulateDefinition()
func TestPopulateDefinition(t *testing.T) {
	//
	// Some variables used throughout the tests
	//

	// We use a specific set of section names to reach some corner cases
	testMap := make(map[string]*types.Script)
	testMap["files"] = &types.Script{Script: "file1 file2"}
	testMap["labels"] = &types.Script{Script: "label1"}
	testFiles := []types.Files{
		{
			Files: []types.FileTransport{
				{Src: "file1", Dst: "file2"},
			},
		},
	}

	emptyMap := make(map[string]*types.Script)
	emptyFiles := []types.Files{}
	emptyAppOrder := []string{}

	//
	// Test with invalid data
	//
	invalidData := new(types.Definition)
	invalidData.Labels = make(map[string]string)
	populateDefinition(emptyMap, &emptyFiles, &emptyAppOrder, invalidData)

	//
	// Test with very specific maps
	//

	// A structure to store results (not really relevant here)
	myData := new(types.Definition)
	myData.Labels = make(map[string]string)

	myerr := populateDefinition(testMap, &testFiles, &emptyAppOrder, myData)
	if myerr != nil {
		t.Fatal("Test failed while testing populateDefinition()")
	}
}

// Specific tests to cover some corners cases of doHeader()
func TestDoHeader(t *testing.T) {
	invalidHeaders := []string{"headerTest", "headerTest: invalid"}
	myData := new(types.Definition)
	myData.Labels = make(map[string]string)

	for _, invalidHeader := range invalidHeaders {
		myerr := doHeader(invalidHeader, myData)
		if myerr == nil {
			t.Fatal("Test succeeded while supposed to fail")
		}
	}
}

func TestIsValidDefinition(t *testing.T) {
	//
	// Test with a bunch of valid files
	//
	validTests := []struct {
		name     string
		defPath  string
		sections string
	}{
		{"Arch", "deffile_test/testdata_good/arch/arch", "deffile_test/testdata_good/arch/arch_sections.json"},
		{"BusyBox", "deffile_test/testdata_good/busybox/busybox", "deffile_test/testdata_good/busybox/busybox_sections.json"},
		{"Debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap", "deffile_test/testdata_good/debootstrap/debootstrap_sections.json"},
		{"Docker", "deffile_test/testdata_good/docker/docker", "deffile_test/testdata_good/docker/docker_sections.json"},
		{"LocalImage", "deffile_test/testdata_good/localimage/localimage", "deffile_test/testdata_good/localimage/localimage_sections.json"},
		{"Scratch", "deffile_test/testdata_good/scratch/scratch", "deffile_test/testdata_good/scratch/scratch_sections.json"},
		// TODO(mem): reenable this; disabled while shub is down
		// {"Shub", "deffile_test/testdata_good/shub/shub", "deffile_test/testdata_good/shub/shub_sections.json"},
		{"Yum", "deffile_test/testdata_good/yum/yum", "deffile_test/testdata_good/yum/yum_sections.json"},
		{"Zypper", "deffile_test/testdata_good/zypper/zypper", "deffile_test/testdata_good/zypper/zypper_sections.json"},
	}

	for _, tt := range validTests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			deffile := tt.defPath

			valid, err := IsValidDefinition(deffile)
			if valid == false || err != nil {
				t.Fatal("Validation of a definition file failed while expected to succeed")
			}
		}))
	}

	//
	// Test with a non-existing file
	//
	valid, err := IsValidDefinition("notExistingDirectory/notExistingFile")
	if valid == true && err == nil {
		t.Fatal("Validation of a non-existing file succeeded while expected to fail")
	}

	//
	// Test passing a valid directory in instead of a file
	//
	valid, err = IsValidDefinition("deffile_test/testdata_bad")
	if valid == true && err != nil {
		t.Fatal("Validation of a directory succeeded while expected to fail")
	}

	//
	// Now test with invalid definition files
	//
	invalidTests := []struct {
		name    string
		defPath string
	}{
		{"BadSection", "deffile_test/testdata_bad/bad_section"},
		{"JSONInput1", "deffile_test/testdata_bad/json_input_1"},
		{"JSONInput2", "deffile_test/testdata_bad/json_input_2"},
		{"Empty", "deffile_test/testdata_bad/empty"},
	}
	for _, tt := range invalidTests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			deffile := tt.defPath

			valid, err := IsValidDefinition(deffile)
			if valid == true && err == nil {
				t.Fatal("Validation of an invalid definition file succeeded while expected to fail")
			}
		}))
	}
}

func TestParseAll(t *testing.T) {
	tests := []struct {
		name     string
		defPath  string
		jsonPath string
	}{
		{"Single", "deffile_test/testdata_multi/single/docker", "deffile_test/testdata_multi/single/docker.json"},
		{"MultiStage", "deffile_test/testdata_multi/simple/simple", "deffile_test/testdata_multi/simple/simple.json"},
		{"NoHeader", "deffile_test/testdata_multi/noheader/noheader", "deffile_test/testdata_multi/noheader/noheader.json"},
		{"NoHeaderComments", "deffile_test/testdata_multi/noheadercomments/noheadercomments", "deffile_test/testdata_multi/noheadercomments/noheadercomments.json"},
		{"NoHeaderWhiteSpace", "deffile_test/testdata_multi/noheaderwhitespace/noheaderwhitespace", "deffile_test/testdata_multi/noheaderwhitespace/noheaderwhitespace.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			defFile, err := os.Open(tt.defPath)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer defFile.Close()

			jsonFile, err := os.OpenFile(tt.jsonPath, os.O_RDWR, 0o644)
			if err != nil {
				t.Fatal("failed to open:", err)
			}
			defer jsonFile.Close()

			defTest, err := All(defFile, tt.defPath)
			if err != nil {
				t.Fatal("failed to parse definition file:", err)
			}

			var defCorrect []types.Definition
			if err := json.NewDecoder(jsonFile).Decode(&defCorrect); err != nil {
				t.Fatal("failed to parse JSON:", err)
			}

			if !reflect.DeepEqual(defTest, defCorrect) {
				t.Fatal("parsed definition did not match reference")
			}
		}))
	}
}
