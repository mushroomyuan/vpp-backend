package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
)

// Revision format version 1.
//
// ResourceRevision is the only implementation of resource_revision. Resource
// and Decision must call it instead of hashing database order, MAX(version),
// or JSON.
//
// Encoding is length-prefixed and then SHA-256. Each field is:
//
//	<decimal byte length>:<raw bytes>\n
//
// Inputs are copied and sorted before encoding:
//   - nodes by (entity_type, entity_id)
//   - capabilities by (cu_id, capability_id)
//   - bindings by (cu_id, metric_id)
const revisionFormatVersion = "1"

const (
	RevisionEntitySite  = "site"
	RevisionEntityAsset = "asset"
	RevisionEntityCU    = "cu"
)

// RevisionScope is the stable identity of a resolved scope.
type RevisionScope struct {
	TenantID  string
	ScopeType string
	ScopeID   string
}

// RevisionNode is one site, asset, or CU version inside the scope.
type RevisionNode struct {
	EntityType string
	EntityID   string
	Version    int64
}

// RevisionCapability is one CU capability instance version inside the scope.
type RevisionCapability struct {
	CUID         string
	CapabilityID string
	Version      int64
}

// RevisionBinding is one active metric binding version inside the scope.
// Version is the point binding revision. Safety-constraint edits bump that
// revision, so the constraint version is not hashed separately.
type RevisionBinding struct {
	CUID     string
	MetricID string
	Version  int64
}

// ResourceRevision returns the lowercase hex SHA-256 of the canonical scope snapshot.
func ResourceRevision(
	scope RevisionScope,
	nodes []RevisionNode,
	capabilities []RevisionCapability,
	bindings []RevisionBinding,
) (string, error) {
	if scope.TenantID == "" || scope.ScopeType == "" || scope.ScopeID == "" {
		return "", fmt.Errorf("contracts: revision scope identity is required")
	}
	if scope.ScopeType != RevisionEntitySite &&
		scope.ScopeType != RevisionEntityAsset &&
		scope.ScopeType != RevisionEntityCU {
		return "", fmt.Errorf("contracts: invalid revision scope type %q", scope.ScopeType)
	}

	nodes = slices.Clone(nodes)
	capabilities = slices.Clone(capabilities)
	bindings = slices.Clone(bindings)

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].EntityType != nodes[j].EntityType {
			return nodes[i].EntityType < nodes[j].EntityType
		}
		if nodes[i].EntityID != nodes[j].EntityID {
			return nodes[i].EntityID < nodes[j].EntityID
		}
		return nodes[i].Version < nodes[j].Version
	})
	sort.SliceStable(capabilities, func(i, j int) bool {
		if capabilities[i].CUID != capabilities[j].CUID {
			return capabilities[i].CUID < capabilities[j].CUID
		}
		if capabilities[i].CapabilityID != capabilities[j].CapabilityID {
			return capabilities[i].CapabilityID < capabilities[j].CapabilityID
		}
		return capabilities[i].Version < capabilities[j].Version
	})
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].CUID != bindings[j].CUID {
			return bindings[i].CUID < bindings[j].CUID
		}
		if bindings[i].MetricID != bindings[j].MetricID {
			return bindings[i].MetricID < bindings[j].MetricID
		}
		return bindings[i].Version < bindings[j].Version
	})

	var buf []byte
	buf = appendLenPrefixed(buf, revisionFormatVersion)
	buf = appendLenPrefixed(buf, "scope")
	buf = appendLenPrefixed(buf, scope.TenantID)
	buf = appendLenPrefixed(buf, scope.ScopeType)
	buf = appendLenPrefixed(buf, scope.ScopeID)

	buf = appendLenPrefixed(buf, "nodes")
	buf = appendLenPrefixed(buf, strconv.Itoa(len(nodes)))
	for _, node := range nodes {
		if err := validateRevisionNode(node); err != nil {
			return "", err
		}
		buf = appendLenPrefixed(buf, node.EntityType)
		buf = appendLenPrefixed(buf, node.EntityID)
		buf = appendLenPrefixed(buf, strconv.FormatInt(node.Version, 10))
	}

	buf = appendLenPrefixed(buf, "capabilities")
	buf = appendLenPrefixed(buf, strconv.Itoa(len(capabilities)))
	for _, capability := range capabilities {
		if capability.CUID == "" || capability.CapabilityID == "" || capability.Version <= 0 {
			return "", fmt.Errorf("contracts: capability revision identity is invalid")
		}
		buf = appendLenPrefixed(buf, capability.CUID)
		buf = appendLenPrefixed(buf, capability.CapabilityID)
		buf = appendLenPrefixed(buf, strconv.FormatInt(capability.Version, 10))
	}

	buf = appendLenPrefixed(buf, "bindings")
	buf = appendLenPrefixed(buf, strconv.Itoa(len(bindings)))
	for _, binding := range bindings {
		if binding.CUID == "" || binding.MetricID == "" || binding.Version <= 0 {
			return "", fmt.Errorf("contracts: binding revision identity is invalid")
		}
		buf = appendLenPrefixed(buf, binding.CUID)
		buf = appendLenPrefixed(buf, binding.MetricID)
		buf = appendLenPrefixed(buf, strconv.FormatInt(binding.Version, 10))
	}

	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

func validateRevisionNode(node RevisionNode) error {
	switch node.EntityType {
	case RevisionEntitySite, RevisionEntityAsset, RevisionEntityCU:
	default:
		return fmt.Errorf("contracts: invalid revision entity type %q", node.EntityType)
	}
	if node.EntityID == "" || node.Version <= 0 {
		return fmt.Errorf("contracts: node revision identity is invalid")
	}
	return nil
}

func appendLenPrefixed(dst []byte, value string) []byte {
	dst = strconv.AppendInt(dst, int64(len(value)), 10)
	dst = append(dst, ':')
	dst = append(dst, value...)
	dst = append(dst, '\n')
	return dst
}
