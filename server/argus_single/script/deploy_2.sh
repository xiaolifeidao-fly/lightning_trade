#!/bin/bash

# Argus_single 项目部署脚本
# 从环境变量获取配置
remote_server="root@${argus_single_remote_server_2:-}"
remote_password="${argus_single_password_2:-}"
remote_port="${argus_single_port_2:-22}"

APP_NAME="argus_single"
REMOTE_PATH="/data/program/app/argus_single"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 检查环境变量是否设置
if [ -z "$remote_server" ] || [ -z "$remote_password" ]; then
    echo "❌ 错误: 请设置环境变量:"
    echo "   export argus_single_remote_server_2=your_server_ip"
    echo "   export argus_single_password_2=your_password"
    echo "   export argus_single_port_2=22  # 可选，默认22"
    exit 1
fi

echo "=========================================="
echo "开始部署 $APP_NAME"
echo "=========================================="
echo "远程服务器: $remote_server"
echo "远程端口: $remote_port"
echo "远程路径: $REMOTE_PATH"
echo ""

# 执行构建脚本
echo "📦 步骤1: 开始构建应用..."
cd "$SCRIPT_DIR" || exit 1
bash build.sh

if [ ! -f "../$APP_NAME" ]; then
    echo "❌ 构建失败，请检查错误信息"
    exit 1
fi

echo ""
echo "📤 步骤2: 准备上传文件..."

# 建立SSH连接并创建远程目录
sshpass -p "$remote_password" ssh -p $remote_port -o StrictHostKeyChecking=no -T "$remote_server" << EOF
  mkdir -p $REMOTE_PATH
  cd $REMOTE_PATH
  # 备份旧版本（如果存在）
  if [ -f $APP_NAME ]; then
    mv $APP_NAME ${APP_NAME}.backup.\$(date +%Y%m%d_%H%M%S)
  fi
EOF

# 上传新的二进制文件
echo "📤 步骤3: 上传二进制文件..."
sshpass -p "$remote_password" scp -P $remote_port "../$APP_NAME" "$remote_server:$REMOTE_PATH/"

# 上传配置文件（如果存在）
# if [ -f "../configs/application.properties" ]; then
#     echo "📤 步骤4: 上传配置文件..."
#     sshpass -p "$remote_password" ssh -p $remote_port -o StrictHostKeyChecking=no -T "$remote_server" << EOF
#       mkdir -p $REMOTE_PATH/configs
# EOF
#     sshpass -p "$remote_password" scp -P $remote_port -r "../configs/" "$remote_server:$REMOTE_PATH/"
# fi

# 上传启动脚本
# echo "📤 步骤5: 上传启动脚本..."
# sshpass -p "$remote_password" scp -P $remote_port "./start.sh" "$remote_server:$REMOTE_PATH/"
# sshpass -p "$remote_password" scp -P $remote_port "./stop.sh" "$remote_server:$REMOTE_PATH/"

# 执行重启命令
echo ""
echo "🔄 步骤6: 重启应用..."
sshpass -p "$remote_password" ssh -p $remote_port -o StrictHostKeyChecking=no -T "$remote_server" << EOF
  cd $REMOTE_PATH
  
  # 停止旧进程
  echo "停止旧进程..."
  bash stop.sh || true
  
  # 等待进程停止
  sleep 2
  
  # 设置执行权限
  chmod +x $APP_NAME
  chmod +x start.sh
  chmod +x stop.sh
  
  # 启动应用
  echo "启动新进程..."
  bash start.sh
  
  # 等待应用启动
  sleep 5
  
  # 检查是否成功启动
  new_pid=\$(ps -ef | grep ${APP_NAME} | grep -v grep | awk '{print \$2}')
  if [ -n "\$new_pid" ]; then
      echo "✅ 应用启动成功，进程ID: \$new_pid"
      echo "📊 查看日志: tail -f $REMOTE_PATH/server.log"
  else
      echo "❌ 应用启动失败，请检查日志: $REMOTE_PATH/server.log"
      exit 1
  fi
EOF

echo ""
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="

