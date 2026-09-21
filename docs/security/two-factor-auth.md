# 登录两步验证（TOTP MFA）

> 适用版本：`main` 分支（2026-09 起）
> 相关代码：`api/handler_mfa.go`、`store/mfa.go`、`web/src/lib/mfa.ts`、
> `web/src/components/settings/SecuritySettings.tsx`、`web/src/components/auth/MfaCodeInput.tsx`

AiT 支持基于 TOTP 的两步验证。**该功能默认关闭**，属于账号级可选设置——
开启后登录时在密码之外再要求一个 6 位动态验证码。

---

## 1. 用户视角

### 开启

`设置 → 安全设置 → 两步验证 → 开启`

1. 用验证器 App（Google Authenticator / 1Password / Authy 等）扫描二维码，
   或手动输入密钥；
2. 输入 App 生成的 6 位验证码完成绑定；
3. **保存恢复码**（10 个，一次性使用）。恢复码只在开启时展示一次，
   之后无法再取回——丢失只能关闭后重新开启。

开启成功后服务端会下发一个新的 JWT，前端会自动替换本地会话令牌。

### 登录

1. 输入邮箱与密码；
2. 密码正确且账号已开启 MFA 时，服务端返回 **HTTP 202** 而不是登录成功，
   响应体带 `mfa_required: true` 与一次性的 `mfa_token`；
3. 前端切换到验证码步骤，输入 6 位验证码（或改用恢复码）；
4. 校验通过才下发真正的会话 JWT。

验证码步骤有 **5 分钟倒计时**（服务端 challenge 的有效期），倒计时结束需重新登录。

### 关闭

`设置 → 安全设置 → 两步验证 → 关闭`，需要输入当前验证器验证码。
关闭后立即生效，后续登录只校验密码。

---

## 2. 接口契约

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| `GET` | `/api/mfa/status` | 需要 JWT | 返回 `{ enabled }` |
| `POST` | `/api/mfa/setup` | 需要 JWT | 生成密钥，返回 `{ secret, otpauth_uri, issuer }` |
| `POST` | `/api/mfa/enable` | 需要 JWT | 校验验证码并开启，返回 `{ message, recovery_codes[10], token }` |
| `POST` | `/api/mfa/disable` | 需要 JWT | 校验验证码并关闭，返回 `{ message }` |
| `POST` | `/api/mfa/login/verify` | **公开**（受 IP 限流） | 用 `mfa_token` + 验证码换取会话 |

### 校验失败的错误契约

`POST /api/mfa/login/verify` 的失败响应带一个机器可读的 `reason` 字段：

| HTTP | `reason` | 含义 | 前端行为 |
|---|---|---|---|
| 401 | `invalid_code` | 验证码不对，**challenge 仍然有效** | 停留在验证码步骤，清空输入框让用户重试 |
| 401 | `expired` | challenge 不存在或已超时 | 回到密码步骤，提示重新登录 |
| 401 | `unavailable` | 账号已关闭 MFA 或配置缺失 | 回到密码步骤 |
| 429 | `too_many_attempts` | 该 challenge 连续错误达 `maxMFAAttempts`（5 次），已被丢弃 | 回到密码步骤 |
| 429 | *（无 `reason`）* | 来自**共享 IP 限流器**，不是 MFA 自己的错误 | 回到密码步骤，提示等待几分钟 |

`invalid_code` 响应还会带 `attempts_left`，前端据此显示「剩余尝试次数」。
最后一个错误码会在提交前给出提示，避免用户莫名被踢回第一步。

> **注意区分两种 429。** MFA handler 自己返回的 429 一定带 `reason`；
> 由 `api/rate_limiter.go` 的 `authRateLimiter` 返回的 429 **没有** `reason`。
> 二者含义完全不同，前端靠 `reason` 是否存在来分流。

---

## 3. 安全模型

