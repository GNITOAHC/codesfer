package url

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const tableName = "short_urls"

var db *sql.DB

var ErrCustomTaken = errors.New("custom url already in use")

type link struct {
	Handle    string
	Username  string
	Custom    string // empty = no custom url
	Target    string
	Password  string
	CreatedAt int64
	ExpiresAt int64 // unix seconds; 0 = never expires
}

func connect(driver, source string) error {
	_db, err := sql.Open(driver, source)
	if err != nil {
		return err
	}
	db = _db
	return createTable()
}

func createTable() error {
	query := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            handle VARCHAR(255) NOT NULL PRIMARY KEY, -- /r/<handle>
            username VARCHAR(255) NOT NULL,
            custom VARCHAR(255),                      -- /u/<username>/<custom>; NULL = none
            target TEXT NOT NULL,
            password VARCHAR(255),
            created_at BIGINT,
            expires_at BIGINT,                        -- unix seconds; NULL = never expires
            UNIQUE (username, custom)
	)`, tableName)
	_, err := db.Exec(query)
	return err
}

// linkColumns and liveLink are shared by the lookups, so an expired link reads
// as missing even before the sweep deletes it.
const (
	linkColumns = "handle, username, COALESCE(custom, ''), target, COALESCE(password, ''), created_at, COALESCE(expires_at, 0)"
	liveLink    = "(expires_at IS NULL OR expires_at > $1)"
)

type scanner interface{ Scan(dest ...any) error }

func scanLink(row scanner) (*link, error) {
	l := &link{}
	err := row.Scan(&l.Handle, &l.Username, &l.Custom, &l.Target, &l.Password, &l.CreatedAt, &l.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

// insert stores l. On failure it tells apart a taken custom url
// (ErrCustomTaken) from a handle collision (taken == true, caller retries).
func insert(l *link) (taken bool, err error) {
	query := fmt.Sprintf("INSERT INTO %s (handle, username, custom, target, password, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7)", tableName)
	_, err = db.Exec(query, l.Handle, l.Username,
		sql.NullString{String: l.Custom, Valid: l.Custom != ""},
		l.Target,
		sql.NullString{String: l.Password, Valid: l.Password != ""},
		l.CreatedAt,
		sql.NullInt64{Int64: l.ExpiresAt, Valid: l.ExpiresAt > 0},
	)
	if err == nil {
		return false, nil
	}
	// The driver errors differ between sqlite and pgx, so look at the rows
	// instead of parsing them. Expired rows count: they keep their keys until
	// the sweep removes them.
	var n int
	if l.Custom != "" {
		if db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE username = $1 AND custom = $2", tableName), l.Username, l.Custom).Scan(&n) == nil && n > 0 {
			return false, ErrCustomTaken
		}
	}
	if db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE handle = $1", tableName), l.Handle).Scan(&n) == nil && n > 0 {
		return true, nil
	}
	return false, err
}

func get(handle string) (*link, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s AND handle = $2", linkColumns, tableName, liveLink)
	return scanLink(db.QueryRow(query, time.Now().Unix(), handle))
}

func getByCustom(username, custom string) (*link, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s AND username = $2 AND custom = $3", linkColumns, tableName, liveLink)
	return scanLink(db.QueryRow(query, time.Now().Unix(), username, custom))
}

func list(username string) ([]link, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s AND username = $2 ORDER BY created_at DESC", linkColumns, tableName, liveLink)
	rows, err := db.Query(query, time.Now().Unix(), username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, *l)
	}
	return links, rows.Err()
}

// remove deletes username's link by handle, reporting whether one was deleted.
func remove(username, handle string) (bool, error) {
	res, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE username = $1 AND handle = $2", tableName), username, handle)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// removeExpired deletes every expired link and returns how many it removed.
func removeExpired() (int64, error) {
	res, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE expires_at IS NOT NULL AND expires_at <= $1", tableName), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
