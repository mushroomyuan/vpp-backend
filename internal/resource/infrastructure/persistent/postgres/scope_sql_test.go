package postgres

import (
	"strings"
	"testing"
)

func TestResolveScopeSQLExpandsByPath(t *testing.T) {
	t.Parallel()

	if !strings.Contains(resolveScopeSQL, "r.type IN ('site', 'asset')") {
		t.Fatal("site and asset expansion must be guarded by scope root type")
	}
	if !strings.Contains(resolveScopeSQL, "n.path LIKE r.path || '/%'") {
		t.Fatal("descendant expansion must use the node path prefix")
	}
	if strings.Contains(resolveScopeSQL, "asset_node.parent_id") || strings.Contains(resolveScopeSQL, "parent_id =") {
		t.Fatal("scope expansion must not use the direct parent join that misses nested CUs")
	}
	if strings.Contains(resolveScopeSQL, "external_address") {
		t.Fatal("resolve scope must not read external_address")
	}
	if !strings.Contains(resolveScopeSQL, "SET TRANSACTION") && !strings.Contains(scopeQueryReadOnly, "READ ONLY") {
		t.Fatal("scope query must run read-only")
	}
}
