package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/grokify/grokifyql"
)

func main() {
	style := flag.String("style", string(grokifyql.FormatMultiline), "format style: multiline or singleline")
	highlight := flag.String("highlight", string(grokifyql.HighlightNone), "syntax highlighting: none or ansi")
	indent := flag.String("indent", "\t", "indent string for multiline output")
	flag.Parse()

	input, err := readInput(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	output, err := grokifyql.Format(string(input), grokifyql.FormatOptions{
		Style:     grokifyql.FormatStyle(*style),
		Indent:    *indent,
		Highlight: grokifyql.HighlightStyle(*highlight),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(output)
}

func readInput(args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(args[0])
}
