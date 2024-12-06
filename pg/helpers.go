package pg

import "context"

type insertOne[DBArg any, DB any] func(ctx context.Context, arg DBArg) (DB, error)
type upsertOne[DBArg any, DB any] func(ctx context.Context, arg DBArg) (DB, error)
type getOne[In any, Out any] func(ctx context.Context, arg In) (Out, error)
type updateOne[In any, Out any] func(ctx context.Context, arg In) (Out, error)
type getMany[In any, Out any] func(ctx context.Context, arg In) ([]Out, error)
type getAll[DB any] func(ctx context.Context) ([]DB, error)

type transform[In any, Out any] func(In) Out

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

func FetchOne[Realm any, DB any, Arg any](ctx context.Context, arg Arg, get getOne[Arg, DB], transform transform[DB, Realm]) (*Realm, error) {
	dbRes, err := get(ctx, arg)
	if err != nil {
		return nil, err
	}

	result := transform(dbRes)

	return &result, nil
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
