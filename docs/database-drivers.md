# 数据库驱动决策 — M1-A

Main 统一修改 go.mod/go.sum；Worker 禁止 go get/升级/替换依赖。运行环境 Go 1.26.3 linux/amd64。

| DB | 固定 Go module/version | License | 选择依据 |
|---|---|---|---|
| SQLite | modernc.org/sqlite v1.60.1 | BSD-3-Clause | database/sql、纯 Go/无需 CGO，便于跨环境测试和后续单二进制发布，支持只读 URI |
| MySQL | github.com/go-sql-driver/mysql v1.10.1 | MPL-2.0 | database/sql 原生 Go 驱动，明确 DSN/超时/TLS/事务语义，避免自行实现协议 |

版本由 Go module proxy 实际查询并下载，不使用浮动 latest 构建。SQLite 模块要求 Go 1.26.0、MySQL 要求 Go 1.24.0，符合当前 Go 1.26.3。
`go mod verify` 已通过；校验哈希在 go.sum。所选 modernc libc 为 v1.77.1，保持模块解析出的兼容版本，禁止单独降级 libc。
已阅读并保留两个直接依赖的许可证：`docs/licenses/modernc-sqlite-LICENSE` 与 `docs/licenses/go-sql-driver-mysql-LICENSE`。

## 许可使用约束

MaskRiver 自有源码仍为 MIT，不能把依赖宣称成 MIT。
BSD-3-Clause 要求保留版权/条件/免责声明，二进制分发也要随带通知，不得借作者名背书。
MPL-2.0 是文件级义务：修改/分发受覆盖驱动文件须保留 MPL 与可获得源码方式；组合成独立 MIT Larger Work 不改变驱动文件许可。发布链接二进制时提供对应驱动源码版本获取说明和许可。
直接驱动源码可按固定 module tag 从 https://gitlab.com/cznic/sqlite 与 https://github.com/go-sql-driver/mysql 获取。
当前未 vendor 或修改依赖源码。传递依赖清单由 `go list -m all` 给出，发行前还需生成完整 third-party notices/SBOM、检查全部传递依赖许可与漏洞；本轮没有冒称完成发行合规或漏洞审计。

## SQLite 使用约束

- M1-B Reader 必须 mode=ro 打开已存在文件；missing file 失败，不能建库。
- test fixture 工具是专门写入路径，不代表 CLI 已支持 SQLite；输出限制 `.local/m1a-synthetic.db`，拒绝覆盖已有文件。
- 开启每连接 foreign_keys，初期单连接/串行批次，读游标关闭后再写，防止 SQLite reader/writer 自锁。
- SQLite 动态类型/NUMERIC affinity 可能损失 decimal；合成 fixture 用 TEXT 存储高精度 decimal。后续 adapter 不得把任意 NUMERIC 的 float64 声称为无损 Decimal。
- 日期/时区、bytes、NULL、复合主键、约束 metadata 必须专测；不支持的 declared/dynamic 类型返回 ErrUnsupported。

## MySQL 使用约束

- 目标测试服务器：本机可用 mysqld 8.0.46-0ubuntu0.24.04.4；MySQL 8.4/其他版本尚未验证，MariaDB 不算自动支持。
- 用 mysql.Config + NewConnector，避免字符串拼接 DSN；禁止日志输出 Config/DSN/原始 driver error。
- 禁止 multiStatements、allowAllFiles、LOCAL INFILE；启用连接/读写超时，配合 context 取消。网络连接必须明确 TLS 策略，禁止生产凭证。
- fixture 仅专用 Unix socket、全新私有数据目录、skip-networking/mysqlx=OFF；不接系统 socket、不读用户凭证、不管理系统服务。
- schema 用 utf8mb4_bin；Decimal 以字符串精确保留，零日期不能当合法 Date，DATETIME 与 TIMESTAMP 不混同。Keyset 用数据库的键排序规则。
- 单批 InnoDB 事务；DDL 会隐式提交，因此 schema 仅由环境工具在空实例创建。受影响行数/乐观比较/commit 不确定性须 adapter 测试。
- MySQL 驱动已下载且编译，但临时实例初始化遇权限阻塞，**未完成 MySQL 连接/集成验收**，见 test-environment.md。

当前 CLI 不导入 DB adapter；依赖仅供合成环境测试与后续组件使用。引入驱动不等于 SQLite/MySQL 业务支持。
