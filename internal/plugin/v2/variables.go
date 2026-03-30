package v2

import "strings"

// VarContext holds the current state for variable substitution in plugin
// command templates.
type VarContext struct {
	Repo      string
	Session   string
	Namespace string
	Provider  string
	Model     string
	WorkDir   string
	HomeDir   string
}

// ResolveOption configures variable resolution behavior.
type ResolveOption func(*resolveOpts)

type resolveOpts struct {
	emptyUndefined bool
}

// WithEmptyUndefined causes undefined variables to resolve to an empty string
// instead of being left as-is.
func WithEmptyUndefined() ResolveOption {
	return func(o *resolveOpts) { o.emptyUndefined = true }
}

// Resolve substitutes known variables in template using values from ctx.
// Supported forms: $VAR and ${VAR}. Use $$ to produce a literal $.
func Resolve(template string, ctx VarContext, opts ...ResolveOption) string {
	var cfg resolveOpts
	for _, o := range opts {
		o(&cfg)
	}

	vars := map[string]string{
		"REPO":      ctx.Repo,
		"SESSION":   ctx.Session,
		"NAMESPACE": ctx.Namespace,
		"PROVIDER":  ctx.Provider,
		"MODEL":     ctx.Model,
		"WORKDIR":   ctx.WorkDir,
		"HOME":      ctx.HomeDir,
	}

	var b strings.Builder
	b.Grow(len(template))

	i := 0
	for i < len(template) {
		if template[i] != '$' {
			b.WriteByte(template[i])
			i++
			continue
		}

		// Escaped dollar: $$ → literal $
		if i+1 < len(template) && template[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}

		// ${VAR} form
		if i+1 < len(template) && template[i+1] == '{' {
			end := strings.IndexByte(template[i+2:], '}')
			if end < 0 {
				// Unterminated brace — emit as-is.
				b.WriteByte('$')
				i++
				continue
			}
			name := template[i+2 : i+2+end]
			if val, ok := vars[name]; ok {
				b.WriteString(val)
			} else if cfg.emptyUndefined {
				// leave empty
			} else {
				b.WriteString("${")
				b.WriteString(name)
				b.WriteByte('}')
			}
			i = i + 2 + end + 1
			continue
		}

		// $VAR form — consume uppercase letters and underscores.
		j := i + 1
		for j < len(template) && (template[j] >= 'A' && template[j] <= 'Z' || template[j] == '_') {
			j++
		}
		if j == i+1 {
			// Bare $ not followed by a letter — emit as-is.
			b.WriteByte('$')
			i++
			continue
		}
		name := template[i+1 : j]
		if val, ok := vars[name]; ok {
			b.WriteString(val)
		} else if cfg.emptyUndefined {
			// leave empty
		} else {
			b.WriteByte('$')
			b.WriteString(name)
		}
		i = j
	}

	return b.String()
}

// ResolveAll applies Resolve to every element in templates.
func ResolveAll(templates []string, ctx VarContext, opts ...ResolveOption) []string {
	out := make([]string, len(templates))
	for i, t := range templates {
		out[i] = Resolve(t, ctx, opts...)
	}
	return out
}
