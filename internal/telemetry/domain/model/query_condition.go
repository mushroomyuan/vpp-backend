package model

import (
	"errors"
	"time"
)

// QueryCondition 时序数据查询条件（领域值对象）
type QueryCondition struct {
	TenantID   string
	CUCode     string
	MetricName string
	StartTime  time.Time
	EndTime    time.Time
}

// NewQueryCondition 领域工厂函数
func NewQueryCondition(tenantID, cuCode, metric string, start, end time.Time) QueryCondition {
	return QueryCondition{
		TenantID:   tenantID,
		CUCode:     cuCode,
		MetricName: metric,
		StartTime:  start,
		EndTime:    end,
	}
}

// Validate 领域能力：校验查询条件的结构性约束。
// 查询时间窗口的上限（30 天）是部署策略，属于应用层关注点，不在此处强制。
func (q QueryCondition) Validate() error {
	if q.TenantID == "" || q.CUCode == "" {
		return errors.New("domain_err: query condition must specify tenant_id and cu_code")
	}
	if q.StartTime.After(q.EndTime) {
		return errors.New("domain_err: start_time cannot be after end_time")
	}
	return nil
}
