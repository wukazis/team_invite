package models

import (
	"time"
)

type UserStatus int

const (
	UserStatusNone UserStatus = iota
	UserStatusPendingSubmission
	UserStatusCompleted
)

type User struct {
	ID             int64      `json:"id"`
	Username       string     `json:"username"`
	DisplayName    string     `json:"displayName"`
	AvatarTemplate string     `json:"avatarTemplate"`
	TrustLevel     int        `json:"trustLevel"`
	Active         bool       `json:"active"`
	Silenced       bool       `json:"silenced"`
	Chances        int        `json:"chances"`
	InviteStatus   UserStatus `json:"inviteStatus"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type UserWithInvite struct {
	User
	InviteCode  *string `json:"inviteCode"`
	InviteUsed  *bool   `json:"inviteUsed"`
	InviteEmail *string `json:"inviteEmail"`
}

type InviteCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Used      bool       `json:"used"`
	UsedEmail *string    `json:"usedEmail"`
	UsedAt    *time.Time `json:"usedAt"`
	UserID    *int64     `json:"userId"`
	TeamAccountID *int64 `json:"teamAccountId"`
	CreatedAt time.Time  `json:"createdAt"`
}

type SpinRecord struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Username  string    `json:"username"`
	Prize     string    `json:"prize"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	SpinID    string    `json:"spinId"`
	CreatedAt time.Time `json:"createdAt"`
}

type PrizeConfigItem struct {
	Type        string  `json:"type"`
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

type HourlySummary struct {
	HourLabel string `json:"hourLabel"`
	Win       int32  `json:"win"`
	Retry     int32  `json:"retry"`
	Lose      int32  `json:"lose"`
}

type SpinOverview struct {
	Total   int32 `json:"total"`
	Win     int32 `json:"win"`
	Retry   int32 `json:"retry"`
	Lose    int32 `json:"lose"`
	Unknown int32 `json:"unknown"`
}
