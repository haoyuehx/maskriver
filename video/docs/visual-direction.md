# MaskRiver 产品概念片 · 视觉与动效规格（Visual Direction）

- 媒介：Remotion 4.0.427 / React 19 / TypeScript，1920×1080，30fps，共 7800 帧（260 秒），H.264 MP4。
- 渲染机：2 CPU 核、无 GPU、SwiftShader 软件光栅，实测约 4 帧/秒。**本文件所有设计决定都以此为第一约束**：优先 flat SVG stroke、linear/radial gradient、opacity、transform；禁用大面积 blur / filter / box-shadow 叠加 / WebGL。
- 字体：仅 Noto Sans SC（400/500/700）与 JetBrains Mono（400/500/700），经 @fontsource 加载（见 `video/src/styles/fonts.ts`）。
- 底部固定字幕带为烧录简体中文字幕预留，任何画面不得侵入（见 §4、§5）。
- 起点基准：`video/src/scenes/EnvTest.tsx`（深底 + 单个 radial gradient + SVG 圆环 dash + 中文大标题）已渲染成功，全片沿用该质感。
- 本文件是唯一视觉规范源。`video/src/styles/tokens.ts` 已含 COLORS / FONT / TYPE / FRAME / SPRING / DURATION / SCENE_START，工程实现时按本文件 §1 补齐 SPACING / RADIUS / STROKE / OPACITY 等 token，不得另造数值。

**事实红线（任何画面不得违反）**

1. 五步工作流只有一种表述：scan → mask（dry-run 预览）→ approve（Ed25519 审批记录绑定计划）→ apply → validate。apply 需 `--approved` + `--apply` + 有效签名记录三者同时成立；strict validate = 全量扫描 + 适配器完整性检查。
2. 有界流式、Outbox 回执、checkpoint/resume 仅限 SQLite（本地副本）；不得出现 MySQL 恢复、全局 exactly-once 的表述。
3. 脱敏确定性 ≠ 匿名化、≠ 无碰撞，出现"确定性"字样时必须伴随该边界说明（小字注解即可）。
4. 画面中所有数据均为合成数据；出现产品界面/工作区/数据表的帧，必须显示角标「流程示意 · 合成数据」。
5. 不出现：版本号、里程碑/阶段代号（如 M1/m2-…-v1 之类）、Issue/PR 编号、CI 日志；不得把界面表述为已实现的 Web 管理后台。
6. 屏上 CLI 命令只使用真实存在的形态：`maskriver scan --source … --target … --scope …`、`maskriver mask … --strategy fake_email …`、`maskriver mask --streaming --apply --approved …`、`maskriver status --target … --run-id …`、`maskriver mask --streaming --apply --resume --run-id …`。`--run-id` 一律显示为 `<32-hex>` 占位，不编造具体值。

---

## 1. Design Tokens

与 `video/src/styles/tokens.ts` 对齐；标 ★ 的为本文件新增、需补进 tokens.ts 的 token。

### 颜色（沿用 tokens.ts 的 COLORS，不得新增色值）

| Token | 值 | 用途 |
|---|---|---|
| `bgDeep` | `#020812` | 最底层背景、字幕带加深、片头片尾黑场过渡色 |
| `bg` | `#06111f` | 场景主背景 |
| `bgSoft` | `#0a1a2a` | 次级背景层、区块分组底色 |
| `panel` | `#0b1b2c` | 面板/卡片底色 |
| `panel2` | `#0e2235` | 面板渐变端点、hover/激活面板 |
| `line` | `#1d3d55` | 1px 分隔线、表格行线 |
| `lineBright` | `#2b5874` | 强调描边、网格线、图标底描 |
| `text` | `#ecf8ff` | 主文本 |
| `muted` | `#93aec1` | 次级文本、正文 |
| `muted2` | `#6f8ca1` | 注解/免责小字（信息性文字的最低亮度档） |
| `cyan` | `#63dcf3` | 主强调：流程主色、进度、链接 |
| `cyanDeep` | `#1e8fb0` | cyan 的描边/渐变深端 |
| `teal` | `#58e0ba` | 安全语义：已脱敏、已验证、PASS |
| `purple` | `#b49cff` | 审批与恢复域：Ed25519 记录、resume 高亮 |
| `amber` | `#ff996a` | 警示语义：源数据敏感值、中断提示 |
| `danger` | `#ff7b6b` | 失败/拒绝语义：fail-closed 瞬间 |
| `dangerDeep` | `#a8321f` | danger 渐变深端，仅故障条使用 |

### 间距（8px 基准）★

| Token | 值 (px) | 用途 |
|---|---|---|
| `sp-1` | 8 | 图标与文字内距、chip 内 padding |
| `sp-2` | 16 | 卡片内边距（紧凑）、表行内距 |
| `sp-3` | 24 | 标准卡片内边距、网格 gutter |
| `sp-4` | 32 | 面板内边距、大卡片间距 |
| `sp-5` | 40 | 分区上下间距 |
| `sp-6` | 48 | 场景标题与舞台区间距 |
| `sp-8` | 64 | 章节切换留白 |
| `sp-10` | 80 | 舞台区垂直呼吸 |
| `sp-12` | 96 | 大型画面元素上下留白 |
| `sp-16` | 128 | 特写画面外框留白 |

### 圆角 ★

| Token | 值 (px) | 用途 |
|---|---|---|
| `r-sm` | 8 | verdict chip、命令行标签 |
| `r-md` | 12 | 数据表、内嵌卡片 |
| `r-lg` | 16 | 标准面板、步骤节点 |
| `r-xl` | 24 | 全屏工作区外框、大表格画面 |
| `r-pill` | 999 | eyebrow 胶囊、范围 chip |

### 描边宽度（SVG stroke）★

| Token | 值 (px) | 用途 |
|---|---|---|
| `sw-hairline` | 1 | 分隔线、网格、表格行线 |
| `sw-line` | 1.5 | 普通连接线、面板边框 |
| `sw-emph` | 2 | 数据流路径、强调线 |
| `sw-icon` | 3 | 线性图标 stroke |
| `sw-ring` | 4 | 进度环、logo 波浪线 |

