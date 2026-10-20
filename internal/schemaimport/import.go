// Package schemaimport translates the schemas other tools write into a
// .yakl library of yak schemas.
package schemaimport

import (
	"fmt"
	"strings"
)

// File is a file to import, named as diagnostics name it.
type File struct {
	Name string
	Data []byte
}

// FormatCRD names files of Kubernetes CustomResourceDefinitions.
const FormatCRD = "crd"

// Formats lists the formats Import reads.
var Formats = []string{FormatCRD}

// Detect answers the format of f, or "" when f holds nothing Import reads.
func Detect(f File) (string, error) {
	docs, err := documents(f)
	if err != nil {
		return "", err
	}
	for _, doc := range docs {
		if holdsCRD(doc) {
			return FormatCRD, nil
		}
	}
	return "", nil
}

// Import translates files, all of them in format, into one library, and
// answers the warnings about what it left out as well.
func Import(format string, files []File) ([]byte, []string, error) {
	switch format {
	case FormatCRD:
		return CRDs(files)
	}
	return nil, nil, fmt.Errorf("unknown input format %q; known formats: %s", format, strings.Join(Formats, ", "))
}
