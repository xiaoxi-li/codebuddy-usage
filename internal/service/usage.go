package service

import (
	"math"
	"sort"
	"strconv"
	"time"

	"codebuddy-usage-desktop/internal/api"
)

var defaultPackageCodes = []string{
	"TCACA_code_008_cfWoLwvjU4",
	"TCACA_code_009_0XmEQc2xOf",
	"TCACA_code_038_OhvqZtiPKr",
	"TCACA_code_007_nzdH5h4Nl0",
	"TCACA_code_028_NtpWi0jzXs",
	"TCACA_code_029_6wCGEWquYy",
	"TCACA_code_030_BjSt89qTvr",
}

type PlanRow struct {
	Name    string  `json:"name"`
	Remain  float64 `json:"remain"`
	Total   float64 `json:"total"`
	Expires string  `json:"expires"`
}

type UsageResult struct {
	Remain   float64   `json:"remain"`
	Total    float64   `json:"total"`
	Accounts []api.Account `json:"-"`
	Visible  []PlanRow `json:"visible"`
	TotalVisible int `json:"totalVisible"`
}

func parseFloat(s api.FlexibleString) float64 {
	str := s.String()
	if str == "" {
		return 0
	}
	v, err := strconv.ParseFloat(str, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func accountValues(a api.Account) (float64, float64) {
	if a.CycleCapacityRemainPrecise != nil && a.CycleCapacitySizePrecise != nil {
		return parseFloat(*a.CycleCapacityRemainPrecise), parseFloat(*a.CycleCapacitySizePrecise)
	}
	return parseFloat(a.CapacityRemainPrecise), parseFloat(a.CapacitySizePrecise)
}

func parseExpiry(s api.FlexibleString) time.Time {
	str := s.String()
	if str == "" {
		return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, str, time.Local); err == nil {
			return t
		}
	}
	return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
}

// BuildUsageResult 按原扩展规则汇总用量并筛选明细。
func BuildUsageResult(accounts []api.Account) UsageResult {
	result := UsageResult{Accounts: accounts}
	visible := make([]api.Account, 0, len(accounts))
	for _, a := range accounts {
		remain, size := accountValues(a)
		result.Remain += remain
		if remain > 0 {
			result.Total += size
			visible = append(visible, a)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		return parseExpiry(visible[i].CycleEndTime).Before(parseExpiry(visible[j].CycleEndTime))
	})
	result.TotalVisible = len(visible)
	for i, a := range visible {
		if i >= 5 {
			break
		}
		remain, size := accountValues(a)
		result.Visible = append(result.Visible, PlanRow{
			Name: a.PackageName.String(), Remain: remain, Total: size, Expires: a.CycleEndTime.String(),
		})
	}
	return result
}

type usageRequestBody struct {
	PageNumber      int      `json:"PageNumber"`
	PageSize        int      `json:"PageSize"`
	ProductCode     string   `json:"ProductCode"`
	Status          []int    `json:"Status"`
	OnlyValidPeriod bool     `json:"OnlyValidPeriod"`
	PackageCodes    []string `json:"PackageCodes"`
	NeedInUsage     bool     `json:"NeedInUsage"`
}

// FetchUsage 拉取用量。
func FetchUsage(client *api.Client) (UsageResult, error) {
	var resp api.UsageResponse
	body := usageRequestBody{
		PageNumber: 1, PageSize: 200, ProductCode: "p_tcaca", Status: []int{0, 3},
		OnlyValidPeriod: true, PackageCodes: defaultPackageCodes, NeedInUsage: true,
	}
	_, err := client.DoJSON("POST", "/billing/meter/get-user-resource", client.UsageHeaders(), body, &resp)
	if err != nil {
		return UsageResult{}, err
	}
	return BuildUsageResult(resp.Data.Response.Data.Accounts), nil
}
