# chatroom-server

从 MallChat 抽取的群聊房间服务，Go 实现。群聊 + 单聊(私聊) + 热门群广播，消息类型：文本/表情/撤回/系统。

## 本地运行

```bash
docker compose up -d       # 起 MySQL + Redis
go run ./cmd/server        # 启动服务，默认监听 :8080
```

> 如果本机是 Docker Desktop for Mac，且 `go run`/`go test` 直连 `127.0.0.1:3306` 时报
> `Access denied for user 'xxx'@'localhost'`（哪怕账号密码确认无误），这是该环境端口转发的已知网络问题，
> 不是代码或凭据问题。绕过方法：把测试/程序放进一个连到 `docker-compose` 网络的容器里跑，用服务名
> `mysql`/`redis` 而不是 `127.0.0.1` 访问，例如：
> ```bash
> docker run --rm -v "$(pwd)":/app -w /app --network chatroom-server_default \
>   -e CHATROOM_TEST_MYSQL_DSN="chatroom:chatroom@tcp(mysql:3306)/chatroom?parseTime=true&charset=utf8mb4" \
>   -e CHATROOM_TEST_REDIS_ADDR="redis:6379" \
>   golang:1.26 go test ./... -v
> ```

## 测试

```bash
go test ./...                                    # 纯逻辑单测（角色权限/收件人解析/撤回权限/JWT/缓存），无需任何依赖
CHATROOM_TEST_MYSQL_DSN="chatroom:chatroom@tcp(127.0.0.1:3306)/chatroom?parseTime=true&charset=utf8mb4" go test ./... -v   # 加上DB集成测试
```

## 接口

- `POST /api/auth/register` / `POST /api/auth/login`
- `POST /api/rooms/groups`（建群）、`.../members`（拉人/查成员）、`.../members/{uid}`（踢人）、`.../members/me`（退群）、`.../admins/{uid}`（设/撤管理员）
- `POST /api/messages`（发消息，需要是房间参与者）、`PUT /api/messages/{id}/recall`（撤回）、`PUT /api/messages/{id}/mark`（标记）、`GET /api/rooms/{roomID}/messages`（分页）
- `GET /ws?token=<jwt>`（WebSocket实时推送）

## 架构要点

- 广播不走 MQ：发消息写库后直接在进程内 `ws.Hub`（内存 map+channel）广播给在线连接；未来要多实例横向扩容时再升级成 Redis Pub/Sub。
- 迁移通过 `go:embed` 打进二进制（`internal/db/migrations/*.sql`），不依赖运行时工作目录。
- 每层都有 Store 接口，handler 单测用内存 fake，不需要真实数据库；Store 本身的 SQL 正确性靠集成测试兜底（`CHATROOM_TEST_MYSQL_DSN` 未设置时自动跳过）。

## 未做的事

图片/语音/文件/视频消息、多实例横向扩容的跨实例广播（Redis Pub/Sub）。落地思路：消息只广播一个媒体引用(file_id)，实际字节走独立的 `/api/upload` + `/api/files/{id}`（Range支持），存储抽象成一个接口方便本地MinIO/生产S3切换。
