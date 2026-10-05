package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

const defaultCorpus = "internal/modules/quotation/service/testdata/meeting_extraction_quality/corpus.json"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("meeting-extraction-quality", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	corpusPath := f.String("corpus", defaultCorpus, "synthetic corpus")
	predictionsPath := f.String("predictions", "", "aggregate prediction input")
	if f.Parse(args) != nil || f.NArg() != 0 || *predictionsPath == "" {
		fmt.Fprintln(errOut, "error=invalid_arguments")
		return 2
	}
	var c corpus
	var p predictionFile
	if readJSON(*corpusPath, &c) != nil || readJSON(*predictionsPath, &p) != nil {
		fmt.Fprintln(errOut, "error=invalid_input")
		return 2
	}
	r, e := evaluate(c, p)
	if e != nil {
		fmt.Fprintln(errOut, "error=invalid_input")
		return 2
	}
	b, e := marshalReport(r)
	if e != nil {
		return 1
	}
	_, _ = out.Write(append(b, '\n'))
	if !r.Pass {
		return 1
	}
	return 0
}
func readJSON(path string, target any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, target)
}
