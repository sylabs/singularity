// Copyright (c) 2018-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package sources

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/sylabs/singularity/v4/internal/pkg/util/bin"
	"github.com/sylabs/singularity/v4/pkg/build/types"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/namespaces"
)

const (
	zypperConf = "/etc/zypp/zypp.conf"
)

// ZypperConveyorPacker only needs to hold the bundle for the container
type ZypperConveyorPacker struct {
	b *types.Bundle
}

func machine() (string, error) {
	var stdout bytes.Buffer
	unamePath, err := bin.FindBin("uname")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(unamePath, `-m`)
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), err
}

// Get downloads container information from the specified source
//
//nolint:maintidx
func (cp *ZypperConveyorPacker) Get(ctx context.Context, b *types.Bundle) error {
	var suseconnectProduct, suseconnectModver string
	var suseconnectPath string
	var pgpfile string
	var iosmajor int
	var otherurl [20]string
	cp.b = b

	// check for zypper on system
	zypperPath, err := bin.FindBin("zypper")
	if err != nil {
		return fmt.Errorf("zypper is not in PATH: %v", err)
	}

	// check for rpm on system
	err = rpmPathCheck()
	if err != nil {
		return err
	}

	include := cp.b.Recipe.Header["include"]

	// check for include environment variable and add it to requires string
	include += ` ` + os.Getenv("INCLUDE")

	// trim leading and trailing whitespace
	include = strings.TrimSpace(include)

	// add aaa_base to start of include list by default
	include = `aaa_base ` + include

	// get mirrorURL, OSVerison, and Includes components to definition
	osversion, osversionOk := cp.b.Recipe.Header["osversion"]
	mirrorurl, mirrorurlOk := cp.b.Recipe.Header["mirrorurl"]
	updateurl, updateurlOk := cp.b.Recipe.Header["updateurl"]
	sleproduct, sleproductOk := cp.b.Recipe.Header["product"]
	sleuser, sleuserOk := cp.b.Recipe.Header["user"]
	sleregcode, sleregcodeOk := cp.b.Recipe.Header["regcode"]
	slepgp, slepgpOk := cp.b.Recipe.Header["productpgp"]
	sleurl, sleurlOk := cp.b.Recipe.Header["registerurl"]
	slemodules, slemodulesOk := cp.b.Recipe.Header["modules"]
	cnt := -1
	if tmp, ok := cp.b.Recipe.Header["otherurl0"]; ok {
		otherurl[0] = tmp
		cnt = 1
	} else {
		if tmp, ok := cp.b.Recipe.Header["otherurl1"]; ok {
			otherurl[0] = tmp
			cnt = 2
		}
	}
	for i := 1; cnt > 0 && i < 20; i++ {
		numS := strconv.Itoa(cnt)
		if tmp, ok := cp.b.Recipe.Header["otherurl"+numS]; ok {
			otherurl[i] = tmp
			cnt++
		} else {
			cnt = -1
		}
	}
	regex := regexp.MustCompile(`(?i)%{OSVERSION}`)

	if sleproductOk || sleuserOk || sleregcodeOk {
		if !sleproductOk || !sleuserOk || !sleregcodeOk {
			return fmt.Errorf("for installation of SLE 'Product', 'User' and 'Regcode' need to be set")
		}
		if !osversionOk {
			return fmt.Errorf("invalid zypper header, OSVersion always required for SLE")
		}
		if !slepgpOk && !mirrorurlOk {
			return fmt.Errorf("no 'ProductPGP' and no 'InstallURL' defined in bootstrap definition")
		}
		suseconnectPath, err = bin.FindBin("SUSEConnect")
		if err != nil {
			return fmt.Errorf("SUSEConnect is not in PATH: %v", err)
		}

		array := strings.SplitN(osversion, ".", -1)
		osmajor := array[0]
		iosmajor, err = strconv.Atoi(osmajor)
		if err != nil {
			return fmt.Errorf("OSVersion has wrong format %v", err)
		}
		osminor := ""
		if len(array) > 1 {
			osminor = "." + array[1]
		}
		if iosmajor > 12 && !mirrorurlOk {
			return fmt.Errorf("for SLE version > 12 'MirrorURL' must be defined and point to the installer")
		}
		osservicepack := ""
		tmp, err := strconv.Atoi(array[1])
		if err != nil {
			return fmt.Errorf("cannot convert minor version string to integer: %v", err)
		}
		if len(array) > 1 && tmp > 0 {
			osservicepack = "." + array[1]
		}
		if mirrorurlOk {
			mirrorurl = regex.ReplaceAllString(mirrorurl, osmajor+osservicepack)
		}
		sleproduct = regex.ReplaceAllString(sleproduct, osmajor+osservicepack)
		array = strings.SplitN(sleproduct, "/", -1)
		machine, _ := machine()
		if len(array) == 3 {
			machine = array[2]
		}
		suseconnectProduct = sleproduct
		suseconnectModver = osmajor + osminor + "/" + machine
		switch len(array) {
		case 1:
		case 2:
			suseconnectProduct += "/" + machine
		case 3:
			suseconnectProduct += "/" + osversion + "/" + machine
		default:
			return fmt.Errorf("malformed Product setting")
		}
		if slepgpOk {
			tmpfile, err := os.CreateTemp("/tmp", "singularity-pgp")
			if err != nil {
				return fmt.Errorf("cannot create pgp-file: %v", err)
			}
			pgpfile = tmpfile.Name()

			if _, err = tmpfile.WriteString(slepgp + "\n"); err != nil {
				return fmt.Errorf("cannot write pgp-file: %v", err)
			}
			if err = tmpfile.Close(); err != nil {
				return fmt.Errorf("cannot close pgp-file %v", err)
			}
		}

		include = include + ` SUSEConnect`
	} else {
		if !mirrorurlOk {
			return fmt.Errorf("invalid zypper header, no MirrorURL specified")
		}
		if regex.MatchString(mirrorurl) || (updateurlOk && regex.MatchString(updateurl)) {
			if !osversionOk {
				return fmt.Errorf("invalid zypper header, OSVersion referenced in mirror but no OSVersion specified")
			}
			mirrorurl = regex.ReplaceAllString(mirrorurl, osversion)
			if updateurlOk {
				updateurl = regex.ReplaceAllString(updateurl, osversion)
			}
		}
	}

	// Create the main portion of zypper config
	err = cp.genZypperConfig()
	if err != nil {
		return fmt.Errorf("while generating zypper config: %w", err)
	}

	cleanupFunc, err := cp.prepareFakerootRpmMacros()
	if err != nil {
		return fmt.Errorf("while generating rpm macros: %w", err)
	}
	defer cleanupFunc()

	err = cp.copyPseudoDevices()
	if err != nil {
		return fmt.Errorf("while copying pseudo devices: %w", err)
	}

	// Add mirrorURL/installURL as repo
	if mirrorurl != "" {
		cmd := exec.CommandContext(ctx, zypperPath, `--root`, cp.b.RootfsPath, `ar`, mirrorurl, `repo`)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while adding zypper mirror: %v", err)
		}
		// Refreshing gpg keys
		cmd = exec.CommandContext(ctx, zypperPath, `--root`, cp.b.RootfsPath, `--gpg-auto-import-keys`, `refresh`)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while refreshing gpg keys: %v", err)
		}
		if updateurl != "" {
			cmd := exec.CommandContext(ctx, zypperPath, `--root`, cp.b.RootfsPath, `ar`, `-f`, updateurl, `update`)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("while adding zypper update: %v", err)
			}
			cmd = exec.CommandContext(ctx, zypperPath, `--root`, cp.b.RootfsPath, `--gpg-auto-import-keys`, `refresh`, `-r`, `update`)
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("while refreshing update %v", err)
			}
		}
	}
	if pgpfile != "" {
		rpmbase := "/usr/lib/sysimage"
		rpmsys := "/var/lib"
		rpmrel := "../.."
		if iosmajor == 12 {
			rpmbase = "/var/lib"
			rpmsys = "/usr/lib/sysimage"
			rpmrel = "../../.."
		}
		if err = os.MkdirAll(cp.b.RootfsPath+rpmbase+`/rpm`, 0o755); err != nil {
			return fmt.Errorf("cannot recreate rpm directories: %v", err)
		}
		if err = os.MkdirAll(cp.b.RootfsPath+rpmsys, 0o755); err != nil {
			return fmt.Errorf("cannot recreate rpm directories: %v", err)
		}
		if err = os.RemoveAll(cp.b.RootfsPath + rpmsys + `/rpm`); err != nil {
			return fmt.Errorf("cannot remove rpm directory")
		}
		if err = os.Symlink(rpmrel+rpmbase+`/rpm`, cp.b.RootfsPath+rpmsys+`/rpm`); err != nil {
			return fmt.Errorf("cannot create rpm symlink")
		}
		cmd := exec.CommandContext(ctx, "rpmkeys", `--root`, cp.b.RootfsPath, `--import`, pgpfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while importing pgp keys: %v", err)
		}
		if err = os.Remove(pgpfile); err != nil {
			return fmt.Errorf("cannot remove pgpfile")
		}
	}

	if suseconnectPath != "" {
		args := []string{
			`--root`, cp.b.RootfsPath,
			`--product`, suseconnectProduct,
			`--email`, sleuser,
			`--regcode`, sleregcode,
		}
		if sleurlOk {
			args = append(args, `--url`, sleurl)
		}
		cmd := exec.Command(suseconnectPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while registering: %v", err)
		}
		if slemodulesOk {
			array := strings.SplitN(slemodules, ",", -1)
			for i := 0; i < len(array); i++ {
				array[i] = strings.TrimSpace(array[i])
				cmd := exec.Command(suseconnectPath, `--root`, cp.b.RootfsPath,
					`--product`, array[i]+`/`+suseconnectModver)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err = cmd.Run(); err != nil {
					return fmt.Errorf("while registering: %v", err)
				}
			}
		}
	}
	for i := 0; otherurl[i] != ""; i++ {
		sID := strconv.Itoa(i)
		cmd := exec.Command(zypperPath, `--root`, cp.b.RootfsPath, `ar`, `-f`, otherurl[i], `repo-`+sID)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while adding zypper url: %s %v", otherurl[i], err)
		}
		cmd = exec.Command(zypperPath, `--root`, cp.b.RootfsPath, `--gpg-auto-import-keys`, `refresh`, `-r`, `repo-`+sID)
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("while refreshing: %s %v", `repo-`+sID, err)
		}
	}

	args := []string{`--non-interactive`, `-c`, filepath.Join(cp.b.RootfsPath, zypperConf), `--root`, cp.b.RootfsPath, `--releasever=` + osversion, `-n`, `install`, `--auto-agree-with-licenses`, `--download-in-advance`}
	args = append(args, strings.Fields(include)...)

	// Zypper install command
	cmd := exec.Command(zypperPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sylog.Debugf("\n\tZypper Path: %s\n\tDetected Arch: %s\n\tOSVersion: %s\n\tMirrorURL: %s\n\tIncludes: %s\n", zypperPath, runtime.GOARCH, osversion, mirrorurl, include)

	// run zypper
	if err = cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 107 {
			sylog.Warningf("Bootstrap succeeded, some RPM scripts failed")
		} else {
			return fmt.Errorf("while bootstrapping from zypper: %v", err)
		}
	}

	return nil
}

