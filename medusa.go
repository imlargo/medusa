// Package medusa is the Gin edge of the Medusa toolkit: a Context that extends
// *gin.Context, adapters that turn handlers returning (result, error) into
// gin.HandlerFunc, and the writers for the response format.
//
// httperr, httpserver, lifecycle and storage take standard library types.
// Only this package, middleware and the health probe handlers depend on Gin.
//
// A typical handler and its route:
//
//	func (h *UserHandler) Get(ctx *medusa.Context) (*dto.User, error) {
//	    id, err := ctx.ParamID("id")
//	    if err != nil {
//	        return nil, err
//	    }
//	    return h.users.ByID(ctx.Ctx(), id)
//	}
//
//	users.GET("/:id", medusa.HandleGet(h.Get))
//
// A returned *httperr.Error sets the status; any other error becomes a 500
// whose cause goes to the access log and never to the client.
package medusa
