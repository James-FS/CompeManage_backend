# 前后端部署笔记（Go + Vue + Nginx + MySQL + Redis）

---

# 第一部分：本地开发部署

> 适用场景：拿到代码后在本地跑起来开发调试。

## 前置环境

| 工具 | 用途 |
|------|------|
| Go 1.24+ | 后端编译运行 |
| Node.js 18+ | 前端构建 |
| Docker Desktop | 运行 MySQL 和 Redis 容器 |

## 1. 克隆代码

```bash
git clone <仓库地址>
cd CompeManage
```

## 2. 配置 `.env`（后端项目根目录）

`CompeManage_backend/.env`：

```env
# 数据库配置（本地连 Docker 容器）
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=123456
DB_NAME=CompeManage

# 服务器配置
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# JWT 密钥
JWT_SECRET=4c8b2a9e7f1d3c6a5b0e8d9f2c1a4b7e6d3f0c9a8b5e2d1f4c7a0b9e6d3f2c1a

# 其他配置
APP_ENV=development

# Redis 配置
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
```

> 这个文件已经 `.gitignore`，不会提交到仓库。拿到代码后需要自己创建。

## 3. 启动 MySQL 和 Redis

```powershell
cd CompeManage_backend
docker compose up -d
```

首次启动会自动拉取 `mysql:8.0` 和 `redis:latest` 镜像，之后再次启动直接运行。

## 4. 启动后端

```bash
cd CompeManage_backend
go run main.go
```

看到以下日志说明启动成功：

```
加载配置环境: dev，配置文件: .../config-dev.yaml
[DEBUG] 数据库配置: 用户=root, 密码=******, 地址:localhost:3306, 库名=CompeManage
数据库连接成功
Redis启动成功 host=localhost port=6379
服务器启动：http://localhost:8080
```

## 5. 启动前端

```powershell
cd CompeManage_frontend\CompeManage
npm install    # 首次运行需安装依赖
npm run dev
```

浏览器访问 `http://localhost:5219`。

---

## 配置机制（本地如何工作）

```
.env 文件 ──(godotenv 加载)──> 系统环境变量
                                       │
                  ┌────────────────────┘
                  ▼
config.Init():
  ① 读 config-dev.yaml    → Docker 容器名（compemanage_mysql、compemanage-redis）
  ② BindEnv 覆盖          → .env 中的 localhost 覆盖 YAML 值
  ③ Unmarshal 到结构体    → AppConfig 拿到 localhost
```

**.env 的存在是为了把 YAML 里的 Docker 容器名覆盖成本地 `localhost`**，让你在本地不用改任何代码就能连到 Docker 容器里的 MySQL 和 Redis。

---

# 第二部分：服务器部署

> 适用场景：把项目部署到 Linux 服务器上，对外提供服务。

## 架构说明

```
浏览器 ──→ Nginx(:80) ──→ 静态文件 (dist/)
              │
              └── /api/* ──→ Go 后端(:8080) ──→ MySQL(:3306)
                              │                 │
                              └── Redis(:6379) ─┘
```

所有容器通过 `app-network` Docker 网络通信，容器名就是主机名。

---

## 一、服务器首次部署

### 1.1 创建目录结构

```bash
mkdir -p /root/CompeManage/nginx
mkdir -p /root/CompeManage/dist
mkdir -p /root/CompeManage/mysql_data
mkdir -p /root/docker-images
```

### 1.2 创建 Docker 网络

```bash
docker network create app-network
```

### 1.3 创建 nginx 配置文件

`/root/CompeManage/nginx/nginx.conf`：

```nginx
worker_processes  1;
events {
    worker_connections 1024;
}

http {
    include       mime.types;
    default_type  application/octet-stream;

    server {
        listen 80;
        server_name localhost;
        root   /usr/share/nginx/html;
        index  index.html;

        location / {
            try_files $uri $uri/ /index.html;
        }

        location /api/ {
            proxy_pass http://go-server:8080/;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }

        location /static/ {
            proxy_pass http://go-server:8080/static/;
            proxy_set_header Host $host;
        }
    }
}
```