| 项目 | 取值 | 位置 |
|---|---|---|
| 算法 | TOTP / HMAC-SHA1 / 6 位 / 30 秒步长 | `validTOTP` |
| 时钟容差 | ±1 个步长（即前后各 30 秒） | `validTOTP` 的 `{-1, 0, 1}` |
| 密钥 | 20 字节随机数，base32 无填充（32 字符） | `newMFASecret` |
| 密钥落库 | 经 `cryptoService.EncryptForStorage(secret, "mfa", userID)` 加密 | `handleMFAEnable` |
| 恢复码 | 10 个，每个 8 字节随机数 → 13 字符 base32；**bcrypt 哈希后存储** | `handleMFAEnable` |
| 恢复码使用 | 校验成功即从列表移除（一次性） | `handleMFALoginVerify` |
| 登录 challenge | 进程内内存 map，TTL 5 分钟 | `mfaChallenges` |
| 单 challenge 尝试上限 | `maxMFAAttempts = 5` | `bumpMFAChallengeAttempts` |
| challenge 生命周期 | 仅在**兑换成功 / 超时 / 尝试耗尽**时丢弃 | `dropMFAChallenge` |

### 两个容易踩的点

1. **错误验证码不会消耗 challenge。** 早期实现里 `delete(mfaChallenges.m, ...)`
   写在校验之前，任何输错都会烧掉 challenge，用户必须重新走密码流程。
   现在改成「先查后删」：输错只累加计数，challenge 保留。
2. **MFA 校验与密码登录共用同一个 IP 限流桶。**
   `authRateLimiter = newIPRateLimiter(10, 15*time.Minute)` 覆盖整个 `authGroup`，
   `/api/login` 与 `/api/mfa/login/verify` 都算在里面。
   一次「登录 + 验证」消耗 2 次配额；一个 challenge 最多 5 次尝试，
   即 1 次登录 + 5 次验证 = 6 次，仍在 10 次预算内。
   **但如果用户在 15 分钟内反复重登，会先撞上限流而不是撞上 `maxMFAAttempts`**——
   这时返回的就是上面那个没有 `reason` 的 429。

---

## 4. 运维与排障

### 用户被锁在外面怎么办

| 现象 | 原因 | 处理 |
|---|---|---|
| 提示「验证窗口已关闭」 | challenge 超时（>5 分钟） | 重新登录即可 |
| 提示「验证次数过多」 | 单个 challenge 输错 5 次 | 重新登录，会拿到新 challenge |
| 提示「尝试过于频繁」 | 撞上 IP 限流（10 次/15 分钟） | 等待窗口滚过，或换出口 IP |
| 验证器丢了、恢复码也丢了 | —— | 需管理员在库中清除该用户的 MFA 记录，之后用密码登录 |

清除某个用户的 MFA（需先停掉该用户会话，操作前备份数据库）：

```bash
python3 - <<'PY'
import sqlite3
c = sqlite3.connect('data/data.db')
c.execute("delete from user_mfa where user_id = ?", ('<user-id>',))
c.commit()
print('removed', c.total_changes)
PY
```

### 自查命令

```bash
# 已开启 MFA 的账号数（0 = 无人开启，属正常默认态）
python3 -c "import sqlite3;print(sqlite3.connect('data/data.db').execute('select count(*) from user_mfa').fetchone()[0])"

# 公开端点在无 challenge 时的响应应为 expired
curl -s -X POST http://127.0.0.1:8080/api/mfa/login/verify \
  -H 'Content-Type: application/json' \
  -d '{"mfa_token":"bogus","code":"123456"}'
# => {"error":"MFA verification expired; sign in again","reason":"expired"}

# 需要鉴权的端点未带 token 应为 401
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/mfa/status   # => 401
```

### 前端注意事项

- MFA 相关请求**必须走原生 `fetch`，不能用 `httpClient`**。
  `httpClient` 的响应拦截器把**所有** 401 当作会话失效处理——
  会清空 localStorage、派发 `unauthorized` 事件并把浏览器踢回登录页。
  而「验证码输错」在语义上就是 401，必须原地渲染提示而不是把用户登出。
  该逻辑集中在 `web/src/lib/mfa.ts` 的注释里。
- `POST /api/mfa/login/verify` 是公开接口，调用时不要带 `Authorization` 头。

### 测试

```bash
# 后端
go test ./api/ ./store/

# 前端（3 个文件）
cd web && npx vitest run \
  src/components/auth/LoginPage.mfa.test.tsx \
  src/components/auth/MfaCodeInput.test.tsx \
  src/components/settings/SecuritySettings.test.tsx
```
