package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"codebuddy-usage-desktop/internal/api"
)

const buddyPath = "/activity/growth/buddy/travel"

// BuddyView 是发送给前端的喵喵旅行视图。
type BuddyView struct {
	State            string  `json:"state"`
	DepartAt         *int64  `json:"departAt"`
	ArriveAt         *int64  `json:"arriveAt"`
	ServerNow        *int64  `json:"serverNow"`
	DurationHours    float64 `json:"durationHours"`
	DailyLimit       bool    `json:"dailyLimit"`
	LocationName     string  `json:"locationName"`
	CountdownSeconds int64   `json:"countdownSeconds"`
	ClaimCredit      *int    `json:"claimCredit,omitempty"`
	ClaimError       string  `json:"claimError,omitempty"`
	DepartError      string  `json:"departError,omitempty"`
	FreshlyClaimed   bool    `json:"freshlyClaimed,omitempty"`
	FreshlyDeparted  bool    `json:"freshlyDeparted,omitempty"`
	FreshlyFailed    bool    `json:"freshlyFailed,omitempty"`
}

// BuddyService 封装 status/claim/depart/records 调用与旅行守卫状态机。
type BuddyService struct {
	mu                sync.Mutex
	lastClaimKey      string
	lastAutoDate      string
	lastClaimErrorDate string
	lastWasTraveling  bool
}

func NewBuddyService() *BuddyService { return &BuddyService{} }

func todayStr() string {
	return time.Now().Format("2006-01-02")
}

func callBuddy(client *api.Client, sub string, method string, body interface{}) (*api.APIEnvelope, error) {
	return client.DoEnvelope(method, buddyPath+"/"+sub, client.BuddyHeaders(), body)
}

func fetchBuddyStatus(client *api.Client) (*api.BuddyStatus, error) {
	env, err := callBuddy(client, "status", "GET", nil)
	if err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("status code=%d msg=%s", env.Code, env.Msg)
	}
	var st api.BuddyStatus
	if err := json.Unmarshal(env.Data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func fetchTravelRewardCredit(client *api.Client, departAt *int64) int {
	env, err := callBuddy(client, "records?page=1&page_size=20", "GET", nil)
	if err != nil || env.Code != 0 {
		return 0
	}
	var data api.BuddyRecordsData
	if err := json.Unmarshal(env.Data, &data); err != nil || len(data.Records) == 0 {
		return 0
	}
	record := data.Records[0]
	if departAt != nil {
		for _, r := range data.Records {
			if r.DepartAt != nil && *r.DepartAt == *departAt {
				record = r
				break
			}
		}
	}
	if record.RewardCredit == nil {
		return 0
	}
	return *record.RewardCredit
}

func resolveHours(st *api.BuddyStatus) float64 {
	if st == nil {
		return 0
	}
	if st.DurationHours != nil && *st.DurationHours > 0 {
		return float64(*st.DurationHours)
	}
	if st.Location != nil && st.Location.DurationHours > 0 {
		return st.Location.DurationHours
	}
	if st.DepartAt != nil && st.ArriveAt != nil && *st.ArriveAt > *st.DepartAt {
		h := float64(*st.ArriveAt-*st.DepartAt) / 3600
		return math.Round(h*10) / 10
	}
	return 0
}

func buildView(st *api.BuddyStatus) *BuddyView {
	if st == nil {
		return nil
	}
	view := &BuddyView{
		State: st.State, DepartAt: st.DepartAt, ArriveAt: st.ArriveAt,
		ServerNow: st.ServerNow, DurationHours: resolveHours(st),
		DailyLimit: st.DailyLimit, LocationName: st.LocationName,
	}
	if st.Location != nil && view.LocationName == "" {
		view.LocationName = st.Location.Name
	}
	if st.State == "traveling" && st.ArriveAt != nil {
		now := time.Now().Unix()
		if st.ServerNow != nil {
			now = *st.ServerNow
		}
		if remain := *st.ArriveAt - now; remain > 0 {
			view.CountdownSeconds = remain
		}
	}
	return view
}

func creditFromNumber(n json.Number) int {
	if n.String() == "" {
		return 0
	}
	var f float64
	if err := json.Unmarshal([]byte(n.String()), &f); err != nil {
		return 0
	}
	return int(f)
}

func claimBuddy(client *api.Client) (int, bool, error) {
	env, err := callBuddy(client, "claim", "POST", map[string]interface{}{})
	if err != nil {
		return 0, false, err
	}
	if env.Code == 0 {
		var data api.BuddyClaimData
		if len(env.Data) > 0 {
			_ = json.Unmarshal(env.Data, &data)
		}
		return creditFromNumber(data.Credit), true, nil
	}
	if strings.Contains(strings.ToLower(env.Msg), "no unclaimed") {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("%s", env.Msg)
}

func departBuddy(client *api.Client) (float64, error) {
	env, err := callBuddy(client, "depart", "POST", map[string]interface{}{"location_id": 1})
	if err != nil {
		return 0, err
	}
	if env.Code != 0 {
		return 0, fmt.Errorf("%s", env.Msg)
	}
	hours := resolveHoursQuiet(func() (*api.BuddyStatus, error) { return fetchBuddyStatus(client) })
	if hours > 0 {
		return hours, nil
	}
	var data api.BuddyDepartData
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &data)
	}
	for _, n := range []json.Number{data.DurationHours, data.Duration} {
		var f float64
		if err := json.Unmarshal([]byte(n.String()), &f); err == nil && f > 0 {
			return f, nil
		}
	}
	return 0, nil
}

