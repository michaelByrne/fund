package pg

import (
	"boardfund/db"
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type insertOne[DBArg any, DB any] func(ctx context.Context, arg DBArg) (DB, error)
type upsertOne[DBArg any, DB any] func(ctx context.Context, arg DBArg) (DB, error)
type getOne[In any, Out any] func(ctx context.Context, arg In) (Out, error)
type updateOne[In any, Out any] func(ctx context.Context, arg In) (Out, error)
type updateMany[In any, Out any] func(ctx context.Context, arg In) ([]Out, error)
type getMany[In any, Out any] func(ctx context.Context, arg In) ([]Out, error)
type getAll[DB any] func(ctx context.Context) ([]DB, error)

type transform[In any, Out any] func(In) Out

func UpdateMany[DBArg any, StoreArg, Realm any, DB any](ctx context.Context, arg StoreArg, update updateMany[DBArg, DB], transformIn transform[StoreArg, DBArg], transformOut transform[DB, Realm]) ([]Realm, error) {
	dbArg := transformIn(arg)
	dbRes, err := update(ctx, dbArg)
	if err != nil {
		return nil, err
	}

	result := make([]Realm, len(dbRes))
	for i, r := range dbRes {
		result[i] = transformOut(r)
	}

	return result, nil
}

func CreateOne[DBArg any, StoreArg, Realm any, DB any](ctx context.Context, arg StoreArg, insert insertOne[DBArg, DB], transformIn transform[StoreArg, DBArg], transformOut transform[DB, Realm]) (*Realm, error) {
	dbArg := transformIn(arg)
	dbRes, err := insert(ctx, dbArg)
	if err != nil {
		return nil, err
	}

	result := transformOut(dbRes)

	return &result, nil
}

func UpsertOne[DBArg any, StoreArg, Realm any, DB any](ctx context.Context, arg StoreArg, upsert upsertOne[DBArg, DB], transformIn transform[StoreArg, DBArg], transformOut transform[DB, Realm]) (*Realm, error) {
	dbArg := transformIn(arg)
	dbRes, err := upsert(ctx, dbArg)
	if err != nil {
		return nil, err
	}

	result := transformOut(dbRes)

	return &result, nil
}

func FetchOne[Realm any, DB any, StoreArg any, Arg any](ctx context.Context, arg StoreArg, get getOne[Arg, DB], transformIn transform[StoreArg, Arg], transformOut transform[DB, Realm]) (*Realm, error) {
	dbArg := transformIn(arg)

	dbRes, err := get(ctx, dbArg)
	if err != nil {
		return nil, err
	}

	result := transformOut(dbRes)

	return &result, nil
}

func FetchScalar[Arg any, Realm any, DB any](ctx context.Context, arg Arg, get getOne[Arg, DB], transform transform[DB, Realm]) (Realm, error) {
	var realmZero Realm
	dbRes, err := get(ctx, arg)
	if err != nil {
		return realmZero, err
	}

	result := transform(dbRes)

	return result, nil
}

func FetchMany[Realm any, DB any, Arg any](ctx context.Context, arg Arg, get getMany[Arg, DB], transform transform[DB, Realm]) ([]Realm, error) {
	dbRes, err := get(ctx, arg)
	if err != nil {
		return nil, err
	}

	result := make([]Realm, len(dbRes))
	for i, r := range dbRes {
		result[i] = transform(r)
	}

	return result, nil
}

func FetchAll[Realm any, DB any](ctx context.Context, get getAll[DB], transform transform[DB, Realm]) ([]Realm, error) {
	dbRes, err := get(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Realm, len(dbRes))
	for i, r := range dbRes {
		result[i] = transform(r)
	}

	return result, nil
}

func UpdateOne[DBArg any, StoreArg any, Realm any, DB any](ctx context.Context, arg StoreArg, update updateOne[DBArg, DB], transformIn transform[StoreArg, DBArg], transformOut transform[DB, Realm]) (*Realm, error) {
	dbArg := transformIn(arg)
	dbRes, err := update(ctx, dbArg)
	if err != nil {
		return nil, err
	}

	result := transformOut(dbRes)

	return &result, nil
}

func GetDBPool(dbURI string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dbURI)
	if err != nil {
		return nil, err
	}

	dbpool, err := pgxpool.NewWithConfig(context.Background(), config)

	customTypes, err := getCustomDataTypes(context.Background(), dbpool)
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		for _, t := range customTypes {
			conn.TypeMap().RegisterType(t)
			conn.TypeMap().RegisterDefaultPgType(&db.DBTime{}, "timestamptz")
			conn.TypeMap().RegisterDefaultPgType(&db.NullDBTime{}, "timestamptz")
			conn.TypeMap().RegisterDefaultPgType(db.DBTime{}, "timestamptz")
			conn.TypeMap().RegisterDefaultPgType(db.NullDBTime{}, "timestamptz")
		}
		return nil
	}

	dbpool.Close()
	dbpool, err = pgxpool.NewWithConfig(context.Background(), config)

	return dbpool, err
}

func getCustomDataTypes(ctx context.Context, pool *pgxpool.Pool) ([]*pgtype.Type, error) {
	// Get a single connection just to load type information.
	conn, err := pool.Acquire(ctx)
	defer conn.Release()
	if err != nil {
		return nil, err
	}

	dataTypeNames := []string{
		"role",
		"_role",
	}

	var typesToRegister []*pgtype.Type
	for _, typeName := range dataTypeNames {
		dataType, err := conn.Conn().LoadType(ctx, typeName)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to load type %s: %v", typeName, err)
		}
		// You need to register only for this connection too, otherwise the array type will look for the register element type.
		conn.Conn().TypeMap().RegisterType(dataType)
		typesToRegister = append(typesToRegister, dataType)
	}

	return typesToRegister, nil
}
