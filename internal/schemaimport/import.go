// Package schemaimport translates the schemas other tools write into a
// .yakl library of yak schemas.
package schemaimport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/ast"
	"github.com/daluz/yak/internal/token"
)

// File is a file to import, named as diagnostics name it.
type File struct {
	Name string
	Data []byte
}

// The formats Import reads.
const (
	FormatCRD        = "crd"
	FormatJSONSchema = "jsonschema"
	FormatOpenAPI    = "openapi"
)

// Formats lists the formats Import reads.
var Formats = []string{FormatCRD, FormatJSONSchema, FormatOpenAPI}

// Detect answers the format of f, or "" when f holds nothing Import reads.
func Detect(f File) (string, error) {
	docs, err := documents(f)
	if err != nil {
		return "", err
	}
	for _, doc := range docs {
		switch {
		case holdsCRD(doc):
			return FormatCRD, nil
		case strings.HasPrefix(str(get(doc, "openapi")), "3."):
			return FormatOpenAPI, nil
		case strings.Contains(str(get(doc, "$schema")), "json-schema.org"):
			return FormatJSONSchema, nil
		}
	}
	return "", nil
}

// Import translates files, all of them in format, into one library, and
// answers the warnings about what it left out as well.
func Import(format string, files []File) ([]byte, []string, error) {
	switch format {
	case FormatCRD:
		return crds(files)
	case FormatJSONSchema:
		return jsonSchemas(files)
	case FormatOpenAPI:
		return openAPI(files)
	}
	return nil, nil, fmt.Errorf("unknown input format %q; known formats: %s", format, strings.Join(Formats, ", "))
}

// importFiles reads every document of files with read, answering the
// library, its warnings, and an error naming what when no document declared
// a schema. open says that an object takes the keys it does not declare
// unless its additionalProperties says otherwise.
func importFiles(files []File, open bool, what string, read func(*importer, *yaml.Node) error) ([]byte, []string, error) {
	im := &importer{open: open, declaredIn: map[string]string{}}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
		im.file = f.Name
		docs, err := documents(f)
		if err != nil {
			return nil, im.warnings, err
		}
		for _, doc := range docs {
			if err := read(im, doc); err != nil {
				return nil, im.warnings, err
			}
		}
	}
	if len(im.schemas) == 0 {
		return nil, im.warnings, fmt.Errorf("found no %s in %s", what, strings.Join(names, ", "))
	}
	return im.library(names), im.warnings, nil
}

type importer struct {
	file     string
	open     bool
	warnings []string
	schemas  []decl
	// declaredIn names what declared each schema, and its file.
	declaredIn map[string]string
	// refs maps each $ref of the document being read to the schema it
	// names, and defs maps that schema to its node.
	refs map[string]string
	defs map[string]*yaml.Node
	// regex says that a check calls std.regex, which the library imports.
	regex bool
}

type decl struct {
	name string
	typ  *typ
}

func (im *importer) warn(path, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if path != "" {
		msg = path + ": " + msg
	}
	im.warnings = append(im.warnings, im.file+": "+msg)
}

// declare records that what declares the schema name, which no other
// definition may declare as well.
func (im *importer) declare(name, what string) error {
	if prev, ok := im.declaredIn[name]; ok {
		return fmt.Errorf("%s: %s and %s both name a schema %s", im.file, what, prev, name)
	}
	im.declaredIn[name] = what + " in " + im.file
	return nil
}

// documents answers the documents of f that are not empty.
func documents(f File) ([]*yaml.Node, error) {
	var docs []*yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(f.Data))
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if len(doc.Content) == 0 || deref(doc.Content[0]).ShortTag() == "!!null" {
			continue
		}
		docs = append(docs, doc.Content[0])
	}
}

// definition is a schema a document declares, under the name it takes.
type definition struct {
	name, what string
	node       *yaml.Node
}

// definitions declares each schema the mapping under prefix holds, such as
// "#/$defs/", named for its key, and makes "prefix+key" a $ref to it.
func (im *importer) definitions(n *yaml.Node, prefix string) ([]definition, error) {
	var out []definition
	n = deref(n)
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i].Value
		what := fmt.Sprintf("the definition %q", key)
		name, err := nameFor(key, what, im.file)
		if err != nil {
			return nil, err
		}
		im.refs[prefix+strings.NewReplacer("~", "~0", "/", "~1").Replace(key)] = name
		im.defs[name] = n.Content[i+1]
		out = append(out, definition{name, what, n.Content[i+1]})
	}
	return out, nil
}

// add declares and translates every definition of defs.
func (im *importer) add(defs []definition) error {
	for _, d := range defs {
		if err := im.declare(d.name, d.what); err != nil {
			return err
		}
	}
	for _, d := range defs {
		im.schemas = append(im.schemas, decl{name: d.name, typ: im.convert(d.node, d.name, false)})
	}
	return nil
}

var notInName = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// nameFor answers the schema name s gives, each run of characters a name
// cannot hold made "_". what names s in the error answered when the name
// is one yak keeps for itself.
func nameFor(s, what, file string) (string, error) {
	name := notInName.ReplaceAllString(s, "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "_" + name
	}
	if name == "_" || slices.Contains(ast.BuiltinTypes, name) || token.IsReserved(name) || token.IsKeyword(name) {
		return "", fmt.Errorf("%s: %s cannot name a schema %q", file, what, name)
	}
	return name, nil
}
