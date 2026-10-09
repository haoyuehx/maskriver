# M1-A 合成测试环境

Owner：Main。文件：`internal/testenv/**`、`tools/fixture/**`、`scripts/test_mysql.py`。
这些路径只做依赖/合成环境准备，不是 go-db 的生产连接实现。Worker 只读引用 helper，不能修改或复制另一套 fixtures。

## SQLite — 已验证

```sh
cd /home/haoyue/Project/maskriver
go test -count=1 -v ./internal/testenv -run '^TestSQLiteFixture$'
go run ./tools/fixture
```

第二条命令已实际生成 `.local/m1a-synthetic.db`（忽略上传、0600）；重跑拒绝覆盖，需 Main 明确决定是否另建/清理，不能把现有副本当空库 seed。
自动测试使用 t.TempDir，新建后自动清理；sqlite driver 已实际打开数据库并完成 SQL 读写验证。

- people：25 行完全虚构 email `fixture-NN@example.invalid`、中文合成标签；id 主键、email unique、born 索引、CHECK(id>0)。
- nickname 包含 NULL 与空字符串，二者分别验证；payload 含二进制 00/01/FF；合法闰日 2000-02-29。
- amount 测试 `12345678901234567890.1200`：SQLite 为 TEXT 防止 NUMERIC affinity 舍入；MySQL 为 DECIMAL(24,4)。
- links：25 行、(tenant,seq) 有序复合主键、people 外键，足够 page size 10 测三页。
- keyless：1 行，只用于无主键限制测试。
- 已测 count、NULL/empty、decimal/blob、FK 拒绝、事务 rollback、取消；SQLite 额外验证 mode=ro 写入失败与不存在文件不创建。

没有复制任何第三方数据、配置或敏感样本；本阶段不提供真实身份证、电话号码等可能碰撞真实身份的 fixture。

## MySQL — 准备脚本完成，实例受阻，未通过验收

```sh
cd /home/haoyue/Project/maskriver
python3 scripts/test_mysql.py
```

脚本只接受本机 mysqld/mysqladmin；--no-defaults 不读取环境中的真实数据库配置，创建唯一 `.local/mysql-m1a-*` 0700 目录。
初始化专用 datadir，然后 --skip-networking --mysqlx=OFF，只暴露目录内 socket；禁 LOCAL INFILE/服务器文件导出。无 TCP 端口，不访问宿主系统数据库或服务。
初始化用 `--initialize-insecure` 的空口令仅限新建、禁网、私有 socket 实例；不是生产凭证，不能用于共享服务或放宽目录权限。测试不接受任意 DSN/口令；该操作仅适用于可信同一 OS 用户的测试环境。
实例 ready 后运行 opt-in TestMySQLFixture，测试读取私有 marker 校验路径；建立全新 maskriver_fixture schema（不用 IF NOT EXISTS），生成同一批合成数据；finally 停止自己创建的进程，不根据外部 PID 文件终止其他进程。
脚本不安装软件、不 sudo、不修改 AppArmor/系统配置、不后台遗留服务；诊断文件和 datadir 留在 `.local/`，由 Main 在确认无存活进程后管理。

实际检查结果：
- Docker CLI 27.1.2 可用，但 daemon 报 `Cannot connect to the Docker daemon at unix:///var/run/docker.sock`；未擅自启动系统 daemon。
- 本机 mysqld 为 8.0.46-0ubuntu0.24.04.4。
- 第一轮预建 data 目录时报 OS errno 17；改为让 mysqld 初始化创建全新目录后，明确返回 `OS errno 13 - Permission denied`。
- 日志在 `.local/mysql-m1a-xl4a8w_c/initialize.log` 与 `.local/mysql-m1a-mawus4or/initialize.log`。没有启动成功的服务，没有修改生产数据库。
- 不推定根因已确认；可能涉及宿主安全策略，但本轮不调整策略、不提权、不把数据挪出授权目录来绕过限制。
- 默认 `go test ./...` 的 MySQL 测试明确 SKIP，不算 MySQL 通过。

当前门禁调整：M1-B 首轮只要求真实 SQLite 可用，MySQL 可 UNVERIFIED/SKIP，但保留实现与全部真实集成测试要求；最终 M1 验收前必须补齐真实 MySQL。不能用 SQLite/mock 替代 MySQL 验收。

### Permission denied 的准确环节与非特权方案

失败发生在 `scripts/test_mysql.py` 的第一段 `mysqld --no-defaults --datadir=<全新目录> --initialize-insecure`，服务启动、socket readiness、go test 尚未发生。最终错误 `Can't create directory .../data/ (OS errno 13 - Permission denied)`，不是驱动认证失败，也不是 SQL 用例失败。原始日志保留在上述忽略目录；不把初次 errno 17 当最终根因，未证实具体宿主 confinement 规则。

可由操作者选择的非特权环境方案（本轮不安装/执行）：
1. 已可用的 rootless Podman/Docker，在用户 namespace 中运行固定 digest MySQL 镜像，独立 disposable volume、合成数据；使用容器内 Unix socket 执行测试或限制为私有网络，禁止连已有实例。host 端不 sudo、不改系统 daemon。
2. 提供已有授权的非生产 CI ephemeral MySQL service，每次独立库/账号与合成 fixture，凭证仅运行时注入；新增测试入口必须由 Main 安全审核，现有本机测试拒绝任意 DSN。
3. 操作者提供可在用户目录运行且经许可/校验的 MySQL 用户态发行包；Main 仅在授权私有目录初始化、禁用网络，不通过复制系统二进制来绕过宿主安全策略。
环境可用后记录服务版本、driver 版本、真实测试日志、清理证据，再解除最终 M1 门禁。
