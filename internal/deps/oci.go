package deps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/daluz/yak/internal/module"
)

const (
	// artifactType is the artifact type of a package's OCI manifest.
	artifactType = "application/vnd.yak.package.v1"
	// layerMediaType is the media type of its one layer, which holds the
	// package's files.
	layerMediaType = "application/vnd.yak.package.layer.v1.tar+gzip"
	// buildDir is where Build writes an OCI image layout, beneath the
	// package root.
	buildDir = ".yak/oci"
)

// BuildOptions configure Build.
type BuildOptions struct {
	// Tag tags the build, in place of the package's version.
	Tag string
	// Output is an OCI image layout to write instead of .yak/oci: a
	// directory, or a tar archive when it ends in ".tar".
	Output string
	Log    io.Writer
}

// Build packs the files of pkg, but for whatever is hidden and its vendor
// directory, into an OCI artifact in an image layout, tagged with the
// package's version.
func Build(pkg *module.Package, opts BuildOptions) error {
	ctx := context.Background()
	tag := opts.Tag
	if tag == "" {
		tag = pkg.Version
	}
	if tag == "" {
		return fmt.Errorf("%s gives no package.version to tag the build with; set one, or give --tag", pkg.Path)
	}
	layer, err := pack(pkg.Root())
	if err != nil {
		return err
	}
	out := opts.Output
	if out == "" {
		out = filepath.Join(pkg.Root(), filepath.FromSlash(buildDir))
	}
	layout := out
	archive := strings.HasSuffix(out, ".tar")
	if archive {
		if layout, err = os.MkdirTemp("", "yak-build-"); err != nil {
			return err
		}
		defer os.RemoveAll(layout)
	}
	store, err := oci.New(layout)
	if err != nil {
		return err
	}
	layerDesc := content.NewDescriptorFromBytes(layerMediaType, layer)
	layerDesc.Annotations = map[string]string{ocispec.AnnotationTitle: pkg.Name + ".tar.gz"}
	if ok, err := store.Exists(ctx, layerDesc); err != nil {
		return err
	} else if !ok {
		if err := store.Push(ctx, layerDesc, bytes.NewReader(layer)); err != nil {
			return err
		}
	}
	annotations := map[string]string{ocispec.AnnotationTitle: pkg.Name}
	if pkg.Version != "" {
		annotations[ocispec.AnnotationVersion] = pkg.Version
	}
	manifest, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, artifactType, oras.PackManifestOptions{
		Layers:              []ocispec.Descriptor{layerDesc},
		ManifestAnnotations: annotations,
	})
	if err != nil {
		return err
	}
	if err := store.Tag(ctx, manifest, tag); err != nil {
		return err
	}
	if archive {
		if err := tarDir(layout, out); err != nil {
			return err
		}
	}
	if opts.Log != nil {
		fmt.Fprintf(opts.Log, "built %s %s (%s) in %s\n", pkg.Name, tag, manifest.Digest, out)
	}
	return nil
}

// Push uploads the build of pkg that .yak/oci holds to ref, which is
// oci://REGISTRY/REPOSITORY[:TAG]. The tag is the package's version when ref
// gives none, and picks the build to upload as well as naming the upload.
func Push(pkg *module.Package, ref string, opts Options) error {
	ctx := context.Background()
	rest, ok := strings.CutPrefix(ref, "oci://")
	if !ok {
		return fmt.Errorf("%s: a package is pushed to oci://REGISTRY/REPOSITORY[:TAG]", ref)
	}
	if strings.Contains(rest, "@") {
		return fmt.Errorf("%s: a package is pushed to a tag, not a digest", ref)
	}
	if i := strings.Index(rest, "/"); i >= 0 && !strings.Contains(rest[i:], ":") {
		if pkg.Version == "" {
			return fmt.Errorf("%s names no tag, and %s gives no package.version", ref, pkg.Path)
		}
		ref += ":" + pkg.Version
	}
	src, err := module.ParseSource(ref)
	if err != nil {
		return err
	}
	layout := filepath.Join(pkg.Root(), filepath.FromSlash(buildDir))
	if _, err := os.Stat(filepath.Join(layout, ocispec.ImageIndexFile)); err != nil {
		return fmt.Errorf("%s holds no build; run yak package build first", layout)
	}
	store, err := oci.New(layout)
	if err != nil {
		return err
	}
	repo, err := repository(src, opts.PlainHTTP)
	if err != nil {
		return err
	}
	desc, err := oras.Copy(ctx, store, src.Ref, repo, src.Ref, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("pushing %s to %s: %w", src.Ref, ref, err)
	}
	if opts.Log != nil {
		fmt.Fprintf(opts.Log, "pushed %s (%s) to %s\n", pkg.Name, desc.Digest, ref)
	}
	return nil
}

