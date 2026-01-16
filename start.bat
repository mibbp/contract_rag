@echo off
REM Windows 本地开发启动脚本

echo =========================================
echo   Eino-Demo 本地开发启动
echo =========================================

echo.
echo [1/3] 检查本地服务是否运行...
echo 请确保以下服务已在本地启动：
echo   - PostgreSQL (localhost:5432)
echo   - Milvus (localhost:19530)
echo   - Elasticsearch (localhost:9200)
echo   - Ollama (localhost:11434)
echo.

pause

echo.
echo [2/3] 启动 Go 应用...
go run main.go

pause
