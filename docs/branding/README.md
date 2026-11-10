# TA「古今星海」品牌资源

2026-11-04，采用用户确认的 C 方案。游戏仍名为 Text Adventure（TA）；这是四世界共同的产品标志，不替代任何世界包自己的配图。

## 实际接入

- 展开及收起的游戏侧栏均显示 TA 标志，原导航收起/展开按钮仍保留。
- 系统设置的关于/更新页显示同一标志。
- 浏览器标签页、Apple touch、PWA 安装图标与旧 favicon.svg 地址统一使用 C 标志。
- `cmd/ta/rsrc_windows_amd64.syso` 与 `cmd/ta-updater/rsrc_windows_amd64.syso` 含相同的七尺寸 Windows 图标。普通 `go build ./cmd/ta` / `./cmd/ta-updater` 会自动链接；仅 Windows amd64 生效。没有更改程序权限、启动方式、Windows manifest 或版本字段。

## 原图与导出

原始文件保留在 `assets/branding/TA-C-Cosmic.original.png`，SHA256：

`8284b6907eb07e65176571c717d5e9f0565f22ff1c8740fb33eb93dd5e49792c`

原图为 1254×1254 RGBA。导出时仅裁掉可见图形外的透明画布（外侧有 1/255 alpha 的不可见噪点），按比例缩放，不重绘字形或颜色。透明标志提供 512px 与 1024px 两档宽度；ICO 包含 16、24、32、48、64、128、256px 七档。常规 PWA 保留透明背景；Apple 与 maskable 使用深色底，maskable 图形全部位于平台安全圆内。

在已有 Node/pnpm 与 Go 工具链的源码根目录运行：

```bash
./scripts/generate-app-icons.sh
node web/scripts/verify-brand-assets.mjs
```

生成脚本固定使用 go-winres v0.3.3（https://github.com/tc-hib/go-winres）。普通构建直接使用已随源码保留的 `.syso`，无需现场下载资源编译器。修改 Logo 后须重新生成 `.syso`；只换 PNG 不会更新 EXE 图标。

## 本轮验证与边界

- 原图 SHA256 与用户确认文件相符；透明通道、输出尺寸、maskable 安全圆及 ICO 全部七档通过校验。
- Windows updater 在 Linux 交叉链接成功；从其 PE 资源重新提取 ICO 后，七档像素与原导出逐一相同。
- TypeScript 全项目类型检查通过；关于/更新页及品牌导航共 8 项单元测试通过，包含反复收起/展开和继续点击游戏、设置。
- 实际查看了原图、512px PWA、32px favicon 和透明标志的导出像素。
- 最终0.6.18-dev.20261104的Windows主程序与updater均已完整交叉构建；两份PE反提取的七档图标均与原导出逐像素一致。先前旧PNG临时构建中断不计作通过。
- 本轮浏览器运行被环境阻断：受控浏览器运行时不可用，本地 Chromium 被系统拒绝创建 socket。已提供 `web/tests/branding-smoke.mjs` 及 `tests/support/branding*` 实际组件视觉夹具，可在可用浏览器环境检查深浅色、中英、390/1280px、收起后再次展开；尚不能声称这些页面组合已通过视觉验收。
- 没有 Windows 原生环境，没有宣称原生 Windows 图标缓存/桌面显示或运行验证通过。
- `LICENSE`、`NOTICE` 与第三方许可证均未改动或删减。

最终整包构建可用 go-winres 反提取程序图标，再将提取的 ICO 路径作为 `verify-brand-assets.mjs` 参数。此检查比较像素，而非仅确认文件名或资源段存在。