// Pack puts relevant objects in a Bundle!
func (cp *ZypperConveyorPacker) Pack(context.Context) (b *types.Bundle, err error) {
	err = cp.insertBaseEnv()
	if err != nil {
		return nil, fmt.Errorf("while inserting base environment: %v", err)
	}

	err = cp.insertRunScript()
	if err != nil {
		return nil, fmt.Errorf("while inserting runscript: %v", err)
	}

	return cp.b, nil
}

func (cp *ZypperConveyorPacker) insertBaseEnv() (err error) {
	if err = makeBaseEnv(cp.b.RootfsPath); err != nil {
		return
	}
	return nil
}

func (cp *ZypperConveyorPacker) insertRunScript() (err error) {
	f, err := os.Create(cp.b.RootfsPath + "/.singularity.d/runscript")
	if err != nil {
		return
	}

	defer f.Close()

	_, err = f.WriteString("#!/bin/sh\n")
	if err != nil {
		return
	}

	if err != nil {
		return
	}

	f.Sync()

	err = os.Chmod(cp.b.RootfsPath+"/.singularity.d/runscript", 0o755)
	if err != nil {
		return
	}

	return nil
}

func (cp *ZypperConveyorPacker) genZypperConfig() (err error) {
	err = os.MkdirAll(filepath.Join(cp.b.RootfsPath, "/etc/zypp"), 0o775)
	if err != nil {
		return fmt.Errorf("while creating %v: %v", filepath.Join(cp.b.RootfsPath, "/etc/zypp"), err)
	}

	err = os.WriteFile(filepath.Join(cp.b.RootfsPath, zypperConf), []byte("[main]\ncachedir=/val/cache/zypp-bootstrap\n\n"), 0o664)
	if err != nil {
		return
	}

	return nil
}

