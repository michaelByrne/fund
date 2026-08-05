package pg

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"strings"
	"time"
)

//go:embed migrations
var migrations embed.FS

func SetupTestDatabase() (testcontainers.Container, *pgxpool.Pool, error) {
	containerReq := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		ExposedPorts: []string{"5432/tcp"},
		// Waiting on the port alone is not enough: Docker's proxy binds the host
		// port as soon as the container starts, so the host-side check succeeds
		// while nothing inside is listening yet, and the first connection is reset.
		//
		// The window is wide because the postgres image starts once to run initdb,
		// shuts down, then starts again for real -- which is also why the ready
		// message has to be seen twice. A fast machine usually wins that race and
		// CI often loses it.
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(2 * time.Minute),
		Env: map[string]string{
			"POSTGRES_DB":       "fund",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_USER":     "fund",
		},
	}
	dbContainer, err := testcontainers.GenericContainer(
		context.Background(),
		testcontainers.GenericContainerRequest{
			ContainerRequest: containerReq,
			Started:          true,
		})
	if err != nil {
		return nil, nil, err
	}
	port, err := dbContainer.MappedPort(context.Background(), "5432")
	if err != nil {
		return nil, nil, err
	}
	host, err := dbContainer.Host(context.Background())
	if err != nil {
		return nil, nil, err
	}

	dbURI := fmt.Sprintf("postgres://fund:test@%v:%v/fund", host, port.Port())
	err = migrateDBUp(dbURI)
	if err != nil {
		return nil, nil, err
	}

	connPool, err := GetDBPool(dbURI)
	if err != nil {
		return nil, nil, err
	}

	return dbContainer, connPool, err
}

func migrateDBUp(dbURI string) error {
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return err
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, strings.Replace(dbURI, "postgres://", "pgx://", 1))
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
