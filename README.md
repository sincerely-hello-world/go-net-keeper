# go-net-keeper

校园网登录维持命令行工具，支持定时检查、掉线认证和逐轮状态输出。

## 开发

`mise.toml` 锁定 Go **1.26.10**：

```sh
mise install
```

项目初始化命令（当前项目已完成，无需重复初始化）：

```sh
mise exec -- go mod init github.com/sincerely-hello-world/go-net-keeper
mise exec -- go mod tidy
```

本地开发账号密码放在 `.env`，由 mise 自动加载。首次配置可参考 `.env.example`；已有文件直接编辑。`.env` 已加入 Git 忽略规则，二进制本身不读取它。

启动与测试：

```sh
mise exec -- go run ./cmd/go-net-keeper
mise exec -- go run ./cmd/go-net-keeper --once
mise exec -- go test ./...
```

## 构建交付

构建当前平台的独立二进制：

```sh
CGO_ENABLED=0 mise exec -- go build -trimpath -o bin/go-net-keeper ./cmd/go-net-keeper
```

交付二进制和 `config.example.toml`，运行端无需安装 Go 或 mise。将配置示例复制为 `config.toml`，填写 `username`、`password`；`interval`、`timeout` 默认均为 `"10s"`。

在交付目录运行：

```sh
./go-net-keeper --config ./config.toml
./go-net-keeper --config ./config.toml --once
```

每轮输出账号、状态及在线 IP，按 Ctrl+C 退出。配置按字段采用 `config.toml > 环境变量 > 默认值`，显式命令行参数可临时调整间隔和超时。

本次交付为 Linux amd64，使用时需能访问校园网关 `10.10.9.9`。`.env` 和本地 `config.toml` 不随交付包分发。