// prepareFakerootRpmMacros implements a workaround to allow zypper-based builds
// to operate in fakeroot mode, by bind-mounting a custom ~/.rpmmacros in the
// user's homedir inside the user namespace
// See https://www.suse.com/support/kb/doc/?id=000020364
func (cp *ZypperConveyorPacker) prepareFakerootRpmMacros() (func(), error) {
	cleanupTasks := [](func()){}
	cleanupFunc := func() {
		// Perform tasks in cleanupTasks in reverse order
		for _, f := range lo.Reverse(cleanupTasks) {
			f()
		}
	}

	if os.Getuid() != 0 {
		return cleanupFunc, nil
	}

	insideUserNs, setgroupsAllowed := namespaces.IsInsideUserNamespace(os.Getpid())
	if !(insideUserNs && setgroupsAllowed) {
		return cleanupFunc, nil
	}

	// We are in a fakeroot environment - proceed

	// Create temporary "grandparent" directory
	sylog.Debugf("Creating 'grandparent' dir for .rpmmacros mount")
	tmpDir, err := os.MkdirTemp("", "user-rpmmacros-grandparentdir")
	if err != nil {
		return cleanupFunc, fmt.Errorf("could not create temporary dir: %w", err)
	}
	cleanupTasks = append(cleanupTasks, func() {
		sylog.Debugf("Removing 'grandparent' dir for .rpmmacros mount: %s", tmpDir)
		os.RemoveAll(tmpDir)
	})

	// Create parent directory inside grandparent directory; parent directory
	// will serve as target for tmpfs mount that will actually house the custom
	// .rpmmacros file
	sylog.Debugf("Creating parent dir for .rpmmacros mount")
	parentDir := filepath.Join(tmpDir, "parentdir")
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		return cleanupFunc, fmt.Errorf("could not create parent dir: %w", err)
	}
	cleanupTasks = append(cleanupTasks, func() {
		sylog.Debugf("Removing parent dir for .rpmmacros mount: %s", parentDir)
		os.RemoveAll(parentDir)
	})

	// Mount tmpfs at parentDir
	sylog.Debugf("Mounting tmpfs at parentDir: %s", parentDir)
	if err := syscall.Mount("tmpfs", parentDir, "tmpfs", syscall.MS_NODEV, "mode=1777,size=1m"); err != nil {
		return cleanupFunc, fmt.Errorf("error while trying to mount tmpfs for custom .rpmmacros: %w", err)
	}
	cleanupTasks = append(cleanupTasks, func() {
		sylog.Debugf("Unmounting tmpfs from parentDir: %s", parentDir)
		syscall.Unmount(parentDir, syscall.MNT_DETACH)
	})

	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cleanupFunc, fmt.Errorf("could not get user's homedir: %w", err)
	}

	homeRpmMacros := filepath.Join(homeDir, ".rpmmacros")
	si, err := os.Stat(homeRpmMacros)
	contents := []byte("")
	// Check if user .rpmmacros already exists
	if os.IsNotExist(err) {
		// User .rpmmacros does not exist; create an empty file, just so we have
		// a mount target
		if err := os.WriteFile(homeRpmMacros, contents, 0o644); err != nil {
			return cleanupFunc, fmt.Errorf("could not blank user .rpmmacros file %q: %w", homeRpmMacros, err)
		}
	} else if err != nil {
		return cleanupFunc, fmt.Errorf("could not check for the existence of user .rpmmacros: %w", err)
	} else if si.IsDir() {
		return cleanupFunc, fmt.Errorf(".rpmmacros in user's homedir (%q) is a directory", homeDir)
	} else {
		// User .rpmmacros is a file and already exists; read its contents
		contents, err = os.ReadFile(homeRpmMacros)
		if err != nil {
			return cleanupFunc, fmt.Errorf("could not read original %q: %w", homeRpmMacros, err)
		}
	}

	// Scan lines of existing .rpmmacros content, looking for %_netsharedpath
	// line
	lines := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	netsharedpathFound := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "%_netsharedpath ") {
			// %_netsharedpath line found; append ":/dev" to existing path
			// specification
			line = line + ":/dev"
			netsharedpathFound = true
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return cleanupFunc, fmt.Errorf("error while reading original user .rpmmacros: %w", err)
	}

	if !netsharedpathFound {
		// No %_netsharedpath line found; create one
		lines = append(lines, "%_netsharedpath /dev")
	}

	// Make sure custom .rpmmacros file will end in a newline
	lines = append(lines, "")

	// Write new custom .rpmmacros file to tmpfs location
	newContents := strings.Join(lines, "\n")
	sylog.Debugf("Custom .rpmmacros contents: %q", newContents)
	customRpmMacros := filepath.Join(parentDir, ".rpmmacros")
	sylog.Debugf("Writing custom .rpmmacros at: %s", customRpmMacros)
	if err := os.WriteFile(customRpmMacros, []byte(newContents), 0o644); err != nil {
		return cleanupFunc, fmt.Errorf("could not write contents to custom .rpmmacros file %q: %w", customRpmMacros, err)
	}
	cleanupTasks = append(cleanupTasks, func() {
		sylog.Debugf("Removing custom .rpmmacros from: %s", customRpmMacros)
		os.RemoveAll(customRpmMacros)
	})

	// Bind-mount custom .rpmmacros over user's .rpmmacros
	sylog.Debugf("Bind-mounting custom .rpmmacros at: %s", homeRpmMacros)
	if err := syscall.Mount(customRpmMacros, homeRpmMacros, "bind", syscall.MS_BIND, ""); err != nil {
		return cleanupFunc, fmt.Errorf("could not create bind-mount with source %q and target %q: %w", customRpmMacros, homeRpmMacros, err)
	}
	cleanupTasks = append(cleanupTasks, func() {
		sylog.Debugf("Unmounting custom .rpmmacros from: %s", homeRpmMacros)
		syscall.Unmount(homeRpmMacros, syscall.MNT_DETACH)
	})

	return cleanupFunc, nil
}

