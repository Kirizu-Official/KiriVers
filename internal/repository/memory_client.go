package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func clientHashKey(projectID uuid.UUID, hash string) string {
	return projectID.String() + "|" + hash
}

func cloneClient(c *model.Client) *model.Client {
	if c == nil {
		return nil
	}
	cp := *c
	if c.Custom != nil {
		cp.Custom = model.JSONObject{}
		for k, v := range c.Custom {
			cp.Custom[k] = v
		}
	} else {
		cp.Custom = model.JSONObject{}
	}
	if c.LastCheckAt != nil {
		t := *c.LastCheckAt
		cp.LastCheckAt = &t
	}
	cp.GeoI18n = c.GeoI18n.Clone()
	return &cp
}

func (m *MemoryProjectStore) UpsertClientLogin(_ context.Context, row *model.Client, custom model.JSONObject, now time.Time) (*model.Client, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertClientLocked(row, custom, true, now)
}

func (m *MemoryProjectStore) UpsertClientCheck(_ context.Context, row *model.Client, now time.Time) (*model.Client, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertClientLocked(row, nil, false, now)
}

func (m *MemoryProjectStore) upsertClientLocked(row *model.Client, custom model.JSONObject, writeCustom bool, now time.Time) (*model.Client, bool, error) {
	if row == nil || row.DeviceHash == "" {
		return nil, false, fmt.Errorf("client upsert requires device_hash")
	}
	key := clientHashKey(row.ProjectID, row.DeviceHash)
	if id, ok := m.clientByHash[key]; ok {
		existing := m.clients[id]
		existing.LastVersion = row.LastVersion
		existing.LastOS = row.LastOS
		existing.LastArch = row.LastArch
		existing.LastChannel = row.LastChannel
		existing.LastIP = row.LastIP
		existing.CountryCode = row.CountryCode
		existing.RegionCode = row.RegionCode
		existing.GeoI18n = row.GeoI18n.Clone()
		t := now
		existing.LastCheckAt = &t
		existing.UpdatedAt = now
		if writeCustom {
			if custom == nil {
				custom = model.JSONObject{}
			}
			existing.Custom = custom
		}
		m.bumpDailyLocked(row.ProjectID, now, false, true)
		return cloneClient(existing), false, nil
	}
	if err := row.BeforeCreate(nil); err != nil {
		return nil, false, err
	}
	row.CreatedAt = now
	row.UpdatedAt = now
	t := now
	row.LastCheckAt = &t
	if writeCustom {
		if custom == nil {
			custom = model.JSONObject{}
		}
		row.Custom = custom
	} else if row.Custom == nil {
		row.Custom = model.JSONObject{}
	}
	m.clients[row.ID] = cloneClient(row)
	m.clientByHash[key] = row.ID
	m.bumpDailyLocked(row.ProjectID, now, true, true)
	return cloneClient(row), true, nil
}

func (m *MemoryProjectStore) bumpDailyLocked(projectID uuid.UUID, now time.Time, isNew, isActive bool) {
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	key := projectID.String() + "|" + day.Format("2006-01-02")
	row := m.dailyStats[key]
	if row == nil {
		row = &model.ClientDailyStats{ProjectID: projectID, Day: day}
		m.dailyStats[key] = row
	}
	if isNew {
		row.NewCount++
	}
	if isActive {
		row.ActiveCount++
	}
	row.UpdatedAt = now.UTC()
}

func (m *MemoryProjectStore) GetClientByID(_ context.Context, projectID, id uuid.UUID) (*model.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clients[id]
	if !ok || c.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneClient(c), nil
}

