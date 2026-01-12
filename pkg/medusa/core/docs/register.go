package docs

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/api/docs"
	"github.com/imlargo/medusa/pkg/medusa/tools"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func RegisterDocs(router *gin.Engine, host string, port int) {
	if tools.IsLocalhost(host) {
		host += ":" + strconv.Itoa(port)
	}

	if tools.IsHTTPS(host) {
		docs.SwaggerInfo.Schemes = []string{"https"}
	} else {
		docs.SwaggerInfo.Schemes = []string{"http"}
	}

	docs.SwaggerInfo.Host = (host)
	docs.SwaggerInfo.BasePath = "/"

	schemaUrl := tools.ToCompleteURL(host)
	schemaUrl += "/docs/doc.json"

	urlSwaggerJson := ginSwagger.URL(schemaUrl)
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, urlSwaggerJson))
}
