package domain

// Skill is one agent skill loaded from a SKILL.md file — the same convention
// as eino's adk/middlewares/skill: a directory per skill under the skills
// root, SKILL.md with optional YAML frontmatter (name/description) and a
// markdown body holding the instructions.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Content is the markdown body (instructions). Empty in list results.
	Content string `json:"content,omitempty"`
	Path    string `json:"path,omitempty"`
}
