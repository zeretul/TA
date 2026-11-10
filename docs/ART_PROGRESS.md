# 独立插图接入进度 · 2026-11-03

## 批量增量状态 · 2026-11-03 15:47 UTC

- 静态清单482条：482条通过模型视觉复核、0条候选。核心最终覆盖575/594，仍待完成19项。
- NPC：101张既有肖像、125张新图、0张候选、1张缺图。核心定义：349/367张真实独立图，其余18张未完成；额外166种材料/消耗品未混入核心完成数。
- 本批607个已登记衍生PNG逐文件解码、尺寸与hash全部通过；482条当前作者来源hash全部匹配，无孤儿身份或原图复用。源文件检查期间未变化，作者身份与最近运行时审计一致。
- 当前482条清单已实际重编译，独立审计：607个PNG完整解码/hash、app静态图冻结/持有/冷重开及HTTP GET/HEAD定向、Go运行时目录审计；另通过真实App混合尺度战斗投影探针，2842份源码锚点无漂移通过，清单前后hash一致。最近编译证据见`docs/art-validation.json`与`/workspace/shared/ta_0617_audit/astra_independent_1500/followup-domain-app-art.jsonl`。
- 离线导入器`python scripts/import-catalog-art_test.py -v`的6项隔离测试通过：完整竖幅与头像、明确裁切、重复身份/原图拒绝、复核后原图变化拒绝、非法路径/hash/裁切拒绝、方形定义图及不上采样。
- 两项旧测试现在按精确静态身份判断新增专图，不再把待图原创NPC或strike硬编码为永久无图；没有专图仍严格为空，并另测未知机器/飞船绝不借装备共享图。
- 星海早期完整来源事后重建已明确记录；凡人10件剑袍仅追加机制文案、6项领航/轻功仅增加mobility数值，均逐字段与原图复审，保留原始hash后另存当前源重审。
- 15名星海人物的年代/身份/联系资格调整已逐条重审，外貌原句不变，图片与裁切字节未改。3名历史人物的图片仅为史料画像，不建立当前存续、复苏或可会面事实。
- 来源重审更新器4项隔离回归通过：保留制作原snapshot/hash/图片引用、幂等、拒绝作者源变化、拒绝原图变化与未审状态；应用前逐条验证并备份旧manifest和溯源记录。
- 宋老橹v3已清楚呈现左耳缺角，经过逐图复核后显式替换未发布旧候选；旧原图、衍生hash与候选来源记录保留在档案。候选原hash的JSON分隔格式差异也留有核验记录。
- 竖幅人物详情保留完整比例（最大512×768），头像单独128方形裁切；需要调整的头像须比较原图与裁切结果，记录审阅和旧hash，完整原图及详情不变。
- 生成审核阻挡与作者未明确形制的条目单列`docs/art-blockers.json`，仍计为缺图，不以占位图充数。
- 来源、提示词、原始hash、当前重审与衍生hash保存在`docs/art-provenance/`；逐条源核查在`docs/art-source-audit.json`。reviewed不是用户人工批准。

以下为接入行为与验证说明；实时数量以本节与最新覆盖JSON为准。

## 当前状态

灵界材料现在只在精确身份存在真实独立图时冻结专图；没有专图的材料、消耗品、机器和飞船绝不借装备共享图。已用真实合体注本验证新故事持有投影与冷重开，显式用户生成图优先级保持。

本批在 0.6.17 输入源码上新增独立静态插图库，不改作者世界 JSON、不改玩家事实。NPC 图按 world + source_key；定义图按 world + definition ID + version 查找。新增 `internal/catalogart/assets/manifest.json` 独立清单，`images/<sha256>.png` 不可变衍生图，只公开已登记的单图路由，不公开清单或作者人物目录。

- 现有凡人 101 人的 128/192px 肖像保持原样，没有上采样冒称原图。
- reviewed 表示模型已逐图视觉复核，不代表用户已人工批准；来源快照/提示词/审阅记录在 `docs/art-provenance/`。

## 行为边界

- 玩家选择/上传/生成图及已有作者显式图继续优先。缺独立定义图保留明确标为共享题材示意的旧图；加载失败显示空态，不借别人的图填充。
- 不根据显示名配图。人物图只装饰已存在的可见人物行，不新增人物、不改变发现门槛；跨世界相同来源键不借图。
- 新局冻结定义图引用，旧存档不迁移、不改故事事实。NPC 展示层可以按旧存档原有 world/source_key 补画面；未知来源仍空。
- `catalog` 定义图只接受已登记 definition 哈希，不能请求任意 URL、用户项目、NPC 图或路径。路由只读 PNG、内容哈希缓存、禁止目录访问、缺图 404。