### 1.4 启动 MySQL

```bash
docker run -d \
  --name compemanage_mysql \
  --network app-network \
  --restart always \
  -e MYSQL_ROOT_PASSWORD=<你的数据库密码> \
  -e MYSQL_DATABASE=CompeManage \
  -e TZ=Asia/Shanghai \
  -p 3306:3306 \
  -v /root/CompeManage/mysql_data:/var/lib/mysql \
  mysql:8.0 \
  --character-set-server=utf8mb4 \
  --collation-server=utf8mb4_unicode_ci \
  --default-authentication-plugin=mysql_native_password
```

### 1.5 启动 Redis

```bash
docker run -d \
  --name compemanage-redis \
  --network app-network \
  --restart always \
  -p 6379:6379 \
  redis:latest \
  redis-server --appendonly yes
```

### 1.6 导入初始数据（如有）

```bash
docker exec -i compemanage_mysql mysql -uroot -p<你的密码> CompeManage < 备份文件.sql
```

---

## 二、后端构建与部署

> 在**本地 Windows 电脑**上操作 2.1 ~ 2.3，在**服务器**上操作 2.4 ~ 2.5。

### 2.1 确认项目文件

构建前确保以下文件存在且正确：

| 文件 | 说明 |
|------|------|
| `CompeManage_backend/Dockerfile` | Go 1.24-alpine，含 GOPROXY 国内代理 |
| `CompeManage_backend/.dockerignore` | 排除 mysql_data/、*.log、static/ 等 |
| `CompeManage_backend/config/config-dev.yaml` | 数据库/Redis host 已写 Docker 容器名 |
| `CompeManage_backend/config/config-prod.yaml` | 同上 |

### 2.2 构建镜像

```powershell
cd D:\Code\CompeManage\CompeManage_backend
docker build -t go-server .
```

> 不需要修改 `.env`，保持本地开发配置即可。`.env` 不会被包含到镜像中。

### 2.3 导出并上传

```powershell
docker save go-server -o D:\docker-images\go-server-new.tar
```

用 WinSCP 将 `go-server-new.tar` 上传到服务器 `/root/docker-images/`。

### 2.4 服务器加载镜像

```bash
docker load -i /root/docker-images/go-server-new.tar
```

### 2.5 启动后端容器

```bash
docker rm -f go-server   # 删除旧容器（如有）

docker run -d \
  --name go-server \
  --network app-network \
  --restart always \
  -e JWT_SECRET=4c8b2a9e7f1d3c6a5b0e8d9f2c1a4b7e6d3f0c9a8b5e2d1f4c7a0b9e6d3f2c1a \
  -e DB_PASSWORD=<你的数据库密码> \
  go-server
```

> 可选环境变量：`DB_HOST`、`DB_PORT`、`DB_USER`、`DB_NAME`、`REDIS_HOST`、`REDIS_PORT`、`REDIS_PASSWORD`。不传则使用 `config-dev.yaml` 中的默认值。

### 2.6 验证

```bash
docker logs go-server
```

应看到：

```
加载配置环境: dev，配置文件: .../config-dev.yaml
[DEBUG] 数据库配置: 用户=root, 密码=******, 地址:compemanage_mysql:3306, 库名=CompeManage
数据库连接成功
Redis启动成功 host=compemanage-redis port=6379
服务器启动：http://localhost:8080
```

---

## 三、前端构建与部署

> 在**本地 Windows 电脑**上操作 3.1 ~ 3.2，在**服务器**上操作 3.3。

### 3.1 构建

```powershell
cd D:\Code\CompeManage\CompeManage_frontend\CompeManage
npm install   # 首次运行
npm run build
```

构建产物在 `dist/` 目录。

### 3.2 上传

将整个 `dist` 目录上传到服务器 `/root/CompeManage/dist`（覆盖旧文件）。

### 3.3 启动 Nginx

