optimization（服务已删除，下面是当时实现的备忘）
1. 一旦 Resource 模块公开了点的角色/标签，自动发现功能将是一个合理的后续改进.
2. YAML 映射推迟到实际业务规则稳定后再进行.
3. 获取telemetry 的快照是否会是旧的？
4. Evaluate 方法直接遍历rules.SOCThresholds，而不是像 alarm 组件的 Evaluator 那样通过 map[RuleID]ruleHandler 注册表进行分发——alarm 组件涉及多种需要路由的规则类型（如调度任务失败与 SOE 变更），而 Optimization v1 尚无此需求。若后续增加第二种规则类型，届时可重新评估引入注册表机制的必要性。