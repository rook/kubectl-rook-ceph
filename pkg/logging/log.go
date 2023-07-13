/*
Copyright 2023 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logging

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

func Info(output string, args ...interface{}) {
	blue := color.New(color.FgBlue).SprintFunc()
	if output != "" {
		fmt.Fprintf(os.Stderr, blue("Info: "))
		fmt.Fprintf(os.Stderr, output, args...)
	}

	fmt.Fprintf(os.Stderr, "\n")
}

func Warning(output string, args ...interface{}) {
	yellow := color.New(color.FgYellow).SprintFunc()
	fmt.Fprintf(os.Stderr, yellow("Warning: "))
	fmt.Fprintf(os.Stderr, output, args...)
	fmt.Fprintf(os.Stderr, "\n")
}

func Error(err error, args ...interface{}) {
	red := color.New(color.FgRed).SprintFunc()
	fmt.Fprintf(os.Stderr, red("Error: "))
	fmt.Fprintf(os.Stderr, err.Error(), args...)
	fmt.Fprintf(os.Stderr, "\n")
}

func Fatal(err error, args ...interface{}) {
	red := color.New(color.FgRed).SprintFunc()
	fmt.Fprintf(os.Stderr, red("Error: "))
	fmt.Fprintf(os.Stderr, err.Error(), args...)
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}
