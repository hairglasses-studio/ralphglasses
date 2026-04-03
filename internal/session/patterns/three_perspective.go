// D3.3: Three-Perspective Review (AX/UX/DX) — specialized MultiReflexion
// with pre-configured Agent/User/Developer experience perspectives.
//
// Informed by Confucius (ArXiv 2512.10398): three-perspective agent platform
// with persistent cross-session principle distillation.
package patterns

// DefaultPerspectives returns the three AX/UX/DX perspectives from Confucius research.
func DefaultPerspectives() []ReflexionPerspective {
	return []ReflexionPerspective{
		{
			Name:        "AX",
			Description: "Agent eXperience — is the code agent-friendly?",
			SystemPrompt: `Review this code change from the Agent eXperience (AX) perspective.
Focus on:
- Are error messages structured and parseable by tools?
- Is the output format consistent and machine-readable?
- Are interfaces clear and well-documented for programmatic use?
- Can automated tools easily interact with this code?
- Are there any ambiguous behaviors that would confuse an agent?

Provide specific, actionable feedback.`,
		},
		{
			Name:        "UX",
			Description: "User eXperience — is the output clear for humans?",
			SystemPrompt: `Review this code change from the User eXperience (UX) perspective.
Focus on:
- Are user-facing messages clear and helpful?
- Do destructive operations require explicit confirmation?
- Are error messages understandable to non-technical users?
- Is progress feedback provided for long-running operations?
- Is the output formatted for readability (not walls of text)?

Provide specific, actionable feedback.`,
		},
		{
			Name:        "DX",
			Description: "Developer eXperience — is the code maintainable?",
			SystemPrompt: `Review this code change from the Developer eXperience (DX) perspective.
Focus on:
- Is the code well-structured and maintainable?
- Are there adequate tests for the changes?
- Does it follow existing conventions and patterns in the codebase?
- Is the code documented where non-obvious?
- Are there any potential performance or security concerns?
- Is error handling complete and consistent?

Provide specific, actionable feedback.`,
		},
	}
}

// ThreePerspectiveReview creates a MultiReflexion pre-configured with the
// AX/UX/DX perspectives from Confucius research.
func ThreePerspectiveReview() *MultiReflexion {
	return NewMultiReflexion(DefaultPerspectives())
}
