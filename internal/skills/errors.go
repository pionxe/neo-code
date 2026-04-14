package skills

import "errors"

var (
	// ErrSkillNotFound indicates registry cannot find a skill by id.
	ErrSkillNotFound = errors.New("skills: skill not found")
	// ErrEmptyContent indicates parsed skill content has no usable instruction.
	ErrEmptyContent = errors.New("skills: content is empty")
	// ErrSkillRootNotFound indicates configured local root does not exist.
	ErrSkillRootNotFound = errors.New("skills: root directory not found")
)