//nolint:dupl
func (cp *ZypperConveyorPacker) copyPseudoDevices() (err error) {
	devPath := filepath.Join(cp.b.RootfsPath, "dev")
	err = os.Mkdir(devPath, 0o775)
	if err != nil {
		return fmt.Errorf("while creating %v: %v", devPath, err)
	}

	devs := []struct {
		major int
		minor int
		path  string
		mode  uint32
	}{
		{1, 3, "/dev/null", syscall.S_IFCHR | 0o666},
		{1, 8, "/dev/random", syscall.S_IFCHR | 0o666},
		{1, 9, "/dev/urandom", syscall.S_IFCHR | 0o666},
		{1, 5, "/dev/zero", syscall.S_IFCHR | 0o666},
	}

	for _, dev := range devs {
		d := int((dev.major << 8) | (dev.minor & 0xff) | ((dev.minor & 0xfff00) << 12))
		path := filepath.Join(cp.b.RootfsPath, dev.path)

		if err := syscall.Mknod(path, dev.mode, d); err != nil {
			return fmt.Errorf("while creating %s: %s", path, err)
		}
	}

	return nil
}

func rpmPathCheck() (err error) {
	var output, stderr bytes.Buffer

	cmd := exec.Command("rpm", "--showrc")
	cmd.Stdout = &output
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		return fmt.Errorf("%v: %v", err, stderr.String())
	}

	rpmDBPath := ""
	scanner := bufio.NewScanner(&output)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		// search for dbpath from showrc output
		if strings.Contains(scanner.Text(), "_dbpath\t") {
			// second field in the string is the path
			rpmDBPath = strings.Fields(scanner.Text())[2]
		}
	}

	if rpmDBPath != `%{_var}/lib/rpm` && rpmDBPath != `%{_usr}/lib/sysimage/rpm` {
		return fmt.Errorf("RPM database is using a non-standard path: %s\n"+
			"There is a way to work around this problem:\n"+
			"Create a file at path %s/.rpmmacros.\n"+
			"Place the following lines into the '.rpmmacros' file:\n"+
			"%s\n"+
			"%s\n"+
			"After creating the file, re-run the bootstrap.\n"+
			"More info: https://github.com/sylabs/singularity/issues/241\n",
			rpmDBPath, os.Getenv("HOME"), `%_var /var`, `%_dbpath %{_var}/lib/rpm`)
	}

	return nil
}