### 透明度档位 ★

| Token | 值 | 用途 |
|---|---|---|
| `o-grid` | 0.15 | 背景网格线 |
| `o-dim` | 0.55 | 弱化层、故障遮罩目标值 |
| `o-soft` | 0.8 | 次要但需可读的元素 |
| `o-full` | 1 | 全亮度 |
| `o-sub` | 0.92 | 字幕文字 |

### 面板与网格处理 ★

- 面板：`linear-gradient(145deg, panel2 0%, panel 100%)`，边框 `1px line`，顶部 1px 内高光 `rgba(255,255,255,0.06)`（单条 inset 线，禁止 box-shadow 叠加），圆角 `r-lg`。
- 激活面板：边框换 `lineBright`，左缘 2px 主色竖线。
- 背景网格：80px 间距，`sw-hairline` 的 `lineBright` 线，透明度 `o-grid`，带静态垂直渐隐 mask（上 100% → 中 0% → 底 60%），每场景最多一份。
- 场景氛围：每场景最多 2 个大型静态 radial gradient（如 `radial-gradient(ellipse 60% 45% at 50% 42%, #12374f 0%, transparent 65%)`，即 EnvTest 用法），必须静止，不得逐帧动画。

---

## 2. Color Palette

语义角色与出场规则（hex 见 §1 表）：

| 语义 | 色 | 允许出现的场合 | 禁止 |
|---|---|---|---|
| 背景层 | `bgDeep/bg/bgSoft` | 全片底色分层 | — |
| 面板/线 | `panel/panel2/line/lineBright` | 所有容器与结构线 | 不得当文本色 |
| 主文本 | `text` | 标题、关键值 | — |
| 次文本 | `muted/muted2` | 正文、注解；`muted2` 是信息文字最低档 | 免责/边界说明不得低于 muted2 |
| 主青 primary cyan | `cyan` | 流程推进、进度、扫描活动、章节高亮、转场扫掠线 | 不用于"已完成"终态 |
| 安全青绿 safety teal | `teal` | 已脱敏值、已提交批次、校验 PASS、恢复完成 | 不用于警示 |
| 次紫 secondary purple | `purple` | 审批域（Ed25519 记录卡）、resume 中"当前批次"高亮、原子单元标注 | 不与 teal 同时表达同一状态 |
| 警示琥珀 warning amber | `amber` | 源库未脱敏敏感值、中断事件条、批注"批次事务 ≠ 整库原子性"类边界提示 | 不用于产品 UI 框架色 |
| 危险 danger | `danger`（配 `dangerDeep` 渐变） | 仅 fail-closed 瞬间：中断闪条、拒绝态；持续时间 ≤ 90 帧 | 不得用于常规按钮/面板/装饰 |

统一规则：同一画面同时出现的强调色 ≤ 3 种（cyan + 最多两种语义色）；`danger` 出现时它是画面唯一高饱和暖色。所有强调色填充都配 `12–15%` 透明度同色底（如 `#63dcf315`），不用 glow/shadow 制造强调。

---

## 3. Typography

字体族只有两支：`SANS = 'Noto Sans SC'`（界面与中文），`MONO = 'JetBrains Mono'`（代码、数据值、英文标签、数字），见 tokens.ts 的 FONT。

| 层级 | 字号 (px) | 字重 | 行高 | 字距 | 每行上限 | 用途 |
|---|---|---|---|---|---|---|
| hero | 92 | Sans 700 | 1.15 | 0.02em | 10 个汉字 | S1 品牌名、S6 收束大字 |
| scene title | 64 | Sans 700 | 1.2 | 0.02em | 16 个汉字 | 各场景主标题 |
| section label（eyebrow） | 22 | Mono 500 | 1.2 | 0.18em（大写） | 26 个拉丁字符 | 左上角场景眉标，如 `STREAMING` |
| lead | 30 | Sans 400 | 1.6 | 0.01em | 28 个汉字 | 标题下一行的导语 |
| body | 24 | Sans 400/500 | 1.7 | 0.01em | 32 个汉字 | 说明正文、批注块 |
| data table | 20 | Mono 400 | 1.4 | 0 | 单元格 18 字符 | 表格单元格、verdict chip |
| mono code | 22 | Mono 400 | 1.6 | 0 | 52 字符/行 | CommandCard 命令行 |
| numeric stat | 56 | Mono 700 | 1.1 | 0 | 8 位数字 | 统计数值（如 `3/6`、字段数） |
| annotation | 20 | Sans 400 | 1.6 | 0.01em | 36 个汉字 | 边界说明/免责小字（muted2） |
| subtitle | 32 | Sans 500 | 1.5 | 0.02em | 40 个汉字 × 最多 2 行 | 底部字幕带烧录字幕 |

**CJK–Latin 混排规则**（由共享组件 `MixedText` 统一执行，禁止手工裸写）：

1. 拉丁字母串、数字、代码 token 一律用 `MONO` 渲染；纯中文用 `SANS`。
2. 汉字与拉丁/数字 token 之间插入 `0.25em` 间距（对拉丁 run 加 `margin: 0 .25em`）。
3. 中文语境用全角标点；mono run 内用半角标点；mono run 与中文之间不加多余标点。
4. 英文术语保持原文（Dry Run、Ed25519、Checkpoint、Outbox、SQLite），不翻译、不混写大小写；眉标一律大写。

---

## 4. Layout Grid

1920×1080，内容水平边距 `marginX = 132`，内容宽 1656px。

- **12 列网格**：列宽 116px × 12 + gutter 24px × 11 = 1656px。左右 margin 各 132px。
- **命名区域**（y 为自画面顶部坐标）：

| 区域 | 矩形 (x, y, w, h) | 用途 |
|---|---|---|
| Eyebrow 眉标区 | (132, 84, 560, 32) | 左上角 Mono 大写场景标签，配 6px cyan 状态点 |
| 免责角标区 | (1452, 84, 336, 36) | 右上角「流程示意 · 合成数据」chip |
| 标题区 | (132, 150, 1656, 130) | 场景主标题 + 导语 |
| 舞台区（stage） | (132, 320, 1656, 560) | 所有主视觉（表格、流水线、时间轴、工作区） |
| 指标区 | (132, 820, 1656, 80) | StatCard 行、进度摘要 |
| 字幕带 | (0, 912, 1920, 168) | 烧录字幕专用，画面内容不得低于 y=912 |

