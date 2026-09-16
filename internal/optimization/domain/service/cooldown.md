更完整的控制系统里，还可能需要：

阈值滞回（Hysteresis）
+
Cooldown
+
状态机
+
任务幂等


Decision Loop 降频
       ↓
减少触发频率

Cooldown
       ↓
防止短时间重复触发

Hysteresis / 状态机
       ↓
解决边界震荡

幂等 / Task 状态
       ↓
保证真正执行层不会重复产生副作用