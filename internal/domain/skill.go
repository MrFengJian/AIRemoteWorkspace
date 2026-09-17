package domain

// Skill is one agent skill loaded from a skill directory — the same convention
// as eino's adk/middlewares/skill (and the wider Agent Skills ecosystem): a
// directory per skill under the skills root, SKILL.md with optional YAML
// frontmatter (name/description) and a markdown body holding the instructions,
// plus optional bundled files (scripts/, references/, …) the agent can read on
// demand.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Content is the markdown body (instructions). Empty in list results.
	Content string `json:"content,omitempty"`
	Path    string `json:"path,omitempty"`
	// Files lists the skill's bundled files (relative, slash-separated,
	// SKILL.md excluded), sorted. Empty for single-file packs. The agent
	// reads them via the skill tool's path argument.
	Files []string `json:"files,omitempty"`
	// Builtin marks skills seeded from the binary's embedded scenario packs
	// (the diagnosis knowledge base). They are only shown as a badge and
	// re-seeded when missing unless the user deleted them (dismissed).
	Builtin bool `json:"builtin,omitempty"`
}
