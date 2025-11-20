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
		"SELECT id, username, password, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt, &user.UpdatedAt)

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
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = DB.Exec(
		"INSERT INTO users (username, password) VALUES (?, ?)",
		username, string(hashedPassword),
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
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
