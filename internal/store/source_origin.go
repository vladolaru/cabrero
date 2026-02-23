package store

import (
	"path/filepath"
	"strings"
)

// InferOrigin parses a skill signal name (e.g., "superpowers:brainstorming")
// and returns (sourceName, origin).
//
// Colon-namespaced names -> plugin origin; bare names -> user origin.
func InferOrigin(signalName string) (name, origin string) {
	if i := strings.Index(signalName, ":"); i > 0 {
		return signalName[i+1:], "plugin:" + signalName[:i]
	}
	return signalName, "user"
}

// InferOriginFromPath parses an absolute file path and returns (sourceName, origin).
//
// Recognized patterns:
//   - ~/.claude/CLAUDE.md                                    -> ("CLAUDE.md", "user")
//   - ~/.claude/skills/<name>.md                             -> ("<name>", "user")
//   - ~/.claude/skills/<name>/SKILL.md                       -> ("<name>", "user")
//   - ~/.claude/plugins/cache/<mkt>/<plugin>/<ver>/skills/<skill>/SKILL.md
//     -> ("<skill>", "plugin:<plugin>")
//   - /any/path/to/<project>/CLAUDE.md                       -> ("CLAUDE.md (<project>)", "project:<project>")
func InferOriginFromPath(path, homeDir string) (name, origin string) {
	claudeDir := filepath.Join(homeDir, ".claude")

	// Paths inside ~/.claude/
	if strings.HasPrefix(path, claudeDir+"/") {
		rel := path[len(claudeDir)+1:] // e.g., "CLAUDE.md", "skills/foo.md", "plugins/cache/..."

		// ~/.claude/CLAUDE.md
		if rel == "CLAUDE.md" {
			return "CLAUDE.md", "user"
		}

		// ~/.claude/plugins/cache/<mkt>/<plugin>/<ver>/skills/<skill>/SKILL.md
		if strings.HasPrefix(rel, "plugins/cache/") {
			parts := strings.Split(rel, "/")
			// parts: [plugins, cache, <mkt>, <plugin>, <ver>, skills, <skill>, SKILL.md]
			if len(parts) >= 8 && parts[5] == "skills" {
				plugin := parts[3]
				skill := parts[6]
				return skill, "plugin:" + plugin
			}
		}

		// ~/.claude/skills/<name>.md (flat file)
		if strings.HasPrefix(rel, "skills/") && !strings.Contains(rel[len("skills/"):], "/") {
			base := filepath.Base(rel)
			ext := filepath.Ext(base)
			return strings.TrimSuffix(base, ext), "user"
		}

		// ~/.claude/skills/<name>/SKILL.md (directory)
		if strings.HasPrefix(rel, "skills/") {
			parts := strings.Split(rel, "/")
			if len(parts) >= 3 {
				return parts[1], "user"
			}
		}
	}

	// Any other CLAUDE.md -> project-level.
	base := filepath.Base(path)
	if base == "CLAUDE.md" {
		dir := filepath.Dir(path)
		project := filepath.Base(dir)
		return "CLAUDE.md (" + project + ")", "project:" + project
	}

	// Fallback: use the file's base name without extension.
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext), "user"
}
