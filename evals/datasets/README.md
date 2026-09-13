# 题库范围与新增失败样本

`solo-skills-v3.json` 是当前回归集，包含 20 个原创受控程序维护任务、111 项隐藏检查。每个任务给 Agent 公开需求和有缺陷的初始模块；参考实现与检查输入不会放进任务提示词。它们不是从生产用户记录中采集的真实事故。

v2 在 `relative-path` 的公开要求中明确“折叠连续斜杠、忽略空段”。参考实现、隐藏检查、初始模块及其余题目与 v1 相同。

v3 明确 `csv-export` 的每条记录都必须以 LF 结束，包括表头和最后一条记录。参考实现、隐藏检查和初始模块与 v2 相同；自动检查逐字段验证版本差异。不同题库版本应分别报告，已见过结果的 holdout 只能用于回归。

| 开发题 | 对应问题 | 冻结验收题 | 迁移验证 |
|---|---|---|---|
| csv-money | CSV 引号、金额精度、排序 | allocate-cents | 精确整数分配与余数 |
| event-latest | 时间戳比较、重复事件 | utc-elapsed | 跨时区实际时长 |
| dependency-order | 拓扑顺序、重复边、循环 | dependency-closure | 共享依赖与可达循环 |
| nested-redaction | 递归脱敏、类型与输入保护 | canonical-json | 递归过滤与稳定序列化 |
| retry-delay | 零值、边界、巨大指数 | batch-boundaries | 数量与 UTF-8 字节限制 |
| markdown-section | 标题与代码围栏 | frontmatter | 精确分隔符与换行 |
| cursor-pagination | 空游标、循环、缺失页 | longest-route | 前缀边界和空值 |
| relative-path | 越界路径、NUL、分隔符 | csv-export | 特殊字符输出与往返 |
| duration-union | 区间并集与重叠 | half-open-overlap | 半开区间与零时长 |
| stable-dedupe | 稳定顺序和版本选择 | patch-config | 深层替换、删除、输入保护 |

表中的对应关系说明涉及的能力，不表示把验收题泄漏给改进 Agent。改进输入只由开发报告生成。原题始终固定，任何题目或判卷规则调整都应生成新实验目录并保留旧版本记录。

新增生产失败样本时，先做脱敏并保留可复现的输入、公开预期、实际错误产物和独立验收。优先加入开发集；验收题在见过结果后不能继续称为未见题。用新的题库版本扩大范围，再验证参考实现、原错误实现和一个无意义实现，最后通过真实 Solo 路径执行。不要只修改提示词让一个已知答案通过。

`humaneval-smoke.json` 是固定公开子集。完整上游压缩数据、MIT 许可、commit、抽样规则与 ID 在 `../vendor/human-eval/`。公开题可能已进入模型训练，不能据此证明长期团队进化。
