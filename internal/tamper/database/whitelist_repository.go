package database

import (
	"database/sql"
	"fmt"
	"time"
)

// WhitelistRepository 白名单仓库接口
type WhitelistRepository interface {
	CreateWhitelist(item *Whitelist) error
	GetWhitelistByID(id int) (*Whitelist, error)
	GetWhitelistByType(whitelistType string) ([]*Whitelist, error)
	GetAllWhitelist() ([]*Whitelist, error)
	DeleteWhitelist(id int) error
	DeleteWhitelistByType(whitelistType string) error
	DeleteWhitelistByValue(value string) error
	ExistsByTypeAndValue(whitelistType, value string) (bool, error)
}

// whitelistRepository 白名单仓库实现
type whitelistRepository struct {
	db *sql.DB
}

// NewWhitelistRepository 创建白名单仓库
func NewWhitelistRepository(db *sql.DB) WhitelistRepository {
	return &whitelistRepository{db: db}
}

// CreateWhitelist 创建白名单
func (r *whitelistRepository) CreateWhitelist(item *Whitelist) error {
	query := `
		INSERT INTO whitelist (type, value, description, created_at)
		VALUES (?, ?, ?, ?)
	`
	
	item.CreatedAt = time.Now()
	
	result, err := r.db.Exec(query, item.Type, item.Value, item.Description, item.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create whitelist: %w", err)
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	item.ID = int(id)
	
	return nil
}

// GetWhitelistByID 根据ID获取白名单
func (r *whitelistRepository) GetWhitelistByID(id int) (*Whitelist, error) {
	query := `
		SELECT id, type, value, description, created_at
		FROM whitelist
		WHERE id = ?
	`
	
	var item Whitelist
	err := r.db.QueryRow(query, id).Scan(
		&item.ID,
		&item.Type,
		&item.Value,
		&item.Description,
		&item.CreatedAt,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("whitelist not found with id: %d", id)
		}
		return nil, fmt.Errorf("failed to get whitelist: %w", err)
	}
	
	return &item, nil
}

// GetWhitelistByType 根据类型获取白名单
func (r *whitelistRepository) GetWhitelistByType(whitelistType string) ([]*Whitelist, error) {
	query := `
		SELECT id, type, value, description, created_at
		FROM whitelist
		WHERE type = ?
		ORDER BY created_at DESC
	`
	
	rows, err := r.db.Query(query, whitelistType)
	if err != nil {
		return nil, fmt.Errorf("failed to query whitelist by type: %w", err)
	}
	defer rows.Close()
	
	var items []*Whitelist
	for rows.Next() {
		var item Whitelist
		err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Value,
			&item.Description,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan whitelist: %w", err)
		}
		items = append(items, &item)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return items, nil
}

// GetAllWhitelist 获取所有白名单
func (r *whitelistRepository) GetAllWhitelist() ([]*Whitelist, error) {
	query := `
		SELECT id, type, value, description, created_at
		FROM whitelist
		ORDER BY type, created_at DESC
	`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all whitelist: %w", err)
	}
	defer rows.Close()
	
	var items []*Whitelist
	for rows.Next() {
		var item Whitelist
		err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Value,
			&item.Description,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan whitelist: %w", err)
		}
		items = append(items, &item)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return items, nil
}

// DeleteWhitelist 删除白名单
func (r *whitelistRepository) DeleteWhitelist(id int) error {
	result, err := r.db.Exec("DELETE FROM whitelist WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete whitelist: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("whitelist not found with id: %d", id)
	}
	
	return nil
}

// DeleteWhitelistByType 根据类型删除白名单
func (r *whitelistRepository) DeleteWhitelistByType(whitelistType string) error {
	result, err := r.db.Exec("DELETE FROM whitelist WHERE type = ?", whitelistType)
	if err != nil {
		return fmt.Errorf("failed to delete whitelist by type: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("no whitelist found with type: %s", whitelistType)
	}
	
	return nil
}

// DeleteWhitelistByValue 根据值删除白名单
func (r *whitelistRepository) DeleteWhitelistByValue(value string) error {
	result, err := r.db.Exec("DELETE FROM whitelist WHERE value = ?", value)
	if err != nil {
		return fmt.Errorf("failed to delete whitelist by value: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("no whitelist found with value: %s", value)
	}
	
	return nil
}

// ExistsByTypeAndValue 检查白名单是否存在
func (r *whitelistRepository) ExistsByTypeAndValue(whitelistType, value string) (bool, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM whitelist WHERE type = ? AND value = ?", whitelistType, value).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check whitelist existence: %w", err)
	}
	
	return count > 0, nil
}

// BatchCreateWhitelist 批量创建白名单
func (r *whitelistRepository) BatchCreateWhitelist(items []*Whitelist) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	query := `
		INSERT INTO whitelist (type, value, description, created_at)
		VALUES (?, ?, ?, ?)
	`
	
	now := time.Now()
	for _, item := range items {
		item.CreatedAt = now
		_, err := tx.Exec(query, item.Type, item.Value, item.Description, item.CreatedAt)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create whitelist: %w", err)
		}
	}
	
	return tx.Commit()
}

// GetWhitelistValuesByType 获取指定类型的所有白名单值
func (r *whitelistRepository) GetWhitelistValuesByType(whitelistType string) ([]string, error) {
	query := `
		SELECT value
		FROM whitelist
		WHERE type = ?
	`
	
	rows, err := r.db.Query(query, whitelistType)
	if err != nil {
		return nil, fmt.Errorf("failed to query whitelist values: %w", err)
	}
	defer rows.Close()
	
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("failed to scan whitelist value: %w", err)
		}
		values = append(values, value)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	return values, nil
}