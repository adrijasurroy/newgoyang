// Copyright 2015 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Program yang parses YANG files, displays errors, and possibly writes
// something related to the input on output.
//
// Usage: yang [--path PATH] [--format FORMAT] [MODULE] [FILE ...]
//
// If MODULE is specified (an argument that does not end in .yang), it is taken
// as the name of the module to display.  Any FILEs specified are read, and the
// tree for MODULE is displayed.  If MODULE was not defined in FILEs (or no
// files were specified), then the file MODULES.yang is read as well.  An error
// is displayed if no definition for MODULE was found.
//
// If MODULE is missing, then all base modules read from the FILEs are
// displayed.  If there are no arguments then standard input is parsed.
//
// If PATH is specified, it is considered a comma separated list of paths
// to append to the search directory.
//
// FORMAT, which defaults to "tree", specifes the format of output to produce.
// Use "goyang --help" for a list of available formats.
//
// THIS PROGRAM IS STILL JUST A DEVELOPMENT TOOL.
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime/trace"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/pborman/getopt"
)

// Each format must register a formatter with register.  The function f will
// be called once with the set of yang Entry trees generated.
type formatter struct {
	name string
	f    func(io.Writer, []*yang.Entry)
	help string
}

var formatters = map[string]*formatter{}

func register(f *formatter) {
	formatters[f.name] = f
}

// exitIfError writes errs to standard error and exits with an exit status of 1.
// If errs is empty then exitIfError does nothing and simply returns.
func exitIfError(errs []error) {
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		stop(1)
	}
}

var stop = os.Exit

func main() {
	format := "tree"
	formats := make([]string, 0, len(formatters))
	for k := range formatters {
		formats = append(formats, k)
	}
	sort.Strings(formats)

	var traceP string
	var help bool
	getopt.CommandLine.ListVarLong(&yang.Path, "path", 0, "comma separated list of directories to add to PATH")
	getopt.CommandLine.StringVarLong(&format, "format", 0, "format to display: "+strings.Join(formats, ", "))
	getopt.CommandLine.StringVarLong(&traceP, "trace", 0, "file to write trace into")
	getopt.CommandLine.BoolVarLong(&help, "help", '?', "display help")

	getopt.Parse()

	if traceP != "" {
		fp, err := os.Create(traceP)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		trace.Start(fp)
		stop = func(c int) { trace.Stop(); os.Exit(c) }
		defer func() { trace.Stop() }()
	}

	if help {
		getopt.CommandLine.PrintUsage(os.Stderr)
		fmt.Fprintf(os.Stderr, "\nFormats:\n")
		for _, fn := range formats {
			f := formatters[fn]
			fmt.Fprintf(os.Stderr, "    %s - %s\n", f.name, f.help)
		}
		stop(0)
	}

	if _, ok := formatters[format]; !ok {
		fmt.Fprintf(os.Stderr, "%s: invalid format.  Choices are %s\n", format, strings.Join(formats, ", "))
		stop(1)

	}

	files := getopt.Args()

	if len(files) > 0 && !strings.HasSuffix(files[0], ".yang") {
		e, errs := yang.GetModule(files[0], files[1:]...)
		exitIfError(errs)
		Write(os.Stdout, e)
		return
	}

	// Okay, either there are no arguments and we read stdin, or there
	// is one or more file names listed.  Read them in and display them.

	ms := yang.NewModules()

	if len(files) == 0 {
		data, err := ioutil.ReadAll(os.Stdin)
		if err == nil {
			err = ms.Parse(string(data), "<STDIN>")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			stop(1)
		}
	}

	for _, name := range files {
		if err := ms.Read(name); err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
	}

	// Process the read files, exiting if any errors were found.
	exitIfError(ms.Process())

	// Keep track of the top level modules we read in.
	// Those are the only modules we want to print below.
	mods := map[string]*yang.Module{}
	var names []string

	for _, m := range ms.Modules {
		if mods[m.Name] == nil {
			mods[m.Name] = m
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	entries := make([]*yang.Entry, len(names))
	for x, n := range names {
		entries[x] = yang.ToEntry(mods[n])
	}

	formatters[format].f(os.Stdout, entries)
}