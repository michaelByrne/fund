package aws

func toPointer[T any](v T) *T {
	return &v
}
