//go:build windows

package config

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const windowsUserEnvironmentKey = `Environment`

// PersistUserEnvVar persists key/value into Windows user environment variables.
func PersistUserEnvVar(key string, value string) error {
	normalizedKey := strings.TrimSpace(key)
	if err := ValidateEnvVarName(normalizedKey); err != nil {
		return err
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("config: env value contains newline")
	}

	envKey, _, err := registry.CreateKey(registry.CURRENT_USER, windowsUserEnvironmentKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("config: open windows user env: %w", err)
	}
	defer envKey.Close()

	if err := envKey.SetStringValue(normalizedKey, value); err != nil {
		return fmt.Errorf("config: set windows user env %q: %w", normalizedKey, err)
	}
	return nil
}

// DeleteUserEnvVar 删除 Windows 用户级环境变量，不存在时视为成功。
func DeleteUserEnvVar(key string) error {
	normalizedKey := strings.TrimSpace(key)
	if err := ValidateEnvVarName(normalizedKey); err != nil {
		return err
	}

	envKey, err := registry.OpenKey(registry.CURRENT_USER, windowsUserEnvironmentKey, registry.SET_VALUE)
	if err != nil {
		if isWindowsRegistryNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: open windows user env: %w", err)
	}
	defer envKey.Close()

	if err := envKey.DeleteValue(normalizedKey); err != nil {
		if isWindowsRegistryNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: delete windows user env %q: %w", normalizedKey, err)
	}
	return nil
}

// LookupUserEnvVar 查询 Windows 用户级环境变量，不存在时返回 exists=false。
func LookupUserEnvVar(key string) (string, bool, error) {
	normalizedKey := strings.TrimSpace(key)
	if err := ValidateEnvVarName(normalizedKey); err != nil {
		return "", false, err
	}

	envKey, err := registry.OpenKey(registry.CURRENT_USER, windowsUserEnvironmentKey, registry.QUERY_VALUE)
	if err != nil {
		if isWindowsRegistryNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("config: open windows user env: %w", err)
	}
	defer envKey.Close()

	value, _, err := envKey.GetStringValue(normalizedKey)
	if err != nil {
		if isWindowsRegistryNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("config: read windows user env %q: %w", normalizedKey, err)
	}
	return value, true, nil
}

// isWindowsRegistryNotExist 统一判断注册表“键/值不存在”错误，兼容包装错误场景。
func isWindowsRegistryNotExist(err error) bool {
	return errors.Is(err, registry.ErrNotExist)
}

// SupportsUserEnvPersistence 返回当前平台是否支持用户级环境变量持久化。
func SupportsUserEnvPersistence() bool {
	return true
}
