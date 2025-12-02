package storage

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

var db *sql.DB

type Object struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Filename  string `json:"filename"`
	Password  string `json:"password"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
}

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
        CREATE TABLE IF NOT EXISTS objects (
            id VARCHAR(255) NOT NULL PRIMARY KEY,
			username VARCHAR(255) NOT NULL,
			filename VARCHAR(255),           -- Object's filename, directory is separated by slashes
			password VARCHAR(255),
            path VARCHAR(255) UNIQUE,        -- Path in R2 object storage
            created_at VARCHAR(255),
            UNIQUE (username, filename)
	)`

	_, err := db.Exec(query)
	return err
}

func showAll() ([]Object, error) {
	query := "SELECT id, username, filename, password, path, created_at FROM objects"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objs []Object
	for rows.Next() {
		obj := Object{}
		err := rows.Scan(&obj.ID, &obj.Username, &obj.Filename, &obj.Password, &obj.Path, &obj.CreatedAt)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	log.Print(objs)
	return objs, nil
}

func show(username string) ([]Object, error) {
	query := "SELECT id, username, filename, password, path, created_at FROM objects WHERE username = ?"
	rows, err := db.Query(query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objs []Object
	for rows.Next() {
		obj := Object{}
		err := rows.Scan(&obj.ID, &obj.Username, &obj.Filename, &obj.Password, &obj.Path, &obj.CreatedAt)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func insert(id, user, filename, password, path string) error {
	query := "INSERT INTO objects (id, username, filename, password, path, created_at) VALUES (?, ?, ?, ?, ?, ?)"
	_, err := db.Exec(query, id, user, filename, password, path, time.Now().Format(time.RFC3339))
	return err
}

func get(id string) (*Object, error) {
	query := "SELECT id, username, filename, password, path FROM objects WHERE id = ?"
	row := db.QueryRow(query, id)
	obj := &Object{}
	err := row.Scan(&obj.ID, &obj.Username, &obj.Filename, &obj.Password, &obj.Path)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return obj, nil
}

// getByUsernamePath returns the object with given username and path.
// The path here refers to the `filename` field that is stored in the db
func getByUsernamePath(username, path string) (*Object, error) {
	query := "SELECT id, username, filename, password, path FROM objects WHERE username = ? AND filename = ?"
	row := db.QueryRow(query, username, path)
	obj := &Object{}
	err := row.Scan(&obj.ID, &obj.Username, &obj.Filename, &obj.Password, &obj.Path)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return obj, nil
}
