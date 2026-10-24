export const GITHUB = 'https://github.com/haoyuehx/maskriver';
export const VERSION = 'v1.0.1';
export const RELEASE = `https://github.com/haoyuehx/maskriver/releases/tag/${VERSION}`;
export const DOWNLOAD = `https://github.com/haoyuehx/maskriver/releases/download/${VERSION}/maskriver-${VERSION}-linux-amd64.tar.gz`;
export const CHECKSUMS = `https://github.com/haoyuehx/maskriver/releases/download/${VERSION}/SHA256SUMS`;
export const DOCS = 'https://github.com/haoyuehx/maskriver/tree/main/docs/guides';
export const QUICKSTART =
  'https://github.com/haoyuehx/maskriver/blob/main/docs/guides/quickstart.md';
export const SQLITE_DOC =
  'https://github.com/haoyuehx/maskriver/blob/main/docs/guides/sqlite.md';
export const MYSQL_DOC =
  'https://github.com/haoyuehx/maskriver/blob/main/docs/guides/mysql.md';
export const SECURITY_DOC =
  'https://github.com/haoyuehx/maskriver/blob/main/docs/guides/security-model.md';
const mediaBase = `${import.meta.env.BASE_URL}media/`;
export const VIDEO_URL = import.meta.env.VITE_VIDEO_URL ?? `${mediaBase}maskriver-demo.mp4`;
export const POSTER_URL = import.meta.env.VITE_POSTER_URL ?? `${mediaBase}poster.png`;
export const CAPTIONS_URL =
  import.meta.env.VITE_CAPTIONS_URL ?? `${mediaBase}maskriver-demo.zh.vtt`;

export const nav = [
  { href: '#top', label: '首页' },
  { href: '#demo', label: '演示视频' },
  { href: '#features', label: '核心功能' },
  { href: '#architecture', label: '技术架构' },
  { href: '#start', label: '快速开始' },
];

export const steps = [
  {
    id: 'scan',
    label: 'Scan',
    title: '识别敏感字段',
    color: 'cyan',
    summary: '基于字段上下文与格式规则抽样识别，证据不足时保留 UNKNOWN，而不是把它当作安全。',
    facts: ['email → 敏感', 'mobile → 敏感', 'nickname / address → 需人工核对'],
  },
  {
    id: 'preview',
    label: 'Preview',
    title: '只读预览计划',
    color: 'purple',
    summary: '明确要处理的表、字段、策略与键集；预览阶段不写入目标副本。',
    facts: ['表：customers', '字段：email、mobile', '目标：尚未写入'],
  },
  {
    id: 'approve',
    label: 'Approve',
    title: '签名审批绑定计划',
    color: 'purple',
    summary: '审批记录由 Ed25519 签名并绑定计划摘要；执行进程只读取部署信任文件，不接触审核私钥。',
    facts: ['Plan → 签名记录', '未获授权：拒绝写入', '过期 / 撤销 / 跨计划：拒绝'],
  },
  {
    id: 'apply',
    label: 'Apply',
    title: '按批次执行脱敏',
    color: 'teal',
    summary: '只有显式 --apply 与有效审批同时存在才进入写入路径；只修改计划字段并做乐观并发检查。',
    facts: ['计划字段：替换', '非计划字段：保持不变', '批次事务：逐个提交'],
  },
  {
    id: 'validate',
    label: 'Validate',
    title: '验证结构与结果',
    color: 'teal',
    summary: '严格模式执行全量扫描，核对行数、Schema、主键集合与计划字段，并调用适配器完整性检查。',
    facts: ['行数与键集：核对', 'Schema 与字段：核对', '覆盖不足：不得 PASS'],
  },
] as const;