```bash
docker rm -f my-nginx   # 删除旧容器（如有）

docker run -d \
  --name my-nginx \
  --network app-network \
  -p 80:80 \
  -v /root/CompeManage/nginx/nginx.conf:/etc/nginx/nginx.conf \
  -v /root/CompeManage/dist:/usr/share/nginx/html \
  --restart always \
  nginx:alpine
```

> **注意**：`-v` 路径必须使用**绝对路径**。

---

## 四、后续更新

### 更新后端

```powershell
# 本地：重新构建 + 导出 + 上传（同 2.2 ~ 2.3）
cd D:\Code\CompeManage\CompeManage_backend
docker build -t go-server .
docker save go-server -o D:\docker-images\go-server-new.tar
# WinSCP 上传到 /root/docker-images/
```

```bash
# 服务器
docker load -i /root/docker-images/go-server-new.tar
docker stop go-server && docker rm go-server
docker run -d \
  --name go-server \
  --network app-network \
  --restart always \
  -e JWT_SECRET=4c8b2a9e7f1d3c6a5b0e8d9f2c1a4b7e6d3f0c9a8b5e2d1f4c7a0b9e6d3f2c1a \
  -e DB_PASSWORD=<你的数据库密码> \
  go-server
docker logs -f go-server
```

### 更新前端

```powershell
# 本地构建 + 上传 dist
cd D:\Code\CompeManage\CompeManage_frontend\CompeManage
npm run build
# 上传 dist 到服务器 /root/CompeManage/dist
```

```bash
# 服务器热重载（无需重启容器）
docker exec my-nginx nginx -s reload
```

---

## 五、常用命令

```bash
# 查看所有容器状态
docker ps

# 查看日志
docker logs -f go-server          # Go 后端实时日志
docker logs my-nginx              # Nginx 访问日志

# 重启
docker restart go-server
docker restart my-nginx

# 进入 MySQL
docker exec -it compemanage_mysql mysql -uroot -p<密码> CompeManage

# 检查容器间网络连通
docker exec go-server ping -c 1 compemanage_mysql
docker exec go-server ping -c 1 compemanage-redis
docker exec my-nginx ping -c 1 go-server
```

---

## 六、常见问题

### 数据库连接失败

1. 确认 `docker run` 时 `-e DB_PASSWORD=...` 与 MySQL 容器的 `MYSQL_ROOT_PASSWORD` 一致
2. 确认 `go-server` 和 `compemanage_mysql` 在同一个 Docker 网络：
   ```bash
   docker network inspect app-network | grep -A 5 "Containers"
   ```

### 启动报 Duplicate entry 'G2026001' for key

数据库中 `comp_code` 有重复记录（含软删除行），执行：

```sql
-- 进入 MySQL 后
SELECT id, comp_code, comp_name, delete_time
FROM comp_directories
WHERE comp_code IN (
  SELECT comp_code FROM (
    SELECT comp_code, COUNT(*) AS cnt
    FROM comp_directories GROUP BY comp_code HAVING cnt > 1
  ) AS tmp
);

-- 改掉已软删除记录的唯一编号
UPDATE comp_directories
SET comp_code = CONCAT(comp_code, '_DELETED_', id)
WHERE delete_time IS NOT NULL AND comp_code IN ('...');
```

然后 `docker restart go-server`。

### 构建报错 "requires go >= 1.24.0"

Dockerfile 中 Go 版本过旧，修改 `FROM golang:1.xx-alpine` 为当前 `go.mod` 要求的版本。

### 构建报错 "proxy.golang.org" 连接超时

Dockerfile 缺少国内代理，确保有 `ENV GOPROXY=https://goproxy.cn,direct`。

### 登录失败 / 无响应

1. 看 nginx 日志有没有 `/api/` 请求：`docker logs my-nginx`
2. 如果没有，浏览器 F12 → Console 看 JS 报错
3. 确认 `nginx.conf` 中 `proxy_pass http://go-server:8080/;`（末尾斜杠不能省）
