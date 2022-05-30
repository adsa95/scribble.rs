package main

import (
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	config2 "github.com/scribble-rs/scribble.rs/config"
	"github.com/scribble-rs/scribble.rs/database"
	"log"
	"os"
)

const migrationsDirectory = "database/migrations"

func main() {
	conf := config2.FromEnv()
	db, err := database.FromDatabaseUrl(conf.DatabaseUrl)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) < 2 {
		log.Fatal(errors.New("migration mode 'up' or 'down' must be specified"))
	}

	mode := os.Args[1]

	switch mode {
	case "up":
		err := migrateUp(db)
		if err != nil {
			if _, ok := err.(ErrNoChange); ok {
				log.Printf("Database: no change.")
			} else {
				log.Fatalf("Database: migration failed: %v", err)
			}
		} else {
			log.Printf("Database: successfully migrated up.")
		}
	case "down":
		err := migrateDown(db)
		if err != nil {
			log.Fatalf("Database: migration failed: %v", err)
		}
		log.Println("Database: successfully migrated down.")
	default:
		log.Fatalf("Database: invalid migration mode %q", mode)
	}
}

type ErrNoChange struct {
	err error
}

func (e ErrNoChange) Error() string {
	return e.err.Error()
}

func migrateUp(db *database.DB) error {
	m, err := newMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			err = ErrNoChange{err}
		}
		return err
	}

	return nil
}

func migrateDown(db *database.DB) error {
	m, err := newMigrateInstance(db)
	if err != nil {
		return err
	}

	if err := m.Down(); err != nil {
		return err
	}

	return nil
}

func newMigrateInstance(db *database.DB) (*migrate.Migrate, error) {
	driver, err := postgres.WithInstance(db.Executor.DB, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("could not create migration driver: %v", err)
	}

	src := "file://" + migrationsDirectory
	m, err := migrate.NewWithDatabaseInstance(src, database.Type, driver)
	if err != nil {
		return nil, fmt.Errorf("failed loading migrations from %v: %v", src, err)
	}

	return m, nil
}
