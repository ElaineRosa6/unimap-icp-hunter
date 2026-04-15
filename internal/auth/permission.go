package auth

import (
	"sync"

	unierror "github.com/unimap-icp-hunter/project/internal/error"
)

// Permission 权限定义
type Permission string

const (
	// 系统权限
	PermissionAdmin        Permission = "admin"
	PermissionRead         Permission = "read"
	PermissionWrite        Permission = "write"
	PermissionDelete       Permission = "delete"
	
	// API权限
	PermissionAPIExecute   Permission = "api:execute"
	PermissionAPIRead      Permission = "api:read"
	PermissionAPIWrite     Permission = "api:write"
	
	// 任务权限
	PermissionTaskCreate   Permission = "task:create"
	PermissionTaskRead     Permission = "task:read"
	PermissionTaskUpdate   Permission = "task:update"
	PermissionTaskDelete   Permission = "task:delete"
	
	// 节点权限
	PermissionNodeRegister Permission = "node:register"
	PermissionNodeManage   Permission = "node:manage"
	
	// 插件权限
	PermissionPluginExecute Permission = "plugin:execute"
	PermissionPluginManage  Permission = "plugin:manage"
	
	// 配置权限
	PermissionConfigRead   Permission = "config:read"
	PermissionConfigWrite  Permission = "config:write"
	
	// 审计权限
	PermissionAuditRead    Permission = "audit:read"
)

// Role 角色定义
type Role struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

// 预定义角色
var (
	RoleAdmin = Role{
		Name:        "admin",
		Description: "管理员角色，拥有所有权限",
		Permissions: []Permission{
			PermissionAdmin,
			PermissionRead,
			PermissionWrite,
			PermissionDelete,
			PermissionAPIExecute,
			PermissionAPIRead,
			PermissionAPIWrite,
			PermissionTaskCreate,
			PermissionTaskRead,
			PermissionTaskUpdate,
			PermissionTaskDelete,
			PermissionNodeRegister,
			PermissionNodeManage,
			PermissionPluginExecute,
			PermissionPluginManage,
			PermissionConfigRead,
			PermissionConfigWrite,
			PermissionAuditRead,
		},
	}

	RoleOperator = Role{
		Name:        "operator",
		Description: "操作员角色，拥有任务管理权限",
		Permissions: []Permission{
			PermissionRead,
			PermissionWrite,
			PermissionAPIExecute,
			PermissionAPIRead,
			PermissionAPIWrite,
			PermissionTaskCreate,
			PermissionTaskRead,
			PermissionTaskUpdate,
			PermissionPluginExecute,
			PermissionConfigRead,
		},
	}

	RoleReadOnly = Role{
		Name:        "readonly",
		Description: "只读角色，只能查看数据",
		Permissions: []Permission{
			PermissionRead,
			PermissionAPIRead,
			PermissionTaskRead,
			PermissionConfigRead,
			PermissionAuditRead,
		},
	}

	RoleNode = Role{
		Name:        "node",
		Description: "节点角色，用于分布式节点认证",
		Permissions: []Permission{
			PermissionAPIExecute,
			PermissionAPIRead,
			PermissionAPIWrite,
			PermissionNodeRegister,
			PermissionPluginExecute,
		},
	}
)

// PermissionManager 权限管理器
type PermissionManager struct {
	roles      map[string]*Role
	mutex      sync.RWMutex
}

// NewPermissionManager 创建权限管理器
func NewPermissionManager() *PermissionManager {
	manager := &PermissionManager{
		roles: make(map[string]*Role),
	}
	
	// 注册预定义角色
	manager.RegisterRole(RoleAdmin)
	manager.RegisterRole(RoleOperator)
	manager.RegisterRole(RoleReadOnly)
	manager.RegisterRole(RoleNode)
	
	return manager
}

// RegisterRole 注册角色
func (m *PermissionManager) RegisterRole(role Role) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.roles[role.Name] = &role
}

// GetRole 获取角色
func (m *PermissionManager) GetRole(name string) *Role {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	return m.roles[name]
}

// HasPermission 检查角色是否有指定权限
func (m *PermissionManager) HasPermission(roleName string, permission Permission) bool {
	role := m.GetRole(roleName)
	if role == nil {
		return false
	}
	
	for _, p := range role.Permissions {
		if p == permission || p == PermissionAdmin {
			return true
		}
	}
	
	return false
}

// HasAnyPermission 检查角色是否有任一指定权限
func (m *PermissionManager) HasAnyPermission(roleName string, permissions ...Permission) bool {
	for _, permission := range permissions {
		if m.HasPermission(roleName, permission) {
			return true
		}
	}
	
	return false
}

// HasAllPermissions 检查角色是否有所有指定权限
func (m *PermissionManager) HasAllPermissions(roleName string, permissions ...Permission) bool {
	for _, permission := range permissions {
		if !m.HasPermission(roleName, permission) {
			return false
		}
	}
	
	return true
}

// CheckPermission 检查权限并返回错误
func (m *PermissionManager) CheckPermission(roleName string, permission Permission) error {
	if !m.HasPermission(roleName, permission) {
		return unierror.APIForbidden("Insufficient permissions for: %s", permission)
	}
	
	return nil
}

// CheckAnyPermission 检查任一权限并返回错误
func (m *PermissionManager) CheckAnyPermission(roleName string, permissions ...Permission) error {
	if !m.HasAnyPermission(roleName, permissions...) {
		return unierror.APIForbidden("Insufficient permissions")
	}
	
	return nil
}

// CheckAllPermissions 检查所有权限并返回错误
func (m *PermissionManager) CheckAllPermissions(roleName string, permissions ...Permission) error {
	if !m.HasAllPermissions(roleName, permissions...) {
		return unierror.APIForbidden("Insufficient permissions")
	}
	
	return nil
}

// GetPermissions 获取角色的所有权限
func (m *PermissionManager) GetPermissions(roleName string) []Permission {
	role := m.GetRole(roleName)
	if role == nil {
		return []Permission{}
	}
	
	return role.Permissions
}

// GetAllRoles 获取所有角色
func (m *PermissionManager) GetAllRoles() []*Role {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	var roles []*Role
	for _, role := range m.roles {
		roles = append(roles, role)
	}
	
	return roles
}

// CreateCustomRole 创建自定义角色
func (m *PermissionManager) CreateCustomRole(name, description string, permissions []Permission) *Role {
	role := &Role{
		Name:        name,
		Description: description,
		Permissions: permissions,
	}
	
	m.RegisterRole(*role)
	return role
}

// UpdateRole 更新角色权限
func (m *PermissionManager) UpdateRole(name string, permissions []Permission) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	role, exists := m.roles[name]
	if !exists {
		return false
	}
	
	role.Permissions = permissions
	return true
}

// DeleteRole 删除角色
func (m *PermissionManager) DeleteRole(name string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	// 不允许删除预定义角色
	if name == RoleAdmin.Name || name == RoleOperator.Name || name == RoleReadOnly.Name || name == RoleNode.Name {
		return false
	}
	
	_, exists := m.roles[name]
	if !exists {
		return false
	}
	
	delete(m.roles, name)
	return true
}