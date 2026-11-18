# 安全说明

mtok 只读本机日志与本地 magpie 接口，默认零网络、零遥测。唯一在线能力是用户显式运行的 `mtok pricing --fetch`，它会从公开的 LiteLLM 价格表拉取 USD 价格并写本地文件。

请不要在公开 issue 里贴真实 token、完整配置、原始日志或包含账号信息的截图。漏洞请通过 GitHub 仓库的「Report a vulnerability」私下报告，并附最小复现步骤。

报告时请说明：

- mtok 版本和系统
- 涉及的命令与脱敏输出
- 影响范围：本地文件、配置、网络请求或输出内容
