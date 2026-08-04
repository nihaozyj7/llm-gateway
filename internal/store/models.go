package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const modelCols = "id, model_id, display_name, price_input, price_output, created_at, updated_at"

func scanModel(row interface{ Scan(...any) error }) (*model.Model, error) {
	var m model.Model
	var createdAt, updatedAt int64
	var pi, po sql.NullFloat64
	err := row.Scan(&m.ID, &m.ModelID, &m.DisplayName, &pi, &po, &createdAt, &updatedAt)
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

// UpdateModelPrice 更新模型价格(元/千 token;nil 表示清除)
func (s *Store) UpdateModelPrice(id int64, priceInput, priceOutput *float64) error {
	_, err := s.db.Exec("UPDATE models SET price_input=?, price_output=?, updated_at=? WHERE id=?",
		nullableFloat(priceInput), nullableFloat(priceOutput), ts(time.Now()), id)
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

// AddChannelModel 关联渠道与模型(含上游模型名映射)
func (s *Store) AddChannelModel(channelID, modelID int64, upstreamName string) error {
	_, err := s.db.Exec(`INSERT INTO channel_models (channel_id, model_id, upstream_model_name, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(channel_id, model_id) DO UPDATE SET upstream_model_name=excluded.upstream_model_name`,
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
type ChannelRef struct {
	ChannelID         int64  `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	Priority          int    `json:"priority"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	UpstreamModelName string `json:"upstream_model_name"`
}

// ListModelsWithChannels 聚合模型 + 渠道引用(路由候选来源)
func (s *Store) ListModelsWithChannels() ([]*ModelWithChannels, error) {
	models, err := s.ListModels()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT cm.model_id, c.id, c.name, c.priority, c.enabled, c.status, cm.upstream_model_name
		FROM channel_models cm
		JOIN channels c ON c.id = cm.channel_id
		JOIN models m ON m.id = cm.model_id
		ORDER BY c.priority ASC, c.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := map[int64][]ChannelRef{} // model_id -> refs
	for rows.Next() {
		var modelPK int64
		var r ChannelRef
		var enabled int
		if err := rows.Scan(&modelPK, &r.ChannelID, &r.ChannelName, &r.Priority, &enabled, &r.Status, &r.UpstreamModelName); err != nil {
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
