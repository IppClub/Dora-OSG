# Dora SSR 开源仓库下载服务

这是一个为 Dora SSR 开源游戏引擎生态提供的自托管 Web 服务，用于同步指定 Git 仓库，将源码打包为 zip，并提供查询与下载 API。

## 功能特性

- 周期性同步 Git 仓库
- 自动打包为 zip 文件
- RESTful API 接口
- 限流与 DDoS 防护
- 监控与日志

## 系统要求

- Go 1.22 或更高版本
- 足够的磁盘空间用于存储仓库和 zip 文件
- 网络连接以访问 Git 仓库

## 快速开始

1. 克隆仓库：

```bash
git clone https://github.com/ippclub/dora-osg.git
cd dora-osg
```

2. 安装依赖：

```bash
go mod download
```

3. 配置：

复制示例配置文件并根据需要修改：

```bash
cp config/config.yaml.example config/config.yaml
```

4. 构建：

```bash
GOOS=linux GOARCH=amd64 go build -o dora-osg cmd/server/main.go
```

5. 运行：

```bash
nohup ./dora-osg > /dev/null 2>&1 &
```

## API 文档

### 获取所有包列表

```http
GET /api/v1/packages
```

### 获取特定包版本列表

```http
GET /api/v1/packages/{name}
```

### 获取最新版本特定包下载地址

```http
GET /api/v1/packages/{name}/latest
```

### 获取包列表的版本号

```http
GET /api/v1/package-list-version
```

### 触发同步

```sh
# 本地执行
curl -X POST http://localhost:8866/admin/sync
```

## 配置说明

配置文件 `config.yaml` 包含以下主要配置项：

- `sync`: 同步配置
- `repos`: 仓库列表
- `storage`: 存储配置
- `download`: 下载配置
- `rate_limit`: 限流配置
- `log`: 日志配置

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License