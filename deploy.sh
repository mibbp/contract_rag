#!/bin/bash

# 部署脚本 - CentOS 7

set -e

echo "========================================="
echo "  Eino-Demo 部署脚本"
echo "========================================="

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. 检查 Docker
echo -e "\n${YELLOW}[1/6] 检查 Docker...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker 未安装，请先安装 Docker${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Docker 已安装${NC}"

# 2. 检查 Docker Compose
echo -e "\n${YELLOW}[2/6] 检查 Docker Compose...${NC}"
if ! docker compose version &> /dev/null; then
    echo -e "${RED}❌ Docker Compose V2 未安装${NC}"
    echo "安装命令: yum install -y docker-compose-plugin"
    exit 1
fi
echo -e "${GREEN}✅ Docker Compose V2 已安装${NC}"

# 3. 检查 NVIDIA GPU 支持
echo -e "\n${YELLOW}[3/6] 检查 NVIDIA GPU...${NC}"
if command -v nvidia-smi &> /dev/null; then
    echo -e "${GREEN}✅ NVIDIA GPU 已检测到${NC}"
    nvidia-smi --query-gpu=name,memory.total --format=csv,noheader
else
    echo -e "${YELLOW}⚠️  未检测到 NVIDIA GPU，Ollama 将使用 CPU 运行（较慢）${NC}"
fi

# 4. 停止旧容器（如果存在）
echo -e "\n${YELLOW}[4/6] 停止旧容器...${NC}"
docker compose down 2>/dev/null || true
echo -e "${GREEN}✅ 清理完成${NC}"

# 5. 构建并启动服务
echo -e "\n${YELLOW}[5/6] 构建并启动服务（首次启动约 5-10 分钟）...${NC}"
docker compose up -d --build

# 6. 等待服务启动
echo -e "\n${YELLOW}[6/6] 等待服务启动...${NC}"
sleep 10

# 检查服务状态
echo -e "\n${GREEN}=========================================${NC}"
echo -e "${GREEN}服务状态${NC}"
echo -e "${GREEN}=========================================${NC}"
docker compose ps

echo -e "\n${YELLOW}=========================================${NC}"
echo -e "${YELLOW}下一步操作${NC}"
echo -e "${YELLOW}=========================================${NC}"
echo -e "1. 下载 Ollama 模型:"
echo -e "   ${GREEN}docker exec eino-demo-ollama ollama pull nomic-embed-text${NC}"
echo -e "   ${GREEN}docker exec eino-demo-ollama ollama pull qwen2.5:3b${NC}"
echo -e ""
echo -e "2. 查看应用日志:"
echo -e "   ${GREEN}docker compose logs -f app${NC}"
echo -e ""
echo -e "3. 测试应用:"
echo -e "   ${GREEN}curl http://localhost:8081${NC}"
echo -e ""
echo -e "4. 访问管理界面:"
echo -e "   - Kibana: ${GREEN}http://$(hostname -I | awk '{print $1}'):5601${NC}"
echo -e "   - Attu:  ${GREEN}http://$(hostname -I | awk '{print $1}'):8000${NC}"
echo -e "${YELLOW}=========================================${NC}"
