//go:build windows

package auth

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	authSDDLDiscretionaryACL = "D:P"
)

// applyAuthDirPermission 在 Windows 平台将凭证目录 ACL 收紧为 SYSTEM/Administrators/当前用户可访问。
func applyAuthDirPermission(dir string) error {
	return applyRestrictedACL(dir, true)
}

// applyAuthFilePermission 在 Windows 平台将凭证文件 ACL 收紧为 SYSTEM/Administrators/当前用户可访问。
func applyAuthFilePermission(path string) error {
	return applyRestrictedACL(path, false)
}

// applyRestrictedACL 根据对象类型写入最小化 ACL，避免凭证被其他本地用户读取。
func applyRestrictedACL(path string, isDir bool) error {
	securityDescriptor, err := buildAuthSecurityDescriptor(isDir)
	if err != nil {
		return err
	}

	dacl, _, err := securityDescriptor.DACL()
	if err != nil {
		return fmt.Errorf("gateway auth: parse dacl: %w", err)
	}
	owner, _, err := securityDescriptor.Owner()
	if err != nil {
		return fmt.Errorf("gateway auth: parse owner sid: %w", err)
	}
	group, _, err := securityDescriptor.Group()
	if err != nil {
		return fmt.Errorf("gateway auth: parse group sid: %w", err)
	}

	securityInfo := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION)
	if owner != nil {
		securityInfo |= windows.OWNER_SECURITY_INFORMATION
	}
	if group != nil {
		securityInfo |= windows.GROUP_SECURITY_INFORMATION
	}

	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("gateway auth: apply acl path is empty")
	}
	if err := windows.SetNamedSecurityInfo(trimmedPath, windows.SE_FILE_OBJECT, securityInfo, owner, group, dacl, nil); err != nil {
		return fmt.Errorf("gateway auth: apply acl: %w", err)
	}
	return nil
}

// buildAuthSecurityDescriptor 生成用于凭证目录/文件的最小权限安全描述符。
func buildAuthSecurityDescriptor(isDir bool) (*windows.SECURITY_DESCRIPTOR, error) {
	currentUserSID, err := currentProcessUserSID()
	if err != nil {
		return nil, err
	}

	systemSID, err := wellKnownSIDString(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("gateway auth: resolve local-system sid: %w", err)
	}
	administratorsSID, err := wellKnownSIDString(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, fmt.Errorf("gateway auth: resolve administrators sid: %w", err)
	}

	allowAccessAce := allowGenericAllAce
	if isDir {
		allowAccessAce = allowGenericAllInheritedAce
	}

	sddl := fmt.Sprintf(
		"%s(%s)(%s)(%s)",
		authSDDLDiscretionaryACL,
		allowAccessAce(systemSID),
		allowAccessAce(administratorsSID),
		allowAccessAce(currentUserSID),
	)

	securityDescriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, fmt.Errorf("gateway auth: parse security descriptor: %w", err)
	}
	return securityDescriptor, nil
}

// currentProcessUserSID 返回当前进程所属用户的 SID。
func currentProcessUserSID() (string, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("gateway auth: query current token user: %w", err)
	}
	if tokenUser == nil || tokenUser.User.Sid == nil {
		return "", fmt.Errorf("gateway auth: current token user sid is empty")
	}
	return tokenUser.User.Sid.String(), nil
}

// wellKnownSIDString 将系统内置 SID 类型转换为 SID 字符串。
func wellKnownSIDString(sidType windows.WELL_KNOWN_SID_TYPE) (string, error) {
	sid, err := windows.CreateWellKnownSid(sidType)
	if err != nil {
		return "", err
	}
	return sid.String(), nil
}

// allowGenericAllAce 生成单个 SID 的“完全控制”ACE。
func allowGenericAllAce(sid string) string {
	return fmt.Sprintf("A;;GA;;;%s", sid)
}

// allowGenericAllInheritedAce 生成带继承标记的“完全控制”ACE（用于目录）。
func allowGenericAllInheritedAce(sid string) string {
	return fmt.Sprintf("A;OICI;GA;;;%s", sid)
}
