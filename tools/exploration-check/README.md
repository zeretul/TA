# Exploration core replay

该命令调用 `internal/exploration` 同一组纯 Go API，读取三份合成数据，输出本地候选契约、输入 SHA-256、图与能力路径诊断、玩家已知视图及路线时间演示。三份样例均为测试素材，未安装到游戏。

在仓库根目录用原生 Windows PowerShell 重放：

```powershell
go run .\tools\exploration-check
```

校验单个 JSON 文件可用 `-input <path>`。任何输入或计算诊断都会输出诊断并以退出码 2 结束；成功为 0。输出没有时间戳，重复输入可逐字节比较。

`schema` 是 `denova exploration local candidate schema v1`，不是已安装的 Story Pack schema。知识快照 JSON 往返只验证投影稳定性，不代表 Story JSONL 恢复或持久化。
