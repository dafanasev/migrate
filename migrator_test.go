package dbmigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewMigrator(t *testing.T) {
	os.Remove("test.db")
	s := &Settings{}
	_, err := NewMigrator(s)
	assert.EqualError(t, err, "database engine not specified")

	s.Engine = "nosql"
	_, err = NewMigrator(s)
	assert.EqualError(t, err, "database name not specified")

	s.Database = "test.db"

	_, err = NewMigrator(s)
	assert.Contains(t, err.Error(), "unknown database engine")

	s.Engine = "sqlite"
	m, err := NewMigrator(s)
	require.NoError(t, err)
	assert.Equal(t, "migrations", m.MigrationsTable)
	projectDir, _ := os.Getwd()
	assert.Equal(t, projectDir, m.projectDir)
	assert.Equal(t, "sqlite3", m.dbWrapper.driver())
	m.Close()
}

func Test_Migrator_Close(t *testing.T) {
	m, err := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	require.NoError(t, err)
	err = m.Close()
	assert.NoError(t, err)
	assert.Nil(t, m.MigrationsCh)
	assert.Nil(t, m.ErrorsCh)
}

func Test_Migrator_getMigration(t *testing.T) {
	os.Remove("test.db")
	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	os.Create(filepath.Join(MigrationsDir, "20180918200632.duplicate.up.sql"))
	defer os.Remove(filepath.Join(MigrationsDir, "20180918200632.duplicate.up.sql"))

	// does not exist at all
	_, err := m.getMigration(time.Date(2018, 9, 10, 11, 12, 13, 0, time.UTC), DirectionUp)
	assert.Contains(t, err.Error(), "does not exist")

	// does not exist for needed direction
	os.Rename(filepath.Join(MigrationsDir, "20180918200453.correct.down.sql"), "20180918200453.correct.down.sql")
	defer os.Rename("20180918200453.correct.down.sql", filepath.Join(MigrationsDir, "20180918200453.correct.down.sql"))
	_, err = m.getMigration(time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC), DirectionDown)
	assert.Contains(t, err.Error(), "does not exist")

	// does not exist for used engine
	_, err = m.getMigration(time.Date(2018, 9, 18, 20, 7, 42, 0, time.UTC), DirectionUp)
	assert.Contains(t, err.Error(), "does not exist")

	// multiple migrations for the version
	_, err = m.getMigration(time.Date(2018, 9, 18, 20, 6, 32, 0, time.UTC), DirectionUp)
	assert.Contains(t, err.Error(), "should be only one")

	// correct for any engine
	v := time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC)
	migration, err := m.getMigration(v, DirectionUp)
	require.NoError(t, err)
	assert.NotNil(t, migration)
	expected := &Migration{Version: v, Name: "correct", Direction: DirectionUp}
	assert.Equal(t, expected, migration)

	// correct for the isSpecific engine
	v = time.Date(2018, 9, 18, 20, 10, 19, 0, time.UTC)
	migration, err = m.getMigration(v, DirectionUp)
	require.NoError(t, err)
	assert.NotNil(t, migration)
	expected = &Migration{Version: v, Name: "specific_engine_correct", Direction: DirectionUp, Engine: "sqlite"}
	assert.Equal(t, expected, migration)
}

func Test_Migrator_findMigrations(t *testing.T) {
	os.Remove("test.db")
	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	os.Create(filepath.Join(MigrationsDir, "20180918200632.duplicate.up.sql"))
	_, err := m.findMigrations(DirectionUp)
	assert.Contains(t, err.Error(), "are duplicated")
	os.Remove(filepath.Join(MigrationsDir, "20180918200632.duplicate.up.sql"))

	migrations, err := m.findMigrations(DirectionUp)
	require.NoError(t, err)
	assert.Len(t, migrations, 3)
}

func Test_Migrator_unappliedMigrations(t *testing.T) {
	os.Remove("test.db")
	defer os.Remove("test.db")

	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()
	migrations, _ := m.findMigrations(DirectionUp)

	for i := 0; i < 4; i++ {
		unappliedMigrations, err := m.unappliedMigrations()
		require.NoError(t, err)
		assert.Len(t, unappliedMigrations, 3-i)

		// we've got migrations we were actually needed
		for j, um := range unappliedMigrations {
			assert.Equal(t, um.Version, migrations[i+j].Version)
		}

		if i < 3 {
			m.dbWrapper.insertMigrationData(migrations[i].Version, time.Now(), nil)
		}
	}
}

