// Package ownership 提供集中化的用户所有权类型与角色系统。
package ownership

import (
	"fmt"
	"strings"
)

// UserRole 定义用户的权限角色。
type UserRole int

const (
	RoleRegular UserRole = iota
	RoleAdmin
	RoleOwner
)

// FromDBRole 从数据库字符串解析角色。
// 未知或缺失值回退到 Regular（最小权限原则）。
func FromDBRole(role string) UserRole {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "owner":
		return RoleOwner
	case "admin":
		return RoleAdmin
	default:
		return RoleRegular
	}
}

// AsDBRole 返回数据库持久化形式。
func (r UserRole) AsDBRole() string {
	switch r {
	case RoleOwner:
		return "owner"
	case RoleAdmin:
		return "admin"
	default:
		return "regular"
	}
}

// IsAdmin 返回角色是否具有管理权限（Owner 也是 Admin）。
func (r UserRole) IsAdmin() bool {
	return r == RoleAdmin || r == RoleOwner
}

// IsOwner 返回是否为部署所有者。
func (r UserRole) IsOwner() bool {
	return r == RoleOwner
}

// IsRegular 返回是否为普通用户。
func (r UserRole) IsRegular() bool {
	return r == RoleRegular
}

// UserIDError 是 UserID 构造错误。
type UserIDError struct {
	Reason string
}

func (e *UserIDError) Error() string {
	return fmt.Sprintf("invalid user id: %s", e.Reason)
}

// UserID 是带有角色的强类型用户标识符。
type UserID struct {
	id   string
	role UserRole
}

// New 创建经过验证的 UserID。拒绝空字符串和纯空白字符串。
func New(id string, role UserRole) (*UserID, error) {
	if id == "" {
		return nil, &UserIDError{Reason: "must not be empty"}
	}
	if strings.TrimSpace(id) == "" {
		return nil, &UserIDError{Reason: "must not be whitespace-only"}
	}
	return &UserID{id: id, role: role}, nil
}

// FromTrusted 从可信来源（如数据库行）创建 UserID，跳过验证。
func FromTrusted(id string, role UserRole) *UserID {
	return &UserID{id: id, role: role}
}

// AsStr 返回原始用户 ID 字符串。
func (u *UserID) AsStr() string {
	return u.id
}

// Role 返回附加的角色。
func (u *UserID) Role() UserRole {
	return u.role
}

// IsOwner 返回是否为所有者。
func (u *UserID) IsOwner() bool {
	return u.role.IsOwner()
}

// IsAdmin 返回是否具有管理权限。
func (u *UserID) IsAdmin() bool {
	return u.role.IsAdmin()
}

// IsRegular 返回是否为普通用户。
func (u *UserID) IsRegular() bool {
	return u.role.IsRegular()
}

// String 实现 fmt.Stringer，只显示 id（不包含角色）。
func (u *UserID) String() string {
	return u.id
}

// Equal 基于 id 比较，忽略角色。
func (u *UserID) Equal(other *UserID) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.id == other.id
}

// ResourceScope 定义工具或技能的作用域。
type ResourceScope int

const (
	ScopeUser ResourceScope = iota
	ScopeGlobal
)

// Owned 是拥有用户的资源接口。
type Owned interface {
	OwnerUserID() string
	IsOwnedBy(userID string) bool
}

// BaseOwned 提供 Owned 接口的基本实现。
type BaseOwned struct {
	UserID string
}

// OwnerUserID 返回资源所有者的用户 ID。
func (b *BaseOwned) OwnerUserID() string {
	return b.UserID
}

// IsOwnedBy 检查给定用户是否拥有此资源。
func (b *BaseOwned) IsOwnedBy(userID string) bool {
	return b.UserID == userID
}
