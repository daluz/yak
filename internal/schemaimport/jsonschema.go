package schemaimport

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// jsonSchemas translates each JSON Schema document of files into a schema
// named for its title, or else its file, and each of its $defs and
// definitions into a schema named for its key. A document holding nothing
// but definitions declares no schema of its own.
func jsonSchemas(files []File) ([]byte, []string, error) {
	return importFiles(files, true, "JSON Schema", (*importer).jsonSchema)
}

// container holds the keywords of a document that only gathers definitions.
var container = []string{"$schema", "$id", "$comment", "title", "description", "$defs", "definitions"}

func (im *importer) jsonSchema(doc *yaml.Node) error {
	im.refs, im.defs = map[string]string{}, map[string]*yaml.Node{}
	var defs []definition
	if deref(doc).Kind != yaml.MappingNode || !onlyKeys(doc, container...) ||
		(get(doc, "$defs") == nil && get(doc, "definitions") == nil) {
		title := str(get(doc, "title"))
		if title == "" {
			if title, _, _ = strings.Cut(filepath.Base(im.file), "."); im.file == "<stdin>" {
				return fmt.Errorf("%s: a schema read from standard input needs a title to name it", im.file)
			}
		}
		name, err := nameFor(title, "the root schema", im.file)
		if err != nil {
			return err
		}
		im.refs["#"], im.defs[name] = name, doc
		defs = append(defs, definition{name, "the root schema", doc})
	}
	for _, key := range []string{"$defs", "definitions"} {
		d, err := im.definitions(get(doc, key), "#/"+key+"/")
		if err != nil {
			return err
		}
		defs = append(defs, d...)
	}
	return im.add(defs)
}