## 续作入口

1. 独立原图与逐图来源/提示词/审阅报告留在制作档案。
2. `python scripts/import-catalog-art.py --world <world> --kind npc|definition --id <stable-id> --source-sha256 <hash> --image <file> --expected-original-sha256 <reviewed-image-hash> --review-state candidate|reviewed` 导入。默认版本 1。先确认视觉审阅再填 reviewed；预期原图hash不符时在写入前拒绝，防止制作文件并发变化混入。
3. 现有身份不能静默覆盖；更换必须明确审阅，保留旧哈希供旧存档。头像 128px、人物详情 512px、定义 768px，绝不上采样来源。
4. `go run ./cmd/ta-art-audit` 输出当前作者目录和真实规则表逐条覆盖，不是只遍历图清单。加 `--require-complete` 可作为最终交付门禁，当前预期失败。
5. 完整交付前重新生成覆盖报告、检查全部独立图和对应人物/定义、构建前后端资源、验证三世界及真实页面。缺图不能冒称全量图已完成。

## 实际检查

基础接入逻辑通过以下Linux云端源码检查；最近资产批次复验以`docs/art-validation.json`为准。这些不是旧0.6.17二进制的运行证据。

- catalogart/portrait/cultivation定向通过：哈希与PNG解码、世界/身份/版本隔离、未知引用/路径拒绝、文件缺损、重复身份、候选/最终状态、旧肖像回退。
- app定向新旧测试通过：新局专图冻结、共享/生成优先级、冷重开、人物未发现时零行、跨世界不借图、资料库旧revision与自选图保护。现有硬编码202条测试已改为对当前真实规则表逐条类别计数。
- API真实server+middleware GET/HEAD路由测试通过：PNG原字节、长度、HEAD无body、MIME/nosniff、不可变缓存、未知文件/清单/路径拒绝。
- 7个相关Go包vet通过（合并串行窗口）。
- EntryArtwork 21项通过；头像/肖像加载失败后换源恢复2项通过（与其他界面回归合计33项）。
- tsc与4820双语键检查通过。早前/tmp满、tsc资源SIGKILL和首次GET/HEAD长度缺口已在最终串行检查中排除。
- `go run ./cmd/ta-art-audit`按当前作者与规则目录逐条报告。来源检查和完整PNG解码另有逐批证据；`--require-complete`在仍有核心缺图时必须失败，不把共享图或候选计为完成。
- 未完成真实页面视觉验收：本地Chromium启动受socket限制；Vite默认预构建曾资源SIGKILL，轻量配置成功启动后独立exec与正式云浏览器的loopback均无法访问，已停止预览服务。夹具保留待支持环境重跑，不能以jsdom测试冒称浏览器画面已验。
- 未运行本批完整打包、Windows原生或自然AI验证。主程序与内嵌前端仍需合并批次统一构建。

基础接入首轮合并日志在同工作区 `ta_next_20261103/evidence/`：`stars-wuxia-final-app.log`、`stars-wuxia-art-api.log`、`stars-wuxia-vet.log`、`stars-wuxia-art-packages.log`。测试由并行任务统一串行执行，未重复占用构建窗口。


## 主要文件

- 静态库、验证与嵌入：`internal/catalogart/`。
- 画像拼接：`internal/portrait/world.go`，人物窗口/头像编辑状态/作者资料入口复用同一规则。
- 定义元数据：`internal/cultivation/definition_art.go`、`internal/app/story_pack_definition_art.go`。
- API：`internal/api/handlers/handler_catalog_art.go` 与 GET/HEAD 路由。
- 前端：`EntryArtwork.tsx`、双语caption、`person-avatar.tsx`失图后换源恢复。
- 离线工具：`cmd/ta-art-audit`、`scripts/import-catalog-art.py`。
- 可重跑页面夹具：`web/tests/support/catalog-art.html`、`web/vite.art-preview.config.ts`、`web/tests/catalog-art-smoke.mjs`。本地启动轻量Vite，再运行smoke脚本；脚本由Playwright请求拦截供应真实图字节，不依赖私人数据。

静态图产物和本文件不是完整新版本交付。主程序、前端内嵌资源及全量最终验收由合并打包步骤完成，不能拿旧0.6.17二进制证明这些新接入口已发布。