- **眉标规格**：y=84 起，Mono 500、22px、letter-spacing 0.18em、大写英文 + `·` + 中文短词（如 `WORKFLOW · 五步流程`），前置 6px 圆点用当场景主语义色。
- **字幕带几何**：顶边 y=912；字幕文本块水平居中，最大宽 1400px（x 260–1660）；字号 32px、行高 48px、最多 2 行（总高 96px），文本块置于 y 936–1032；底部安全余量 48px（y=1032 以下不放任何东西）。字幕带底部加静态线性遮罩 `linear-gradient(180deg, rgba(2,8,18,0) 0%, rgba(2,8,18,0.85) 100%)`（仅限带内，912–1080），保证字幕对比度。字幕文字 `text` 色、透明度 `o-sub`。
- 场景切换时眉标淡出/淡入（12 帧），不位移；字幕带永远不动。

---

## 5. Safe Area

- **内容安全框**：x ∈ [48, 1872]，y ∈ [48, 1032]。任何可读文字、数据、可辨识图形必须在框内。
- **字幕带**：y ∈ [912, 1032] 为字幕专属；叙事画面内容下边界为 y=912（即 tokens.ts 的 `contentBottom`）。
- **外围 48px**（四边）为禁放区：不放置任何重要信息；允许出现：背景网格、渐变边缘、装饰性 overflow 的舞台元素裁切。
- 实现上所有场景根组件包一层 `SafeFrame`（padding 48px 的校验容器，仅在开发模式绘制红色参考线，正式渲染不绘制）。

---

## 6. Shot Types（镜头原型目录）

| # | 原型 | 构图规则 | 使用场景 |
|---|---|---|---|
| SH-01 | 数据库全景 full-frame panorama | 合成数据表占舞台区 (132, 320, 1656, 560)，8–10 行，敏感列 amber 高亮；镜头仅横向缓移 ≤ 100px | S1-B3、S3-CH1 |
| SH-02 | 数据流追踪 data-flow tracking | 左源右汇（或上源下汇），2 条 SVG 贝塞尔路径（每条 ≤ 40 命令），6 个圆点 token 沿路径位移；源点 pulse 6 帧先行 | S1-B5、S2 各步骤间、S6 |
| SH-03 | 字段特写 field close-up | 相机 push 至 1.12 聚焦单行/单列，行卡 720×420 居左，右侧 880px 批注列；特写持续 ≤ 120 帧 | S1-B4、S2-B3、S3-CH2 |
| SH-04 | 产品工作区 product workspace | 1320×660 窗口 (300, 230)，内部左栏 320px 列表 + 右侧 952px 主区；右上角必须有免责角标 | S3 全场、S6 回顾 |
| SH-05 | 前后对比分屏 before/after split | 两张等宽表格各 640×420 并排（gap 24px），中间 56px 箭头列；左 amber 系（源/遮蔽）、右 teal 系（脱敏后） | S2-B4、S3-CH3、S3-CH5 |
| SH-06 | 流程俯视 pipeline top-down | N 个节点卡水平排布（宽 = (1656−(N−1)×24)/N），节点间 24px 连接线 + 流动 token；节点内含 3px 有界 meter 条 | S2-B2、S3 章节轨、S4 全场 |
| SH-07 | 检查点时间轴 checkpoint timeline | 6 个批次格 256×140（间距 24px）横排于 y 400–540；下方 y 600–880 放状态卡/命令卡；格子状态 = 灰(待) / teal(已提交) / purple(进行中) | S4-B4、S5 全场 |
| SH-08 | 故障事件 fault event | 全场静止 4 帧 → 遮罩升至 `o-dim` → 顶部 y 320 处 8px `danger`→`dangerDeep` 横条 scaleX 0→1；其余元素冻结，只有遮罩与横条动 | S5-B4 |
| SH-09 | 恢复 recovery | 已提交格逐个 ring sweep 复核（12 帧/格），当前格 purple 高亮，数据 token 从断点继续右移 | S5-B5 |
| SH-10 | Logo 收束 logo lockup | 320px 环形 mark（EnvTest 圆环 + Logo.tsx 波浪线）居中 (960, 460)，下方 wordmark 92px；径向氛围 gradient 1 个 | S1-B6、S6-B3 |

使用规则：每场景同时只允许一种"大"镜头原型（占舞台区的那种）；SH-03 特写通过相机 push 从全景/工作区自然进入，不硬切。

---

## 7. Motion Principles

**Spring 配置**（已在 tokens.ts SPRING，全片唯一来源）：

| 预设 | config | 用途 |
|---|---|---|
| `soft` | `{damping: 200, mass: 0.9, stiffness: 110}` | 相机 push/pull（几乎无回弹，60–90 帧内完成） |
| `settle` | `{damping: 32, mass: 1, stiffness: 92}` | 大型面板、wordmark 进场 |
| `snappy` | `{damping: 26, mass: 0.7, stiffness: 150}` | 卡片、chip、命令行元素 |
| `crisp` | `{damping: 20, mass: 0.6, stiffness: 190}` | 微 UI：锁扣、check、verdict chip |

**时长（帧，30fps）**：

| 动作 | 帧数 |
|---|---|
| 微 UI 进场 | 12–15 |
| 文本行进场 | 15–18 |
| 卡片/面板进场 | 18–24 |
| 大型画面（表格全景、工作区） | 24–30 |
| 退场 | 12 |
| 相机 push / pull | 60–90 |
| 数据 token 单程流动 | 40–90 |
| 静置 hold（可读信息） | ≥ 45 |

**Stagger 规则**：同级元素间隔 6 帧；一条 stagger 链 ≤ 5 个元素（总窗口 ≤ 30 帧）；checklist 类（校验项、批次格复核）用 12 帧间隔。

