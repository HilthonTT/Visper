package logging

func logParamsToZapParams(keys map[ExtraKey]any) []any {
	params := make([]any, 0, len(keys))

	for k, v := range keys {
		params = append(params, string(k))
		params = append(params, v)
	}

	return params
}

func logParamsToZeroParams(keys map[ExtraKey]any) map[string]any {
	params := map[string]any{}

	for k, v := range keys {
		params[string(k)] = v
	}

	return params
}
