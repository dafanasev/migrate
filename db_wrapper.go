package migrate

import (
	"database/sql"
	"time"
	
	"github.com/pkg/errors"
	
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

var (
	errDBNameNotProvided = errors.New("dbWrapper name is not provided")
	errUserNotProvided   = errors.New("user is not provided")
)

type dbWrapper struct {
	settings *Settings
	db       *sql.DB
	provider
	placeholdersProvider
}

func newDBWrapper(settings *Settings, provider provider) *dbWrapper {
	w := &dbWrapper{
		settings: settings,
		provider: provider,
	}
	if pp, ok := w.provider.(placeholdersProvider); ok {
		w.placeholdersProvider = pp
	}
	return w
}

func (w *dbWrapper) open() error {
	dsn, err := w.provider.dsn(w.settings)
	if err != nil {
		return err
	}
	
	w.db, err = sql.Open(w.settings.DriverName, dsn)
	if err != nil {
		return errors.Wrap(err, "can't open database")
	}
	
	return nil
}

func (w *dbWrapper) close() error {
	err := w.db.Close()
	if err != nil {
		return errors.Wrap(err, "Error shutting down migrator")
	}
	return nil
}

func (w *dbWrapper) setPlaceholders(s string) string {
	if w.placeholdersProvider == nil {
		return s
	}
	return w.placeholdersProvider.setPlaceholders(s)
}

func (w *dbWrapper) hasTable() (bool, error) {
	var table string
	err := w.db.QueryRow(w.setPlaceholders(w.provider.hasTableQuery()), w.settings.MigrationsTable).Scan(&table)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	
	return true, nil
}

func (w *dbWrapper) createMigrationsTable() error {
	_, err := w.db.Exec(w.setPlaceholders("CREATE TABLE ? (version TIMESTAMP NOT NULL, PRIMARY KEY(version));"), w.settings.MigrationsTable)
	if err != nil {
		return errors.Wrapf(err, "can't create migrations table")
	}
	return nil
}

func (w *dbWrapper) lastMigrationTimestamp() (time.Time, error) {
	var v time.Time
	err := w.db.QueryRow(w.setPlaceholders("SELECT version FROM ? ORDER BY version DESC LIMIT 1"), w.settings.MigrationsTable).Scan(&v)
	if err != nil {
	    return time.Time{}, errors.Wrap(err,"can't select last migration version from database")
	}
	return v, nil
}

func (w *dbWrapper) appliedMigrationsTimestamps() ([]time.Time, error) {
	rows, err := w.db.Query(w.setPlaceholders("SELECT version FROM ? ORDER BY version ASC"), w.settings.MigrationsTable)
	if err != nil {
		return nil, errors.Wrap(err, "can't get applied migrations versions")
	}
	defer rows.Close()
	
	vs := []time.Time{}
	var v time.Time
	for rows.Next() {
		err = rows.Scan(&v)
		if err != nil {
			return nil, errors.Wrap(err, "can't scan migration version row")
		}
		vs = append(vs, v)
	}
	return vs, nil
}

func (w *dbWrapper) insertMigrationTimestamp(version time.Time) error {
	_, err := w.db.Exec(w.setPlaceholders("INSERT INTO ? (migration) VALUES (?)"), w.settings.MigrationsTable, version)
	if err != nil {
		return errors.Wrap(err, "can't insert migration version")
	}
	return nil
}

func (w *dbWrapper) deleteMigrationTimestamp(version time.Time) error {
	_, err := w.db.Exec(w.setPlaceholders("DELETE FROM ? WHERE migration = ?"), w.settings.MigrationsTable, version)
	if err != nil {
		return errors.Wrap(err, "can't delete migration version")
	}
	return nil
}