**进场/退场三元组**（只动 transform + opacity）：

- 卡片进场：`opacity 0→1`、`translateY 24→0`、`scale 0.96→1`（spring `settle`/`snappy`）。
- 文本进场：`opacity 0→1`、`translateY 24→0`（从下）或 `-16→0`（从上）；**文本不做 scale**（避免整段文字逐帧重光栅）。
- 退场：`opacity 1→0`、`translateY 0→-16`，scale 不变。
- 相机：push 范围 `1.0 → 1.12`（最大），pan 位移 ≤ 80px，`rotate` 永远为 0；相机是全场景唯一的外层 transform 包装（`CameraRig`）。

**「同一时刻最多一个大动作」**：任意 30 帧窗口内，只允许一个元素产生 > 200px 位移或 > 0.05 scale 变化或相机移动；其余元素只能做 opacity 与 ≤ 8px 微位移。相机移动期间，新面板只允许 fade + 8px 内微移进场。

**因果性表达**（动效即逻辑）：每个"A → B"因果链按固定四段：

1. 源 pulse：源元素 scale 1→1.06→1（6 帧，`crisp`）；
2. 传输：token（6px 圆点）沿路径位移 24–60 帧，或路径 stroke-dash 揭示；
3. 汇应答：目标元素在 token 到达前 6 帧开始进场 spring；
4. 状态落定：目标 verdict/状态色切换在到位后 6 帧内完成。

方向语义：成功路径一律从左到右、或从上到下（与阅读方向、流程方向一致）；**只有 fail-closed / 拒绝才允许从右到左的运动**（如 gate 锁未通过时 token 弹回）。观众不需要字幕也应能从运动方向读出"数据从源流向目标"。

---

## 8. Transition Specifications

| 边界 | 窗口（绝对帧） | 类型 | 参数 |
|---|---|---|---|
| 开场 | 0–30 | 黑场淡入 | 全画面从 `#000` opacity 0→1，linear；S1 内容从第 30 帧起才进场 |
| S1→S2 | 735–765（30 帧） | **① 线性扫掠 wipe** | 4px 宽 `cyan` 竖线 x 从 −8 → 1928（linear）；线左侧显示 S2（用宽度 = lineX 的 clip 矩形揭示），线右侧仍是 S1；S1 元素 735–750 opacity 1→0；S2 眉标在 765 后进场 |
| S2→S3 | 1925–1975（50 帧） | **② 匹配剪辑 match-cut** | S2 五步链（y 360 起）位移 translateY 0→−148、scale 1→0.9（spring `soft`），落位为 S3 顶部章节轨 (y 212)；背景 1925–1975 交叉淡化；S3 工作区 1950–1975 fade 进场（+8px 微移） |
| S3→S4 | 4485–4515（30 帧） | **③ 纵向滑移 slide** | S3 内容 translateY 0→−80、opacity→0（4485–4500）；y=540 处 2px `teal` 水平细线 scaleX 0→1（4488–4500）再 1→0（4500–4512）；S4 内容 translateY 80→0、opacity 0→1（4500–4515） |
| S4→S5 | 5985–6015（30 帧） | **④ 后拉交叉淡化 pull-crossfade** | S4 scale 1.0→1.06、opacity→0；S5 scale 0.97→1.0、opacity 0→1；交叉中点 6000；两者共用底层 `bg` 不动，避免全帧重绘闪烁 |
| S5→S6 | 7335–7365（30 帧） | **⑤ 环形展开 iris** | 以 S5 末帧 check 徽章中心 (960, 560) 为圆心，`teal` 1.5px 描边圆 r 0→1400（linear）；圆内填充 `bgSoft`，7355–7365 与 S6 内容交叉淡化 |
| 收尾 | 7740–7800 | 双段淡出 | 7740–7780 全内容 opacity→0（落到 `bgDeep`）；7780–7800 全画面 `bgDeep`→`#000`，linear |

实现归属：每个场景组件自己负责窗口内的出/入半段（S1 负责 735–750，S2 负责 750–765，依此类推），转场助手组件见附录。五种类型轮换出现（wipe → match-cut → slide → crossfade → iris），全片无连续两个同类型转场。

---

## 9. Scene Art Direction

通用（全部场景适用）：

- 眉标每场景一组：`PROBLEM · 情景演示` / `WORKFLOW · 五步流程` / `WALKTHROUGH · 完整走查` / `STREAMING · 有界流式` / `RECOVERY · 中断恢复` / `SUMMARY · 总结`。
- **免责规则**：画面出现数据值、表格或产品界面的帧，右上角 (1452, 84) 必须显示免责 chip「流程示意 · 合成数据」（20px Sans 400、muted2、1px line 边框、r-pill、内 padding 8px）。纯 logo/文字帧（S1-B6、S6-B3 起）可不显示。S6 结尾另有整行免责文字。
- 屏上正文/批注使用 §3 字号表；下列 beat 表中的「文案」为屏上文字（短中文），字幕另行烧录。

### S1 问题 + 品牌（绝对帧 0–750，本场景帧 = 绝对帧）

**目的**：用合成数据建立"测试库里藏着真实个人信息"的焦虑，再以品牌一句话给出解法方向。镜头语言：全景 → 特写 → 收束，节奏由慢到定。

