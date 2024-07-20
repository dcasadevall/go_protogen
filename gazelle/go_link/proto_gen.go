package golink

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

type xlang struct {
	mu                  sync.Mutex
	allProtoLinks       []string
	rootProcessed       atomic.Bool
	modifyRootBuildOnce sync.Once
}

func NewLanguage() language.Language {
	return &xlang{}
}

func (x *xlang) Name() string {
	return "go_proto_link"
}

func (x *xlang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"go_proto_link": {},
		"filegroup": {
			NonEmptyAttrs: map[string]bool{"srcs": true},
		},
	}
}

func (x *xlang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{
			Name:    "@golink//proto:proto.bzl",
			Symbols: []string{"go_proto_link"},
		},
	}
}

func (x *xlang) RegisterFlags(fs *flag.FlagSet, cmd string, c *config.Config) {
}

func (x *xlang) CheckFlags(fs *flag.FlagSet, c *config.Config) error {
	return nil
}

func (x *xlang) KnownDirectives() []string {
	return nil
}

func (x *xlang) Configure(c *config.Config, rel string, f *rule.File) {
	if f == nil {
		return
	}

	// Accumulate all go_proto_link rules
	for _, r := range f.Rules {
		if r.Kind() == "go_proto_link" {
			x.mu.Lock()
			x.allProtoLinks = append(x.allProtoLinks, "//"+rel+":"+r.Name())
			x.mu.Unlock()
		}
	}

	// Mark that the root is being processed
	if rel == "" {
		x.rootProcessed.Store(true)
	}
}

func (x *xlang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	rules := make([]*rule.Rule, 0)
	imports := make([]interface{}, 0)

	for _, r := range args.OtherGen {
		if r.Kind() == "go_proto_library" {
			depName := r.Name()
			protoLinkName := r.Name() + "_link"
			protoLinkRule := rule.NewRule("go_proto_link", protoLinkName)
			protoLinkRule.SetAttr("dep", ":"+depName)
			protoLinkRule.SetAttr("version", "v1")
			rules = append(rules, protoLinkRule)
			imports = append(imports, nil)
			x.mu.Lock()
			x.allProtoLinks = append(x.allProtoLinks, "//"+args.Rel+":"+protoLinkName)
			x.mu.Unlock()
		}
	}

	return language.GenerateResult{
		Gen:     rules,
		Imports: imports,
	}
}

func (x *xlang) Fix(c *config.Config, f *rule.File) {
	// Do any migrations here
}

func (x *xlang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	return nil
}

func (x *xlang) Embeds(r *rule.Rule, from label.Label) []label.Label {
	return nil
}

// Resolve is called to finalize dependencies and ensure that the root BUILD file is updated after all packages have been processed.
// We use sync.Once to guarantee that the root BUILD file is modified only once, preventing duplicate modifications.
// This method is called after all packages have been configured, ensuring that all proto links have been accumulated.
func (x *xlang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	// Ensure that root `BUILD` file modification happens only once
	x.modifyRootBuildOnce.Do(func() {
		// Check if the root directory has been processed
		if x.rootProcessed.Load() {
			// Lock to ensure thread-safe access to allProtoLinks
			x.mu.Lock()
			defer x.mu.Unlock()

			if len(x.allProtoLinks) > 0 {
				// Create a new filegroup rule to include all proto links
				filegroupRule := rule.NewRule("filegroup", "all_proto_links")
				filegroupRule.SetAttr("srcs", x.allProtoLinks)

				rootBuildPath := filepath.Join(c.RepoRoot, "BUILD")
				rootFile, err := rule.LoadFile(rootBuildPath, "")
				if err != nil {
					if os.IsNotExist(err) {
						// Create an empty file if it does not exist
						rootFile = rule.EmptyFile("BUILD", "")
					} else {
						log.Printf("Error loading root BUILD file: %v", err)
						return // Exit if there's an error loading the file
					}
				}

				// Check if the filegroup rule already exists and delete it if found
				for _, r := range rootFile.Rules {
					if r.Kind() == "filegroup" && r.Name() == "all_proto_links" {
						r.Delete()
						break
					}
				}

				// Properly insert the new rule into the root BUILD file
				filegroupRule.Insert(rootFile)

				// Save the changes to the root BUILD file
				if err := rootFile.Save(rootBuildPath); err != nil {
					log.Printf("Error saving root BUILD file: %v", err)
				}
			}
		}
	})
}
