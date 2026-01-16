# 快速参考

## 本地开发（Windows）

```bash
# 1. 启动依赖服务（PostgreSQL, Milvus, ES, Ollama）
# 手动启动或使用 Docker

# 2. 运行 Go 应用
go run main.go

# 3. 或使用启动脚本
start.bat
```

## 服务器部署（CentOS 7）

### 一键部署

```bash
# 1. 上传项目到服务器
scp -r D:\Go_workspace\eino-demo root@服务器IP:/root/eino-demo

# 2. 登录服务器
ssh root@服务器IP

# 3. 进入项目目录
cd /root/eino-demo

# 4. 给脚本添加执行权限
chmod +x deploy.sh
chmod +x download-models.sh

# 5. 运行部署脚本
./deploy.sh

# 6. 下载模型
./download-models.sh
```

### 常用命令

```bash
# 启动所有服务
docker compose up -d

# 停止所有服务
docker compose down

# 查看服务状态
docker compose ps

# 查看日志
docker compose logs -f app

# 重启某个服务
docker compose restart app

# 进入容器
docker exec -it eino-demo-app sh
docker exec -it eino-demo-postgres psql -U root -d einoDB
docker exec -it eino-demo-ollama bash
```

### 服务地址

| 服务 | 地址 |
|------|------|
| Go 应用 | http://服务器IP:8081 |
| Kibana | http://服务器IP:5601 |
| Attu (Milvus UI) | http://服务器IP:8000 |

### 测试接口

```bash
# 健康检查
curl http://localhost:8081/health

# 上传合同
curl -X POST http://localhost:8081/api/upload \
  -F "file=@contract.pdf"

# 搜索
curl -X POST http://localhost:8081/api/search \
  -H "Content-Type: application/json" \
  -d '{"query": "张三签署的合同"}'
```

## 环境变量

所有环境变量都在 `vars/const.go` 中配置，支持 Docker 环境变量覆盖：

| 环境变量 | 默认值 | 说明 |
|---------|-------|------|
| PGHOST | localhost | PostgreSQL 地址 |
| PGUSER | root | PostgreSQL 用户名 |
| PGPWD | Llh2002908 | PostgreSQL 密码 |
| PGDB | einoDB | PostgreSQL 数据库名 |
| MILVUSADDR | 127.0.0.1:19530 | Milvus 地址 |
| ESADDR | http://localhost:9200 | Elasticsearch 地址 |
| OLLAMA_PATH | http://localhost:11434 | Ollama 地址 |