func Test_Migrator_findProjectDir(t *testing.T) {
	os.Remove("test.db")
	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	wd, _ := os.Getwd()
	projectDir, err := FindProjectDir(wd)
	require.NoError(t, err)
	assert.Equal(t, wd, projectDir)

	projectDir, err = FindProjectDir(filepath.Join(wd, "cmd"))
	require.NoError(t, err)
	assert.Equal(t, wd, projectDir)

	os.Rename(MigrationsDir, "!"+MigrationsDir)
	_, err = FindProjectDir(wd)
	assert.EqualError(t, err, "dbmigrations dir not found")
	os.Rename("!"+MigrationsDir, MigrationsDir)
}

func TestMigrator_Migrator_LatestVersionAndLastAppliedMigration(t *testing.T) {
	os.Remove("test.db")
	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	lvm, err := m.LatestVersionMigration()
	require.NoError(t, err)
	assert.Nil(t, lvm)
	lam, err := m.LastAppliedMigration()
	require.NoError(t, err)
	assert.Nil(t, lam)

	v1 := time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC)
	v2 := time.Date(2018, 9, 18, 20, 6, 32, 0, time.UTC)

	_ = m.dbWrapper.insertMigrationData(v1, time.Now(), nil)
	lvm, err = m.LatestVersionMigration()
	require.NoError(t, err)
	assert.Equal(t, v1, lvm.Version)
	lam, err = m.LastAppliedMigration()
	require.NoError(t, err)
	assert.Equal(t, v1, lam.Version)

	// earlier applied_at
	_ = m.dbWrapper.insertMigrationData(v2, time.Now().Add(-5*time.Second), nil)
	lvm, err = m.LatestVersionMigration()
	require.NoError(t, err)
	assert.Equal(t, v2, lvm.Version)
	lam, err = m.LastAppliedMigration()
	require.NoError(t, err)
	assert.Equal(t, v1, lam.Version)

	// not existing migration
	_ = m.dbWrapper.insertMigrationData(time.Date(2018, 9, 18, 22, 2, 34, 0, time.UTC), time.Now(), nil)
	_, err = m.LatestVersionMigration()
	assert.Contains(t, err.Error(), "can't get latest migration with version")
	_, err = m.LastAppliedMigration()
	assert.Contains(t, err.Error(), "can't get last applied migration with version")
}

func Test_Migrator_run(t *testing.T) {
	os.Remove("test.db")

	migrationsCh := make(chan *Migration)
	errorsCh := make(chan error)
	done := make(chan struct{})

	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db", MigrationsCh: migrationsCh, ErrorsCh: errorsCh})
	defer m.Close()

	migration, _ := migrationFromFileName("20180918100423.incorrect.up.sql")
	err := m.run(migration)
	assert.Contains(t, err.Error(), "can't read migration")

	migration, _ = migrationFromFileName("20180918200742.wrong_engine.up.postgres.sql")
	err = m.run(migration)
	assert.EqualError(t, err, "empty query")

	go func() {
		migration := <-migrationsCh
		assert.Equal(t, time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC), migration.Version)
		done <- struct{}{}
	}()
	migration, _ = migrationFromFileName("20180918200453.correct.up.sql")
	err = m.run(migration)
	require.NoError(t, err)
	<-done

	migration, _ = migrationFromFileName("20180918200742.wrong_engine.down.postgres.sql")
	err = m.run(migration)
	assert.EqualError(t, err, "empty query")

	go func() {
		err := <-errorsCh
		assert.EqualError(t, err, "empty query")
		done <- struct{}{}
	}()

	m.AllowMissingDowns = true
	m.ErrorsCh = errorsCh
	err = m.run(migration)
	require.NoError(t, err)
	<-done
}

