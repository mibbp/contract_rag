#!/bin/bash

# Ollama 模型下载脚本

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}开始下载 Ollama 模型...${NC}"

models=(
    "nomic-embed-text"
    "qwen2.5:3b"
)

for model in "${models[@]}"; do
    echo -e "\n${YELLOW}正在下载模型: $model${NC}"
    docker exec eino-demo-ollama ollama pull "$model"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ $model 下载成功${NC}"
    else
        echo -e "${RED}❌ $model 下载失败${NC}"
    fi
done

echo -e "\n${GREEN}=========================================${NC}"
echo -e "${GREEN}所有模型下载完成！${NC}"
echo -e "${GREEN}=========================================${NC}"

# 列出已安装的模型
echo -e "\n${YELLOW}已安装的模型:${${NC}}"
docker exec eino-demo-ollama ollama list
