package database

import (
	"database/sql"
	"fmt"
	"time"
)

// RuleRepository 规则仓库接口
type RuleRepository interface {
	CreateRule(rule *Rule) error
	GetRuleByID(id int) (*Rule, error)
	GetRulesByType(ruleType string) ([]*Rule, error)
	GetAllRules() ([]*Rule, error)
	UpdateRule(rule *Rule) error
	DeleteRule(id int) error
	GetEnabledRules() ([]*Rule, error)
	GetRulesByPriority() ([]*Rule, error)
}

// ruleRepository 规则仓库实现
type ruleRepository struct {
	db *sql.DB
}

// NewRuleRepository 创建规则仓库
func NewRuleRepository(db *sql.DB) RuleRepository {
	return &ruleRepository{db: db}
}

// CreateRule 创建规则
func (r *ruleRepository) CreateRule(rule *Rule) error {
	query := `
		INSERT INTO rules (name, type, pattern, description, severity, enabled, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	
	result, err := r.db.Exec(query, rule.Name, rule.Type, rule.Pattern, rule.Description, rule.Severity, rule.Enabled, rule.Priority, rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create rule: %w", err)
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	rule.ID = int(id)
	
	// 创建初始版本
	return r.createRuleVersion(rule)
}

// GetRuleByID 根据ID获取规则
func (r *ruleRepository) GetRuleByID(id int) (*Rule, error) {
	query := `
		SELECT id, name, type, pattern, description, severity, enabled, priority, created_at, updated_at
		FROM rules
		WHERE id = ?
	`
	
	var rule Rule
	err := r.db.QueryRow(query, id).Scan(
		&rule.ID,
		&rule.Name,
		&rule.Type,
		&rule.Pattern,
		&rule.Description,
		&rule.Severity,
		&rule.Enabled,
		&rule.Priority,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("rule not found with id: %d", id)
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}
	
	return &rule, nil
}

// GetRulesByType 根据类型获取规则
func (r *ruleRepository) GetRulesByType(ruleType string) ([]*Rule, error) {
	query := `
		SELECT id, name, type, pattern, description, severity, enabled, priority, created_at, updated_at
		FROM rules
		WHERE type = ?
		ORDER BY priority DESC
	`
	
	rows, err := r.db.Query(query, ruleType)
	if err != nil {
		return nil, fmt.Errorf("failed to query rules by type: %w", err)
	}
	defer rows.Close()
	
	var rules []*Rule
	for rows.Next() {
		var rule Rule
		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Type,
			&rule.Pattern,
			&rule.Description,
			&rule.Severity,
			&rule.Enabled,
			&rule.Priority,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return rules, nil
}

// GetAllRules 获取所有规则
func (r *ruleRepository) GetAllRules() ([]*Rule, error) {
	query := `
		SELECT id, name, type, pattern, description, severity, enabled, priority, created_at, updated_at
		FROM rules
		ORDER BY priority DESC
	`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all rules: %w", err)
	}
	defer rows.Close()
	
	var rules []*Rule
	for rows.Next() {
		var rule Rule
		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Type,
			&rule.Pattern,
			&rule.Description,
			&rule.Severity,
			&rule.Enabled,
			&rule.Priority,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return rules, nil
}

// UpdateRule 更新规则
func (r *ruleRepository) UpdateRule(rule *Rule) error {
	query := `
		UPDATE rules
		SET name = ?, type = ?, pattern = ?, description = ?, severity = ?, enabled = ?, priority = ?, updated_at = ?
		WHERE id = ?
	`
	
	rule.UpdatedAt = time.Now()
	
	result, err := r.db.Exec(query, rule.Name, rule.Type, rule.Pattern, rule.Description, rule.Severity, rule.Enabled, rule.Priority, rule.UpdatedAt, rule.ID)
	if err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("rule not found with id: %d", rule.ID)
	}
	
	// 创建新版本
	return r.createRuleVersion(rule)
}

// DeleteRule 删除规则
func (r *ruleRepository) DeleteRule(id int) error {
	// 开始事务
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	// 删除相关的版本记录
	_, err = tx.Exec("DELETE FROM rule_versions WHERE rule_id = ?", id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete rule versions: %w", err)
	}
	
	// 删除规则
	result, err := tx.Exec("DELETE FROM rules WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		tx.Rollback()
		return fmt.Errorf("rule not found with id: %d", id)
	}
	
	return tx.Commit()
}

// GetEnabledRules 获取启用的规则
func (r *ruleRepository) GetEnabledRules() ([]*Rule, error) {
	query := `
		SELECT id, name, type, pattern, description, severity, enabled, priority, created_at, updated_at
		FROM rules
		WHERE enabled = TRUE
		ORDER BY priority DESC
	`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled rules: %w", err)
	}
	defer rows.Close()
	
	var rules []*Rule
	for rows.Next() {
		var rule Rule
		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Type,
			&rule.Pattern,
			&rule.Description,
			&rule.Severity,
			&rule.Enabled,
			&rule.Priority,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return rules, nil
}

// GetRulesByPriority 按优先级获取规则
func (r *ruleRepository) GetRulesByPriority() ([]*Rule, error) {
	query := `
		SELECT id, name, type, pattern, description, severity, enabled, priority, created_at, updated_at
		FROM rules
		ORDER BY priority DESC
	`
	
	return r.getAllRules(query)
}

// getAllRules 通用获取规则方法
func (r *ruleRepository) getAllRules(query string) ([]*Rule, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query rules: %w", err)
	}
	defer rows.Close()
	
	var rules []*Rule
	for rows.Next() {
		var rule Rule
		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Type,
			&rule.Pattern,
			&rule.Description,
			&rule.Severity,
			&rule.Enabled,
			&rule.Priority,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}
		rules = append(rules, &rule)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return rules, nil
}

// createRuleVersion 创建规则版本
func (r *ruleRepository) createRuleVersion(rule *Rule) error {
	// 获取当前版本号
	var maxVersion int
	err := r.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM rule_versions WHERE rule_id = ?", rule.ID).Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("failed to get max version: %w", err)
	}
	
	newVersion := maxVersion + 1
	
	query := `
		INSERT INTO rule_versions (rule_id, version, name, type, pattern, description, severity, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	_, err = r.db.Exec(query, rule.ID, newVersion, rule.Name, rule.Type, rule.Pattern, rule.Description, rule.Severity, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create rule version: %w", err)
	}
	
	return nil
}