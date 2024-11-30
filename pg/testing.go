package pg

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"strings"
)

//go:embed migrations
var migrations embed.FS

func SetupTestDatabase() (testcontainers.Container, *pgxpool.Pool, error) {
	containerReq := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort("5432/tcp"),
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

	connPool, err := pgxpool.Connect(context.Background(), dbURI)
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
	defer m.Close()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
