// Copyright (c) 2021, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package slice

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/samber/lo"
)

func TestContainsString(t *testing.T) {
	type args struct {
		s     []string
		match string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "NoMatchSingle",
			args: args{[]string{"a"}, "1"},
			want: false,
		},
		{
			name: "NoMatchMulti",
			args: args{[]string{"a", "b", "c"}, "1"},
			want: false,
		},
		{
			name: "NoMatchEmpty",
			args: args{[]string{}, "1"},
			want: false,
		},
		{
			name: "MatchSingle",
			args: args{[]string{"a"}, "a"},
			want: true,
		},
		{
			name: "MatchMulti",
			args: args{[]string{"a", "b", "c"}, "a"},
			want: true,
		},
		{
			name: "EmptyMatch",
			args: args{[]string{"a", "b", "c"}, ""},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsString(tt.args.s, tt.args.match); got != tt.want {
				t.Errorf("ContainsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsAnyString(t *testing.T) {
	type args struct {
		s       []string
		matches []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "NoMatchSingle",
			args: args{[]string{"a"}, []string{"1"}},
			want: false,
		},
		{
			name: "NoMatchMulti",
			args: args{[]string{"a", "b", "c"}, []string{"1"}},
			want: false,
		},
		{
			name: "NoMatchEmpty",
			args: args{[]string{}, []string{"1"}},
			want: false,
		},
		{
			name: "NoMatchesSingle",
			args: args{[]string{}, []string{"1", "2", "3"}},
			want: false,
		},
		{
			name: "NoMatchesMulti",
			args: args{[]string{}, []string{"1", "2", "3"}},
			want: false,
		},
		{
			name: "NoMatchesEmpty",
			args: args{[]string{}, []string{"1", "2", "3"}},
			want: false,
		},
		{
			name: "MatchSingle",
			args: args{[]string{"a"}, []string{"a"}},
			want: true,
		},
		{
			name: "MatchMulti",
			args: args{[]string{"a", "b", "c"}, []string{"a"}},
			want: true,
		},
		{
			name: "MatchesSingle",
			args: args{[]string{"a"}, []string{"1", "a", "b"}},
			want: true,
		},
		{
			name: "MatchesMulti",
			args: args{[]string{"a", "b", "c"}, []string{"1", "a", "b"}},
			want: true,
		},
		{
			name: "EmptyMatch",
			args: args{[]string{"a", "b", "c"}, []string{""}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsAnyString(tt.args.s, tt.args.matches); got != tt.want {
				t.Errorf("ContainsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsInt(t *testing.T) {
	type args struct {
		s     []int
		match int
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "NoMatchSingle",
			args: args{[]int{1}, 0},
			want: false,
		},
		{
			name: "NoMatchMulti",
			args: args{[]int{1, 2, 3}, 0},
			want: false,
		},
		{
			name: "NoMatchEmpty",
			args: args{[]int{}, 0},
			want: false,
		},
		{
			name: "MatchSingle",
			args: args{[]int{1}, 1},
			want: true,
		},
		{
			name: "MatchMultiStart",
			args: args{[]int{1, 2, 3}, 1},
			want: true,
		},
		{
			name: "MatchMultiMid",
			args: args{[]int{1, 2, 3}, 2},
			want: true,
		},
		{
			name: "MatchMultiEnd",
			args: args{[]int{1, 2, 3}, 2},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsInt(tt.args.s, tt.args.match); got != tt.want {
				t.Errorf("ContainsInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	type args[T any] struct {
		a    []T
		b    []T
		want []T
	}
	intTests := []struct {
		name string
		args args[int]
	}{
		{
			name: "Identical",
			args: args[int]{
				a:    []int{3, 9, 5, 7, 2, 1, 0, 4},
				b:    []int{3, 9, 5, 7, 2, 1, 0, 4},
				want: []int{},
			},
		},
		{
			name: "EmptyA",
			args: args[int]{
				a:    []int{},
				b:    []int{3, 9, 5, 7, 2, 1, 0, 4},
				want: []int{},
			},
		},
		{
			name: "EmptyB",
			args: args[int]{
				a:    []int{3, 9, 5, 7, 2, 1, 0, 4},
				b:    []int{},
				want: []int{3, 9, 5, 7, 2, 1, 0, 4},
			},
		},
		{
			name: "EmptyBoth",
			args: args[int]{
				a:    []int{},
				b:    []int{},
				want: []int{},
			},
		},
		{
			name: "AsupersetofB",
			args: args[int]{
				a:    []int{3, 9, 5, 7, 2, 1, 0, 4},
				b:    []int{3, 9, 7, 0, 4},
				want: []int{5, 2, 1},
			},
		},
		{
			name: "AsubsetofB",
			args: args[int]{
				a:    []int{5, 2, 1},
				b:    []int{5, 7, 2, 1, 0, 4},
				want: []int{},
			},
		},
		{
			name: "Intersection",
			args: args[int]{
				a:    []int{3, 5, 2, 0},
				b:    []int{3, 9, 7, 2, 4},
				want: []int{5, 0},
			},
		},
	}

	convertor := func(x int, _ int) string {
		return fmt.Sprintf("Have an int whose value is %#v, why don't you", x)
	}

	for _, tt := range intTests {
		t.Run("Int"+tt.name, func(t *testing.T) {
			if got := Subtract(tt.args.a, tt.args.b); !reflect.DeepEqual(got, tt.args.want) {
				t.Errorf("Subtract(%#v, %#v) = %#v, want %#v", tt.args.a, tt.args.b, got, tt.args.want)
			}
		})

		strArgs := args[string]{
			a:    lo.Map(tt.args.a, convertor),
			b:    lo.Map(tt.args.b, convertor),
			want: lo.Map(tt.args.want, convertor),
		}
		t.Run("String"+tt.name, func(t *testing.T) {
			if got := Subtract(strArgs.a, strArgs.b); !reflect.DeepEqual(got, strArgs.want) {
				t.Errorf("Subtract(%#v, %#v) = %#v, want %#v", strArgs.a, strArgs.b, got, strArgs.want)
			}
		})
	}
}
