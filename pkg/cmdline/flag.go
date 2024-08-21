// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package cmdline

import (
	"fmt"
	"os"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flag holds information about a command flag
type Flag struct {
	ID           string
	Value        interface{}
	DefaultValue interface{}
	Name         string
	ShortHand    string
	Usage        string
	Tag          string
	Deprecated   string
	Hidden       bool
	Required     bool
	EnvKeys      []string
	EnvHandler   EnvHandler
	// Export envar also without prefix
	WithoutPrefix bool
	// When Value is a []String:
	// If true, will use pFlag StringArrayVar(P) type, where values are not split on comma.
	// If false, will use pFlag StringSliceVar(P) type, where a single value is split on commas.
	StringArray bool
}

type FlagValTypeErr struct {
	name     string
	expected string
	found    string
}

func (e FlagValTypeErr) Error() string {
	return fmt.Sprintf("expected value of flag %q to be of type %s, but encountered %s instead", e.name, e.expected, e.found)
}

// flagManager manages cobra command flags and store them
// in a hash map
type flagManager struct {
	flags map[string]*Flag
}

// newFlagManager instantiates a flag manager and returns it
func newFlagManager() *flagManager {
	return &flagManager{
		flags: make(map[string]*Flag),
	}
}

func (m *flagManager) setFlagOptions(flag *Flag, cmd *cobra.Command) {
	cmd.Flags().SetAnnotation(flag.Name, "argtag", []string{flag.Tag})
	cmd.Flags().SetAnnotation(flag.Name, "ID", []string{flag.ID})

	if len(flag.EnvKeys) > 0 {
		cmd.Flags().SetAnnotation(flag.Name, "envkey", flag.EnvKeys)

		// Environment flags can also be exported without a prefix (e.g. DOCKER_*)
		if flag.WithoutPrefix {
			cmd.Flags().SetAnnotation(flag.Name, "withoutPrefix", []string{"true"})
		}
	}
	if flag.Deprecated != "" {
		cmd.Flags().MarkDeprecated(flag.Name, flag.Deprecated)
	}
	if flag.Hidden {
		cmd.Flags().MarkHidden(flag.Name)
	}
	if flag.Required {
		cmd.MarkFlagRequired(flag.Name)
	}
}

func (m *flagManager) registerFlagForCmd(flag *Flag, cmds ...*cobra.Command) error {
	for _, c := range cmds {
		if c == nil {
			return fmt.Errorf("nil command provided")
		}
	}
	if flag == nil {
		return fmt.Errorf("nil flag provided")
	}
	if flag.EnvHandler == nil {
		flag.EnvHandler = EnvSetValue
	}
	switch flag.DefaultValue.(type) {
	case string:
		m.registerStringVar(flag, cmds)
	case map[string]string:
		m.registerStringMapVar(flag, cmds)
	case []string:
		if flag.StringArray {
			m.registerStringArrayVar(flag, cmds)
		} else {
			m.registerStringSliceVar(flag, cmds)
		}
	case bool:
		m.registerBoolVar(flag, cmds)
	case int:
		m.registerIntVar(flag, cmds)
	case uint32:
		m.registerUint32Var(flag, cmds)
	default:
		return fmt.Errorf("flag %s of type %T is not supported", flag.Name, flag.DefaultValue)
	}
	m.flags[flag.ID] = flag
	return nil
}

func (m *flagManager) registerStringVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*string)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "string", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.(string)
		if flag.ShortHand != "" {
			c.Flags().StringVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().StringVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) registerStringSliceVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*[]string)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "[]string", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.([]string)
		if flag.ShortHand != "" {
			c.Flags().StringSliceVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().StringSliceVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) registerStringArrayVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*[]string)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "[]string", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.([]string)
		if flag.ShortHand != "" {
			c.Flags().StringArrayVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().StringArrayVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

// registerStringArrayCommas uses StringToStringVarP, a variant to allow commas (and a map of string/string)
func (m *flagManager) registerStringMapVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*map[string]string)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "map[string]string", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.(map[string]string)
		if flag.ShortHand != "" {
			c.Flags().StringToStringVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().StringToStringVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) registerBoolVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*bool)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "bool", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.(bool)
		if flag.ShortHand != "" {
			c.Flags().BoolVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().BoolVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) registerIntVar(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*int)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "int", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.(int)
		if flag.ShortHand != "" {
			c.Flags().IntVarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().IntVar(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) registerUint32Var(flag *Flag, cmds []*cobra.Command) error {
	for _, c := range cmds {
		val, ok := flag.Value.(*uint32)
		if !ok {
			return FlagValTypeErr{name: flag.Name, expected: "uint32", found: reflect.TypeOf(flag.Value).String()}
		}

		//nolint:forcetypeassert
		defaultVal := flag.DefaultValue.(uint32)
		if flag.ShortHand != "" {
			c.Flags().Uint32VarP(val, flag.Name, flag.ShortHand, defaultVal, flag.Usage)
		} else {
			c.Flags().Uint32Var(val, flag.Name, defaultVal, flag.Usage)
		}
		m.setFlagOptions(flag, c)
	}
	return nil
}

func (m *flagManager) updateCmdFlagFromEnv(cmd *cobra.Command, prefix string) error {
	var errs []error

	fn := func(flag *pflag.Flag) {
		envKeys, ok := flag.Annotations["envkey"]
		if !ok {
			return
		}
		id, ok := flag.Annotations["ID"]
		if !ok {
			return
		}
		mflag, ok := m.flags[id[0]]
		if !ok {
			return
		}
		for _, key := range envKeys {
			// First priority goes to prefixed variable
			val, set := os.LookupEnv(prefix + key)
			if !set {
				// Determine if environment keys should be looked for without prefix
				// This annotation just needs to be present, period
				_, withoutPrefix := flag.Annotations["withoutPrefix"]
				if !withoutPrefix {
					continue
				}
				// Second try - looking for the same without prefix!
				val, set = os.LookupEnv(key)
				if !set {
					continue
				}
			}
			if mflag.EnvHandler != nil {
				if err := mflag.EnvHandler(flag, val); err != nil {
					errs = append(errs, err)
					break
				}
			}
		}
	}

	// visit each child commands
	cmd.Flags().VisitAll(fn)
	if len(errs) > 0 {
		errStr := ""
		for _, e := range errs {
			errStr += fmt.Sprintf("\n%s", e.Error())
		}
		return fmt.Errorf("while updating flags from environment: %v", errStr)
	}

	return nil
}
