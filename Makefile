.PHONY: gen
gen: genproto gengateway

.PHONY: genproto
genproto:
	@./scripts/genproto.sh

.PHONY: gengateway
gengateway:
	@./scripts/gengateway.sh

.PHONY:fmt
fmt:
	goimports -l -w internal/

.PHONY:lint
lint:
	@./scripts/lint.sh
