package go_link

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

type xlang struct {
	allProtoLinks []string
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
	// No specific configuration needed for this example
}

func (x *xlang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	rules := make([]*rule.Rule, 0)
	imports := make([]interface{}, 0)

	for _, r := range args.OtherGen {
		if r.Kind() == "go_proto_library" {
			depName := r.Name()
			protoLinkRule := rule.NewRule("go_proto_link", r.Name()+"_link")
			protoLinkRule.SetAttr("dep", ":"+depName)
			protoLinkRule.SetAttr("version", "v1")
			protoLinkRule.SetAttr("visibility", []string{"//visibility:public"})
			rules = append(rules, protoLinkRule)
			imports = append(imports, nil)
			x.allProtoLinks = append(x.allProtoLinks, "//"+args.Rel+":"+protoLinkRule.Name())
		}
	}

	return language.GenerateResult{
		Gen:     rules,
		Imports: imports,
	}
}

func (x *xlang) Fix(c *config.Config, f *rule.File) {
	// No specific fix needed for this example
}

func (x *xlang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	return nil
}

func (x *xlang) Embeds(r *rule.Rule, from label.Label) []label.Label {
	return nil
}

func (x *xlang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	// Defer to ensure root `BUILD` modification happens only once
	defer func() {
		rootBuildPath := filepath.Join(c.RepoRoot, "BUILD")
		rootFile, err := rule.LoadFile(rootBuildPath, "")
		if err != nil {
			log.Printf("Error loading root BUILD file: %v", err)
			return // Exit if there's an error loading the file
		}

		// Check for existing multirun rule and delete if it exists
		for _, r := range rootFile.Rules {
			if r.Kind() == "multirun" && r.Name() == "go_proto_link" {
				r.Delete()
			}
		}

		// Create a new multirun rule to include all proto links
		multirunRule := rule.NewRule("multirun", "go_proto_link")
		multirunRule.SetAttr("commands", x.allProtoLinks)
		multirunRule.SetAttr("jobs", 0)
		multirunRule.Insert(rootFile)

		// Ensure the load statement is present
		load := rule.NewLoad("@rules_multirun//:defs.bzl")
		load.Add("command")
		load.Add("multirun")
		load.Insert(rootFile, 0)

		if err := rootFile.Save(rootBuildPath); err != nil {
			log.Printf("Error saving root BUILD file: %v", err)
		}
	}()
}
