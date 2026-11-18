package api

import "encoding/json"

// UsageResponse 对应 /billing/meter/get-user-resource 的响应。
type UsageResponse struct {
	Code int `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Response struct {
			Data struct {
				Accounts []Account `json:"Accounts"`
			} `json:"Data"`
		} `json:"Response"`
	} `json:"data"`
}

// FlexibleString 兼容服务端在数字与字符串间摇摆的字段。
type FlexibleString string

func (f *FlexibleString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = FlexibleString(s)
		return nil
	}
	*f = FlexibleString(string(data))
	return nil
}

func (f FlexibleString) String() string { return string(f) }

// Account 是单个套餐。
type Account struct {
	AccountID                  FlexibleString  `json:"AccountId"`
	PackageCode                FlexibleString  `json:"PackageCode"`
	PackageName                FlexibleString  `json:"PackageName"`
	CapacityRemainPrecise      FlexibleString  `json:"CapacityRemainPrecise"`
	CapacitySizePrecise        FlexibleString  `json:"CapacitySizePrecise"`
	CycleCapacityRemainPrecise *FlexibleString `json:"CycleCapacityRemainPrecise"`
	CycleCapacitySizePrecise   *FlexibleString `json:"CycleCapacitySizePrecise"`
	CycleEndTime               FlexibleString  `json:"CycleEndTime"`
}

// APIEnvelope 是通用响应壳。
type APIEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// CheckinStatusData 是签到状态响应 data。
type CheckinStatusData struct {
	TodayCheckedIn bool `json:"today_checked_in"`
}

// CheckinData 是签到响应 data。
type CheckinData struct {
	Credit json.Number `json:"credit"`
}

// BuddyStatus 是喵喵状态。
type BuddyStatus struct {
	State            string `json:"state"`
	DepartAt         *int64 `json:"depart_at"`
	ArriveAt         *int64 `json:"arrive_at"`
	ServerNow        *int64 `json:"server_now"`
	DurationHours    *int64 `json:"duration_hours"`
	RewardCredit     *int   `json:"reward_credit"`
	DailyLimit       bool   `json:"daily_limit_reached"`
	LocationName     string `json:"location_name"`
	Location         *struct {
		Name          string  `json:"name"`
		DurationHours float64 `json:"duration_hours"`
	} `json:"location"`
}

// BuddyRecord 是旅行记录。
type BuddyRecord struct {
	DepartAt    *int64 `json:"depart_at"`
	RewardCredit *int  `json:"reward_credit"`
}

// BuddyRecordsData 是旅行记录响应 data。
type BuddyRecordsData struct {
	Records []BuddyRecord `json:"records"`
}

// BuddyClaimData 是领取响应 data。
type BuddyClaimData struct {
	Credit json.Number `json:"credit"`
}

// BuddyDepartData 是派出响应 data。
type BuddyDepartData struct {
	DurationHours json.Number `json:"duration_hours"`
	Duration      json.Number `json:"duration"`
}
