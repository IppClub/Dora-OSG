# Dora SSR 开源仓库下载服务 — 详细设计需求（Go 版）

> 版本：v1.0
>  作者：李瑾
>  日期：2025-06-05

------

## 1. 总体目标

为 **Dora SSR** 开源游戏引擎生态提供一个自托管的 Web 服务，按日同步指定 Git 仓库，将源码打包为 zip，提供查询与下载 API，并具备高效下载、限流与抗 DDoS 能力。

------

## 2. 范围

| 模块                  | 描述                                       |
| --------------------- | ------------------------------------------ |
| **Repo Sync**         | 周期性拉取/更新公开 Git 仓库到本地缓存目录 |
| **Packaging**         | 生成不含 `.git/` 等冗余文件的 zip 包       |
| **Public API**        | 提供包列表、包元数据及下载 URL             |
| **Static Download**   | 高效分块下载（Range 支持）与断点续传       |
| **Rate Limit & DDoS** | 服务端 & 边缘双层限流、防护                |

------

## 3. 术语

| 缩写     | 含义                      |
| -------- | ------------------------- |
| **RPO**  | Repository Pull Operation |
| **PKG**  | Packaging Job             |
| **ZDir** | zip 文件输出根目录        |

------

## 4. 技术栈

| 领域          | 选型                                                         |
| ------------- | ------------------------------------------------------------ |
| **语言**      | Go 1.22+                                                     |
| **Git 操作**  | [go-git](https://github.com/go-git/go-git)（纯 Go，无外部 `git` 依赖） |
| **HTTP 框架** | `net/http` + [Chi](https://github.com/go-chi/chi)（轻量路由） |
| **任务调度**  | [robfig/cron/v3](https://github.com/robfig/cron)             |
| **压缩**      | Go 标准库 `archive/zip`                                      |
| **速率限制**  | `golang.org/x/time/rate`（Token Bucket）                     |
| **配置管理**  | `yaml.v3`（热加载可选：fsnotify）                            |
| **监控**      | Prometheus + Grafana                                         |
| **日志**      | zap（JSON 日志，支持结构化字段）                             |
| **边缘防护**  | Nginx (或 Caddy) + CDN (Cloudflare / 阿里云 CDN)             |

------

## 5. 架构概览

```
+------------+          +-----------------+
|  Scheduler |----RPO-->| Repo Cache (/data/repos) |
+------------+          +-----------------+
       |                         |
       | PKG                     |         +---------------------+
       +---->+-----------------+ |         | Nginx / CDN (edge)  |
             |  Zipper Worker  |----ZIP--->| • 静态文件限流       |
             +-----------------+           | • 防护 / 缓存       |
                    |                      +---------+-----------+
                    |                                 |
            +-------v--------+                +-------v-------+
            |  Public  API   |<---HTTP/JSON---| Rate-Limit MW |
            |   (Chi)        |                +---------------+
            +-------+--------+
                    |
              +-----v-----+
              | FileSrv   | (X-Accel-Redirect / Sendfile)
              +-----------+
```

------

## 6. 功能需求

### 6.1 配置文件 (`config.yaml`)

```yaml
sync_cron: "0 3 * * *"          # 每日 03:00 拉取
repos:
  - name: "dora-demo"
    url:  "https://atomgit.com/ippclub/dora-demo.git"
  - name: "dora-story"
    url:  "https://atomgit.com/ippclub/dora-story.git"
storage:
  path: "/data"                  # 根目录，子目录 repos/、zips/
download:
  base_url: "https://39.155.148.157:8866"
rate_limit:
  rps: 10                        # 每 IP 每秒 10 次 API 调用
  burst: 20
```

> *要求*：配置文件热重载；字段变更实时生效，错误回滚。

------

### 6.2 Repo Sync

| 编号      | 描述                                                     | 约束                                                        |
| --------- | -------------------------------------------------------- | ----------------------------------------------------------- |
| **FR-S1** | 定时拉取仓库（clone/ fetch --all）。                     | 支持 `sync_cron`; 手动触发 API：`POST /admin/sync`          |
| **FR-S2** | 失败重试 3 次，每次指数退避 (2^n)。                      | 同一仓库并行拉取互斥 (mutex by `name`)                      |
| **FR-S3** | 拉取完成后写入元数据：`lastSyncAt`, `commitHash`, `tag`. | 用 BoltDB / SQLite 元数据表，tag 为拉取的 commit 对应的标签 |

------

### 6.3 Packaging

| 编号      | 描述                                                         |
| --------- | ------------------------------------------------------------ |
| **FR-P1** | 每次拉取成功后立即异步生成 zip。                             |
| **FR-P2** | 排除路径：`.git`, `.github`, `docs/`, `*.md`（可在 config 设定）。 |
| **FR-P3** | 文件名：`<name>-<commitHash[0:7]>.zip`.                      |
| **FR-P4** | 历史 zip 最多保留 `N` 个版本（缺省 3），超出自动删除。       |
| **FR-P5** | 生成过程中使用临时文件并原子替换，避免下载脏文件。           |

------

### 6.4 公共 API

| 方法  | 路径                             | 描述                             |
| ----- | -------------------------------- | -------------------------------- |
| `GET` | `/api/v1/packages`               | 返回所有仓库最新 zip 元数据。    |
| `GET` | `/api/v1/packages/{name}`        | 返回指定仓库所有可下载版本列表。 |
| `GET` | `/api/v1/packages/{name}/latest` | 302 跳转到最新 zip。             |

**响应示例**

```jsonc
// GET /api/v1/packages
[
  {
    "name": "dora-demo",
    "latest": {
      "file": "dora-demo-a1b2c3d.zip",
      "size": 1048576,
      "tag": "v0.1.0",
      "commit": "a1b2c3d",
      "download": "https://39.155.148.157:8866/zips/dora-demo-a1b2c3d.zip",
      "updatedAt": "2025-06-05T03:10:27Z"
    },
    "v0.1.0": {
      "file": "dora-demo-a1b2c3d.zip",
      "size": 1048576,
      "tag": "v0.1.0",
      "commit": "a1b2c3d",
      "download": "https://39.155.148.157:8866/zips/dora-demo-a1b2c3d.zip",
      "updatedAt": "2025-06-05T03:10:27Z"
    }
  }
]
```

> *错误码*：
>  `400` 参数错误；`404` 未找到；`429` 超限；`500` 服务异常。

------

### 6.5 静态文件下载

| 需求     | 说明                                                         |
| -------- | ------------------------------------------------------------ |
| **DR-1** | 支持 `Range` 请求、断点续传 (`Accept-Ranges: bytes`)         |
| **DR-2** | 设置 `ETag` 为 commitHash；`Cache-Control: public, max-age=86400` |
| **DR-3** | 文件由 Nginx 反向代理，使用 `X-Accel-Redirect` 或 `sendfile` 提供零拷贝传输 |
| **DR-4** | CDN 缓存：ZIP 文件内容哈希不变，TTL 可设 7 天                |

------

## 7. 非功能需求

### 7.1 性能

| 指标            | 目标值               |
| --------------- | -------------------- |
| API P99 延迟    | ≤ 50 ms              |
| 单 ZIP 下载吞吐 | ≥ 200 MB/s（局域网） |
| 并发下载连接    | 2 000 个 / 实例      |

### 7.2 可伸缩性

- **水平扩容**：无状态 API；静态文件放对象存储可选（OSS / S3）。
- **异步任务**：Repo Sync & PKG 通过工作池，Channel 大小可配置。

### 7.3 安全

| 分类         | 要求                                        |
| ------------ | ------------------------------------------- |
| **TLS**      | 强制 HTTPS；HSTS 1 年                       |
| **认证**     | 公共 API 可匿名访问；管理接口需 Token 鉴权  |
| **输入校验** | 所有路径参数正则白名单 (`^[a-zA-Z0-9_-]+$`) |
| **Zip Slip** | 打包写入时确保路径不含 `..`                 |
| **DDoS**     | 见 7.4                                      |

### 7.4 限流 & DDoS 防护

1. **应用层 (Go)**
	- `rate.Limiter` 按 IP 计数 (`X-Real-IP` / `X-Forwarded-For`)
	- 全局并发下载计数器 (`atomic.Int64`)，超限返回 503 Retry-After
2. **边缘层 (Nginx)**
	- `limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;`
	- `limit_conn_zone $binary_remote_addr zone=dl:10m; limit_conn dl 2;`
3. **网络层**
	- CDN / WAF：启用 L7 性能防护、流量清洗
	- 自建防护时配置 SYN Cookie、连接速率限制

------

## 8. 详细流程

### 8.1 定时同步 & 打包

```
Cron Tick
  └─► for repo in cfg.repos
         ├─► PullOrClone(repo)
         ├─► if updated
         │      └─► Enqueue(PackageJob(repo))
         └─► UpdateMeta()
WorkerPool
  └─► PackageJob
         ├─► Create temp zip
         ├─► Walk repo dir (skip .git etc.)
         ├─► Flush & Sync
         └─► Atomically move to ZDir
```

### 8.2 下载请求

```
Client
  └─► GET /api/v1/packages/dora-demo/latest
Server
  └─► 302 Location: /zips/dora-demo-a1b2c3d.zip
Nginx
  └─► X-Accel-Redirect -> /internal/zips/… (sendfile + rate_limit)
```

------

## 9. 数据结构

```go
type RepoMeta struct {
    Name       string    `json:"name"`
    URL        string    `json:"url"`
    Tag        string    `json:"tag"`
    LastSync   time.Time `json:"lastSync"`
    CommitHash string    `json:"commitHash"`
    ZipFile    string    `json:"zipFile"`   // 相对路径
    Size       int64     `json:"size"`
}
```

------

## 10. 监控 & 运维

| 指标                         | 说明         |
| ---------------------------- | ------------ |
| `repo_sync_duration_seconds` | 成功同步耗时 |
| `zip_build_fail_total`       | 打包失败计数 |
| `http_requests_total{code}`  | 按状态码统计 |
| `download_bytes_total`       | 下载流量     |
| `rate_limited_total`         | 被限流请求数 |

**告警规则**

- 5 分钟同步错误 > 3 次
- 下载 5xx 数 ≥ 1 %

------

## 11. 日志

- **格式**：JSON（`time`, `level`, `caller`, `msg`, `trace_id` …）
- **采样**：下载成功日志采样率 10 %
- **链路追踪**：OpenTelemetry + Jaeger（可选）

------

## 12. 部署

| 环节       | 建议                                                         |
| ---------- | ------------------------------------------------------------ |
| **容器化** | Dockerfile 多阶段构建；`scratch` 镜像降低体积                |
| **CI/CD**  | GitHub Actions：单元测试 → 构建 → 镜像推送 → Helm Chart      |
| **运行时** | Kubernetes / Docker Compose；Pod 水平自动扩缩(HPA)；挂载 PersistentVolume `/data` |

------

## 13. 风险 & 缓解

| 风险                    | 缓解措施                                      |
| ----------------------- | --------------------------------------------- |
| CDN 缓存不一致          | 文件名含 commitHash；更新文件名即缓存穿透     |
| 高并发导致磁盘 I/O 瓶颈 | SSD/NVMe；zip 文件落对象存储；只保留最新 N 份 |

------

## 14. 里程碑（示例）

| 时间         | 目标                           |
| ------------ | ------------------------------ |
| **T + 1 周** | 完成 API & 配置模块原型        |
| **T + 2 周** | 完成 Repo 同步 / 打包流程      |
| **T + 3 周** | 限流、防护、监控上线；E2E 测试 |
| **T + 4 周** | 部署生产，灰度发布             |