func (m *MemoryProjectStore) GetClientByDeviceHash(_ context.Context, projectID uuid.UUID, hash string) (*model.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.clientByHash[clientHashKey(projectID, hash)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneClient(m.clients[id]), nil
}

func (m *MemoryProjectStore) DeleteClient(_ context.Context, projectID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clients[id]
	if !ok || c.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	delete(m.clientByHash, clientHashKey(projectID, c.DeviceHash))
	delete(m.clients, id)
	return nil
}

func (m *MemoryProjectStore) ListClients(_ context.Context, projectID uuid.UUID, filter ClientListFilter) ([]model.Client, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []model.Client
	q := strings.ToLower(strings.TrimSpace(filter.Q))
	for _, c := range m.clients {
		if c.ProjectID != projectID {
			continue
		}
		if os := strings.TrimSpace(filter.OS); os != "" && c.LastOS != os {
			continue
		}
		if arch := strings.TrimSpace(filter.Arch); arch != "" && c.LastArch != arch {
			continue
		}
		if ver := strings.TrimSpace(filter.Version); ver != "" && c.LastVersion != ver {
			continue
		}
		if filter.ActiveSince != nil && (c.LastCheckAt == nil || c.LastCheckAt.Before(filter.ActiveSince.UTC())) {
			continue
		}
		if q != "" {
			blob := strings.ToLower(c.LastVersion + " " + c.LastOS + " " + c.LastArch + " " + c.LastChannel + " " + c.LastIP)
			custom := ""
			for k, v := range c.Custom {
				custom += " " + k + fmt.Sprint(v)
			}
			if !strings.Contains(blob, q) && !strings.Contains(strings.ToLower(custom), q) {
				continue
			}
		}
		all = append(all, *cloneClient(c))
	}
	sort.Slice(all, func(i, j int) bool {
		ai, aj := all[i].LastCheckAt, all[j].LastCheckAt
		if ai == nil && aj == nil {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		}
		if ai == nil {
			return false
		}
		if aj == nil {
			return true
		}
		return ai.After(*aj)
	})
	total := int64(len(all))
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	off := filter.Offset
	if off > len(all) {
		return []model.Client{}, total, nil
	}
	end := off + limit
	if end > len(all) {
		end = len(all)
	}
	return all[off:end], total, nil
}

func (m *MemoryProjectStore) CountClients(_ context.Context, projectID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, c := range m.clients {
		if c.ProjectID == projectID {
			n++
		}
	}
	return n, nil
}

func (m *MemoryProjectStore) ListClientsByIDs(_ context.Context, projectID uuid.UUID, ids []uuid.UUID) ([]model.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	wanted := map[uuid.UUID]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var list []model.Client
	for _, c := range m.clients {
		if c.ProjectID == projectID && wanted[c.ID] {
			list = append(list, *cloneClient(c))
		}
	}
	return list, nil
}

func (m *MemoryProjectStore) CountActiveClients(_ context.Context, projectID uuid.UUID, since time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, c := range m.clients {
		if c.ProjectID == projectID && c.LastCheckAt != nil && !c.LastCheckAt.Before(since.UTC()) {
			n++
		}
	}
	return n, nil
}

func (m *MemoryProjectStore) ClientBuckets(_ context.Context, projectID uuid.UUID, now time.Time) (ClientBuckets, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := ClientBuckets{}
	ver := map[string]int64{}
	osC := map[string]int64{}
	archC := map[string]int64{}
	chC := map[string]int64{}
	countryC := map[string]int64{}
	countryN := map[string]map[string]string{}
	rec := map[int64]int64{}
	since24 := now.Add(-24 * time.Hour)
	since7 := now.Add(-7 * 24 * time.Hour)
	for _, c := range m.clients {
		if c.ProjectID != projectID {
			continue
		}
		out.Total++
		if c.LastCheckAt != nil && !c.LastCheckAt.Before(since24) {
			out.Active24h++
		}
		if c.LastCheckAt != nil && !c.LastCheckAt.Before(since7) {
			out.Active7d++
		}
		ver[c.LastVersion]++
		osC[c.LastOS]++
		archC[c.LastArch]++
		chC[c.LastChannel]++
		code := c.CountryCode
		countryC[code]++
		if countryN[code] == nil {
			countryN[code] = map[string]string{}
		}
		for k, v := range c.GeoI18n.Country {
			if v == "" {
				continue
			}
			if _, ok := countryN[code][k]; !ok {
				countryN[code][k] = v
			}
		}
		if c.LastCheckAt != nil {
			h := int64(now.Sub(*c.LastCheckAt).Hours())
			if h < 0 {
				h = 0
			}
			rec[h]++
		}
	}
	out.Versions = nameCounts(ver)
	out.OS = nameCounts(osC)
	out.Arch = nameCounts(archC)
	out.Channels = nameCounts(chC)
	out.Countries = countryCounts(countryC, countryN)
	hours := make([]int64, 0, len(rec))
	for h := range rec {
		hours = append(hours, h)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })
	for _, h := range hours {
		out.RecencyHours = append(out.RecencyHours, NameCount{Name: fmt.Sprintf("%d", h), Count: rec[h]})
	}
	return out, nil
}

func nameCounts(in map[string]int64) []NameCount {
	out := make([]NameCount, 0, len(in))
	for k, v := range in {
		out = append(out, NameCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func countryCounts(counts map[string]int64, names map[string]map[string]string) []CountryCount {
	out := make([]CountryCount, 0, len(counts))
	for code, n := range counts {
		nm := names[code]
		if nm == nil {
			nm = map[string]string{}
		}
		out = append(out, CountryCount{Code: code, Count: n, Names: nm})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func (m *MemoryProjectStore) ListClientDailyStats(_ context.Context, projectID uuid.UUID, from, to time.Time) ([]model.ClientDailyStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.ClientDailyStats
	for _, row := range m.dailyStats {
		if row.ProjectID != projectID {
			continue
		}
		if !from.IsZero() && row.Day.Before(from.UTC().Truncate(24*time.Hour)) {
			continue
		}
		if !to.IsZero() && row.Day.After(to.UTC().Truncate(24*time.Hour)) {
			continue
		}
		list = append(list, *row)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Day.Before(list[j].Day) })
	return list, nil
}

func (m *MemoryProjectStore) InsertGraySnapshot(_ context.Context, snap *model.GrayRolloutSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if snap == nil {
		return nil
	}
	if err := snap.BeforeCreate(nil); err != nil {
		return err
	}
	cp := *snap
	m.graySnapshots = append(m.graySnapshots, &cp)
	return nil
}

func (m *MemoryProjectStore) ListGraySnapshots(_ context.Context, versionID uuid.UUID, from, to time.Time) ([]model.GrayRolloutSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.GrayRolloutSnapshot
	for _, s := range m.graySnapshots {
		if s.VersionID != versionID {
			continue
		}
		if !from.IsZero() && s.TakenAt.Before(from.UTC()) {
			continue
		}
		if !to.IsZero() && s.TakenAt.After(to.UTC()) {
			continue
		}
		list = append(list, *s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].TakenAt.Before(list[j].TakenAt) })
	return list, nil
}
