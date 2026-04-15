package auth

import (
	"testing"
)

// --- PermissionManager Tests ---

func TestNewPermissionManager(t *testing.T) {
	pm := NewPermissionManager()
	if pm == nil {
		t.Fatal("expected non-nil PermissionManager")
	}
	// Verify predefined roles are registered
	if pm.GetRole("admin") == nil {
		t.Error("expected 'admin' role to be registered")
	}
	if pm.GetRole("operator") == nil {
		t.Error("expected 'operator' role to be registered")
	}
	if pm.GetRole("readonly") == nil {
		t.Error("expected 'readonly' role to be registered")
	}
	if pm.GetRole("node") == nil {
		t.Error("expected 'node' role to be registered")
	}
}

func TestPredefinedRoles(t *testing.T) {
	tests := []struct {
		roleName   string
		permission Permission
		shouldHave bool
	}{
		// Admin has everything
		{"admin", PermissionRead, true},
		{"admin", PermissionWrite, true},
		{"admin", PermissionDelete, true},
		{"admin", PermissionAPIExecute, true},
		{"admin", PermissionTaskCreate, true},
		{"admin", PermissionNodeRegister, true},
		{"admin", PermissionPluginManage, true},
		{"admin", PermissionConfigWrite, true},
		{"admin", PermissionAuditRead, true},

		// Operator
		{"operator", PermissionRead, true},
		{"operator", PermissionWrite, true},
		{"operator", PermissionAPIExecute, true},
		{"operator", PermissionTaskCreate, true},
		{"operator", PermissionTaskUpdate, true},
		{"operator", PermissionPluginExecute, true},
		{"operator", PermissionConfigRead, true},
		{"operator", PermissionDelete, false},
		{"operator", PermissionNodeManage, false},
		{"operator", PermissionPluginManage, false},
		{"operator", PermissionAuditRead, false},

		// ReadOnly
		{"readonly", PermissionRead, true},
		{"readonly", PermissionAPIRead, true},
		{"readonly", PermissionTaskRead, true},
		{"readonly", PermissionConfigRead, true},
		{"readonly", PermissionAuditRead, true},
		{"readonly", PermissionWrite, false},
		{"readonly", PermissionDelete, false},

		// Node
		{"node", PermissionAPIExecute, true},
		{"node", PermissionAPIRead, true},
		{"node", PermissionAPIWrite, true},
		{"node", PermissionNodeRegister, true},
		{"node", PermissionPluginExecute, true},
		{"node", PermissionRead, false},
		{"node", PermissionNodeManage, false},
	}

	pm := NewPermissionManager()
	for _, tt := range tests {
		t.Run(tt.roleName+"_"+string(tt.permission), func(t *testing.T) {
			got := pm.HasPermission(tt.roleName, tt.permission)
			if got != tt.shouldHave {
				t.Errorf("HasPermission(%q, %q) = %v, want %v", tt.roleName, tt.permission, got, tt.shouldHave)
			}
		})
	}
}

func TestPermissionManager_HasPermission(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("nonexistent role", func(t *testing.T) {
		if pm.HasPermission("nonexistent", PermissionRead) {
			t.Error("expected false for nonexistent role")
		}
	})
}