export const features = [
  {
    icon: 'scan',
    title: '敏感信息识别',
    text: '字段上下文与格式规则结合，支持 Email、手机号、身份证、银行卡、USCC、日期等规则；UNKNOWN 不会被当作安全结论。',
    tag: '13 类规则',
    span: 'wide',
  },
  {
    icon: 'shield',
    title: '安全审批',
    text: 'Ed25519 签名审批与精确计划绑定。执行只读取部署信任文件，不接收审核私钥。',
    tag: 'Plan-bound',
    span: 'normal',
  },
  {
    icon: 'wand',
    title: '确定性脱敏策略',
    text: '支持格式约束替换、确定性 HMAC 策略与多种字段类型。',
    tag: 'Deterministic',
    span: 'normal',
  },
  {
    icon: 'check',
    title: '严格验证',
    text: '双向键集、行数、Schema 与完整性检查；SQLite quick/FK 与 MySQL CHECK TABLE + 可见 FK 检查，失败即 fail-closed。',
    tag: 'Fail closed',
    span: 'normal',
  },
  {
    icon: 'stream',
    title: '有界流式处理',
    text: '面向较大数据规模的分批读取、批次事务、资源预算与事务 Outbox 回执。',
    tag: 'SQLite only',
    span: 'wide',
  },
  {
    icon: 'recovery',
    title: '检查点与恢复',
    text: '中断后读取 Checkpoint 与回执，跳过已正确提交的批次并继续未完成部分；冲突时拒绝覆盖。',
    tag: 'Linux · SQLite',
    span: 'normal',
  },
] as const;

export const useCases = [
  {
    title: '开发测试数据准备',
    text: '从授权的隔离副本生成格式合理的测试数据，减少在开发和测试环境中传播真实敏感值。',
    icon: 'database',
  },
  {
    title: '隔离测试库脱敏',
    text: '在 SQLite 或 MySQL 8.0.46 的独立目标副本上执行计划，源副本保持只读。',
    icon: 'layers',
  },
  {
    title: '数据治理流程验证',
    text: '用签名审批、只读预览与严格验证串起可审计的处理步骤，验证团队的数据治理流程。',
    icon: 'clipboard',
  },
  {
    title: '授权数据处理实验',
    text: '在明确授权、隔离、可丢弃的环境中评估检测规则、脱敏策略和恢复流程。',
    icon: 'flask',
  },
] as const;

export const faqs = [
  {
    q: 'MaskRiver 可以直接处理生产数据库吗？',
    a: '不应直接用于生产数据库。当前定位是授权的、隔离的、可丢弃的副本；默认只读预览，写入必须显式 --apply 并通过与计划绑定的审批。生产使用还需要独立的权限、备份、审计与运维控制。',
  },
  {
    q: '是否必须审批才能写入？',
    a: '是。mask --apply 还需要 --approved、部署信任文件、历史目录，以及一条由 approve 命令签发并绑定当前计划的 Ed25519 记录。仅有 --approved 或仅有 --apply 都不构成授权。',
  },
  {
    q: '支持哪些数据库？',
    a: 'SQLite 支持完整 M1 流程，并在 Linux amd64 支持 SQLite-only streaming/outbox/checkpoint/recovery。MySQL 8.0.46 的 Scan → Preview → Approve → Apply → Validate CLI 已通过隔离 CI；其他 MySQL 版本未验证。',
  },
  {
    q: '哪些场景支持恢复？',
    a: '仅 Linux amd64、rollback journal、synchronous=FULL 的本地 SQLite，并且只覆盖确定性策略。MySQL recovery、Windows/macOS streaming/recovery、WAL recovery 都不支持。',
  },
  {
    q: '数据脱敏是否等于匿名化？',
    a: '两者不等同。脱敏通过替换降低敏感数据暴露，并尽量保持字段格式；匿名化还需要评估不可关联性，并结合具体法规与数据用途判断。',
  },
  {
    q: '会收集我的数据或使用情况吗？',
    a: '这个展示网站是纯静态页面，不包含分析、追踪或真实脱敏 API。视频和页面资源只从站点自身加载；GitHub 链接会跳转到公开仓库。',
  },
] as const;
