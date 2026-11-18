# CodeBuddy Usage for Windows 11

 **CodeBuddy Usage**  Windows 11 独立软件。右下角系统托盘常态化显示积分余量，鼠标悬浮弹出深色玻璃质感明细卡，并自动完成每日签到与喵喵旅行领积分/派出。

## 功能

- 托盘图标直接绘制余量数字（如 `4028`），悬浮弹出明细卡，点击/右键亦可打开。
- 明细卡：账号头像与昵称、余量/总量/占比进度条、套餐明细（前 5 条，按到期升序）、签到状态、喵喵旅行倒计时，以及「领积分」「去旅行」按钮。
- 自动读取本机 CodeBuddy / VS Code 登录态（DPAPI + AES-GCM 解密），结果经 DPAPI 加密缓存到 exe 同目录的 `token.bin`，临近过期才重读；支持手动粘贴 Token 兜底。
- 自动每日签到、自动领取喵喵旅行积分并派出下一趟；成功时托盘气泡提示，无可领/已完成时静默。
- 右键菜单：刷新 / 领积分 / 去旅行 / 设置 / 修复凭据 / 打开用量页 / 退出。
- 设置：accessToken、刷新间隔（分钟，0 关闭）、自动签到、喵喵旅行、apiBase（国内/国际站）、开机自启。

## 构建

需要 Go 1.23+ 与 Wails v2 CLI。

```powershell
# 安装 Wails CLI（若未安装）
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2

# 开发调试（热重载）
$env:PATH = "D:\software\go\bin;$env:PATH"
wails dev

# 生产构建，输出 build\bin\CodeBuddyUsage.exe
wails build
```

> 注意：必须用 `wails build`（或 `go build -tags wails`）。普通 `go build` 会因缺少构建标签而弹出 Wails 错误对话框并卡住。

## 运行与配置

直接运行 `build\bin\CodeBuddyUsage.exe`，程序常驻托盘且无主窗口。首次运行会自动读取 CodeBuddy 登录态；若失败，在右键菜单选择「修复凭据」→ 手动粘贴 Token。

配置、缓存与日志位于 **exe 所在目录**（绿色便携，便于携带与清理）：
- `settings.json` — 用户设置
- `token.bin` — DPAPI 加密的 token 缓存
- `app.log` — 运行日志（不含 token）

## 结论摘要

关键实现事实：

- **登录态**：`%APPDATA%\<Product>\User\globalStorage\state.vscdb` 的 `ItemTable`，key 含 `Tencent-Cloud.coding-copilot.new.accessToken`；value 为 JSON，其 `data` 数组即密文字节。需叠加 `-wal` 未回写页。
- **密钥**：`%APPDATA%\<Product>\Local State` 的 `os_crypt.encrypted_key`，去 `DPAPI` 5 字节前缀后经 `CryptUnprotectData(CurrentUser)` 解出 32 字节 AES key。
- **解密**：密文 `v10` + AES-256-GCM，nonce = 第 3~15 字节，tag = 末 16 字节。正则提取 `accessToken` / `nickname` / `avatarUrl`。
- **接口**（Bearer Token 鉴权）：
  - 用量 `POST /billing/meter/get-user-resource`
  - 签到 `POST /billing/meter/checkin-status`、`/daily-checkin`（`code=10001` 视为已签到）
  - 喵喵 `GET/POST /activity/growth/buddy/travel/{status,claim,depart,records}`
- **编排**：先并行签到与喵喵领积分/派出，再拉用量；claim 按「一趟旅行一次」守卫，depart 一日一次；`status.reward_credit` 恒为 0，真实到账积分从 `records` 取。
- **401** 才判登录失效并重试一次，**403** 按业务结果处理。

本项目仅在本机读取与解密用户自己的 CodeBuddy 登录态，凭据不上传任何第三方。
