# 分享版干净源码基线

来源为未修改亲密提示词的0.6.18最终源码，合入共同的人物年龄、明确拒绝与JSON兼容修复。其他代码、前端和资源由开发根共用。这里的.go.txt只供Go构建映射，不形成第二个Go包。

使用scripts/build-editions.ps1构建local或share。share先核对本地对应文件与本基线的哈希；任一变化必须先核对通用修复是否要同步，再更新manifest，避免旧基线悄悄覆盖新修复。构建不改开发源码，输出不自动安装或发布。

开发版与正式版用local产物；TA-github使用share产物。全部版本号读取web/package.json，只用数字。


## 0.7.0 本机冒险分享构建

在隔离的当前开发快照上应用 [冒险版差异](0.7.0-adventure-only.patch)。它移除历史推演导航与接口、三国冒险注册，并在分享版 Game Agent 内置共享创作红线。构建源还排除 internal/history、internal/app 的 history_*、历史 API handler、前端 features/history、public/history、两种语言的历史推演模块及三国候选包 JSON。与历史推演和三国绑定的测试夹具仅用于开发快照。

前端在 Git 忽略规则之外的隔离目录构建，避免样式扫描漏掉界面。将同一 dist 写入分享候选的内嵌和外置前端，再用 build-editions.ps1 的 share 选项核对标准覆盖及生成主程序。媒体只装经过审核的普通动态头像，以及已有运行文件并属于分享冒险世界的审核剧情肖像。根目录和 game 内均维护 SHA256SUMS.txt，安装版别与源码锚点见 TA-github/game/edition.json。
