package store

import (
	"database/sql"
	"fmt"
	"time"

	"gateway/internal/model"
)

const modelCols = "id, model_id, display_name, price_input, price_output, price_cache_read, created_at, updated_at"

func scanModel(row interface{ Scan(...any) error }) (*model.Model, error) {
	var m model.Model
	var createdAt, updatedAt int64
	var pi, po, pc sql.NullFloat64
	err := row.Scan(&m.ID, &m.ModelID, &m.DisplayName, &pi, &po, &pc, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if pi.Valid {
		v := pi.Float64
		m.PriceInput = &v
	}
	if po.Valid {
		v := po.Float64
		m.PriceOutput = &v
	}
	if pc.Valid {
		v := pc.Float64
		m.PriceCacheRead = &v
	}
	m.CreatedAt = fromTS(createdAt)
	m.UpdatedAt = fromTS(updatedAt)
	return &m, nil
}

// ListModels 列出全部聚合模型
func (s *Store) ListModels() ([]*model.Model, error) {
	rows, err := s.db.Query("SELECT " + modelCols + " FROM models ORDER BY model_id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.Model, 0)
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetModelByID 按 model_id 字符串取模型
func (s *Store) GetModelByID(modelID string) (*model.Model, error) {
	row := s.db.QueryRow("SELECT "+modelCols+" FROM models WHERE model_id = ?", modelID)
	m, err := scanModel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// GetModelByPK 按主键取模型
func (s *Store) GetModelByPK(id int64) (*model.Model, error) {
	row := s.db.QueryRow("SELECT "+modelCols+" FROM models WHERE id = ?", id)
	m, err := scanModel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// UpsertModel 按 model_id 插入或更新模型,返回模型主键
func (s *Store) UpsertModel(modelID, displayName string) (int64, error) {
	now := ts(time.Now())
	_, err := s.db.Exec(`INSERT INTO models (model_id, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET display_name=excluded.display_name, updated_at=excluded.updated_at`,
		modelID, displayName, now, now)
	if err != nil {
		return 0, err
	}
	m, err := s.GetModelByID(modelID)
	if err != nil {
		return 0, err
	}
	return m.ID, nil
}

// UpdateModelPrice 更新模型价格(元/百万 token;nil 表示清除)
func (s *Store) UpdateModelPrice(id int64, priceInput, priceOutput, priceCacheRead *float64) error {
	_, err := s.db.Exec("UPDATE models SET price_input=?, price_output=?, price_cache_read=?, updated_at=? WHERE id=?",
		nullableFloat(priceInput), nullableFloat(priceOutput), nullableFloat(priceCacheRead), ts(time.Now()), id)
	return err
}

// DeleteModel 删除模型(级联删除 channel_models)
func (s *Store) DeleteModel(id int64) error {
	_, err := s.db.Exec("DELETE FROM models WHERE id = ?", id)
	return err
}

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// ---- Channel-Model 关联 ----

// AddChannelModel 关联渠道与模型(含上游模型名映射;priority=0 表示继承渠道全局优先级)
func (s *Store) AddChannelModel(channelID, modelID int64, upstreamName string) error {
	_, err := s.db.Exec(`INSERT INTO channel_models (channel_id, model_id, upstream_model_name, priority, created_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(channel_id, model_id) DO UPDATE SET upstream_model_name=excluded.upstream_model_name`,
		channelID, modelID, upstreamName, ts(time.Now()))
	return err
}

// RemoveChannelModel 移除关联
func (s *Store) RemoveChannelModel(channelID, modelID int64) error {
	_, err := s.db.Exec("DELETE FROM channel_models WHERE channel_id=? AND model_id=?", channelID, modelID)
	return err
}

// ListChannelModels 某渠道的模型关联
func (s *Store) ListChannelModels(channelID int64) ([]*model.ChannelModel, error) {
	rows, err := s.db.Query(`SELECT id, channel_id, model_id, upstream_model_name, created_at FROM channel_models WHERE channel_id=?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.ChannelModel, 0)
	for rows.Next() {
		var cm model.ChannelModel
		var createdAt int64
		if err := rows.Scan(&cm.ID, &cm.ChannelID, &cm.ModelID, &cm.UpstreamModelName, &createdAt); err != nil {
			return nil, err
		}
		cm.CreatedAt = fromTS(createdAt)
		out = append(out, &cm)
	}
	return out, rows.Err()
}

// ModelWithChannels 模型 + 其可用渠道(含上游映射与渠道状态),用于模型聚合页与路由
type ModelWithChannels struct {
	*model.Model
	Channels []ChannelRef `json:"channels"`
}

// ChannelRef 模型在某渠道上的引用
// Priority 为实际生效优先级(模型级自定义 >0 时取自定义,否则继承渠道全局);
// ModelPriority 为模型级原始值,0 = 继承渠道全局优先级(前端可提示)。
type ChannelRef struct {
	ChannelID         int64  `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	Priority          int    `json:"priority"`
	ModelPriority     int    `json:"model_priority"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	UpstreamModelName string `json:"upstream_model_name"`
}

// ListModelsWithChannels 聚合模型 + 渠道引用(路由候选来源)。
// 模型级优先级:>0 为拖拽自定义;0 表示继承渠道全局优先级。
func (s *Store) ListModelsWithChannels() ([]*ModelWithChannels, error) {
	models, err := s.ListModels()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT cm.model_id, c.id, c.name,
		CASE WHEN cm.priority > 0 THEN cm.priority ELSE c.priority END AS eff_priority,
		cm.priority, c.enabled, c.status, cm.upstream_model_name
		FROM channel_models cm
		JOIN channels c ON c.id = cm.channel_id
		JOIN models m ON m.id = cm.model_id
		ORDER BY eff_priority ASC, c.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := map[int64][]ChannelRef{} // model_id -> refs
	for rows.Next() {
		var modelPK int64
		var r ChannelRef
		var enabled int
		if err := rows.Scan(&modelPK, &r.ChannelID, &r.ChannelName, &r.Priority, &r.ModelPriority, &enabled, &r.Status, &r.UpstreamModelName); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		refs[modelPK] = append(refs[modelPK], r)
	}
	out := make([]*ModelWithChannels, 0, len(models))
	for _, m := range models {
		ch := refs[m.ID]
		if ch == nil {
			ch = []ChannelRef{}
		}
		out = append(out, &ModelWithChannels{Model: m, Channels: ch})
	}
	return out, rows.Err()
}

// ReorderModelChannels 批量更新某模型关联渠道的模型级优先级(拖拽排序保存)。
// 仅影响该模型的调用顺序,不修改渠道全局优先级。
func (s *Store) ReorderModelChannels(modelID int64, items []ChannelPriority) error {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(items))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, it := range items {
		if it.ID <= 0 {
			return fmt.Errorf("invalid channel id %d", it.ID)
		}
		if it.Priority < 1 {
			return fmt.Errorf("priority 必须 >= 1(渠道 %d)", it.ID)
		}
		if _, dup := seen[it.ID]; dup {
			return fmt.Errorf("重复的渠道 id %d", it.ID)
		}
		seen[it.ID] = struct{}{}
		if _, err := tx.Exec("UPDATE channel_models SET priority=? WHERE channel_id=? AND model_id=?", it.Priority, it.ID, modelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetModelWithChannels 取单个模型 + 渠道引用
func (s *Store) GetModelWithChannels(modelID string) (*ModelWithChannels, error) {
	all, err := s.ListModelsWithChannels()
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if m.ModelID == modelID {
			return m, nil
		}
	}
	return nil, nil
}
