package router

import (
	"eino-demo/api/handler"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, contractH *handler.ContractHandler) {
	api := r.Group("/api/v1")
	{
		contract := api.Group("/contract")
		{
			contract.POST("/upload", contractH.Upload)
			// contract.GET("/list", contractH.GetList)
		}
		retrieval := api.Group("/retrieval")
		{
			retrieval.POST("/search", contractH.Search)
		}
		// chat := api.Group("/chat")
		// ...
	}
}
