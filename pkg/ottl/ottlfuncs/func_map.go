// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package ottlfuncs // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
)

// MapArguments holds the arguments for the Map() converter.
type MapArguments[K any] struct {
	Target ottl.PMapGetter[K]
	Lambda ottl.LambdaExpression[K]
}

// NewMapFactory creates a new factory for the Map() converter.
//
// Experimental: *NOTE* this function is subject to change or removal in the future.
func NewMapFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("Map", &MapArguments[K]{}, createMapFunction[K])
}

func createMapFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*MapArguments[K])
	if !ok {
		return nil, errors.New("MapFactory args must be of type *MapArguments[K]")
	}
	return mapFunc(args.Target, &args.Lambda)
}

func mapFunc[K any](target ottl.PMapGetter[K], lambda *ottl.LambdaExpression[K]) (ottl.ExprFunc[K], error) {
	return func(ctx context.Context, tCtx K) (any, error) {
		m, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}

		result := pcommon.NewMap()
		result.EnsureCapacity(m.Len())

		for k, v := range m.All() {
			// Pass key as string, value as pcommon.Value.
			// Passing pcommon.Value directly allows String($v) to call AsString() on it,
			// which matches the behavior of stringify_all (value.AsString()).
			evalResult, err := lambda.Eval(ctx, tCtx, []any{k, v})
			if err != nil {
				return nil, fmt.Errorf("Map lambda evaluation failed for key %q: %w", k, err)
			}

			// The lambda should return a list [newKey, newValue]
			pair, ok := evalResult.([]any)
			if !ok {
				return nil, fmt.Errorf("Map lambda must return a list [key, value], got %T", evalResult)
			}
			if len(pair) != 2 {
				return nil, fmt.Errorf("Map lambda must return exactly 2 elements [key, value], got %d", len(pair))
			}

			newKey, ok := pair[0].(string)
			if !ok {
				return nil, fmt.Errorf("Map lambda first element (key) must be a string, got %T", pair[0])
			}

			// Set the new value in the result map
			newVal := result.PutEmpty(newKey)
			if err := setMapValue(newVal, pair[1]); err != nil {
				return nil, fmt.Errorf("Map lambda result for key %q: %w", newKey, err)
			}
		}

		return result, nil
	}, nil
}

// setMapValue sets a pcommon.Value from a Go any value.
func setMapValue(dst pcommon.Value, src any) error {
	switch v := src.(type) {
	case string:
		dst.SetStr(v)
	case int64:
		dst.SetInt(v)
	case int:
		dst.SetInt(int64(v))
	case float64:
		dst.SetDouble(v)
	case bool:
		dst.SetBool(v)
	case []byte:
		dst.SetEmptyBytes().FromRaw(v)
	case pcommon.Map:
		v.CopyTo(dst.SetEmptyMap())
	case pcommon.Slice:
		v.CopyTo(dst.SetEmptySlice())
	case []any:
		sl := dst.SetEmptySlice()
		sl.EnsureCapacity(len(v))
		for _, elem := range v {
			if err := setMapValue(sl.AppendEmpty(), elem); err != nil {
				return err
			}
		}
	case pcommon.Value:
		v.CopyTo(dst)
	case nil:
		// leave as empty / nil
	default:
		return fmt.Errorf("unsupported value type %T", src)
	}
	return nil
}
