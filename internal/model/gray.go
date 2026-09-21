package model

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// GrayCoverage 按 D4 计算目标覆盖百分比与应放号人数。
// N==0 时调用方应直接转全量，本函数 desired=0。
// k = floor(elapsed/interval)；target = min(100, start+k*step)；desired = ceil(N*target/100)。
func GrayCoverage(n, startPercent, stepPercent, intervalSeconds int, startedAt, now time.Time) (targetPercent, desired int) {
	if n <= 0 {
		return 100, 0
	}
	if intervalSeconds <= 0 {
		intervalSeconds = DefaultGrayIntervalSeconds
	}
	if startPercent < 0 {
		startPercent = 0
	}
	if startPercent > 100 {
		startPercent = 100
	}
	if stepPercent < 0 {
		stepPercent = 0
	}
	elapsed := now.Sub(startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	k := int(math.Floor(elapsed.Seconds() / float64(intervalSeconds)))
	if k < 0 {
		k = 0
	}
	targetPercent = startPercent + k*stepPercent
	if targetPercent > 100 {
		targetPercent = 100
	}
	desired = int(math.Ceil(float64(n) * float64(targetPercent) / 100.0))
	if desired > n {
		desired = n
	}
	return targetPercent, desired
}

// RankClientsForGray 对待放号队列排序。weighted=false：created_at,id ASC。
// weighted=true：tenure×recency 相对名次乘积降序，id ASC 打散。
func RankClientsForGray(remaining []Client, weighted bool) []Client {
	out := append([]Client(nil), remaining...)
	if len(out) == 0 {
		return out
	}
	if !weighted {
		sort.SliceStable(out, func(i, j int) bool {
			if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			return uuidLess(out[i].ID, out[j].ID)
		})
		return out
	}
	m := len(out)
	ageRank := rankBy(out, m, func(a, b Client) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return 0
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1 // older first → will invert to high rank
		}
		return 1
	}, true)
	recencyRank := rankBy(out, m, func(a, b Client) int {
		at, bt := a.LastCheckAt, b.LastCheckAt
		if at == nil && bt == nil {
			return 0
		}
		if at == nil {
			return 1 // null is oldest
		}
		if bt == nil {
			return -1
		}
		if at.Equal(*bt) {
			return 0
		}
		if at.After(*bt) {
			return -1 // more recent first → invert to high
		}
		return 1
	}, true)
	type scored struct {
		c      Client
		weight int
	}
	scoredList := make([]scored, len(out))
	for i := range out {
		scoredList[i] = scored{c: out[i], weight: ageRank[i] * recencyRank[i]}
	}
	sort.SliceStable(scoredList, func(i, j int) bool {
		if scoredList[i].weight != scoredList[j].weight {
			return scoredList[i].weight > scoredList[j].weight
		}
		return uuidLess(scoredList[i].c.ID, scoredList[j].c.ID)
	})
	for i := range scoredList {
		out[i] = scoredList[i].c
	}
	return out
}

func uuidLess(a, b uuid.UUID) bool {
	for i := 0; i < 16; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// rankBy 给出 1..M 名次；invert 时最先比较到的（cmp=-1）得到最高分 M。
// 并列共享较小名次（RANK 语义：同键同 rank）。
func rankBy(list []Client, m int, cmp func(a, b Client) int, invert bool) []int {
	idx := make([]int, len(list))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		c := cmp(list[idx[i]], list[idx[j]])
		if c == 0 {
			return uuidLess(list[idx[i]].ID, list[idx[j]].ID)
		}
		return c < 0
	})
	ranks := make([]int, len(list))
	pos := 1
	for p, i := range idx {
		if p > 0 && cmp(list[idx[p]], list[idx[p-1]]) == 0 {
			ranks[i] = ranks[idx[p-1]]
			pos++
			continue
		}
		ranks[i] = pos
		pos++
	}
	if invert {
		for i := range ranks {
			ranks[i] = m + 1 - ranks[i]
		}
	}
	return ranks
}
