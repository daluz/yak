package schemaimport

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// CRDs translates every served version of the CustomResourceDefinitions in
// files into a schema named for its kind and version, such as WidgetV1. It
// answers the library, and a warning for every keyword it leaves out and
// every document that is not a CustomResourceDefinition.
func CRDs(files []File) ([]byte, []string, error) {
	return importFiles(files, false, "CustomResourceDefinition", (*importer).document)
}

// holdsCRD says whether n is a CustomResourceDefinition, or a List holding
// one.
func holdsCRD(n *yaml.Node) bool {
	switch str(get(n, "kind")) {
	case "CustomResourceDefinition":
		return true
	case "List", "CustomResourceDefinitionList":
		for _, item := range deref(get(n, "items")).Content {
			if holdsCRD(item) {
				return true
			}
		}
	}
	return false
}

// document imports a CustomResourceDefinition, or each of those a List
// holds, as kubectl writes several.
func (im *importer) document(n *yaml.Node) error {
	switch kind := str(get(n, "kind")); kind {
	case "CustomResourceDefinition":
		return im.crd(n)
	case "List", "CustomResourceDefinitionList":
		for _, item := range deref(get(n, "items")).Content {
			if err := im.document(item); err != nil {
				return err
			}
		}
		return nil
	case "":
		im.warn("", "skipped a document that is not a CustomResourceDefinition")
	default:
		im.warn("", "skipped a %s, which is not a CustomResourceDefinition", kind)
	}
	return nil
}

var schemaName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func (im *importer) crd(n *yaml.Node) error {
	label := "CustomResourceDefinition " + fmt.Sprintf("%q", str(get(get(n, "metadata"), "name")))
	if api := str(get(n, "apiVersion")); api != "apiextensions.k8s.io/v1" {
		return fmt.Errorf("%s: %s is %s, but only apiextensions.k8s.io/v1 can be imported", im.file, label, api)
	}
	spec := get(n, "spec")
	group, kind := str(get(spec, "group")), str(get(get(spec, "names"), "kind"))
	if group == "" || kind == "" {
		return fmt.Errorf("%s: %s needs a spec.group and a spec.names.kind", im.file, label)
	}
	for _, v := range deref(get(spec, "versions")).Content {
		if str(get(v, "served")) != "true" {
			continue
		}
		version := str(get(v, "name"))
		name := kind + strings.ToUpper(version[:min(1, len(version))]) + version[min(1, len(version)):]
		if !schemaName.MatchString(name) {
			return fmt.Errorf("%s: %s has a version %q that cannot name a schema", im.file, label, version)
		}
		root := get(get(v, "schema"), "openAPIV3Schema")
		if root == nil {
			return fmt.Errorf("%s: version %s of %s has no schema.openAPIV3Schema", im.file, version, label)
		}
		t, err := im.root(name, group+"/"+version, kind, root)
		if err != nil {
			return err
		}
		if err := im.declare(name, label); err != nil {
			return err
		}
		im.schemas = append(im.schemas, decl{name: name, typ: t})
	}
	return nil
}

// root translates the schema of an object of the kind, whose apiVersion and
// kind are the one value each can be, and are filled in when left out.
// Every object takes the same metadata, which the API server checks, so the
// schema a CRD declares for it is not imported.
func (im *importer) root(path, apiVersion, kind string, n *yaml.Node) (*typ, error) {
	t := im.convert(n, path, true)
	if t.mapping == nil {
		return nil, fmt.Errorf("%s: the openAPIV3Schema of %s is not an object", im.file, path)
	}
	fields := []field{
		{name: "apiVersion", t: literal(quote(apiVersion)), def: quote(apiVersion)},
		{name: "kind", t: literal(quote(kind)), def: quote(kind)},
		{name: "metadata", optional: true, t: builtin("mapping")},
	}
	t.mapping.fields = append(fields, t.mapping.fields...)
	return t, nil
}

// envelope holds the fields every Kubernetes object has, which root declares
// itself and an embedded resource takes.
var envelope = []string{"apiVersion", "kind", "metadata"}
