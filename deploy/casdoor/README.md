# Casdoor (local IdP)

完整说明与端到端联调：[`docs/CASDOOR.md`](../../docs/CASDOOR.md)。  
凭据：[`conf/credentials.yaml`](conf/credentials.yaml)。

```bash
make infra-up && make casdoor-up && make casdoor-init
make apisix-up && make apisix-init
# 经 APISIX 访问 Resource 前：resource.yaml → auth.trust-proxy-headers: true
```

| 项 | 值 |
|----|-----|
| Image | `casbin/casdoor:3.125.0` |
| UI | http://127.0.0.1:8000 |
| Discovery | http://127.0.0.1:8000/.well-known/openid-configuration（容器内用 `host.docker.internal`） |
| Token | `make -s casdoor-token` / `DECODE=1` |
