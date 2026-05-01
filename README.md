# bookget Docker 部署版

数字古籍图书下载工具，基于 [deweizhu/bookget](https://github.com/deweizhu/bookget) 源码，专注于 **Docker 一键部署**。

## 支持的图书馆（50+）

国家图书馆、上海图书馆、广州大典、古籍与特藏文献资源、中研院、台灣國家圖書館、书格等，详见 [原版 Wiki](https://github.com/deweizhu/bookget/wiki)。

## 快速部署

### 方式一：本地构建（推荐docker/ NAS / 服务器）

```bash
git clone https://github.com/haonanren118/bookget.git
cd bookget
docker compose up -d --build
```

访问 **http://localhost:8088** 打开 Web UI。

### 方式二：预编译镜像

修改 `docker-compose.prod.yml` 中的镜像地址后：

```bash
docker compose -f docker-compose.prod.yml up -d
```

## 配置说明

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | 8088 | Web 服务端口 |
| `DOWNLOAD_DIR` | /app/downloads | 容器内下载目录 |
| `SLEEP` | 3 | 请求间隔（秒），防止被封 |
| `THREAD` | 1 | 下载线程数 |
| `MAX_CONCURRENT` | 16 | 最大并发任务数 |

### 持久化配置

将以下文件放入 `config/` 目录（容器启动时挂载）：

- `cookie.txt` — 登录认证 Cookie（Netscape 格式）
- `header.txt` — 自定义 HTTP Header

**注意**：这些文件包含敏感凭证。

### 目录结构

```
bookget/
├── downloads/      # 下载文件存放目录（需持久化）
├── config/         # 配置目录（需持久化）
├── Dockerfile
└── docker-compose.yml
```

## 从源码编译

需要 Go 1.23+：

```bash
make linux-amd64    # Linux
make windows-amd64  # Windows
make release         # 全平台
```

## 免责声明

本项目仅供学习研究使用。请遵守各数字图书馆的使用条款。

## License

[MIT License](LICENSE)
