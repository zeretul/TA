# 碎冠诸境作者源

这里保留本次定稿的世界与任务作者输入、生成器和五份生成JSON。运行 python authoring/test_authoring_rebuild.py 会在临时目录重建并逐字节比较，不改源码或已打包资产。生成器不调用外部模型。

正式运行资产位于源码 internal/storypack/assets 与 internal/quest/assets。本目录是可复现的作者材料，不是第二份运行配置。修改作者输入后应审核生成JSON并明确导入，禁止自动覆盖正在测试的世界。
