package database

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a system user
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // Never expose in JSON
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateUserTable creates the users table
func CreateUserTable() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'admin' CHECK(role IN ('admin', 'user')),
		status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'disabled')),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := DB.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	return nil
}

// HasUser checks if any user exists
func HasUser() (bool, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check users: %w", err)
	}
	return count > 0, nil
}

// GetUser retrieves a user by username
func GetUser(username string) (*User, error) {
	user := &User{}
	err := DB.QueryRow(
		"SELECT id, username, password, role, status, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// CreateUser creates a new user with hashed password
func CreateUser(username, password string) error {
	return CreateUserWithRoleStatus(username, password, "admin", "active")
}

// CreateUserWithRoleStatus creates a new user with role and status
func CreateUserWithRoleStatus(username, password, role, status string) error {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = DB.Exec(
		"INSERT INTO users (username, password, role, status) VALUES (?, ?, ?, ?)",
		username, string(hashedPassword), role, status,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func GetUserByID(id int64) (*User, error) {
	user := &User{}
	err := DB.QueryRow(
		"SELECT id, username, password, role, status, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// ListUsers retrieves all users without exposing passwords
func ListUsers() ([]*User, error) {
	rows, err := DB.Query("SELECT id, username, role, status, created_at, updated_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, nil
}

// UpdateUser updates username/role/status
func UpdateUser(user *User) error {
	_, err := DB.Exec(
		"UPDATE users SET username = ?, role = ?, status = ?, updated_at = datetime('now') WHERE id = ?",
		user.Username, user.Role, user.Status, user.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// UpdateUserPassword updates a user's password
func UpdateUserPassword(id int64, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	_, err = DB.Exec(
		"UPDATE users SET password = ?, updated_at = datetime('now') WHERE id = ?",
		string(hashedPassword), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

// UpdateUserStatus updates a user's status
func UpdateUserStatus(id int64, status string) error {
	_, err := DB.Exec(
		"UPDATE users SET status = ?, updated_at = datetime('now') WHERE id = ?",
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}
	return nil
}

// DeleteUser deletes a user by ID
func DeleteUser(id int64) error {
	_, err := DB.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// CountAdmins returns the number of admin users.
func CountAdmins() (int, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admins: %w", err)
	}
	return count, nil
}

// ValidatePassword checks if the password is correct
func ValidatePassword(username, password string) (bool, error) {
	user, err := GetUser(username)
	if err != nil {
		return false, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	return err == nil, nil
}

// DeleteAllUsers deletes all users (for password reset)
func DeleteAllUsers() error {
	_, err := DB.Exec("DELETE FROM users")
	if err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}
	return nil
}
