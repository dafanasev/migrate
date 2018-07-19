package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	
	"github.com/pkg/errors"
)

const allSteps = 0

type Migrator struct {
	// dir holding migrations
	migrationsDir string
	// migrations table
	migrationsTable string
	// project dir (the one that has migrationsDir as straight subdir)
	projectDir string
	dbWrapper  *dbWrapper
}

// NewMigrator returns migrator instance
func NewMigrator(settings *Settings) (*Migrator, error) {
	if settings.MigrationsDir == "" {
		settings.MigrationsDir = "migrations"
	}
	if settings.MigrationsTable == "" {
		settings.MigrationsTable = "migrations"
	}
	
	m := &Migrator{migrationsDir: settings.MigrationsDir, migrationsTable: settings.MigrationsTable}
	
	provider, ok := providers[settings.DriverName]
	if !ok {
		return nil, errors.Errorf("unknown database provider name %s", settings.DriverName)
	}
	m.dbWrapper = newDBWrapper(settings, provider)
	
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err,"can't get working directory")
	}
	
	m.projectDir, err = m.findProjectDir(wd)
	if err != nil {
		return nil, err
	}
	
	return m, nil
}

// Done frees resources acquired by migrator
func (m *Migrator) Done() error {
	err := m.dbWrapper.close()
	if err != nil {
	    return errors.Wrap(err,"can't close migrator ")
	}
	return nil
}

func (m *Migrator) Run(direction Direction) (int, error) {
	return m.RunSteps(direction, allSteps)
}

func (m *Migrator) RunSteps(direction Direction, steps uint) (int, error) {
	migrations, err := m.findUnappliedMigrations(direction, steps)
	if err != nil {
		return 0, errors.Wrap(err, "can't find unapplied migrations")
	}
	
	for _, migration := range migrations {
		migration.run()
	}
	return len(migrations), nil
}

func (m *Migrator) LastMigration() (string, error) {
	t, err := m.dbWrapper.lastMigrationTimestamp()
	if err != nil {
	    return "", errors.Wrap(err, "can't get last migration")
	}
	return t.Format(printTimestampFormat), nil
}

// findProjectDir recursively find project dir (the one that has migrations subdir)
func (m *Migrator) findProjectDir(dirPath string) (string, error) {
	if isDirExists(filepath.Join(dirPath, m.migrationsDir)) {
		return dirPath, nil
	}
	
	if dirPath == "/" {
		return "", errors.New("Project dir not found")
	}
	
	return m.findProjectDir(filepath.Dir(dirPath))
}

// findMigrationFiles finds all valid migrations in the migrations dir
func (m *Migrator) findMigrationFiles(direction Direction) []*migration {
	migrations := []*migration{}
	migrationsDirPath := filepath.Join(m.projectDir, m.migrationsDir)
	
	filepath.Walk(migrationsDirPath, func(mpath string, info os.FileInfo, err error) error {
		if mpath != migrationsDirPath && info.IsDir() {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(mpath)) != "sql" {
			return nil
		}
		
		parts := strings.Split(info.Name(), ".")
		
		ts, err := time.Parse(timestampFromFileFormat, parts[0])
		if err != nil {
			return nil
		}
		
		name := parts[1]
		
		if parts[2] != direction.String() {
			return nil
		}
		
		// migration that should be run on specific dbWrapper only
		if len(parts) > 3 && parts[3] != m.dbWrapper.settings.DriverName {
			return nil
		}
		
		migrations = append(migrations, &migration{name: name, timestamp: ts})
		return nil
	})
	
	sort.Sort(byTimestamp(migrations))
	
	return migrations
}

func (m *Migrator) findUnappliedMigrations(direction Direction, steps uint) ([]*migration, error) {
	migrations := m.findMigrationFiles(direction)
	appliedMigrationsTimestamps, err := m.dbWrapper.appliedMigrationsTimestamps()
	if err != nil {
	    return nil, err
	}
	
	unappliedMigrations := []*migration{}
	for _, m := range migrations {
		found := false
		for _, ts := range appliedMigrationsTimestamps {
			if m.timestamp == ts {
				found = true
				break
			}
		}
		if !found {
			unappliedMigrations = append(unappliedMigrations, m)
		}
	}
	
	return unappliedMigrations, nil
}
