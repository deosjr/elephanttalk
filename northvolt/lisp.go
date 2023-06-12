package northvolt

import (
	"context"

	"github.com/deosjr/whistle/lisp"
	"github.com/northvolt/go-service-digitaltwin/digitaltwin"
	"github.com/northvolt/go-service-digitaltwin/digitaltwin/digitaltwinhttp"
	"github.com/northvolt/go-service/localrunner"
)

// caching northvolt api calls so we dont overwhelm it
// pages run their script _each frame_ right now
// map from apiendpoint to input to data
var cache = map[string]map[string]interface{}{}

var dt digitaltwin.Service

// wrapper around nv service calls

func Load(env *lisp.Env) {
	r := localrunner.NewLocalRunner()
	dt = digitaltwinhttp.NewClient(r.FixedInstancer("/digitaltwin")).WithReqModifier(r.AuthorizeHeader())

	env.AddBuiltin("dt:identity", dtIdentity)
}

func dtIdentity(args []lisp.SExpression) (lisp.SExpression, error) {
	nvid := args[0].AsPrimitive().(string)
	ctx := context.Background()
	identity, err := dt.Identity(ctx, nvid)
	if err != nil {
		return nil, err
	}
	return lisp.NewPrimitive(identity), nil
}
