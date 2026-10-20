package schemaimport

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// openAPI translates each schema under the components.schemas of the
// OpenAPI v3 documents of files into a schema named for its key.
func openAPI(files []File) ([]byte, []string, error) {
	return importFiles(files, true, "schema under components.schemas", (*importer).openAPI)
}

func (im *importer) openAPI(doc *yaml.Node) error {
	if v := str(get(doc, "openapi")); !strings.HasPrefix(v, "3.") {
		if s := str(get(doc, "swagger")); v == "" && s != "" {
			v = s
		} else if v == "" {
			return fmt.Errorf("%s: found a document that is not an OpenAPI v3 document", im.file)
		}
		return fmt.Errorf("%s: the document is OpenAPI %s, but only OpenAPI v3 can be imported", im.file, v)
	}
	im.refs, im.defs = map[string]string{}, map[string]*yaml.Node{}
	defs, err := im.definitions(get(get(doc, "components"), "schemas"), "#/components/schemas/")
	if err != nil {
		return err
	}
	return im.add(defs)
}