// repository connects to the registry of src with the credentials that
// Podman or Docker logged in with.
func repository(src module.Source, plainHTTP bool) (*remote.Repository, error) {
	repo, err := remote.NewRepository(src.Repo)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src.Repo, err)
	}
	repo.PlainHTTP = plainHTTP
	store, err := credentialStore()
	if err != nil {
		return nil, err
	}
	repo.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.NewCache(),
		Credential: credentials.Credential(store),
	}
	return repo, nil
}

// authFiles are the auth files Podman logs in to, in the order they are
// searched: $REGISTRY_AUTH_FILE, then $XDG_RUNTIME_DIR/containers/auth.json,
// then $XDG_CONFIG_HOME/containers/auth.json.
func authFiles() []string {
	var files []string
	if f := os.Getenv("REGISTRY_AUTH_FILE"); f != "" {
		files = append(files, f)
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		files = append(files, filepath.Join(dir, "containers", "auth.json"))
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		if home, err := os.UserHomeDir(); err == nil {
			config = filepath.Join(home, ".config")
		}
	}
	if config != "" {
		files = append(files, filepath.Join(config, "containers", "auth.json"))
	}
	return files
}

// credentialStore searches the auth files of Podman that exist, and then
// Docker's config.json, for a registry's credentials.
func credentialStore() (credentials.Store, error) {
	var stores []credentials.Store
	for _, f := range authFiles() {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		store, err := credentials.NewStore(f, credentials.StoreOptions{})
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		stores = append(stores, store)
	}
	docker, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, err
	}
	stores = append(stores, docker)
	return credentials.NewStoreWithFallbacks(stores[0], stores[1:]...), nil
}

// pull writes the files of the package that src names at ref, a tag or a
// digest, to dest, and answers the digest of its manifest.
func pull(src module.Source, ref string, plainHTTP bool, dest string) (string, error) {
	ctx := context.Background()
	repo, err := repository(src, plainHTTP)
	if err != nil {
		return "", err
	}
	desc, rc, err := oras.Fetch(ctx, repo, ref, oras.DefaultFetchOptions)
	if err != nil {
		return "", fmt.Errorf("fetching %s at %s: %w", src.Repo, ref, err)
	}
	defer rc.Close()
	raw, err := content.ReadAll(rc, desc)
	if err != nil {
		return "", err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("%s at %s: %w", src.Repo, ref, err)
	}
	if manifest.ArtifactType != artifactType || len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != layerMediaType {
		return "", fmt.Errorf("%s at %s is not a yak package", src.Repo, ref)
	}
	layer, err := content.FetchAll(ctx, repo, manifest.Layers[0])
	if err != nil {
		return "", err
	}
	if err := unpack(layer, dest); err != nil {
		return "", fmt.Errorf("unpacking %s at %s: %w", src.Repo, ref, err)
	}
	return desc.Digest.String(), nil
}

// pack archives the files of the package at root, leaving out whatever is
// hidden and its vendor directory, as a gzipped tar.
func pack(root string) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") || rel == module.VendorDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file, which a package cannot hold", filepath.ToSlash(rel))
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(src)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(src)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unpack writes the files of a gzipped tar that pack wrote beneath dest.
func unpack(layer []byte, dest string) error {
	zr, err := gzip.NewReader(bytes.NewReader(layer))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(hdr.Name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("%q is outside the package", hdr.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			src, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, src, 0o644); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%q is not a regular file, which a package cannot hold", hdr.Name)
		}
	}
}

// tarDir writes the files beneath dir to a tar archive at out.
func tarDir(dir, out string) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(src)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = tw.Write(src)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}
