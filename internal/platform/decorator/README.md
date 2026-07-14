# CQRS Decorator（Middleware / Chain）

本包给 Command / Query Handler 统一加上横切能力：**Logging → Metrics → Tracing**。  
业务 Handler 只写领域逻辑；可观测性由装饰链完成。

---

## 这是什么模式？

不完全是「Function Adapter」一个词能概括，实际是三层常见手法叠在一起：


| 名称                           | 在本包里的对应                      | 一句话                                        |
| ---------------------------- | ---------------------------- | ------------------------------------------ |
| **Decorator（装饰器）**           | 包名、整体意图                      | 不改业务 Handler，外面包一层增强行为                     |
| **Middleware + Chain（中间件链）** | `Middleware`、`Chain`、`With`* | 像 Gin / net/http 那样：`mw(next) → next`，洋葱模型 |
| **Func → Interface 适配**      | `handlerFunc`                | 把普通函数变成实现了 `Handler` 的类型                   |


第三点就是你觉得「花哨」的那部分，Go 标准库里很常见，例如：

```go
// net/http
type HandlerFunc func(ResponseWriter, *Request)
func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) { f(w, r) }
```

本包等价写法：

```go
type handlerFunc[C, R any] func(ctx context.Context, in C) (R, error)

func (f handlerFunc[C, R]) Handle(ctx context.Context, in C) (R, error) {
	return f(ctx, in)
}
```

所以：

- 说 **「函数适配成接口」** / **HandlerFunc 适配器** —— 准确（对应 `handlerFunc`）
- 说 **「Middleware」** —— 准确（对应 `WithLogging` / `WithMetrics` / `WithTracing` + `Chain`）
- 说 **「Decorator」** —— 也准确（整体仍是装饰 Handler）

「Function Adapter」作为口语可以，更精确的说法是：**Go 风格的 HandlerFunc 适配 + Middleware 链**。

---



## 核心类型

```go
type Handler[C, R any] interface {
	Handle(ctx context.Context, in C) (R, error)
}

// Command / Query 共用同一形状，只是类型别名
type CommandHandler[C, R any] = Handler[C, R]
type QueryHandler[C, R any] = Handler[C, R]

// 中间件：吃掉 next，返回一个新的 Handler
type Middleware[C, R any] func(next Handler[C, R]) Handler[C, R]
```

---



## 调用顺序（洋葱）

`Apply*` 组装为：

```text
请求进入
  → Logging          （最外）
    → Metrics
      → Tracing
        → 业务 Handler （最内）
      ← Tracing
    ← Metrics
  ← Logging
响应返回
```

对应代码：

```go
Chain(handler,
	WithLogging[C, R]("command"),   // mws[0] 最外
	WithMetrics[C, R]("command", metricsClient),
	WithTracing[C, R]("command"),   // mws[最后] 最内（紧贴业务）
)
```

`Chain` 从**右往左**包裹，因此列表从左到右读就是「从外到内」：

```go
func Chain[C, R any](h Handler[C, R], mws ...Middleware[C, R]) Handler[C, R] {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
```

展开后等价于以前的俄罗斯套娃：

```text
Logging( Metrics( Tracing( handler ) ) )
```

只是列表写法比嵌套 struct 更可读，也更容易增删一层。

---



## 单个 Middleware 长什么样

以 Tracing 为例（Logging / Metrics 同构）：

```go
func WithTracing[C, R any](kind string) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (result R, err error) {
			// ... 前置：开 span
			result, err = next.Handle(ctx, in)
			// ... 后置：记错误、关 span
			return result, err
		})
	}
}
```

要点：

1. **闭包**捕获配置（`kind`、`metricsClient` 等）
2. 返回的 `handlerFunc(...)` 把「匿名函数」适配成 `Handler` 接口
3. 通过 `next.Handle` 把控制权交给下一层

---



## 对外入口（业务侧只用这两个）

```go
decorator.ApplyCommandDecorators(handler, metricsClient)
decorator.ApplyQueryDecorators(handler, metricsClient)
```

走了 `Apply*` 的 Handler **不必再手写** `telemetry.Start`：装饰链里的 Tracing 会自动建 `command.<Type>` / `query.<Type>` span。

Span / Metrics 的 action 名来自入参类型的短名（见 `generateActionName`），例如 `CreateSiteCommand` → `command.CreateSiteCommand`。

---



## 文件一览


| 文件           | 职责                                                 |
| ------------ | -------------------------------------------------- |
| `handler.go` | `Handler` / `Middleware` / `handlerFunc` / `Chain` |
| `apply.go`   | `ApplyCommandDecorators` / `ApplyQueryDecorators`  |
| `logging.go` | `WithLogging`                                      |
| `metrics.go` | `WithMetrics` + `MetricsClient`                    |
| `tracing.go` | `WithTracing`                                      |
| `helpers.go` | `generateActionName`                               |


---



## 如何加一层新能力

1. 新增 `WithXxx[C, R any](...) Middleware[C, R]`
2. 在 `apply.go` 的 `Chain(...)` 列表里按期望顺序插入
3. 业务调用方无需改动（仍走 `Apply*`）

示例：若希望在 Metrics 外侧再加 Auth：

```go
return Chain(handler,
	WithAuth[C, R](...),      // 新：最外
	WithLogging[C, R]("command"),
	WithMetrics[C, R]("command", metricsClient),
	WithTracing[C, R]("command"),
)
```

---



## 和旧写法的对比

**旧（嵌套 struct）**

```go
return CommandLoggingDecorator[C, R]{
	base: CommandMetricsDecorator[C, R]{
		base: CommandTracingDecorator[C, R]{
			base: handler,
		},
		client: metricsClient,
	},
}
```

**新（Middleware 列表）**

```go
return Chain(handler,
	WithLogging[C, R]("command"),
	WithMetrics[C, R]("command", metricsClient),
	WithTracing[C, R]("command"),
)
```

行为相同；新写法更易读、Command/Query 共用一套中间件实现，不再各维护一套 `CommandXxxDecorator` / `QueryXxxDecorator` 类型。