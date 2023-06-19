package args

func IgnoreFirst[T any](_ T, err error) error {
	return err
}