| Beat | 帧（场景内/绝对） | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–30 / 0–30 | 黑场淡入 `bgDeep`→场景底 | 全画面 opacity 0→1 | 无 | — |
| B2 | 30–90 / 30–90 | 背景网格 + 径向氛围显现；眉标进场 | 网格 opacity 0→`o-grid`（30–70，linear）；眉标 spring `snappy` 自 45 起 | 无 | 眉标 `PROBLEM · 情景演示` |
| B3 | 90–300 / 90–300 | SH-01 数据全景：表 (360, 300, 1200, 520)，8 行合成数据（姓名/邮箱/手机号/身份证/城市），敏感单元格 amber 底 12% + amber 文字 | 表行 stagger 6 帧进场（translateY 24→0）；敏感高亮在行进场后 6 帧切换 | 横向缓移 translateX 40→−60（210 帧 linear） | 标题「每一行测试数据，都可能是真实的人」；导语「合成数据演示 · 识别、脱敏、验证、恢复」 |
| B4 | 300–420 / 300–420 | SH-03 字段特写：聚焦单行（y≈560），3 个 amber chip（邮箱/手机号/身份证）弹出；右侧 (980, 400, 808, 240) 批注卡 | 相机 push 1.0→1.12（60 帧 spring `soft`）；chip stagger 8 帧弹入（scale 0.96→1，`crisp`） | 推近 | 「直接分发副本，等于分发隐私」；小字「识别不足 ≠ 数据安全」 |
| B5 | 420–600 / 420–600 | SH-02 数据流：表行退场，6 个 token 沿 2 条贝塞尔路径流向画面中心 (960, 560)；中心 320px 圆环 stroke-dash 0→1 揭示 | 行退场 stagger 6 帧；token 60 帧/程、错相 12 帧；圆环 dash 420–510 | 无（元素级） | — |
| B6 | 600–735 / 600–735 | SH-10 品牌收束：EnvTest 同款圆环+波浪 mark 居中 (960, 460)，wordmark 与副标在下 | wordmark spring `settle` translateY 28→0；副标 `snappy` 延迟 18 帧；三徽章（只读预览 / Ed25519 审批 / 严格验证）stagger 6 帧 | 无 | 「MaskRiver」(hero 92)；「安全、可验证、可恢复的数据脱敏工具」(lead 30, cyan)；徽章行 (annotation 20) |
| B7 | 735–750 / 735–750 | 转场出：扫掠线 | 内容 opacity 1→0 | 无 | — |

布局速写：B3 表格 x 360–1560；B4 特写行卡 (132, 420, 720, 420) + 批注 (980, 400, 808, 240)；B6 环心 (960, 460)，wordmark y 640，副标 y 720，徽章 y 800。

### S2 五步能力总览（绝对帧 750–1950，场景内帧 = 绝对 − 750）

**目的**：把五步工作流一次讲清，每步一个可记忆的画面 + 一句硬事实。镜头基本固定，靠元素级动画推进；结尾匹配剪辑直通 S3。

| Beat | 场景内/绝对 | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–30 / 750–780 | 转场入：扫掠线完成 | wipe 揭示 | 无 | — |
| B2 | 30–150 / 780–900 | SH-06 五步链：5 节点 312×200（y 360–560，间距 24），图标 SVG stroke 描线 | 节点 stagger 6 帧（`snappy`）；图标 dash 揭示 18 帧 | 无 | 眉标 `WORKFLOW · 五步流程`；标题「五步闭环，处处有据可查」；节点名 Scan/Preview/Approve/Apply/Validate（data 20 mono） |
| B3 | 150–330 / 900–1080 | SCAN：链上移缩小（translateY −140, scale 0.85, opacity 0.5）作顶条；下方 SH-03：列卡 (132, 400, 720, 420) + 规则 chips (900, 400, 888, 420)；一个样本值扫描 → verdict chip | 链上移 24 帧；chips stagger 6 帧；样本值 token 流入 → `SENSITIVE` chip `crisp` 弹出 | 无 | 「13 类格式与上下文规则」；「证据不足 = UNKNOWN，不放行」 |
| B4 | 330–510 / 1080–1260 | PREVIEW：SH-05 分屏两表各 640×420（x 132 与 x 824，y 400），中缝箭头；左侧原值以 `****` 遮蔽 | 两表 settle 进场（左右各 translateY ±24）；遮蔽格逐个 12 帧切换 | 无 | 「默认 Dry Run，预览零写入」；chip「默认隐藏原值」 |
| B5 | 510–690 / 1260–1440 | APPROVE：紫域。plan digest chip（mono）从左侧卡流向钥匙图标，产出审批记录卡 (700, 400, 1088, 420)（purple 边框 + 12% 底） | 四段因果链（§7）：pulse→token 40 帧→记录卡进场；scope 行 stagger 6 帧 | 无 | 「Ed25519 审批记录，绑定精确计划」 |
| B6 | 690–870 / 1440–1620 | APPLY：三把锁图标（声明 `--approved` / 意图 `--apply` / 有效签名记录）依次扣合，全 teal 后"门"开、token 流过 | 锁 `crisp` 各 12 帧、间隔 12 帧；门开 = 两条 1px 线位移 | 无 | 「三者缺一即拒绝」；锁下标签 (annotation 20) |
| B7 | 870–1050 / 1620–1800 | VALIDATE：5 项 checklist（行数 / 双向键集 / Schema / 计划字段 / 适配器完整性）(132, 400, 720, 420) 逐项 check；右侧 PASS 横幅 (900, 400, 888, 120) | check stagger 12 帧（`crisp`）；横幅在末项后 12 帧弹入 | 无 | 「严格验证 = 全量扫描 + 完整性检查」；横幅「VALIDATE · PASS」(teal) |
| B8 | 1050–1175 / 1800–1925 | 收束 recap：顶条五节点左→右逐个点亮（20 帧/个），流程箭头 token 连贯流过 | 节点点亮 + 单条 token 全程 90 帧 | 缓推 1.0→1.03（90 帧 `soft`） | 「扫描 → 预览 → 审批 → 执行 → 验证」(body 24) |
| B9 | 1175–1200 / 1925–1950 | 转场出：匹配剪辑预备 | 链开始上移（见 §8） | 无 | — |

布局速写：链节点 x = 132 + i×336（312 宽 + 24 间距）；特写区统一 y 400–820。

### S3 完整走查（绝对帧 1950–4500，场景内帧 = 绝对 − 1950）

**目的**：以"设计概念工作区"（SH-04，全程带免责角标）完整走一遍六步：数据源 → 扫描 → 预览 → 审批 → 执行 → 验证。这是全片信息密度最高的场景，靠章节轨 + 工作区视图切换保持可读。

**场景骨架**：顶部章节轨（S2 链的落位形态）y 132–212，6 个章节 chip（240×56，间距 24）；工作区窗口 (300, 230, 1320, 660)：左栏 (内 24, 320 宽) 表列表，右侧主区 952 宽。章节切换 = 主区视图 fade（12 帧）+ 当前 chip 高亮 cyan。

