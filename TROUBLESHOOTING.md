# 故障排除指南

## 400 错误："请求参数错误: Improperly formed request"

这个错误通常由以下原因引起：

### 1. 模型名称不正确
确保使用支持的模型名称：
- `claude-3-5-sonnet-20241022`
- `claude-3-5-sonnet-20240620`
- `claude-3-5-haiku-20241022`
- `claude-3-opus-20240229`
- `claude-3-sonnet-20240229`
- `claude-3-haiku-20240307`

### 2. 请求格式问题
确保请求包含必需字段：
```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1000,
  "messages": [
    {
      "role": "user",
      "content": "Your message here"
    }
  ]
}
```

### 3. Token 问题
- 检查 token 是否有效：`./kiro2cc read`
- 刷新 token：`./kiro2cc refresh`
- 确保已正确登录 Kiro

### 4. 网络连接问题
- 检查网络连接
- 确认防火墙设置
- 验证 DNS 解析

## 调试步骤

1. **启动服务器并查看日志**：
   ```bash
   ./kiro2cc server
   ```

2. **测试健康检查**：
   ```bash
   curl http://localhost:8080/health
   ```

3. **发送测试请求**：
   ```bash
   curl -X POST http://localhost:8080/v1/messages \
     -H "Content-Type: application/json" \
     -d '{
       "model": "claude-3-5-sonnet-20241022",
       "max_tokens": 100,
       "messages": [{"role": "user", "content": "Hello"}]
     }'
   ```

4. **检查服务器日志**：
   - 查看 "Anthropic 请求体" 部分
   - 查看 "CodeWhisperer 请求体" 部分
   - 注意任何错误消息

## 常见解决方案

### Token 过期
```bash
./kiro2cc refresh
```

### 模型不支持
检查并使用支持的模型名称。

### 请求格式错误
确保 JSON 格式正确，包含所有必需字段。

### 权限问题
重新登录 Kiro 应用程序。

## 环境变量

可以设置以下环境变量：
- `KIRO_PROFILE_ARN`: 自定义 Profile ARN
- `ANTHROPIC_BASE_URL`: API 基础 URL (默认: http://localhost:8080)
- `ANTHROPIC_API_KEY`: 从 `./kiro2cc export` 获取

## 联系支持

如果问题仍然存在，请提供：
1. 完整的错误消息
2. 服务器日志输出
3. 使用的请求示例
4. 操作系统和版本信息