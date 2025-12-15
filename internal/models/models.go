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
	ID            int64      `json:"id"`
	Code          string     `json:"code"`
	Used          bool       `json:"used"`
	UsedEmail     *string    `json:"usedEmail"`
	UsedAt        *time.Time `json:"usedAt"`
	UserID        *int64     `json:"userId"`
	TeamAccountID *int64     `json:"teamAccountId"`
	CreatedAt     time.Time  `json:"createdAt"`
}
