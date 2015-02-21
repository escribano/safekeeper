package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/alecthomas/kingpin"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var (
	keyNames = kingpin.Flag("keys", "Comma-delimited list of keys to be replaced by their respective environment variable value.").Required().String()
	output   = kingpin.Flag("output", "Output file name. default srcdir/source.go").String()
	paths    = kingpin.Arg("paths", "directories or files").Strings()
)

type errWriter struct {
	b   *bytes.Buffer
	err error
}

func (ew *errWriter) writeString(value string) {
	if ew.err != nil {
		return
	}
	_, ew.err = ew.b.WriteString(value)
}

func main() {
	kingpin.Version("1.0.0")
	kingpin.Parse()

	run(*keyNames, *output, *paths)
}

func run(keys string, out string, inputPaths []string) {
	k := strings.Split(keys, ",")
	keyValues, err := loadKeyValues(k)
	if err != nil {
		log.Fatal(err)
	}

	// We accept either one directory or a list of files. Which do we have?
	if len(inputPaths) == 1 && isFile(inputPaths[0]) {
		var buffer bytes.Buffer

		if err := writeHeader(&buffer); err != nil {
			log.Fatal(err)
		}

		src, err := substituteValues(inputPaths[0], keyValues, &buffer)
		if err != nil {
			log.Fatal(err)
		}

		// Write to file.
		if out == "" {
			out = inputPaths[0]
		}
		err = ioutil.WriteFile(out, src, 0644)
		if err != nil {
			log.Fatalf("writing output: %s", err)
		}
	} else {
		log.Fatal("Only single file inputs are currently supported")
	}
}

// loadKeyValues loads all values for the keys specified via the command-line flag
func loadKeyValues(keys []string) (map[string]string, error) {
	keyValues := make(map[string]string)
	for _, key := range keys {
		if value := os.Getenv(key); value == "" {
			return nil, errors.New(fmt.Sprintf("Environment variable [%s] not found", key))
		} else {
			keyValues[key] = value
		}
	}

	return keyValues, nil
}

// isFile reports whether the named file is a file (not a directory).
func isFile(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		log.Fatal(err)
	}
	return !info.IsDir()
}

// substituteValues replaces all occurences of keys in the source file by the env value
// of that key
func substituteValues(path string, keyValues map[string]string, buffer *bytes.Buffer) ([]byte, error) {
	file, err := openTemplateFile(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	replacers := setupReplacers(keyValues)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// Any go:generate safekeeper line should be ignored since it was read from the original source and
		// is going to be included in the header
		if !(strings.Contains(line, "go:generate") && strings.Contains(line, "safekeeper")) {
			for _, replacer := range replacers {
				line = replacer.Replace(line)
			}
			buffer.WriteString(fmt.Sprintln(line))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// openTemplateFile opens the template source for the current file (by appending .safekeeper to the path)
func openTemplateFile(path string) (*os.File, error) {
	templateFileName := fmt.Sprintf("%s.safekeeper", path)
	return os.Open(templateFileName)

}

// writeHeader writes the header of the file (code generation warning as well as the go:generate line)
func writeHeader(buffer *bytes.Buffer) error {
	ew := &errWriter{b: buffer}
	ew.writeString(fmt.Sprintln("// GENERATED by safekeeper (https://github.com/alexandre-normand/safekeeper, DO NOT EDIT"))
	ew.writeString(fmt.Sprintf("//go:generate safekeeper --keys=%s", *keyNames))
	if *output != "" {
		ew.writeString(fmt.Sprintf(" --output=%s", *output))
	}
	ew.writeString(" $GOFILE\n")

	return ew.err
}

// setupReplacers creates a string replacer for each key/value pair
func setupReplacers(keyValues map[string]string) []strings.Replacer {
	replacers := make([]strings.Replacer, len(keyValues))
	i := 0
	for key, value := range keyValues {
		replacers[i] = *strings.NewReplacer(fmt.Sprintf("ENV_%s", key), value)
		i = i + 1
	}

	return replacers
}