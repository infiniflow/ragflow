//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Command ragflow-deps prepares everything the Go server needs to build and run.
//
//	go run ./cmd/ragflow-deps          # local development
//	go run ./cmd/ragflow-deps --image  # dependency image (infiniflow/ragflow_deps)
//
// The default mode downloads the native static libraries build.sh links
// (pdfium, pdf_oxide, office_oxide, ONNX Runtime) into the version-keyed cache
// ~/ragflow-native-libs/<lib>/v<version>/ (RAGFLOW_NATIVE_LIBS_DIR overrides the
// root), then the runtime resources: deps/cl100k_base.tiktoken, the DeepDoc
// weights in internal/rag/res/deepdoc/, and the embedding tokenizer assets in
// deps/huggingface.co/. --image skips the native libraries and adds the Noto
// Sans CJK font the PDF writer embeds.
//
// Native library versions come from their single sources of truth: go.mod for
// office_oxide and pdf_oxide, DeepDocORTVersion in internal/common/environments.go
// for ONNX Runtime, and tools/scripts/native_lib_version.sh for pdfium. Every
// step skips files that are already in place, so re-running is cheap.
//
// The command imports only the standard library and builds with CGO_ENABLED=0,
// because it runs before the native libraries it provisions exist.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultHFEndpoint = "https://huggingface.co"
	chinaHFEndpoint   = "https://hf-mirror.com"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ragflow-deps", flag.ContinueOnError)
	image := fs.Bool("image", false, "prepare the dependency image resources (skips native libraries, adds the CJK font)")
	chinaMirrors := fs.Bool("china-mirrors", false, "route GitHub downloads through gh-proxy.com and Hugging Face through hf-mirror.com")
	targetOS := fs.String("target-os", os.Getenv("RAGFLOW_TARGET_OS"), "target GOOS for native libraries (linux or darwin); defaults to the host")
	targetArch := fs.String("target-arch", os.Getenv("RAGFLOW_TARGET_ARCH"), "target GOARCH for native libraries (amd64 or arm64); defaults to the host")
	if err := fs.Parse(args); err != nil {
		return err
	}

	d, err := newDownloader(out, *chinaMirrors)
	if err != nil {
		return err
	}
	if !*image {
		if d.platform, err = resolvePlatform(*targetOS, *targetArch); err != nil {
			return err
		}
		fmt.Fprintf(out, "Target platform: %s/%s\n", d.platform.goos, d.platform.goarch)
		if err := d.provisionNative(); err != nil {
			return err
		}
	}
	if err := d.provisionRuntime(*image); err != nil {
		return err
	}

	fmt.Fprintln(out)
	if *image {
		fmt.Fprintf(out, "✓ Dependency image resources ready under %s/\n", d.depsDir)
		fmt.Fprintf(out, "  Build the image with: cd %s && docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .\n", d.depsDir)
	} else {
		fmt.Fprintf(out, "✓ Native libraries ready under %s/ and runtime resources in place.\n", d.nativeDir)
		fmt.Fprintln(out, "  Run: bash build.sh")
	}
	return nil
}

// downloader carries the resolved locations and endpoints every step uses.
type downloader struct {
	out        io.Writer
	client     *http.Client
	repoRoot   string
	depsDir    string
	nativeDir  string
	modelDir   string // MODEL_DIR: a pre-baked DeepDoc weight tree to reuse before downloading
	ghProxy    string
	hfEndpoint string
	hfToken    string
	platform   platform
	retryDelay func(attempt int) // sleeps between download attempts
}

func newDownloader(out io.Writer, chinaMirrors bool) (*downloader, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	proxyEnv := os.Getenv("GH_PROXY")
	hfEndpoint := os.Getenv("HF_ENDPOINT")
	if chinaMirrors {
		proxyEnv = ghProxyURL
		hfEndpoint = chinaHFEndpoint
	}
	proxy, err := resolveGHProxy(proxyEnv)
	if err != nil {
		return nil, err
	}
	if hfEndpoint == "" {
		hfEndpoint = defaultHFEndpoint
	}

	return &downloader{
		out:        out,
		client:     newHTTPClient(),
		repoRoot:   root,
		depsDir:    filepath.Join(root, "deps"),
		nativeDir:  nativeLibsDir(os.Getenv("RAGFLOW_NATIVE_LIBS_DIR"), home),
		modelDir:   os.Getenv("MODEL_DIR"),
		ghProxy:    proxy,
		hfEndpoint: strings.TrimRight(hfEndpoint, "/"),
		hfToken:    os.Getenv("HF_TOKEN"),
		retryDelay: backoff,
	}, nil
}
