package bootstrap

import (
	"strconv"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"

	"github.com/imlargo/medusa/app/internal/hosturl"
)

// docsRoutePath is where Swagger UI and the JSON spec are mounted.
const docsRoutePath = "/docs"

// registerDocs mounts Swagger UI at /docs and points it at spec.
//
// It rewrites the spec's host and scheme so "Try it out" targets the running
// instance instead of whatever was baked in at generation time. host is the
// public host of the API, with or without a scheme; port is only appended for
// local addresses, where it is part of how the API is reached.
func registerDocs(router *gin.Engine, spec *swag.Spec, host string, port int) {
	if hosturl.IsLocalhost(host) {
		host = hosturl.CleanHost(host) + ":" + strconv.Itoa(port)
	}

	spec.Host = host
	spec.BasePath = "/"
	spec.Schemes = []string{docsScheme(host)}

	specURL := ginSwagger.URL(hosturl.Complete(host) + docsRoutePath + "/doc.json")
	router.GET(docsRoutePath+"/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, specURL))
}

// docsScheme picks the scheme the documented API is reached over.
func docsScheme(host string) string {
	if hosturl.IsHTTPS(host) || !hosturl.IsLocalhost(host) {
		return "https"
	}
	return "http"
}
