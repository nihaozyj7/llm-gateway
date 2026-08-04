package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const adminCols = "id, username, password_hash, created_at"

// GetAdminByUsername 按用户名取管理员
func (s *Store) GetAdminByUsername(username string) (*model.Admin, error) {
	row := s.db.QueryRow("SELECT "+adminCols+" FROM admins WHERE username = ?", username)
	var a model.Admin
	var createdAt int64
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = fromTS(createdAt)
	return &a, nil
}

// CountAdmins 管理员数量(判断是否首次启动)
func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM admins").Scan(&n)
	return n, err
}

// CreateAdmin 创建管理员
func (s *Store) CreateAdmin(username, passwordHash string) error {
	_, err := s.db.Exec("INSERT INTO admins (username, password_hash, created_at) VALUES (?, ?, ?)",
		username, passwordHash, ts(time.Now()))
	return err
}

// GetAdminByID 按主键取管理员
func (s *Store) GetAdminByID(id int64) (*model.Admin, error) {
	row := s.db.QueryRow("SELECT "+adminCols+" FROM admins WHERE id = ?", id)
	var a model.Admin
	var createdAt int64
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = fromTS(createdAt)
	return &a, nil
}

// UpdateAdminPassword 更新密码
func (s *Store) UpdateAdminPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec("UPDATE admins SET password_hash=? WHERE id=?", passwordHash, id)
	return err
}
