package auth

import (
	"database/sql"
	"errors"
	"log"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

type User struct {
	Email     string
	Password  string
	Username  string
	CreatedAt string
}

type Session struct {
	ID        string `json:"-"`
	Email     string `json:"-"`
	Location  string `json:"location"`
	Agent     string `json:"agent"`
	LastSeen  string `json:"last_seen"`
	CreatedAt string `json:"created_at"`
}

type AuthError string

// implement the error interface
func (e AuthError) Error() string {
	return string(e)
}

// define auth errors
const (
	ErrUserAlreadyExists AuthError = "user already exists"
	ErrUserNotFound      AuthError = "user not found"
)

func connect(driver, source string) error {
	_db, err := sql.Open(driver, source)
	if err != nil {
		panic(err)
	}

	db = _db
	return createTable()
}

func createTable() error {
	query := `
        CREATE TABLE IF NOT EXISTS users (
            email VARCHAR(255) PRIMARY KEY,
			password VARCHAR(255),
            username VARCHAR(255) UNIQUE,
            created_at VARCHAR(255)
        );
		CREATE TABLE IF NOT EXISTS sessions (
            id VARCHAR(255) PRIMARY KEY,
            email VARCHAR(255),
			location VARCHAR(255),
			agent VARCHAR(255),
			last_seen VARCHAR(255),
            created_at VARCHAR(255),

			FOREIGN KEY (email) REFERENCES users(email) ON DELETE CASCADE
	)`

	_, err := db.Exec(query)
	return err
}

// hash a plain text password
func hashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// compare a plain text password with a hashed password
func checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		log.Println(err)
		return false
	}
	return err == nil
}

func getUsers() ([]User, error) {
	rows, err := db.Query("SELECT email, password, username, created_at FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		user := User{}
		err := rows.Scan(&user.Email, &user.Password, &user.Username, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func getAllSessions() ([]Session, error) {
	rows, err := db.Query("SELECT id, email, created_at FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		session := Session{}
		err := rows.Scan(&session.ID, &session.Email, &session.CreatedAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func reset(table string) error {
	_, err := db.Exec("DELETE FROM " + table)
	return err
}

func createUser(email, password, username string) error {
	user, err := getUser(email)
	if err != nil && err != ErrUserNotFound {
		return err
	}
	if user != nil {
		return ErrUserAlreadyExists
	}
	hashed, err := hashPassword(password)
	if err != nil {
		return err
	}
	db.Exec(
		"INSERT INTO users (email, password, username, created_at) VALUES (?, ?, ?, ?)",
		email, hashed, username, time.Now().Format(time.RFC3339),
	)
	return nil
}

func getUser(email string) (*User, error) {
	row := db.QueryRow("SELECT email, password, username FROM users WHERE email = ?", email)
	user := &User{}
	err := row.Scan(&user.Email, &user.Password, &user.Username)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func getUserFromSessionID(sessionID string) (*User, error) {
	session, err := getSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}
	return getUser(session.Email)
}

func createSession(email, agent, ip string) (string, error) {
	uniqueID := generateUniqueID()
	location, err := ip2Location(ip)
	if err != nil {
		location = "unknown"
	}

	query := "INSERT INTO sessions (id, email, location, agent, last_seen, created_at) VALUES (?, ?, ?, ?, ?, ?)"
	_, err = db.Exec(query, uniqueID, email, location, agent, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return uniqueID, nil
}

func getSession(sessionID string) (*Session, error) {
	row := db.QueryRow("SELECT id, email, location, agent, last_seen, created_at FROM sessions WHERE id = ?", sessionID)
	session := &Session{}
	err := row.Scan(&session.ID, &session.Email, &session.Location, &session.Agent, &session.LastSeen, &session.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return session, nil
}

func getSessions(email string) ([]Session, error) {
	rows, err := db.Query("SELECT id, location, agent, last_seen, created_at FROM sessions WHERE email = ?", email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		session := Session{}
		err := rows.Scan(&session.ID, &session.Location, &session.Agent, &session.LastSeen, &session.CreatedAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func deleteSession(sessionID string) error {
	query := "DELETE FROM sessions WHERE id = ?"
	_, err := db.Exec(query, sessionID)
	if err != nil {
		return err
	}
	return nil
}

func updateSessionLastSeen(sessionID string) error {
	query := "UPDATE sessions SET last_seen = ? WHERE id = ?"
	_, err := db.Exec(query, time.Now().Format(time.RFC3339), sessionID)
	if err != nil {
		return err
	}
	return nil
}

func usernameExists(username string) bool {
	row := db.QueryRow("SELECT username FROM users WHERE username = ?", username)
	user := &User{}
	err := row.Scan(&user.Username)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false
		}
		return false
	}
	return true
}
