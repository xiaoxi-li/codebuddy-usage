package service

import (
	"math"
	"testing"

	"codebuddy-usage-desktop/internal/api"
)

func fs(s string) api.FlexibleString { return api.FlexibleString(s) }
func pfs(s string) *api.FlexibleString { v := fs(s); return &v }

func TestBuildUsageResult_PreciseFields(t *testing.T) {
	accounts := []api.Account{
		{PackageName: fs("A"), CycleCapacityRemainPrecise: pfs("10.5"), CycleCapacitySizePrecise: pfs("100"), CapacityRemainPrecise: fs("999"), CapacitySizePrecise: fs("999"), CycleEndTime: fs("2026-02-01")},
		{PackageName: fs("B"), CycleCapacityRemainPrecise: pfs("0"), CycleCapacitySizePrecise: pfs("50"), CapacityRemainPrecise: fs("0"), CapacitySizePrecise: fs("50"), CycleEndTime: fs("2026-01-01")},
	}
	r := BuildUsageResult(accounts)
	if math.Abs(r.Remain-10.5) > 1e-9 {
		t.Fatalf("remain = %v, want 10.5", r.Remain)
	}
	// total 只累加 remain>0 的套餐 → 只算 A 的 100，不算 B 的 50
	if math.Abs(r.Total-100) > 1e-9 {
		t.Fatalf("total = %v, want 100", r.Total)
	}
	if r.TotalVisible != 1 {
		t.Fatalf("visible = %d, want 1", r.TotalVisible)
	}
}

func TestBuildUsageResult_FallbackFields(t *testing.T) {
	accounts := []api.Account{
		{PackageName: fs("C"), CapacityRemainPrecise: fs("7.25"), CapacitySizePrecise: fs("20"), CycleEndTime: fs("2026-03-01")},
	}
	r := BuildUsageResult(accounts)
	if math.Abs(r.Remain-7.25) > 1e-9 || math.Abs(r.Total-20) > 1e-9 {
		t.Fatalf("remain=%v total=%v, want 7.25/20", r.Remain, r.Total)
	}
}

func TestBuildUsageResult_SortAndTruncate(t *testing.T) {
	accounts := make([]api.Account, 0, 8)
	for i := 0; i < 8; i++ {
		accounts = append(accounts, api.Account{
			PackageName: fs(string(rune('A' + i))),
			CycleCapacityRemainPrecise: pfs("1"),
			CycleCapacitySizePrecise: pfs("1"),
			CycleEndTime: fs("2026-12-0" + string(rune('1'+i))),
		})
	}
	r := BuildUsageResult(accounts)
	if len(r.Visible) != 5 {
		t.Fatalf("visible rows = %d, want 5", len(r.Visible))
	}
	if r.TotalVisible != 8 {
		t.Fatalf("totalVisible = %d, want 8", r.TotalVisible)
	}
	if r.Visible[0].Name != "A" || r.Visible[4].Name != "E" {
		t.Fatalf("sort/truncate wrong: first=%s last=%s", r.Visible[0].Name, r.Visible[4].Name)
	}
}
