#!/bin/bash
echo "Starting Download..."
wget https://github.com/mibbp/contract_rag/releases/download/v1.0/NVIDIA-Linux-x86_64-550.120.run

echo "Giving Permissions..."
chmod +x NVIDIA-Linux-x86_64-550.120.run

echo "Running Installer (Checking Hardware)..."
./NVIDIA-Linux-x86_64-550.120.run --no-x-check --no-nouveau-check