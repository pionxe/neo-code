package skills

import "strings"

func allowBySource(input ListInput, descriptor Descriptor) bool {
	if len(input.SourceKinds) == 0 {
		return true
	}
	for _, kind := range input.SourceKinds {
		if descriptor.Source.Kind == kind {
			return true
		}
	}
	return false
}

func allowByScope(input ListInput, descriptor Descriptor) bool {
	if len(input.Scopes) > 0 {
		for _, scope := range input.Scopes {
			if descriptor.Scope == scope {
				return true
			}
		}
		return false
	}

	switch descriptor.Scope {
	case ScopeWorkspace:
		return strings.TrimSpace(input.Workspace) != ""
	default:
		return true
	}
}