| Beat | 场景内/绝对 | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–45 / 1950–1995 | 转场入 + 工作区建立（SH-04 建立镜头） | 链落位；工作区 fade（1955–1990） | 无 | 眉标 `WALKTHROUGH · 完整走查` |
| B2 | 45–180 / 1995–2130 | CH1 数据源：左栏表列表（people / orders / addresses，合成），右侧表预览 6 行 | 列表项 stagger 6 帧；预览行 stagger 6 帧 | 缓推 1.0→1.04（90 帧） | 「隔离副本 · 只读连接」；列表项旁标 `read only`（mono 20） |
| B3 | 180–600 / 2130–2560 | CH2 扫描：主区切扫描视图——行流过中央判定列（8 行循环，60 帧/行），右侧 verdict chips 三色图例；后段聚焦 people.phone 列 | 行 token 循环流动（同一时刻 ≤ 3 个移动 token）；verdict chip 随行切换；图例 stagger 6 帧 | 续推 1.04→1.12（180 帧）聚焦单列 | 「字段判定：SENSITIVE / NOT_SENSITIVE / UNKNOWN」；「证据不足 ≠ 安全」 |
| B4 | 600–1030 / 2560–2990 | CH3 预览：底部 CommandCard (300, 700, 1320, 160)：`maskriver scan --source source.db --target target.db --scope people.email`、`maskriver mask --source … --target … --scope … --strategy fake_email`；主区 SH-05 before/after 表 + plan digest chip | 命令行逐行显现（10 帧/行，`snappy`）；digest chip 从表格流向命令卡（40 帧 token） | 拉回 1.12→1.0（24 帧）后静止 | 「默认隐藏原值 · 预览不写入」；小字注解「确定性脱敏：同输入同输出；不等于匿名化，不承诺无碰撞」(annotation 20, muted2) |
| B5 | 1030–1460 / 2990–3420 | CH4 审批：紫域。digest → Ed25519 钥匙 → 审批记录卡（reviewer 标签 + scope 列表 + 有效状态）；四段因果链 ×2 | token 流动 40 帧/程；记录卡 scope 行 stagger 6 帧 | 无 | 「审批记录绑定计划摘要与策略范围」；卡内字段名 mono 20 |
| B6 | 1460–1890 / 3420–3850 | CH5 执行：三锁快检（8 帧/个）全 teal → SH-05 diff：3 行值 amber(暗)→teal 切换，中缝箭头；底部进度条 0→100%（240 帧） | 值切换 60 帧/行（旧值 fade 出、新值 fade 入 + 12px 位移）；进度条 linear | 无 | 「仅写入计划字段」；命令卡 `maskriver mask --streaming --apply --approved …`（截短显示，52 字符内） |
| B7 | 1890–2320 / 3850–4260 | CH6 验证：checklist 5 项（行数 / 双向键集 / Schema / 计划字段 / 适配器完整性），末项展开子标签 `quick_check`（mono）；PASS 横幅 teal | check stagger 12 帧；横幅弹入 | 缓拉 1.0→? 本 beat 内 1.12→1.0（150 帧）回到全景 | 「严格验证：全量扫描 + 适配器完整性检查」；横幅「VALIDATE · PASS」 |
| B8 | 2320–2485 / 4260–4435 | 收束：工作区全景 + 章节轨 6 章全亮；中部总结行 | 章节点亮 20 帧/个；总结行 fade | 无 | 「从扫描到验证，全程留痕」 |
| B9 | 2485–2550 / 4435–4500 | 转场出：纵向滑移（4485 起正式转场，前 50 帧为静置） | 见 §8③ | 无 | — |

布局速写：工作区 (300, 230, 1320, 660)；CommandCard (300, 700, 1320, 160)；checklist (348, 400, 600, 420)；PASS 横幅 (972, 400, 624, 120)。CH3–CH5 期间相机 1.0–1.12，通过窗口内裁切自然聚焦。

### S4 有界流式（绝对帧 4500–6000，场景内帧 = 绝对 − 4500）

**目的**：讲清"数据按批次流动"：有界读取、批次事务、Outbox 回执；明确 SQLite-only 边界。镜头：全景 → 事务特写 → 指标收束。

| Beat | 场景内/绝对 | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–15 / 4500–4515 | 转场入：纵向滑移 | 见 §8③ | 无 | — |
| B2 | 15–180 / 4515–4680 | 标题 + 范围 chip | 标题 settle 进场；chip `snappy` | 无 | 眉标 `STREAMING · 有界流式`；标题「让数据按批次流动」；chip「SQLite · 本地副本」(r-pill, line 边框) |
| B3 | 180–600 / 4680–5100 | SH-06 流水线：6 节点 256×220（y 380–600，间距 24）：有界读取 → 批次队列 → 受控处理 → 批次事务 → Outbox 回执 → 目标副本；节点间 24px 连接线 | 节点 stagger 6 帧；4 个 token 同时流动（90 帧/段、错相 22 帧）；每节点 meter 条填至固定刻度后**停住**（有界的视觉隐喻） | 无 | 导语「分批读取 · 批次事务 · 显式预算」(lead 30) |
| B4 | 600–1050 / 5100–5550 | 批次事务特写：推近"批次事务 + Outbox 回执"两节点，上方展开原子单元括号（1 条 2px purple 括线包住两节点），两节点间 token 往返同拍 | 相机 push 1.0→1.1（90 帧 `soft`）；括线 stroke-dash 揭示 24 帧 | 推近 | 「业务更新与 Outbox 回执，同一事务提交」；amber 小字「批次事务 ≠ 整库原子性」(annotation 20) |
| B5 | 1050–1425 / 5550–5925 | 指标与边界：4 张 StatCard 384×120（y 700–820，间距 24）：有界分页 / 批次事务 / 显式预算 / Outbox 回执 | 卡片 stagger 6 帧；下方边界行 fade | 拉回 1.1→1.0（60 帧） | 边界行「范围仅 SQLite；MySQL 流式不支持」(annotation 20, muted2) |
| B6 | 1425–1500 / 5925–6000 | 转场出：后拉交叉淡化 | 见 §8④ | 后拉 | — |

