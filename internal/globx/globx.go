// Package globx expands log-path patterns. filepath.Glob has no "**", so this
// walks from the deepest literal directory, prunes subtrees that cannot match,
// and matches the remaining segments one by one.
package globx

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Expand turns a pattern like ~/.claude/projects/**/*.jsonl into the files that
// exist right now. A missing root yields an empty slice, not an error.
func Expand(pattern, home string) []string {
	root, pat, err := split(pattern, home)
	if err != nil || hasWildcard(root) {
		return nil
	}
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		segs := splitPath(rel)
		if d.IsDir() {
			if !MatchPrefix(pat, segs) {
				return fs.SkipDir
			}
			return nil
		}
		if MatchSegments(pat, segs) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// split separates the literal directory prefix from the pattern segments that
// still contain wildcards.
func split(pattern, home string) (string, []string, error) {
	p := strings.ReplaceAll(pattern, "\\", "/")
	if strings.HasPrefix(p, "~/") {
		p = filepath.ToSlash(home) + p[1:]
	}
	p = os.ExpandEnv(p)
	parts := strings.Split(p, "/")
	root := ""
	i := 0
	if len(parts) > 0 && parts[0] == "" {
		root = "/"
		i = 1
	} else if len(parts) > 0 && strings.HasSuffix(parts[0], ":") {
		root = parts[0] + "/"
		i = 1
	}
	for ; i < len(parts) && !hasWildcard(parts[i]); i++ {
		root = filepath.Join(root, parts[i])
	}
	var segs []string
	for ; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		segs = append(segs, parts[i])
	}
	if root == "" {
		root = "."
	}
	return root, segs, nil
}

func splitPath(p string) []string {
	p = filepath.ToSlash(p)
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" && s != "." {
			out = append(out, s)
		}
	}
	return out
}

func hasWildcard(s string) bool { return strings.ContainsAny(s, "*?[") }

// MatchSegments reports whether a path (already split) matches the pattern.
func MatchSegments(pat, path []string) bool {
	if len(pat) == 0 {
		return len(path) == 0
	}
	if pat[0] == "**" {
		// "**" consumes zero or more segments.
		if MatchSegments(pat[1:], path) {
			return true
		}
		if len(path) > 0 {
			return MatchSegments(pat, path[1:])
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	ok, err := filepath.Match(pat[0], path[0])
	if err != nil || !ok {
		return false
	}
	return MatchSegments(pat[1:], path[1:])
}

// MatchPrefix reports whether a path that starts with prefix could still match
// the pattern, which is the test used to prune directory walks.
func MatchPrefix(pat, prefix []string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(pat) == 0 {
		return false
	}
	if pat[0] == "**" {
		return MatchPrefix(pat[1:], prefix) || MatchPrefix(pat, prefix[1:])
	}
	ok, err := filepath.Match(pat[0], prefix[0])
	if err != nil || !ok {
		return false
	}
	return MatchPrefix(pat[1:], prefix[1:])
}

// Match reports whether a single path matches the pattern.
func Match(pattern, path, home string) bool {
	_, pat, err := split(pattern, home)
	if err != nil {
		return false
	}
	return MatchSegments(pat, splitPath(path))
}
