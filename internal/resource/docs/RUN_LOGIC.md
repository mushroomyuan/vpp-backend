Run()

创建 rootCtx
创建 errgroup

启动:
 ├─ grpc server
 ├─ http server
 ├─ metrics watcher
 ├─ signal watcher
 └─ worker

等待事件:

[情况A]
Ctrl+C
 -> rootCancel()

[情况B]
HTTP server error
 -> return err

[情况C]
metrics error
 -> return err

任一触发后:

egCtx.Done()
 -> shutdown goroutine 执行:

1 grpc graceful stop
2 http shutdown
3 worker stop
4 metrics stop

最后:
eg.Wait()
return