package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// LoadBundledSkills 从 embed.FS 的指定目录加载所有 SKILL.md 文件。
// 每个文件应为 frontmatter + markdown body 格式。
func LoadBundledSkills(fsys embed.FS, dir string) ([]*Skill, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read bundled skills dir: %w", err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		data, err := fsys.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read skill file %q: %w", name, err)
		}

		manifest, body, err := ParseManifest(string(data))
		if err != nil {
			// 缺少 frontmatter 时使用文件名作为名称
			base := strings.TrimSuffix(name, filepath.Ext(name))
			manifest = &Manifest{
				Name:        base,
				Version:     "0.1.0",
				Description: "Bundled skill",
			}
			body = string(data)
		}

		skills = append(skills, &Skill{
			Manifest: *manifest,
			Content:  body,
			Trust:    TrustTrusted,
			Source:   SourceBundled,
		})
	}

	return skills, nil
}