### S5 Checkpoint · 中断 · 恢复（绝对帧 6000–7350，场景内帧 = 绝对 − 6000）

**目的**：用批次时间轴演示中断与恢复的信任链：先核对、再继续。S4 讲"怎么流"，S5 讲"断了怎么办"。故障是全片唯一 danger 时刻。

| Beat | 场景内/绝对 | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–15 / 6000–6015 | 转场入：交叉淡化 | 见 §8④ | 无 | — |
| B2 | 15–180 / 6015–6180 | SH-07 时间轴建立：6 批次格 256×140（y 400–540）全灰；下方预留状态区 (132, 600, 1656, 280) | 格子 stagger 6 帧；标题进场 | 无 | 眉标 `RECOVERY · 中断恢复`；标题「中断之后，从核对过的位置继续」 |
| B3 | 180–540 / 6180–6540 | 流式执行：格 1–3 依次转 teal（60 帧/个：边框色过渡 + check 图标 dash 揭示）；右上计数 `3/6`（numeric stat 56 mono） | 逐格提交；check `crisp`；计数数字切换 6 帧 | 无 | 状态行「批次事务提交中 · 同事务写入回执与检查点」(body 24) |
| B4 | 540–720 / 6540–6720 | SH-08 故障：6540 全场冻结 4 帧 → 遮罩升至 `o-dim`（6 帧）→ y 320 处 8px danger 横条 scaleX 0→1（12 帧）；状态区浮出只读 status 卡：`maskriver status --target target.db --run-id <32-hex>`，显示 checkpoint 状态与已提交计数 | 冻结（所有动画停止）→ 仅遮罩与横条动；status 卡 `snappy` 进场；已提交格保持 teal、未提交格降为 0.4 透明度 | **完全静止**（紧张感来源） | 「任务中断 · 已提交批次保留」；amber chip「只读查询 · 零写入」 |
| B5 | 720–1080 / 6720–7080 | SH-09 恢复：遮罩 12 帧撤除；命令卡 `maskriver mask --streaming --apply --resume --run-id <32-hex>`；格 1–3 逐个 ring sweep 复核（12 帧/格、间隔 12 帧）→ 格 4 purple 高亮继续 → 格 4–6 依次转 teal；计数 `3/6`→`6/6` | 复核环 dash 揭示；当前格 purple 边框呼吸（opacity 0.6→1 循环，周期 30 帧，≤ 90 帧）；提交动画同 B3 | 无 | 「已提交批次不重写」；「先核对已提交前缀，再继续未完成批次」 |
| B6 | 1080–1290 / 7080–7290 | 验证 + 边界：teal PASS 徽章于状态区；边界三行小字 | 徽章 `crisp` 弹入；边界行 fade（stagger 12 帧） | 无 | 「恢复后再次严格验证 · 通过」；边界行「仅本地 SQLite」「MySQL 恢复不支持」「不宣称通用 exactly-once」(annotation 20, muted2) |
| B7 | 1290–1350 / 7290–7350 | 转场出：环形展开（7335 起正式转场） | 见 §8⑤；7290–7335 静置 | 无 | — |

### S6 总结 + GitHub（绝对帧 7350–7800，场景内帧 = 绝对 − 7350）

**目的**：15 秒收束——三支柱一句话各表，logo 与仓库地址定版，免责与淡出。

| Beat | 场景内/绝对 | 画面 | 动画 | 镜头 | 屏上文案 |
|---|---|---|---|---|---|
| B1 | 0–15 / 7350–7365 | 转场入：iris 完成 | 见 §8⑤ | 无 | — |
| B2 | 15–150 / 7365–7500 | 总结：顶部 mini 五步链（5 chip 120×48，y 360–408，全 teal 点亮）；下方 3 张支柱卡 536×240（y 480–720，间距 24）：安全审批 / 严格验证 / 可恢复执行 | 链 chip stagger 6 帧点亮；支柱卡 stagger 6 帧 settle 进场 | 无 | 眉标 `SUMMARY · 总结`；支柱卡内一行 body 24：如「Ed25519 审批绑定计划」「全量扫描 + 完整性检查」「Checkpoint · 仅 SQLite」 |
| B3 | 150–330 / 7500–7680 | SH-10 logo 收束：支柱卡 12 帧退场；EnvTest 同款 320px 环+波浪 mark 居中 (960, 460)，wordmark 其下；仓库地址 mono 行 | mark 圆环 dash 0→1（30 帧）；wordmark `settle`；地址行延迟 18 帧 | 无 | 「MaskRiver」(hero 92)；「github.com/haoyuehx/maskriver」(mono 28, cyan)；「MIT · 开源」(annotation 20) |
| B4 | 330–390 / 7680–7740 | 收尾说明行 + 静置 | fade 进场后完全静止 ≥ 45 帧 | 无 | 「演示使用合成数据 · 界面为设计示意」(annotation 20, muted2) |
| B5 | 390–450 / 7740–7800 | 双段淡出 | 7740–7780 → `bgDeep`；7780–7800 → `#000` | 无 | — |

---

## 10. Animation Constraints

**性能预算（SwiftShader 2 核，约 4 帧/秒）**：

- 每场景同时动画元素上限：S1 ≤ 8、S2 ≤ 10、S3 ≤ 12、S4 ≤ 12（其中移动 token ≤ 6）、S5 ≤ 10、S6 ≤ 6。
- CSS blur ≤ 8px，且只允许用于 ≤ 64px 的小元素（如状态点光晕），每场景 ≤ 2 处；**优先用静态 radial-gradient 圆片替代任何 blur**。
- 禁止任何覆盖 > 30% 画面的 `filter` / `backdrop-filter`；全帧 filter 为零。故障遮罩用纯色 + opacity，不用 blur。
- SVG：单条 path ≤ 40 个命令、长度 ≤ 800 单位；每场景并发动画 path ≤ 8；stroke-dash 动画并发 ≤ 6。
- 大型 radial gradient 每场景 ≤ 2 个，必须静态；linear gradient 不限（用于面板/字幕遮罩）。
- 文本只允许 transform + opacity 动画；禁止逐帧动画 font-size / letter-spacing / width（整段重光栅）。
- 每帧目标计算预算 ≤ 150ms（含 React 渲染与布局），超出即删减同时动画元素。

