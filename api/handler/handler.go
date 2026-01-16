package handler

import (
	"eino-demo/api/response"
	"eino-demo/service"
	"eino-demo/types"
	"fmt"
	"github.com/gin-gonic/gin"
)

type ContractHandler struct {
	ingestionSvc *service.ContractService
	retrievalSvc *service.RetrievalService
}

func NewContractHandler(ingestionSvc *service.ContractService, retrievalSvc *service.RetrievalService) *ContractHandler {
	return &ContractHandler{
		ingestionSvc: ingestionSvc,
		retrievalSvc: retrievalSvc,
	}
}

// Upload 上传合同接口
func (h *ContractHandler) Upload(c *gin.Context) {
	fmt.Println(">>> [DEBUG] 1. 进入 Handler")
	form, err := c.MultipartForm()
	if err != nil {
		fmt.Println(">>> [DEBUG] error: 表单解析失败", err)
		response.Fail(c, "文件上传失败或格式错误")
		return
	}
	// 1. 获取文件
	files := form.File["file"]
	if len(files) == 0 {
		response.Fail(c, "未接收到文件，请检查参数名是否为 'file'")
		return
	}
	fmt.Printf(">>> [DEBUG] 2. 收到文件列表，共 %d 个文件\n", len(files))

	var allDocIDs []string
	var errorFiles []string
	// 2. 调用 Service
	for _, file := range files {
		fmt.Printf(">>> [DEBUG] ---> 开始处理文件: %s, 大小: %d\n", file.Filename, file.Size)

		// 调用现有的 Service (它负责单个文件处理)
		ids, err := h.ingestionSvc.UploadAndProcess(c.Request.Context(), file)
		if err != nil {
			fmt.Printf(">>> [ERROR] 文件 %s 处理失败: %v\n", file.Filename, err)
			errorFiles = append(errorFiles, file.Filename)
			// 这里使用 continue，即使一个文件失败，也不影响其他文件上传
			continue
		}

		// 汇总 ID
		allDocIDs = append(allDocIDs, ids...)
	}

	fmt.Printf(">>> [DEBUG] 3. 批量处理完成，成功生成 ID 总数: %d\n", len(allDocIDs))

	// 3. 返回结果
	// 如果所有文件都失败了
	if len(allDocIDs) == 0 && len(errorFiles) > 0 {
		response.Fail(c, fmt.Sprintf("所有文件处理失败: %v", errorFiles))
		return
	}

	// 返回成功的部分 (也可以把失败列表带上)
	response.Success(c, map[string]any{
		"doc_ids":     allDocIDs,
		"status":      "indexed",
		"total_count": len(allDocIDs),
		"fail_files":  errorFiles, // 告诉前端哪些文件失败了
	})
}

func (h *ContractHandler) Search(c *gin.Context) {
	var req types.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, "参数错误: query 不能为空")
		return
	}

	fmt.Printf(">>> [DEBUG] 收到搜索请求: %s\n", req.Query)

	// 调用 RetrievalService
	result, err := h.retrievalSvc.Search(c.Request.Context(), req.Query)
	if err != nil {
		response.Fail(c, err.Error())
		return
	}

	response.Success(c, result)
}
