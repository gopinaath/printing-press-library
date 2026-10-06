package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	tbDocumentsDir = tbKnownDocumentsDir
	tbFinalPath    = tbResolveFinalPath
)

func tbMCPSurface() bool {
	return os.Getenv("THUNDERBIRD_LEARN_SURFACE") == "mcp" || strings.TrimSpace(os.Getenv(mcpBoundProfileEnv)) != ""
}

func tbResolveMCPFile(flag, p string) (string, error) {
	root, err := tbDocumentsDir()
	if err != nil {
		return "", fmt.Errorf("%s: cannot locate the Documents folder: %w", flag, err)
	}
	rootFinal, err := tbFinalPath(root)
	if err != nil {
		return "", fmt.Errorf("%s: cannot resolve the Documents folder %q: %w", flag, root, err)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", flag, p, err)
	}
	final, err := tbFinalPath(abs)
	if err != nil {
		return "", fmt.Errorf("%s %q: not a readable file", flag, p)
	}
	rel, err := filepath.Rel(rootFinal, final)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s %q: via MCP only files inside the Documents folder (%s) can be used", flag, p, rootFinal)
	}
	return final, nil
}

func tbSplitMCPAttachments(values []string) []string {
	var out []string
	for _, v := range values {
		if info, err := os.Stat(strings.TrimSpace(v)); err == nil && !info.IsDir() {
			out = append(out, strings.TrimSpace(v))
			continue
		}
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func tbResolveMCPAttachment(p string) (string, error) {
	return tbResolveMCPFile("--attach", p)
}
