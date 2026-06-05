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

type MapValuesArguments[K any] struct {
	Target ottl.PMapGetSetter[K]
	Lambda ottl.LambdaExpression[K]
}

func NewMapValuesFactory[K any]() ottl.Factory[K] {
	return ottl.NewFactory("map_values", &MapValuesArguments[K]{}, createMapValuesFunction[K])
}

func createMapValuesFunction[K any](_ ottl.FunctionContext, oArgs ottl.Arguments) (ottl.ExprFunc[K], error) {
	args, ok := oArgs.(*MapValuesArguments[K])
	if !ok {
		return nil, errors.New("MapValuesFactory args must be of type *MapValuesArguments[K]")
	}
	return mapValuesFunc(args.Target, &args.Lambda)
}

func mapValuesFunc[K any](target ottl.PMapGetSetter[K], lambda *ottl.LambdaExpression[K]) (ottl.ExprFunc[K], error) {
	return func(ctx context.Context, tCtx K) (any, error) {
		val, err := target.Get(ctx, tCtx)
		if err != nil {
			return nil, err
		}
		for k, v := range val.All() {
			result, err := lambda.Eval(ctx, tCtx, []any{v})
			if err != nil {
				return nil, fmt.Errorf("map_values lambda evaluation failed for key %q: %w", k, err)
			}
			switch typedVal := result.(type) {
			case string:
				v.SetStr(typedVal)
			case int64:
				v.SetInt(typedVal)
			case float64:
				v.SetDouble(typedVal)
			case bool:
				v.SetBool(typedVal)
			case []byte:
				v.SetEmptyBytes().FromRaw(typedVal)
			case pcommon.Map:
				typedVal.CopyTo(v.SetEmptyMap())
			case pcommon.Slice:
				typedVal.CopyTo(v.SetEmptySlice())
			case pcommon.Value:
				typedVal.CopyTo(v)
			default:
				v.SetStr(fmt.Sprintf("%v", typedVal))
			}
		}
		return nil, target.Set(ctx, tCtx, val)
	}, nil
}
