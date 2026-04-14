package session

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// TodoStatus 表示 Todo 项的稳定状态枚举。
type TodoStatus string

const (
	// TodoStatusPending 表示尚未开始的待办项。
	TodoStatusPending TodoStatus = "pending"
	// TodoStatusInProgress 表示正在执行中的待办项。
	TodoStatusInProgress TodoStatus = "in_progress"
	// TodoStatusCompleted 表示已经完成的待办项。
	TodoStatusCompleted TodoStatus = "completed"
)

// TodoItem 表示会话级结构化待办项。
type TodoItem struct {
	ID           string     `json:"id"`
	Content      string     `json:"content"`
	Status       TodoStatus `json:"status"`
	Dependencies []string   `json:"dependencies,omitempty"`
	Priority     int        `json:"priority,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Clone 返回 Todo 项的深拷贝，避免依赖切片共享底层存储。
func (i TodoItem) Clone() TodoItem {
	i.Dependencies = append([]string(nil), i.Dependencies...)
	return i
}

// Valid 判断当前状态值是否属于受支持的 Todo 状态。
func (s TodoStatus) Valid() bool {
	switch s {
	case TodoStatusPending, TodoStatusInProgress, TodoStatusCompleted:
		return true
	default:
		return false
	}
}

// FindTodo 按 ID 查找 Todo 项并返回深拷贝结果。
func (s Session) FindTodo(id string) (TodoItem, bool) {
	id, err := ensureTodoID(id)
	if err != nil {
		return TodoItem{}, false
	}

	for _, item := range s.Todos {
		if item.ID == id {
			return item.Clone(), true
		}
	}
	return TodoItem{}, false
}

// AddTodo 向会话追加一个新的 Todo 项，并补齐默认状态与时间戳。
func (s *Session) AddTodo(item TodoItem) error {
	if s == nil {
		return errors.New("session: session is nil")
	}

	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}

	normalized, err := normalizeAndValidateTodos(append(append([]TodoItem(nil), s.Todos...), item))
	if err != nil {
		return err
	}

	s.Todos = normalized
	return nil
}

// UpdateTodoStatus 按 ID 更新 Todo 状态，并刷新该项的更新时间。
func (s *Session) UpdateTodoStatus(id string, status TodoStatus) error {
	if s == nil {
		return errors.New("session: session is nil")
	}

	var err error
	id, err = ensureTodoID(id)
	if err != nil {
		return err
	}
	if !status.Valid() {
		return fmt.Errorf("session: invalid todo status %q", status)
	}

	items := append([]TodoItem(nil), s.Todos...)
	for i := range items {
		if items[i].ID != id {
			continue
		}
		items[i].Status = status
		items[i].UpdatedAt = time.Now()

		normalized, err := normalizeAndValidateTodos(items)
		if err != nil {
			return err
		}
		s.Todos = normalized
		return nil
	}

	return fmt.Errorf("session: todo %q not found", id)
}

// DeleteTodo 按 ID 删除 Todo 项，并在删除前显式阻止仍被其他 Todo 依赖的记录。
func (s *Session) DeleteTodo(id string) error {
	if s == nil {
		return errors.New("session: session is nil")
	}

	var err error
	id, err = ensureTodoID(id)
	if err != nil {
		return err
	}
	if dependents := findTodoDependents(s.Todos, id); len(dependents) > 0 {
		return fmt.Errorf("session: todo %q is still required by %s", id, strings.Join(dependents, ", "))
	}

	items := make([]TodoItem, 0, len(s.Todos))
	found := false
	for _, item := range s.Todos {
		if item.ID == id {
			found = true
			continue
		}
		items = append(items, item)
	}
	if !found {
		return fmt.Errorf("session: todo %q not found", id)
	}

	normalized, err := normalizeAndValidateTodos(items)
	if err != nil {
		return err
	}
	s.Todos = normalized
	return nil
}

// findTodoDependents 返回依赖指定 Todo ID 的所有待办项 ID，并保持原有顺序。
func findTodoDependents(items []TodoItem, id string) []string {
	if len(items) == 0 || strings.TrimSpace(id) == "" {
		return nil
	}

	dependents := make([]string, 0)
	for _, item := range items {
		for _, dependency := range item.Dependencies {
			if dependency == id {
				dependents = append(dependents, item.ID)
				break
			}
		}
	}
	if len(dependents) == 0 {
		return nil
	}
	return dependents
}

// normalizeAndValidateTodos 统一收敛 Todo 的文本、状态与依赖合法性。
func normalizeAndValidateTodos(items []TodoItem) ([]TodoItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	normalized := make([]TodoItem, 0, len(items))
	ids := make(map[string]struct{}, len(items))
	for _, item := range items {
		next, err := normalizeTodoItem(item)
		if err != nil {
			return nil, err
		}
		if _, exists := ids[next.ID]; exists {
			return nil, fmt.Errorf("session: duplicate todo id %q", next.ID)
		}
		ids[next.ID] = struct{}{}
		normalized = append(normalized, next)
	}

	for _, item := range normalized {
		for _, dependency := range item.Dependencies {
			if dependency == item.ID {
				return nil, fmt.Errorf("session: todo %q cannot depend on itself", item.ID)
			}
			if _, exists := ids[dependency]; !exists {
				return nil, fmt.Errorf("session: todo %q references unknown dependency %q", item.ID, dependency)
			}
		}
	}

	return normalized, nil
}

// normalizeTodoItem 规范化单个 Todo 项，确保持久化结构稳定。
func normalizeTodoItem(item TodoItem) (TodoItem, error) {
	item = item.Clone()
	item.ID = strings.TrimSpace(item.ID)
	item.Content = strings.TrimSpace(item.Content)
	item.Dependencies = normalizeTodoDependencies(item.Dependencies)
	if item.Status == "" {
		item.Status = TodoStatusPending
	}

	switch {
	case item.ID == "":
		return TodoItem{}, errors.New("session: todo id is empty")
	case item.Content == "":
		return TodoItem{}, fmt.Errorf("session: todo %q content is empty", item.ID)
	case !item.Status.Valid():
		return TodoItem{}, fmt.Errorf("session: invalid todo status %q", item.Status)
	}

	return item, nil
}

// normalizeTodoDependencies 对依赖列表做去空白、去重并保持顺序。
func normalizeTodoDependencies(dependencies []string) []string {
	if len(dependencies) == 0 {
		return nil
	}

	result := make([]string, 0, len(dependencies))
	seen := make(map[string]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		if _, exists := seen[dependency]; exists {
			continue
		}
		seen[dependency] = struct{}{}
		result = append(result, dependency)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ensureTodoID 统一校验并返回规范化后的 Todo ID。
func ensureTodoID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("session: todo id is empty")
	}
	return id, nil
}
