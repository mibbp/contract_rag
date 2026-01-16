# 部署到 CentOS 7 服务器

## 前提条件

- CentOS 7 服务器已安装 Docker
- 服务器有 GPU（NVIDIA）
- 已安装 Docker Compose V2
- 已安装 NVIDIA Container Toolkit（支持 GPU 容器）

---

## 一、服务器环境准备

### 1. 安装 NVIDIA Container Toolkit（如未安装）

```bash
# 添加 NVIDIA 仓库
distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.repo | sudo tee /etc/yum.repos.d/nvidia-docker.repo

# 安装 nvidia-container-toolkit
sudo yum install -y nvidia-container-toolkit

# 重启 Docker
sudo systemctl restart docker

# 测试 GPU 是否可用
docker run --rm --gpus all nvidia/cuda:11.0.3-base-ubuntu20.04 nvidia-smi
```

### 2. 验证 Docker Compose 版本

```bash
docker compose version
# 如果显示版本号，说明是 V2（推荐）
# 如果报错，可能需要安装：
# sudo yum install -y docker-compose-plugin
```

---

## 二、上传项目到服务器

### 方式 1：使用 SCP（从本地 Windows）

```bash
# 在 PowerShell 或 Git Bash 中执行
scp -r D:\Go_workspace\eino-demo root@你的服务器IP:/root/eino-demo
```

### 方式 2：使用 Git（推荐）

```bash
# 在服务器上执行
cd /root
git clone <你的仓库地址> eino-demo
cd eino-demo
```

---

## 三、部署步骤

### 1. 进入项目目录

```bash
cd /root/eino-demo
```

### 2. 检查 Elasticsearch 镜像构建目录

你的 docker-compose.yml 引用了 `./deploy/es/Dockerfile`，确保目录存在：

```bash
# 检查是否有这个目录
ls -la deploy/es/

# 如果没有，需要创建（假设你有 ES 的 Dockerfile）
# 如果不想自定义 ES，可以修改 docker-compose.yml 使用官方镜像
```

**如果不使用自定义 ES**，修改 `docker-compose.yml` 第 89-93 行：

```yaml
elasticsearch:
  container_name: elasticsearch
  image: elasticsearch:8.11.1  # 改为直接使用官方镜像
  environment:
    # ... 其他配置保持不变
```

### 3. 构建并启动所有服务

```bash
# 构建镜像并启动（第一次会比较慢）
docker compose up -d --build

# 查看启动状态
docker compose ps

# 查看日志（排查问题）
docker compose logs -f app
```

### 4. 下载 Ollama 模型

进入 Ollama 容器下载模型：

```bash
# 方式 1：进入容器下载
docker exec -it eino-demo-ollama bash
ollama pull nomic-embed-text
ollama pull qwen2.5:3b
exit

# 方式 2：直接执行命令（推荐）
docker exec eino-demo-ollama ollama pull nomic-embed-text
docker exec eino-demo-ollama ollama pull qwen2.5:3b
```

### 5. 验证服务状态

```bash
# 检查 PostgreSQL
docker exec eino-demo-postgres pg_isready -U root -d einoDB

# 检查 Milvus
curl http://localhost:19530/healthz

# 检查 Elasticsearch
curl http://localhost:9200/_cluster/health

# 检查 Ollama
curl http://localhost:11434/api/tags

# 检查 Go 应用
curl http://localhost:8081/health
```

---

## 四、常用管理命令

### 启动/停止服务

```bash
# 启动所有服务
docker compose up -d

# 停止所有服务
docker compose down

# 重启某个服务
docker compose restart app

# 查看服务状态
docker compose ps
```

### 查看日志

```bash
# 查看所有服务日志
docker compose logs

# 查看特定服务日志
docker compose logs -f app
docker compose logs -f ollama

# 查看最近 100 行日志
docker compose logs --tail=100 app
```

### 进入容器调试

```bash
# 进入 Go 应用容器
docker exec -it eino-demo-app sh

# 进入 PostgreSQL
docker exec -it eino-demo-postgres psql -U root -d einoDB

# 进入 Ollama
docker exec -it eino-demo-ollama bash
```

---

## 五、端口映射说明

| 服务 | 容器端口 | 主机端口 | 访问地址 |
|------|---------|---------|---------|
| Go 应用 | 8081 | 8081 | http://服务器IP:8081 |
| PostgreSQL | 5432 | 5432 | 服务器IP:5432 |
| Milvus | 19530 | 19530 | 服务器IP:19530 |
| Elasticsearch | 9200 | 9200 | http://服务器IP:9200 |
| Ollama | 11434 | 11434 | http://服务器IP:11434 |
| Kibana | 5601 | 5601 | http://服务器IP:5601 |
| Attu (Milvus UI) | 3000 | 8000 | http://服务器IP:8000 |

---

## 六、故障排查

### 1. 容器启动失败

```bash
# 查看容器日志
docker compose logs <服务名>

# 查看容器详细信息
docker inspect eino-demo-app
```

### 2. GPU 不可用

```bash
# 检查 NVIDIA 驱动
nvidia-smi

# 检查 Docker GPU 支持
docker run --rm --gpus all nvidia/cuda:11.0.3-base-ubuntu20.04 nvidia-smi
```

### 3. 网络连接问题

```bash
# 进入 app 容器测试网络连接
docker exec -it eino-demo-app sh
ping postgres
ping milvus-standalone
ping elasticsearch
ping ollama
```

### 4. 数据持久化

所有数据存储在 Docker Volume 中：

```bash
# 查看所有卷
docker volume ls

# 备份数据（示例：PostgreSQL）
docker run --rm -v eino-demo_postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/postgres_backup.tar.gz /data
```

---

## 七、生产环境优化建议

### 1. 修改默认密码

编辑 `docker-compose.yml`，修改以下密码：
- PostgreSQL 密码：`Llh2002908`
- MinIO 密码：`minioadmin`

### 2. 资源限制

在 `docker-compose.yml` 中添加资源限制：

```yaml
services:
  app:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
```

### 3. 配置反向代理（Nginx）

使用 Nginx 反向代理到 80 端口，并配置 HTTPS：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 4. 日志轮转

配置 Docker 日志大小限制：

```yaml
services:
  app:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

---

## 八、更新部署

当代码更新后：

```bash
# 拉取最新代码
git pull

# 重新构建并启动
docker compose up -d --build

# 仅重启 app 服务
docker compose up -d --build app
```
