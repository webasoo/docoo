package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/webasoo/docoo/core"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil || len(*s) == 0 {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func commandName() string {
	if len(os.Args) == 0 {
		return "docoo"
	}
	base := filepath.Base(os.Args[0])
	if strings.HasSuffix(strings.ToLower(base), ".exe") {
		base = base[:len(base)-len(filepath.Ext(base))]
	}
	base = strings.TrimSpace(base)
	if base == "" || strings.EqualFold(base, "main") {
		return "docoo"
	}
	return base
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate", "gen":
		if err := runGenerate(os.Args[2:]); err != nil {
			log.Fatalf("docoo generate: %v", err)
		}
	case "help", "-h", "--help", "-help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	output := fs.String("o", "", "output file (default <module-root>/openapi.json)")
	root := fs.String("root", "", "workspace root to scan (defaults to current module)")
	title := fs.String("title", "", "override the generated document title")
	enableAuthUI := fs.Bool("enable-auth", false, "include Bearer auth + global security in generated openapi.json")
	var routes stringSliceFlag
	var skips stringSliceFlag
	fs.Var(&routes, "route", "additional directory to scan for routes (repeatable)")
	fs.Var(&skips, "skip", "path prefix to exclude from documentation (repeatable)")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s generate [flags]\n\n", commandName())
		fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output())
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg := core.ProjectConfig{
		RoutePaths:   routes,
		SkipPrefixes: skips,
		EnableAuthUI: *enableAuthUI,
	}
	if strings.TrimSpace(*root) != "" {
		cfg.WorkspaceRoot = strings.TrimSpace(*root)
	}
	if strings.TrimSpace(*output) != "" {
		cfg.OutputPath = strings.TrimSpace(*output)
	}
	if strings.TrimSpace(*title) != "" {
		cfg.ProjectName = strings.TrimSpace(*title)
	}

	dst, _, err := core.GenerateAndSaveOpenAPI(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("✅ generated %s\n", dst)
	return nil
}

func printUsage() {
	cmd := commandName()
	fmt.Printf(`%s - Go DOCOO CLI

Usage: %s <command> [arguments]

Available Commands:
  generate    Discover routes and emit openapi.json
  help        Show this help message

Examples:
  %[1]s generate
  %[1]s generate -o api/openapi.json
  %[1]s generate -route ./cmd/api -skip /internal
`, cmd, cmd)
}
