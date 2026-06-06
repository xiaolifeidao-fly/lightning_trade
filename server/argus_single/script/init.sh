#!/bin/bash

# Argus 初始化脚本 - 上传启动/停止脚本到远程服务器

remote_server="root@${argus_remote_server_1:-}"
remote_password="${argus_password_1:-}"
remote_port="${argus_port_1:-22}"
remote_path="/data/program/app/argus"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 检查环境变量是否设置
if [ -z "$remote_server" ] || [ -z "$remote_password" ]; then
    echo "❌ 错误: 请设置环境变量:"
    echo "   export argus_remote_server_1=your_server_ip"
    echo "   export argus_password_1=your_password"
    echo "   export argus_port_1=22  # 可选，默认22"
    exit 1
fi

echo "=========================================="
echo "初始化 Argus 远程服务器脚本"
echo "=========================================="
echo "远程服务器: $remote_server"
echo "远程端口: $remote_port"
echo "远程路径: $remote_path"
echo ""

# 建立SSH连接并创建远程目录
echo "📁 创建远程目录..."
sshpass -p "$remote_password" ssh -p $remote_port -o StrictHostKeyChecking=no -T "$remote_server" << EOF
  mkdir -p $remote_path
  mkdir -p $remote_path/configs
  mkdir -p $remote_path/logs
  # 清理旧的脚本文件
  rm -f $remote_path/*.sh
EOF

# 上传脚本文件
echo "📤 上传启动脚本..."
sshpass -p "$remote_password" scp -P $remote_port "$SCRIPT_DIR/start.sh" "$remote_server:$remote_path/"
sshpass -p "$remote_password" scp -P $remote_port "$SCRIPT_DIR/stop.sh" "$remote_server:$remote_path/"

# 设置执行权限
echo "🔧 设置执行权限..."
sshpass -p "$remote_password" ssh -p $remote_port -o StrictHostKeyChecking=no -T "$remote_server" << EOF
  chmod +x $remote_path/start.sh
  chmod +x $remote_path/stop.sh
EOF

echo ""
echo "✅ 初始化完成！"
echo "远程服务器脚本已就绪:"
echo "  - $remote_path/start.sh"
echo "  - $remote_path/stop.sh"

