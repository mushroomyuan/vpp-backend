                 DecisionLoop
                     │
              每 60 秒一个 tick
                     │
                     ↓
             tenant-a / tenant-b
                     │
                     ↓
          RunDecisionCycleHandler
                     │
                     ↓
                 Evaluator
                     │
             ┌───────┴────────┐
             │                │
       Cooldown检查        Telemetry
             │                │
             └───────┬────────┘
                     ↓
                SOC Rule
                     │
              条件满足？
               │          │
              No         Yes
               │          ↓
               │       Target
               │          ↓
               │       Allocate
               │          ↓
               │    CommandSpec
               │          ↓
               │    DispatchPort
               │          ↓
               │     SubmitTask
               │
               └────→ 下一轮