package core

import (
	"encoding/json"
	"fmt"
	"time"

	"codebuddy-usage-desktop/internal/auth"
	"codebuddy-usage-desktop/internal/service"
)

// CheckinView 是快照中的签到状态。
type CheckinView struct {
	State          string  `json:"state"`
	Credit         float64 `json:"credit,omitempty"`
	FreshlyClaimed bool    `json:"freshlyClaimed,omitempty"`
	Error          string  `json:"error,omitempty"`
}

// UsageSnapshot 是推送给前端的只读快照。
type UsageSnapshot struct {
	Remain    float64          `json:"remain"`
	Total     float64          `json:"total"`
	Visible   []service.PlanRow `json:"visible"`
	TotalVisible int           `json:"totalVisible"`
	Profile   auth.Profile     `json:"profile"`
	Checkin   CheckinView      `json:"checkin"`
	Buddy     *service.BuddyView `json:"buddy"`
	AuthNote  string           `json:"authNote"`
	Mode      string           `json:"mode"`
	ExpiresAt int64            `json:"expiresAt"`
	UpdatedAt string           `json:"updatedAt"`
}

func (s UsageSnapshot) ToJSON() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func formatUpdatedAt(t time.Time) string {
	return t.Format("2006/01/02 15:04:05")
}

func newErrorSnapshot(note string) UsageSnapshot {
	return UsageSnapshot{
		Checkin:  CheckinView{State: "unknown", Error: note},
		AuthNote: note,
		Mode:     "none",
		UpdatedAt: formatUpdatedAt(time.Now()),
	}
}

func snapshotFrom(authInfo *auth.AuthInfo, usage service.UsageResult, checkin service.CheckinResult, buddy *service.BuddyView, note string) UsageSnapshot {
	mode := string(authInfo.Mode)
	snap := UsageSnapshot{
		Remain: usage.Remain, Total: usage.Total,
		Visible: usage.Visible, TotalVisible: usage.TotalVisible,
		Profile: authInfo.Profile,
		Checkin: CheckinView{
			State: checkin.State, Credit: checkin.Credit,
			FreshlyClaimed: checkin.FreshlyClaimed, Error: checkin.Error,
		},
		Buddy: buddy, AuthNote: note, Mode: mode,
		ExpiresAt: authInfo.ExpiresAt,
		UpdatedAt: formatUpdatedAt(time.Now()),
	}
	if snap.Visible == nil {
		snap.Visible = []service.PlanRow{}
	}
	return snap
}

func describeAuthError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s", err)
}