func resolveHoursQuiet(fn func() (*api.BuddyStatus, error)) float64 {
	st, err := fn()
	if err != nil {
		return 0
	}
	return resolveHours(st)
}

// EnsureBuddy 自动领积分并派出下一趟，返回用于展示的视图。
func (s *BuddyService) EnsureBuddy(client *api.Client, enabled bool) *BuddyView {
	if !enabled {
		return nil
	}
	st, err := fetchBuddyStatus(client)
	if err != nil || st == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	view := buildView(st)
	isTraveling := st.State == "traveling"
	backFromTravel := st.State != "" && !isTraveling
	travelKey := ""
	if st.DepartAt != nil {
		travelKey = fmt.Sprintf("d%d", *st.DepartAt)
	} else if st.ArriveAt != nil {
		travelKey = fmt.Sprintf("a%d", *st.ArriveAt)
	} else {
		travelKey = st.State
	}
	travelJustEnded := s.lastWasTraveling && !isTraveling

	if backFromTravel && (travelJustEnded || travelKey != s.lastClaimKey) {
		credit, claimed, err := claimBuddy(client)
		if err != nil {
			view.ClaimError = err.Error()
			if s.lastClaimErrorDate != todayStr() {
				view.FreshlyFailed = true
				s.lastClaimErrorDate = todayStr()
			}
		} else {
			if claimed && credit <= 0 {
				credit = fetchTravelRewardCredit(client, st.DepartAt)
			}
			view.ClaimCredit = &credit
			if claimed {
				view.FreshlyClaimed = true
			}
			s.lastClaimKey = travelKey
		}
	}

	canDepart := backFromTravel && !st.DailyLimit
	if canDepart && s.lastAutoDate != todayStr() {
		hours, err := departBuddy(client)
		if err == nil {
			view.DurationHours = hours
			view.FreshlyDeparted = true
		} else {
			view.DepartError = err.Error()
		}
		if st2, e := fetchBuddyStatus(client); e == nil && st2 != nil {
			view = buildView(st2)
			view.FreshlyDeparted = view.FreshlyDeparted || hours > 0
		}
		s.lastAutoDate = todayStr()
	}

	s.lastWasTraveling = isTraveling
	return view
}

// ManualClaim 手动领积分。
func (s *BuddyService) ManualClaim(client *api.Client) (int, bool, error) {
	credit, claimed, err := claimBuddy(client)
	if err == nil && claimed && credit <= 0 {
		st, _ := fetchBuddyStatus(client)
		var departAt *int64
		if st != nil {
			departAt = st.DepartAt
		}
		credit = fetchTravelRewardCredit(client, departAt)
	}
	return credit, claimed, err
}

// ManualDepart 手动派出。
func (s *BuddyService) ManualDepart(client *api.Client) (float64, error) {
	return departBuddy(client)
}

// FetchStatus 查询当前喵喵状态（供手动操作后刷新）。
func (s *BuddyService) FetchStatus(client *api.Client) *BuddyView {
	st, err := fetchBuddyStatus(client)
	if err != nil {
		return nil
	}
	return buildView(st)
}
