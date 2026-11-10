# 秘境实例纯状态重放

从仓库根目录用项目自带 Windows Go 工具链运行：

```powershell
go run .\tools\exploration-check\instance-replay
```

命令读取同目录下的 `samples/synthetic-archive-cave.json`，将稳定重放报告写入 `work/luna6-exploration/q2/report.json`，诊断写入 `diagnostics.json`。成功的生命周期演示退出码为 `0`；其中列出的预期拒绝是验证成功。畸形输入或计算失败退出码为 `2`。可用 `-input work/luna6-exploration/q2/<name>.json` 指定 Q2 本地负向夹具。

样例和快照都是合成的提议数据，报告标记 `synthetic_only` 与 `not_installable`。每个动作许可都绑定实例 ID、世界、分支、定义内容哈希和签发时 revision；创建许可绑定新实例及 revision 0。JSON 往返仅验证表示和投影一致，不代表 Story JSONL 保存或恢复。`source_ref` 必须来自应用层已经核验的正式事实；本纯逻辑包只核对其结构、绑定和调用关系，无法证明任意引用真实存在。状态差异会保留触发变更的当前动作来源，重复无变化不产生差异或增加 revision。快照需由未来产品层按最新正式事实与 expected revision 提交，本工具不实现存档事务幂等。