**可读性下限**：

- 信息性文字最小 20px（tokens TYPE.micro=16 仅可用于纯装饰坐标类字符，不承载信息）。
- 对比度 ≥ 3:1（近似值，实现时用工具复核）：`text/bgDeep` ≈ 15:1；`muted/panel` ≈ 6.9:1；`muted2/panel` ≈ 4.3:1（信息文字最低档）；`cyan/bgDeep` ≈ 9.8:1；`amber/panel` ≈ 6.8:1；`teal/panel` ≈ 9.5:1；`purple/panel` ≈ 7.4:1；`danger/panel` ≈ 4.9:1。
- 静置规则：任何承载新信息的文字进场后静止 ≥ 45 帧才允许退场或切换。

**反模式清单（禁止出现）**：

1. box-shadow 叠加、多层投影、发光文字（text-shadow > 1px）。
2. 任意元素上的 backdrop-filter；全帧 filter；> 8px 的 blur。
3. 每帧 Math.random / 噪点纹理 / 逐帧重排（如动画 width）。
4. WebGL、图片序列、视频嵌套、外部位图。
5. letter-spacing / font-size / gradient stop 的逐帧动画。
6. 全帧 scale 动画叠加在全帧文字上（相机 scale 只包舞台层，文本进相机层时优先 translateY）。
7. 同一 30 帧窗口两个"大动作"（见 §7）。
8. 用红/紫渐变、闪烁、震动表达"科技感"；danger 只留给 fail-closed。
9. 删除或模糊免责角标来"简化画面"；免责 chip 不得低于 `muted2` 亮度。
10. 数据 token 反向（右→左）流动表达成功路径（反向只属于拒绝语义）。

---

## 附录 · Component Inventory

共享原子组件（跨场景复用，全部位于 `video/src/components/`，数值取自 tokens.ts 与本文件）：

| 组件 | Props | 职责 |
|---|---|---|
| `MixedText` | `text: string; size: keyof TYPE; color?: string` | §3 混排规则的唯一执行者：Latin run 包 MONO + 0.25em 边距，中文用 SANS |
| `StageBackdrop` | `variant: 'deep' \| 'soft'; grid: boolean` | 背景层：底色 linear gradient + ≤2 静态 radial + 80px 网格（§1） |
| `CameraRig` | `scale: number; x: number; y: number; children` | 场景唯一外层 transform 包装；只做 scale(≤1.12) 与 translate(≤80px) |
| `SafeFrame` | `children`（dev 模式画参考线） | §5 安全区容器与校验 |
| `SceneEyebrow` | `label: string; dotColor: string; delay: number` | 左上眉标（§4 规格与几何） |
| `SubtitleBand` | `cues: {from: number; to: number; text: string}[]` | 底部字幕带：几何、遮罩、双行排版（§4）；读取字幕数据逐帧取当前 cue |
| `DisclaimerChip` | `visible: boolean` | 右上「流程示意 · 合成数据」chip，出现数据/界面时恒显 |
| `DataTable` | `columns: string[]; rows: string[][]; highlight: ('sensitive'\|'masked'\|'none')[]; width; rowHeight` | flat 表格（div 网格），敏感/脱敏单元格着色规则 |
| `VerdictChip` | `kind: 'SENSITIVE' \| 'NOT_SENSITIVE' \| 'UNKNOWN'` | 三态判定 chip（cyan/teal 背景 12% + 同色文字） |
| `WorkflowRail` | `mode: 'hero' \| 'mini'; active: number; lit: boolean[]` | 五步链：S2 大形态与 S3 顶条/S6 mini 复用同一几何换算 |
| `ChapterRail` | `chapters: string[]; active: number` | S3 六章轨（240×56 chip） |
| `CommandCard` | `lines: string[]; revealAt: number[]` | CLI 卡：mono 22、逐行显现、`$` 提示符、r-md |
| `ApprovalCard` | `digest: string; scopes: string[]; state: 'pending' \| 'valid'` | 紫域审批记录卡（Ed25519 绑定展示） |
| `GateLocks` | `locks: [boolean, boolean, boolean]; labels: string[]` | apply 三条件锁序列 |
| `CheckList` | `items: {label: string; sub?: string}[]; checkAt: number[]` | 校验清单，check stagger 12 帧 |
| `StatCard` | `label: string; value: string` | 指标卡（label 20 / value numeric stat 56 mono） |
| `BeforeAfterSplit` | `columns: string[]; before: string[][]; after: string[][]` | SH-05 分屏对比 + 中缝箭头 |
| `PipelineFlow` | `nodes: {label: string; detail: string}[]; packets: number; meterAt: number` | SH-06 流水线：节点、连接线、流动 token、有界 meter |
| `BatchTimeline` | `total: number; committed: number; current: number; state: 'idle'\|'running'\|'fault'\|'resume'\|'done'` | SH-07 批次格时间轴（S4/S5 复用） |
| `FaultOverlay` | `start: number` | SH-08：冻结纪律 + 遮罩 + danger 横条（只在 S5-B4 实例化） |
| `RingMark` | `size: number; progress: number; color?: string` | EnvTest 圆环 motif 的通用化（logo mark / 复核 sweep 复用） |
| `LogoLockup` | `markSize: number; showWord: boolean` | 环+波浪 mark（复刻 Logo.tsx 几何）+ wordmark |
| `WipeTransition` / `SlideTransition` / `PullCrossfade` / `IrisExpand` | `progress: number`（0–1） | §8 各转场的半段实现，由场景组件在自己的出入帧调用 |

场景组件 `Scene1`–`Scene6` 只做编排：按 §9 beat 表组合上述组件 + `interpolate/spring` 驱动；转场的出入半段由相邻两个场景各自负责，帧窗口以 §8 表为准。