func Test_Migrator_Migrate_Rollback(t *testing.T) {
	os.Remove("test.db")

	errorsCh := make(chan error)
	done := make(chan struct{})

	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db", ErrorsCh: errorsCh})
	defer m.Close()

	n, err := m.Rollback()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	lm, _ := m.LatestVersionMigration()
	assert.Nil(t, lm)

	n, err = m.RollbackSteps(1)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	lm, _ = m.LatestVersionMigration()
	assert.Nil(t, lm)

	n, err = m.MigrateSteps(1)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	lm, _ = m.LatestVersionMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC), lm.Version)

	// not existing down
	os.Rename(filepath.Join(MigrationsDir, "20180918200453.correct.down.sql"), "20180918200453.correct.down.sql")

	n, err = m.Rollback()
	assert.Contains(t, err.Error(), "can't get migration for")
	assert.Equal(t, 0, n)

	go func() {
		err := <-errorsCh
		assert.Contains(t, err.Error(), "can't get migration for")
		done <- struct{}{}
	}()

	m.AllowMissingDowns = true
	n, err = m.Rollback()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	<-done
	m.AllowMissingDowns = false

	os.Rename("20180918200453.correct.down.sql", filepath.Join(MigrationsDir, "20180918200453.correct.down.sql"))
	// END not existing down

	n, err = m.Rollback()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	lm, _ = m.LatestVersionMigration()
	assert.Nil(t, lm)

	n, err = m.MigrateSteps(2)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	lm, _ = m.LatestVersionMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 6, 32, 0, time.UTC), lm.Version)

	n, err = m.RollbackSteps(2)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	lm, _ = m.LatestVersionMigration()
	assert.Nil(t, lm)

	n, err = m.Migrate()
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	lm, _ = m.LatestVersionMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 10, 19, 0, time.UTC), lm.Version)

	n, err = m.RollbackSteps(1)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	lm, _ = m.LatestVersionMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 6, 32, 0, time.UTC), lm.Version)

	n, err = m.Rollback()
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	lm, _ = m.LatestVersionMigration()
	assert.Nil(t, lm)

	// not successive migrates
	os.Rename(filepath.Join(MigrationsDir, "20180918200453.correct.up.sql"), "20180918200453.correct.up.sql")
	os.Rename(filepath.Join(MigrationsDir, "/20180918200632.other_correct.up.sql"), "20180918200632.other_correct.up.sql")

	n, err = m.Migrate()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	lm, _ = m.LastAppliedMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 10, 19, 0, time.UTC), lm.Version)
	// pretend to travel in time
	ts := lm.Version.Add(-1 * time.Second)
	_, err = m.dbWrapper.db.Exec("UPDATE migrations SET applied_at = ?", ts.Format(TimestampFormat))
	require.NoError(t, err)

	os.Rename("20180918200453.correct.up.sql", filepath.Join(MigrationsDir, "20180918200453.correct.up.sql"))
	os.Rename("20180918200632.other_correct.up.sql", filepath.Join(MigrationsDir, "20180918200632.other_correct.up.sql"))

	n, err = m.Migrate()
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	lm, _ = m.LastAppliedMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 6, 32, 0, time.UTC), lm.Version)

	n, err = m.Rollback()
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	lm, _ = m.LastAppliedMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 10, 19, 0, time.UTC), lm.Version)

	m.Migrate()
	n, err = m.RollbackSteps(1)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	lm, _ = m.LastAppliedMigration()
	assert.Equal(t, time.Date(2018, 9, 18, 20, 4, 53, 0, time.UTC), lm.Version)

	m.Rollback()
	m.Migrate()
	// from two batches
	n, err = m.RollbackSteps(4)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	lm, _ = m.LastAppliedMigration()
	assert.Nil(t, lm)

	// END not successive migrates

}

func Test_Migrator_GenerateMigration(t *testing.T) {
	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	_, err := m.GenerateMigration("wrong engine", "nodb")
	assert.EqualError(t, err, "database engine nodb is not exists/supported")

	testData := []struct {
		descr  string
		engine string
	}{
		{" test  Migration \n ", ""},
		{" test\tSpecific migration \n ", "sqlite"},
	}
	for _, data := range testData {
		var fpaths []string
		var err error
		if data.engine == "" {
			fpaths, err = m.GenerateMigration(data.descr)
		} else {
			fpaths, err = m.GenerateMigration(data.descr, data.engine)
		}
		assert.NoError(t, err)
		for _, fpath := range fpaths {
			if data.engine == "" {
				assert.Contains(t, fpath, "test_migration")
				assert.Regexp(t, `^`+MigrationsDir+`/\d+\.[_\w]+\.(down|up)\.sql$`, fpath)
			} else {
				assert.Contains(t, fpath, "test_specific_migration")
				assert.Contains(t, fpath, "sqlite.sql")
				assert.Regexp(t, `^`+MigrationsDir+`/\d+\.[_\w]+\.(down|up)\.sqlite\.sql$`, fpath)
			}
			assert.True(t, FileExists(fpath))
		}

		if data.engine == "" {
			_, err = m.GenerateMigration(data.descr)
		} else {
			_, err = m.GenerateMigration(data.descr, data.engine)
		}
		assert.Contains(t, err.Error(), "already exists")

		for _, fpath := range fpaths {
			os.Remove(fpath)
		}
	}
}

func Test_Migrator_Status(t *testing.T) {
	os.Remove("test.db")

	m, _ := NewMigrator(&Settings{Engine: "sqlite", Database: "test.db"})
	defer m.Close()

	m.MigrateSteps(1)

	migrations, err := m.Status()
	require.NoError(t, err)
	for i, migration := range migrations {
		if i == 0 {
			assert.NotEqual(t, time.Time{}, migration.AppliedAt)
		} else {
			assert.Equal(t, time.Time{}, migration.AppliedAt)
		}
	}
}
