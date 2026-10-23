# 演示指南

## 网站

1. 打开 https://haoyuehx.github.io/maskriver/ 。
2. 首屏阅读产品定位；点击“观看演示”播放当前 4:20 视频。
3. 在 Core Workflow 中依次点击 Scan / Preview / Approve / Apply / Validate。
4. 查看 Streaming 与 Recovery 示意；点击“模拟中断”再“恢复执行”，观察批次、Outbox 和验证状态。
5. 在 Get Started 下载 v1.0.0、验证 SHA-256、打开 Quick Start / SQLite / MySQL 文档。

## CLI（合成/隔离副本）

```sh
maskriver scan     --source source.db --target target.db --scope people.email ...
maskriver mask     --source source.db --target target.db --scope people.email \
                   --strategy fake_email --key-id demo --key-file key.bin   # 只读预览
maskriver approve  ... --trust-file trust.json --history-dir history \
                   --reviewer-key-id reviewer-1 --reviewer-key-file reviewer.key --reviewer-id reviewer
maskriver mask     ... --trust-file trust.json --history-dir history --approved --apply
maskriver validate ... --strict
```

MySQL 使用 `--dialect mysql` 与两个数据库 DSN；具体命令见 [MySQL 教程](../guides/mysql.md)。

## 视频

当前视频是无配音视觉版，画面含中文字幕；后续最终视频由用户单独提供。网站视频区域已集成，但本轮不修改视频文件、封面、字幕或播放器。
