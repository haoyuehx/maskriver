# 跨平台合成测试环境

Owner：Main / 测试环境协调。范围：`internal/testenv/**`、`tools/fixture/**`、`scripts/test_mysql*.py`、`.github/workflows/**`。模块成员复用 helper，不自行修改共享环境或复制 fixture。

## Windows、Linux、macOS 默认入口

在自己的克隆根目录执行，Go 版本见 `go.mod`：

```sh
go test -count=1 -v ./...
go vet ./...
go test -count=1 -v ./internal/testenv -run '^TestSQLiteFixture$'
```

SQLite 用 `t.TempDir` 新建、关闭并清理数据库，不依赖原机器缓存。`go run ./tools/fixture` 可生成忽略上传的 `.local/m1a-synthetic.db`，拒绝覆盖已有文件；不要求开发者运行该手工工具。

合成 fixture 包括：25 行虚构 email `fixture-NN@example.invalid`、中文标签、NULL/空字符串、二进制、闰日；SQLite TEXT / MySQL DECIMAL(24,4) 保存精度；复合键、外键、unique、CHECK、索引和无键表。测试 count、NULL/empty、decimal/blob、FK、回滚、取消；SQLite 额外验证只读拒写和不存在文件不创建。

#6 go-detect 和 #7 go-mask 可纯内存测试，#8 使用 fake Reader；均不需要 MySQL。fake 或 fixture 测试不等于业务适配器/端到端测试。

## MySQL：优先一次性容器或 CI

Ubuntu GitHub Actions 的 `mysql-fixture` job 执行：

```sh
python3 scripts/test_mysql_container.py
```

已有获授权 Docker daemon 的 Linux 开发机可运行同一命令；Windows/macOS 成员可使用 PR 的 Ubuntu CI，不要求更改本机系统服务。无 Docker/不支持 socket 时入口返回非零并明确 SKIP/UNVERIFIED，CI 不会把它当成功。

容器使用 `mysql:8.0.46`，输出实际 RepoDigest 与测试中的服务器版本；驱动版本由 `go.mod` 锁定。每次新建唯一容器和 `.local/mysql-m1a-*` 0700 目录，使用当前用户 UID/GID，无网络、无宿主端口、无命名数据卷。自行初始化空 datadir，只暴露私有 Unix socket，禁用 LOCAL INFILE 和服务端文件导出。空口令 root 仅适用于该新建禁网私有实例，不接受任意 DSN、外部数据库或生产凭证。

复用 `TestMySQLFixture` 的 marker、目录权限和 socket guard；启动失败/超时、SQL 失败均报失败。finally 删除本次唯一容器及其匿名卷；成功后删除合成目录，失败诊断仅留在忽略的 `.local`，不上传数据文件。取消 CI 时 runner 为一次性环境；本机强制中断后由设备持有人确认残留资源，不自动处理其他容器。

当前容器入口只执行已有真实 MySQL **fixture**。#5/#10 必须继续交付并接入只读、元数据、复合键分页、乐观冲突、回滚、取消、DECIMAL/DATE/BLOB 等适配器契约测试，最终 M1 前全部真实运行。不得以 fixture、Mock、编译或 SKIP 冒充 MySQL 产品支持。

## 可选 POSIX 用户态 mysqld

`python3 scripts/test_mysql.py` 可在已获授权的本机 mysqld 环境创建禁网私有 socket 实例，不 sudo、不修改安全策略或系统服务。旧设备曾在初始化 datadir 时权限失败，该记录不约束其他机器；遇到本机限制使用 CI，不绕过宿主策略。

## PR 验收证据

记录 OS、Go/driver/server 版本、实际镜像 digest、精确命令、测试输出、清理结果和未验证项；不提交凭证、数据库或原始敏感数据。首轮 SQLite 必须真实通过，MySQL 可标记未验证支持其他模块继续；最终 M1 仍必须通过完整真实 MySQL 集成。
