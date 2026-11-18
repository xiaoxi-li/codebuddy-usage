package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"codebuddy-usage-desktop/internal/api"
)

type CheckinResult struct {
	State          string  `json:"state"` // claimed / unclaimed / unknown
	Credit         float64 `json:"credit,omitempty"`
	FreshlyClaimed bool    `json:"freshlyClaimed,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func parseCredit(n json.Number) float64 {
	if n.String() == "" {
		return 0
	}
	var f float64
	if err := json.Unmarshal([]byte(n.String()), &f); err != nil {
		return 0
	}
	return f
}

func checkinError(status int, env *api.APIEnvelope) string {
	body := ""
	if env != nil && len(env.Data) > 0 {
		body = string(env.Data)
	} else {
		body = fmt.Sprintf("HTTP_%d", status)
	}
	if len(body) > 300 {
		body = body[:300] + "…"
	}
	return body
}

// FetchCheckinStatus 查询今日签到状态。
func FetchCheckinStatus(client *api.Client) (CheckinResult, error) {
	env, err := client.DoEnvelope("POST", "/billing/meter/checkin-status", client.CheckinHeaders(), map[string]interface{}{})
	if err != nil {
		if isCredentialError(err) {
			return CheckinResult{}, err
		}
		return CheckinResult{State: "unknown", Error: err.Error()}, nil
	}
	if env.Code == 10001 {
		return CheckinResult{State: "claimed"}, nil
	}
	var data api.CheckinStatusData
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &data)
	}
	if data.TodayCheckedIn {
		return CheckinResult{State: "claimed"}, nil
	}
	return CheckinResult{State: "unclaimed"}, nil
}

// DoCheckin 执行签到领取。
func DoCheckin(client *api.Client) (CheckinResult, error) {
	env, err := client.DoEnvelope("POST", "/billing/meter/daily-checkin", client.CheckinHeaders(), map[string]interface{}{})
	if err != nil {
		if isCredentialError(err) {
			return CheckinResult{}, err
		}
		return CheckinResult{State: "unknown", Error: err.Error()}, nil
	}
	if env.Code == 10001 {
		return CheckinResult{State: "claimed"}, nil
	}
	if env.Code != 0 {
		return CheckinResult{State: "unknown", Error: fmt.Sprintf("code=%d msg=%s", env.Code, env.Msg)}, nil
	}
	var data api.CheckinData
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &data)
	}
	return CheckinResult{State: "claimed", Credit: parseCredit(data.Credit), FreshlyClaimed: true}, nil
}

// EnsureCheckin 查询状态，未签到则领取。
func EnsureCheckin(client *api.Client) (CheckinResult, error) {
	status, err := FetchCheckinStatus(client)
	if err != nil || status.State != "unclaimed" {
		return status, err
	}
	return DoCheckin(client)
}

func isCredentialError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no_credentials") || strings.Contains(s, "auth_expired")
}
