package ref

func Ref[k any](input k) *k {
	return &input
}

func DerefOrZero[k any](input *k) k {
	if input != nil {
		return *input
	}
	var zero k
	return zero
}

func DerefOrDefault[k any](input *k, def k) k {
	if input != nil {
		return *input
	}
	return def
}