func TestPermissionManager_HasAnyPermission(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("at least one match", func(t *testing.T) {
		if !pm.HasAnyPermission("operator", PermissionRead, PermissionDelete) {
			t.Error("expected true: operator has read")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		if pm.HasAnyPermission("readonly", PermissionDelete, PermissionNodeManage) {
			t.Error("expected false: readonly has neither delete nor node:manage")
		}
	})

	t.Run("nonexistent role", func(t *testing.T) {
		if pm.HasAnyPermission("ghost", PermissionRead) {
			t.Error("expected false for nonexistent role")
		}
	})

	t.Run("empty permissions", func(t *testing.T) {
		if pm.HasAnyPermission("admin") {
			t.Error("expected false with no permissions to check")
		}
	})
}

func TestHasAllPermissions(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("all match", func(t *testing.T) {
		if !pm.HasAllPermissions("admin", PermissionRead, PermissionWrite) {
			t.Error("expected admin to have all permissions")
		}
	})

	t.Run("one missing", func(t *testing.T) {
		if pm.HasAllPermissions("readonly", PermissionRead, PermissionWrite) {
			t.Error("expected false: readonly lacks write")
		}
	})

	t.Run("nonexistent role", func(t *testing.T) {
		if pm.HasAllPermissions("ghost", PermissionRead) {
			t.Error("expected false for nonexistent role")
		}
	})
}

func TestPermissionManager_CheckPermission(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("has permission returns nil", func(t *testing.T) {
		err := pm.CheckPermission("admin", PermissionRead)
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("lacks permission returns error", func(t *testing.T) {
		err := pm.CheckPermission("readonly", PermissionDelete)
		if err == nil {
			t.Error("expected error for lacking permission")
		}
	})
}

func TestCheckAnyPermission(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("has any returns nil", func(t *testing.T) {
		err := pm.CheckAnyPermission("readonly", PermissionWrite, PermissionRead)
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("has none returns error", func(t *testing.T) {
		err := pm.CheckAnyPermission("readonly", PermissionDelete, PermissionNodeManage)
		if err == nil {
			t.Error("expected error for lacking all permissions")
		}
	})
}

func TestCheckAllPermissions(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("has all returns nil", func(t *testing.T) {
		err := pm.CheckAllPermissions("admin", PermissionRead, PermissionWrite)
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("missing one returns error", func(t *testing.T) {
		err := pm.CheckAllPermissions("readonly", PermissionRead, PermissionDelete)
		if err == nil {
			t.Error("expected error for missing permission")
		}
	})
}

func TestGetPermissions(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("returns all permissions", func(t *testing.T) {
		perms := pm.GetPermissions("admin")
		if len(perms) != len(RoleAdmin.Permissions) {
			t.Errorf("expected %d permissions, got %d", len(RoleAdmin.Permissions), len(perms))
		}
	})

	t.Run("nonexistent role returns empty", func(t *testing.T) {
		perms := pm.GetPermissions("nonexistent")
		if len(perms) != 0 {
			t.Errorf("expected 0 permissions for nonexistent role, got %d", len(perms))
		}
	})
}

func TestGetAllRoles(t *testing.T) {
	pm := NewPermissionManager()
	roles := pm.GetAllRoles()
	if len(roles) < 4 {
		t.Errorf("expected at least 4 roles, got %d", len(roles))
	}
}

func TestCreateCustomRole(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("creates and registers", func(t *testing.T) {
		role := pm.CreateCustomRole("custom", "A custom role", []Permission{PermissionRead})
		if role.Name != "custom" {
			t.Errorf("expected name 'custom', got %q", role.Name)
		}
		if role.Description != "A custom role" {
			t.Errorf("expected description 'A custom role', got %q", role.Description)
		}
		if len(role.Permissions) != 1 {
			t.Errorf("expected 1 permission, got %d", len(role.Permissions))
		}

		// Verify it was registered
		if !pm.HasPermission("custom", PermissionRead) {
			t.Error("expected custom role to have read permission")
		}
	})
}

func TestUpdateRole(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("updates existing role", func(t *testing.T) {
		// Use a fresh PermissionManager to avoid interference from other tests
		pm := NewPermissionManager()
		success := pm.UpdateRole("readonly", []Permission{PermissionRead, PermissionWrite})
		if !success {
			t.Fatal("expected update to succeed")
		}
		// Verify the permissions were actually updated
		perms := pm.GetPermissions("readonly")
		t.Logf("Updated readonly permissions: %v", perms)
		if !pm.HasPermission("readonly", PermissionRead) {
			t.Error("expected readonly to still have read permission")
		}
		if !pm.HasPermission("readonly", PermissionWrite) {
			t.Error("expected readonly to now have write permission")
		}
	})

	t.Run("fails for nonexistent role", func(t *testing.T) {
		success := pm.UpdateRole("ghost", []Permission{PermissionRead})
		if success {
			t.Error("expected update to fail for nonexistent role")
		}
	})
}

func TestDeleteRole(t *testing.T) {
	pm := NewPermissionManager()

	t.Run("deletes custom role", func(t *testing.T) {
		pm.CreateCustomRole("temporary", "To be deleted", []Permission{PermissionRead})
		success := pm.DeleteRole("temporary")
		if !success {
			t.Error("expected delete to succeed")
		}
		if pm.GetRole("temporary") != nil {
			t.Error("expected role to be deleted")
		}
	})

	t.Run("cannot delete predefined roles", func(t *testing.T) {
		predefined := []string{"admin", "operator", "readonly", "node"}
		for _, name := range predefined {
			if pm.DeleteRole(name) {
				t.Errorf("expected deletion of predefined role %q to fail", name)
			}
		}
	})

	t.Run("fails for nonexistent role", func(t *testing.T) {
		success := pm.DeleteRole("ghost")
		if success {
			t.Error("expected delete to fail for nonexistent role")
		}
	})
}

func TestRoleConstants(t *testing.T) {
	// Verify permission constants are correct values
	if PermissionAdmin != "admin" {
		t.Errorf("expected PermissionAdmin = 'admin', got %q", PermissionAdmin)
	}
	if PermissionRead != "read" {
		t.Errorf("expected PermissionRead = 'read', got %q", PermissionRead)
	}
	if PermissionAPIExecute != "api:execute" {
		t.Errorf("expected PermissionAPIExecute = 'api:execute', got %q", PermissionAPIExecute)
	}
	if PermissionTaskCreate != "task:create" {
		t.Errorf("expected PermissionTaskCreate = 'task:create', got %q", PermissionTaskCreate)
	}
	if PermissionNodeRegister != "node:register" {
		t.Errorf("expected PermissionNodeRegister = 'node:register', got %q", PermissionNodeRegister)
	}
}

func TestPermissionManagerConcurrency(t *testing.T) {
	pm := NewPermissionManager()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			pm.CreateCustomRole("concurrent-role", "test", []Permission{PermissionRead})
			pm.HasPermission("concurrent-role", PermissionRead)
			pm.HasAnyPermission("concurrent-role", PermissionRead)
			pm.HasAllPermissions("concurrent-role", PermissionRead)
			pm.GetAllRoles()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
