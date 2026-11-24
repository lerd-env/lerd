package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// envFramework is the framework whose env `lerd env` writes for a site, or why
// there is none to write.
func envFramework(site *config.Site, dir string) (*config.Framework, error) {
	name := site.Framework
	if name == "" {
		name, _ = config.DetectFrameworkForDir(dir)
	}
	if name == "" {
		return nil, fmt.Errorf("no framework detected for this site\nDefine one with 'lerd framework add' or add a framework YAML to %s", config.FrameworksDir())
	}
	fw, ok := config.GetFrameworkForDir(name, dir)
	if !ok {
		return nil, fmt.Errorf("framework %q is not defined\nDefine it with 'lerd framework add'", name)
	}
	if fw.Env.File == "" && fw.Env.Format == "" && len(fw.Env.Services) == 0 {
		return nil, fmt.Errorf("'lerd env' is not supported for %s\nConfigure the env section in the framework YAML to enable it", fw.Label)
	}
	return fw, nil
}

func envExampleFile(fw *config.Framework) string {
	if fw.Env.ExampleFile != "" {
		return fw.Env.ExampleFile
	}
	return ".env.example"
}

// WritesEnv reports whether `lerd env` has an env file to write for a site.
func WritesEnv(site *config.Site) bool {
	fw, err := envFramework(site, site.Path)
	if err != nil {
		return false
	}
	if len(fw.Env.Services) > 0 {
		return true
	}
	file, _ := fw.Env.ResolveWrite(site.Path)
	for _, name := range []string{file, envExampleFile(fw)} {
		if _, err := os.Stat(filepath.Join(site.Path, name)); err == nil {
			return true
		}
	}
	return false
}
